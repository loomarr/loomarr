package fillerenrichment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/taxonomy"
)

const textPromptVersion = "filler-progressive-text-v2"

var textAxes = []Axis{
	AxisKind, AxisAudience, AxisBrand, AxisProduct, AxisFormat,
	AxisSeasonal, AxisAudienceCue, AxisPresentation,
}

// TextSelection is the live household text capability. An empty selection means that the
// household has not configured a model; progressive enrichment then stays quiet and leaves the
// axes eligible for a later run.
type TextSelection struct {
	Provider     llm.Provider
	ProviderName string
	Model        string
}

// TextRepository is the persistence surface needed by the model pass. The completion stamp and
// accepted axis changes still commit through Repository.ApplyPass as one transaction.
type TextRepository interface {
	Repository
	ListStates(ctx context.Context, clipHash string) ([]State, error)
	ListTaxa(ctx context.Context) ([]taxonomy.Taxon, error)
}

// Coordinator runs the free deterministic pass before the optional household text pass. Both
// passes are independently versioned and bounded; readiness is deliberately absent from this
// interface so a Ready clip can gain details without being held or replaying its conveyor.
type Coordinator struct {
	deterministic *Runner
	repository    TextRepository
	selection     func() TextSelection
	load          SignalLoader
	limit         func() int
	now           func() time.Time
}

func NewCoordinator(deterministic *Runner, repository TextRepository, selection func() TextSelection,
	load SignalLoader, limit func() int, now func() time.Time) *Coordinator {
	if limit == nil {
		limit = func() int { return 10 }
	}
	if now == nil {
		now = time.Now
	}
	return &Coordinator{deterministic: deterministic, repository: repository, selection: selection,
		load: load, limit: limit, now: now}
}

func (c *Coordinator) Run(ctx context.Context) (RunResult, error) {
	var result RunResult
	if c == nil {
		return result, nil
	}
	if c.deterministic != nil {
		free, err := c.deterministic.Run(ctx)
		if err != nil {
			return result, err
		}
		result.Considered += free.Considered
		result.Updated += free.Updated
	}
	if c.repository == nil || c.selection == nil || c.load == nil || c.limit() <= 0 {
		return result, nil
	}
	selection := c.selection()
	if selection.Provider == nil || strings.TrimSpace(selection.Model) == "" {
		return result, nil
	}
	providerName := strings.TrimSpace(selection.ProviderName)
	if providerName == "" {
		providerName = selection.Provider.Name()
	}
	taxa, err := c.repository.ListTaxa(ctx)
	if err != nil {
		return result, fmt.Errorf("load filler enrichment taxonomy: %w", err)
	}
	forest := taxonomy.New(taxa)
	taxonomyVersion, err := taxonomyIdentity(forest)
	if err != nil {
		return result, err
	}
	producer := "text-model:" + providerName
	producerVersion := textPromptVersion + ":" + strings.TrimSpace(selection.Model)
	candidates, err := c.repository.ListCandidates(ctx, producer, producerVersion, taxonomyVersion, c.limit())
	if err != nil {
		return result, fmt.Errorf("list text enrichment candidates: %w", err)
	}
	result.Considered += len(candidates)
	var modelErrors []error
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		completedAt := c.now().UTC()
		states, err := c.repository.ListStates(ctx, candidate.ClipHash)
		if err != nil {
			return result, fmt.Errorf("load enrichment state for %s: %w", candidate.ClipHash, err)
		}
		axes := unresolvedTextAxes(states, producer, producerVersion, taxonomyVersion)
		var proposed []State
		if len(axes) > 0 {
			signals, err := c.load(ctx, candidate, completedAt)
			if err != nil {
				return result, fmt.Errorf("load text enrichment signals for %s: %w", candidate.ClipHash, err)
			}
			signals.ClipHash = candidate.ClipHash
			if signals.Kind == "" {
				signals.Kind = candidate.Kind
			}
			if signals.Title == "" {
				signals.Title = candidate.Name
			}
			signals.ObservedAt = completedAt
			proposed, err = classifyText(ctx, selection.Provider, forest, axes, signals, producer, producerVersion, taxonomyVersion)
			if err != nil {
				result.Failed++
				modelErrors = append(modelErrors, fmt.Errorf("enrich filler text for %s: %w", candidate.ClipHash, err))
				continue
			}
		}
		changed, err := c.repository.ApplyPass(ctx, Pass{
			ClipHash: candidate.ClipHash, Producer: producer, ProducerVersion: producerVersion,
			TaxonomyVersion: taxonomyVersion, CompletedAt: completedAt, States: proposed,
		})
		if err != nil {
			return result, fmt.Errorf("apply text enrichment for %s: %w", candidate.ClipHash, err)
		}
		result.Updated += changed
	}
	return result, errors.Join(modelErrors...)
}

func unresolvedTextAxes(states []State, producer, producerVersion, taxonomyVersion string) []Axis {
	current := make(map[Axis]State, len(states))
	for _, state := range states {
		current[state.Axis] = state
	}
	var out []Axis
	for _, axis := range textAxes {
		state, found := current[axis]
		if found {
			if state.Evidence.Kind == EvidenceOperator {
				continue
			}
			currentModelAnswer := state.Evidence.Kind == EvidenceInference &&
				strings.HasPrefix(state.Evidence.Producer, "text-model:")
			identityChanged := state.Evidence.Producer != producer || state.Evidence.ProducerVersion != producerVersion ||
				(state.Evidence.TaxonomyVersion != "" && state.Evidence.TaxonomyVersion != taxonomyVersion)
			if !state.Value.empty() && (!currentModelAnswer || !identityChanged) {
				continue
			}
		}
		out = append(out, axis)
	}
	return out
}

type textOutput struct {
	Kind         scalarString    `json:"kind"`
	Audience     scalarString    `json:"audience"`
	Brand        scalarString    `json:"brand"`
	Product      stringList      `json:"product"`
	Format       stringList      `json:"format"`
	Seasonal     stringList      `json:"seasonal"`
	AudienceCue  stringList      `json:"audienceCue"`
	Presentation stringList      `json:"presentation"`
	Confidence   confidenceScore `json:"confidence"`
}

// confidenceScore accepts either the requested 0-100 percentage or the common 0-1 model
// convention. Confidence is only supporting provenance here; the validation and grounding gates
// remain identical whichever representation the provider chooses.
type confidenceScore int

func (c *confidenceScore) UnmarshalJSON(raw []byte) error {
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("expected a numeric confidence: %w", err)
	}
	if value >= 0 && value <= 1 {
		value *= 100
	}
	*c = confidenceScore(math.Round(value))
	return nil
}

// scalarString is the inverse tolerance of stringList: a few JSON-mode models use [] for an
// unknown scalar and ["value"] for a known one. The prompt remains precise, but accepting that
// equivalent representation makes the integration provider-neutral without weakening any
// grounding rule.
type scalarString string

func (s *scalarString) UnmarshalJSON(raw []byte) error {
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		*s = scalarString(value)
		return nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return fmt.Errorf("expected a string or single-item string array: %w", err)
	}
	if len(values) > 1 {
		return fmt.Errorf("expected at most one string, got %d", len(values))
	}
	if len(values) == 0 {
		*s = ""
	} else {
		*s = scalarString(values[0])
	}
	return nil
}

// stringList accepts the exact array shape requested by the prompt and the harmless scalar
// shorthand some JSON-mode providers still return for a single selection. This keeps one
// provider formatting quirk from starving every later clip while the taxonomy gate below still
// decides whether the value is valid for that axis.
type stringList []string

func (s *stringList) UnmarshalJSON(raw []byte) error {
	var values []string
	if err := json.Unmarshal(raw, &values); err == nil {
		*s = values
		return nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("expected a string or string array: %w", err)
	}
	if strings.TrimSpace(value) == "" {
		*s = nil
	} else {
		*s = []string{value}
	}
	return nil
}

func classifyText(ctx context.Context, provider llm.Provider, forest *taxonomy.Forest, axes []Axis,
	signals Signals, producer, producerVersion, taxonomyVersion string) ([]State, error) {
	requested := make(map[Axis]bool, len(axes))
	axisNames := make([]string, 0, len(axes))
	for _, axis := range axes {
		requested[axis] = true
		axisNames = append(axisNames, string(axis))
	}
	system := `Classify only the requested filler details from the supplied text. Return one JSON object and no prose.
Use only taxonomy slugs from the vocabulary. Use an empty string or array when the text does not support an answer.
Do not guess a year, country, location, safety rating, or facts based only on nostalgia, visual style, or general knowledge.
Kind must be one of commercial, bumper, station_id, psa, trailer, interstitial, or empty.
Audience must be one of kids, family, general, late_night, or empty. Brand must appear literally in the supplied text.
Confidence must be an integer percentage from 0 through 80.
The product, format, seasonal, audienceCue, and presentation values must always be JSON arrays, even for one item.
JSON keys: kind, audience, brand, product, format, seasonal, audienceCue, presentation, confidence.`
	user := fmt.Sprintf("Requested axes: %s\nTaxonomy:\n%s\n\nClip text:\n%s",
		strings.Join(axisNames, ", "), forest.Vocab(), signalText(signals))
	response, err := provider.Chat(ctx, []llm.Message{{Role: llm.System, Content: system}, {Role: llm.User, Content: user}},
		llm.ChatOptions{JSONMode: true})
	if err != nil {
		return nil, err
	}
	var output textOutput
	if err := json.Unmarshal([]byte(llm.ExtractJSONObject(response.Content)), &output); err != nil {
		return nil, fmt.Errorf("model output is not JSON: %w", err)
	}
	confidence := int(output.Confidence)
	if confidence <= 0 || confidence > 80 {
		confidence = 80
	}
	evidence := func(reference string, kind EvidenceKind) Evidence {
		return Evidence{Kind: kind, Reference: reference, Confidence: confidence, Producer: producer,
			ProducerVersion: producerVersion, TaxonomyVersion: taxonomyVersion, ObservedAt: signals.ObservedAt}
	}
	state := func(axis Axis, value Value, reference string, kind EvidenceKind) State {
		return State{ClipHash: signals.ClipHash, Axis: axis, Status: StatusComplete, Value: value,
			Evidence: evidence(reference, kind)}
	}
	var out []State
	if requested[AxisKind] {
		kind := strings.ToLower(strings.TrimSpace(string(output.Kind)))
		switch kind {
		case "commercial", "bumper", "station_id", "psa", "trailer", "interstitial":
		default:
			kind = ""
		}
		out = append(out, state(AxisKind, Value{Text: kind}, "model.text_signals", EvidenceInference))
	}
	if requested[AxisAudience] {
		audience := strings.ToLower(strings.TrimSpace(string(output.Audience)))
		switch audience {
		case "kids", "family", "general", "late_night":
		default:
			audience = ""
		}
		out = append(out, state(AxisAudience, Value{Text: audience}, "model.text_signals", EvidenceInference))
	}
	if requested[AxisBrand] {
		brand := strings.TrimSpace(string(output.Brand))
		kind, reference := EvidenceInference, "model.text_signals"
		if brand != "" && containsGroundedText(signalText(signals), brand) {
			kind, reference = EvidenceItem, "item.text_signals"
		} else {
			brand = ""
		}
		out = append(out, state(AxisBrand, Value{Text: brand}, reference, kind))
	}
	tagValues := []struct {
		axis Axis
		tags []string
		tax  taxonomy.Axis
	}{
		{AxisProduct, []string(output.Product), taxonomy.AxisProduct},
		{AxisFormat, []string(output.Format), taxonomy.AxisFormat},
		{AxisSeasonal, []string(output.Seasonal), taxonomy.AxisSeasonal},
		{AxisAudienceCue, []string(output.AudienceCue), taxonomy.AxisAudienceCue},
		{AxisPresentation, []string(output.Presentation), taxonomy.AxisPresentation},
	}
	for _, candidate := range tagValues {
		if !requested[candidate.axis] {
			continue
		}
		out = append(out, state(candidate.axis, Value{Tags: resolveAxisTags(forest, candidate.tax, candidate.tags)},
			"model.taxonomy_grounded", EvidenceInference))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Axis < out[j].Axis })
	return out, nil
}

func resolveAxisTags(forest *taxonomy.Forest, axis taxonomy.Axis, raw []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range raw {
		slug, ok := forest.Resolve(value)
		if !ok || seen[slug] {
			continue
		}
		taxon, ok := forest.Get(slug)
		if !ok || taxon.Axis != axis {
			continue
		}
		seen[slug] = true
		out = append(out, slug)
	}
	sort.Strings(out)
	return out
}

func signalText(signals Signals) string {
	parts := []string{signals.Title, signals.Description, signals.OriginalName, signals.Transcript, signals.VisibleText}
	var out []string
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			out = append(out, value)
		}
	}
	return strings.Join(out, "\n")
}

func containsGroundedText(haystack, needle string) bool {
	foldedNeedle := foldForGrounding(strings.TrimSpace(needle))
	return foldedNeedle != "" && strings.Contains(foldForGrounding(haystack), foldedNeedle)
}

func foldForGrounding(value string) string {
	var out strings.Builder
	out.Grow(len(value))
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func taxonomyIdentity(forest *taxonomy.Forest) (string, error) {
	raw, err := json.Marshal(forest.All())
	if err != nil {
		return "", fmt.Errorf("encode filler enrichment taxonomy identity: %w", err)
	}
	sum := sha256.Sum256(raw)
	return "taxonomy:" + hex.EncodeToString(sum[:8]), nil
}
