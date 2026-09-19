package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillerenrichment"
	"github.com/loomarr/loomarr/internal/fillerresearch"
	"github.com/loomarr/loomarr/internal/metrics"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/taxonomy"
)

type fillerEnrichmentRepository struct{ st store.Store }

type fillerResearchRepository struct{ st store.Store }

func activeFillerTextSelection(set resolved, recorder *metrics.Recorder) fillerenrichment.TextSelection {
	selection := resolveSelection(set)
	if selection.URL == "" || selection.Model == "" {
		return fillerenrichment.TextSelection{}
	}
	return fillerenrichment.TextSelection{Provider: buildProviderFor(selection, recorder),
		ProviderName: selection.Provider, Model: selection.Model}
}

func capabilityVersion(version string, identity ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(identity, "\x00")))
	return version + ":" + hex.EncodeToString(sum[:8])
}

func activeFillerTranscriptCapability(set resolved) fillerenrichment.CapabilitySelection {
	if !set.boolv("filler.transcribe.enabled") {
		return fillerenrichment.CapabilitySelection{}
	}
	if set.str("filler.transcribe.provider") == "hosted" {
		selection := resolveSelection(set)
		model := strings.TrimSpace(set.str("filler.transcribe.model"))
		if selection.URL == "" || model == "" || selection.Provider == "" {
			return fillerenrichment.CapabilitySelection{}
		}
		return fillerenrichment.CapabilitySelection{Available: true,
			Producer: "transcript:" + selection.Provider,
			ProducerVersion: capabilityVersion(filler.TranscriptionPassVersion,
				selection.Provider, selection.URL, model)}
	}
	model := strings.TrimSpace(set.str("ingest.whisper_model"))
	if model == "" {
		return fillerenrichment.CapabilitySelection{}
	}
	return fillerenrichment.CapabilitySelection{Available: true, Producer: "transcript:whisper",
		ProducerVersion: capabilityVersion(filler.TranscriptionPassVersion,
			set.str("ingest.whisper_path"), model)}
}

func activeFillerVisionCapability(set resolved) fillerenrichment.CapabilitySelection {
	if !set.boolv("filler.vision.enabled") {
		return fillerenrichment.CapabilitySelection{}
	}
	wiring := visionEndpoint(set)
	if wiring.url == "" || strings.TrimSpace(wiring.model) == "" ||
		(wiring.provider != "openai" && wiring.provider != "ollama") {
		return fillerenrichment.CapabilitySelection{}
	}
	identity := wiring.identity
	if identity == "" {
		identity = wiring.provider
	}
	return fillerenrichment.CapabilitySelection{Available: true, Producer: "vision:" + identity,
		ProducerVersion: capabilityVersion(filler.VisionPromptVersion,
			identity, wiring.url, wiring.model)}
}

type fillerTranscriptObserver interface {
	Observe(context.Context, filler.StoreClip) (filler.StageResult, string, error)
}

type fillerVisionObserver interface {
	Observe(context.Context, filler.StoreClip) (filler.StageResult, filler.VisionObservation, error)
}

// fillerMediaExecutor is intentionally narrower than filler.Stage: the enrichment coordinator has
// already decided that useful work remains, and this adapter can load media and produce observations
// but has no readiness, persistence, or conveyor mutation in its interface.
type fillerMediaExecutor struct {
	store      store.Store
	transcript fillerTranscriptObserver
	vision     fillerVisionObserver
	kind       fillerenrichment.Capability
}

func (e fillerMediaExecutor) Run(ctx context.Context, candidate fillerenrichment.Candidate) (fillerenrichment.CapabilityResult, error) {
	if e.store == nil {
		return fillerenrichment.CapabilityResult{}, nil
	}
	clip, err := e.store.GetClip(ctx, candidate.ClipHash)
	if err != nil {
		return fillerenrichment.CapabilityResult{}, fmt.Errorf("load filler clip: %w", err)
	}
	storeClip := filler.StoreClip{Clip: clip.Clip, UpdatedAt: clip.UpdatedAt}
	if e.kind == fillerenrichment.CapabilityTranscript {
		if e.transcript == nil {
			return fillerenrichment.CapabilityResult{}, nil
		}
		_, transcript, err := e.transcript.Observe(ctx, storeClip)
		if err != nil {
			return fillerenrichment.CapabilityResult{}, err
		}
		return fillerenrichment.CapabilityResult{Observation: &fillerenrichment.MediaObservation{
			Transcript: &transcript,
		}}, nil
	}
	if e.kind != fillerenrichment.CapabilityVision || e.vision == nil {
		return fillerenrichment.CapabilityResult{}, nil
	}
	currentStates, err := e.store.ListFillerEnrichment(ctx, candidate.ClipHash)
	if err != nil {
		return fillerenrichment.CapabilityResult{}, fmt.Errorf("load filler enrichment before vision: %w", err)
	}
	current := make(map[fillerenrichment.Axis]fillerenrichment.State, len(currentStates))
	for _, state := range currentStates {
		current[state.Axis] = state
	}
	_, observation, err := e.vision.Observe(ctx, storeClip)
	if err != nil {
		return fillerenrichment.CapabilityResult{}, err
	}
	var states []fillerenrichment.State
	evidence := func(reference string) fillerenrichment.Evidence {
		return fillerenrichment.Evidence{Kind: fillerenrichment.EvidenceContent,
			Reference: reference, Confidence: 100}
	}
	if observation.Brand != "" {
		states = append(states, fillerenrichment.State{Axis: fillerenrichment.AxisBrand,
			Status: fillerenrichment.StatusComplete, Value: fillerenrichment.Value{Text: observation.Brand},
			Evidence: evidence("frame.visible_text")})
	}
	if observation.Era > 0 {
		states = append(states, fillerenrichment.State{Axis: fillerenrichment.AxisEra,
			Status: fillerenrichment.StatusComplete, Value: fillerenrichment.Value{Year: observation.Era},
			Evidence: evidence("frame.visible_text")})
	}
	taxa, err := e.store.ListTaxa(ctx)
	if err != nil {
		return fillerenrichment.CapabilityResult{}, fmt.Errorf("load filler taxonomy after vision: %w", err)
	}
	tagAxes := make(map[string]fillerenrichment.Axis, len(taxa))
	for _, taxon := range taxa {
		tagAxes[taxon.Slug] = fillerenrichment.Axis(taxon.Axis)
	}
	byAxis := map[fillerenrichment.Axis][]string{}
	for _, tag := range observation.Tags {
		axis := tagAxes[tag]
		if axis.Valid() {
			byAxis[axis] = append(byAxis[axis], tag)
		}
	}
	for axis, tags := range byAxis {
		if accepted, ok := current[axis]; ok {
			tags = append(append([]string(nil), accepted.Value.Tags...), tags...)
		}
		states = append(states, fillerenrichment.State{Axis: axis, Status: fillerenrichment.StatusComplete,
			Value: fillerenrichment.Value{Tags: tags}, Evidence: evidence("frame.taxonomy_grounded")})
	}
	return fillerenrichment.CapabilityResult{
		States: states,
		Observation: &fillerenrichment.MediaObservation{Vision: &fillerenrichment.VisionObservation{
			VisibleText: observation.VisibleText, SuggestedEra: observation.SuggestedEra,
		}},
	}, nil
}

func (r fillerEnrichmentRepository) ListCandidates(ctx context.Context, producer, producerVersion, taxonomyVersion string, limit int) ([]fillerenrichment.Candidate, error) {
	clips, err := r.st.ListFillerEnrichmentCandidates(ctx, producer, producerVersion, taxonomyVersion, limit)
	return fillerEnrichmentCandidates(clips, err)
}

func (r fillerEnrichmentRepository) ListCapabilityCandidates(ctx context.Context, producer, producerVersion, taxonomyVersion string, limit int) ([]fillerenrichment.Candidate, error) {
	clips, err := r.st.ListFillerEnrichmentCapabilityCandidates(ctx, producer, producerVersion, taxonomyVersion, limit)
	return fillerEnrichmentCandidates(clips, err)
}

func fillerEnrichmentCandidates(clips []store.Clip, err error) ([]fillerenrichment.Candidate, error) {
	if err != nil {
		return nil, err
	}
	out := make([]fillerenrichment.Candidate, len(clips))
	for i, clip := range clips {
		out[i] = fillerenrichment.Candidate{ClipHash: clip.Hash, Path: clip.Path, Name: clip.Name,
			Kind: string(clip.Kind), Transcript: clip.Transcript, VisibleText: clip.VisibleText,
			VisionTagged: clip.VisionTagged}
	}
	return out, nil
}

func (r fillerEnrichmentRepository) ListStates(ctx context.Context, clipHash string) ([]fillerenrichment.State, error) {
	return r.st.ListFillerEnrichment(ctx, clipHash)
}

func (r fillerEnrichmentRepository) ListTaxa(ctx context.Context) ([]taxonomy.Taxon, error) {
	return r.st.ListTaxa(ctx)
}

func (r fillerEnrichmentRepository) ApplyPass(ctx context.Context, pass fillerenrichment.Pass) (int, error) {
	return r.st.ApplyFillerEnrichmentPass(ctx, pass)
}

func (r fillerResearchRepository) ListCandidates(ctx context.Context, producer, producerVersion,
	adapter, adapterVersion string, limit int) ([]fillerresearch.Candidate, error) {
	return r.st.ListFillerResearchCandidates(ctx, producer, producerVersion, adapter, adapterVersion, limit)
}

func (r fillerResearchRepository) SaveReport(ctx context.Context, report fillerresearch.Report) error {
	return r.st.SaveFillerResearchReport(ctx, report)
}

type fillerResearchSignals struct{ files fs.FS }

func (l fillerResearchSignals) Load(_ context.Context, candidate fillerresearch.Candidate) (fillerresearch.Input, error) {
	input := fillerresearch.Input{Title: candidate.Name}
	if metadata, ok := filler.ReadSourceMetadataFS(l.files, candidate.Path); ok {
		input.Title = metadata.Title
		input.Description = metadata.Description
		input.SourceURL = metadata.WebpageURL
	}
	input.SourceKind, _, _ = strings.Cut(candidate.SourceID, ":")
	return input, nil
}

// fillerDetailRunner keeps one scheduler row while the modules retain different authority:
// enrichment writes verified axes; research writes only cited suggestions.
type fillerDetailRunner struct {
	enrichment fillerEnrichmentRunner
	research   *fillerresearch.Runner
}

func (r fillerDetailRunner) Run(ctx context.Context) (fillerenrichment.RunResult, error) {
	var out fillerenrichment.RunResult
	var enrichmentErr error
	if r.enrichment != nil {
		out, enrichmentErr = r.enrichment.Run(ctx)
	}
	if r.research == nil {
		return out, enrichmentErr
	}
	researched, researchErr := r.research.Run(ctx)
	out.Considered += researched.Considered
	out.Updated += researched.Updated
	out.Failed += researched.Failed
	return out, errors.Join(enrichmentErr, researchErr)
}

type fillerEnrichmentSignals struct {
	store store.FillerSourceStore
	files fs.FS
}

func (l fillerEnrichmentSignals) Load(ctx context.Context, candidate fillerenrichment.Candidate, observedAt time.Time) (fillerenrichment.Signals, error) {
	signals := fillerenrichment.Signals{ClipHash: candidate.ClipHash, Kind: candidate.Kind,
		Title: candidate.Name, OriginalName: candidate.Name, Transcript: candidate.Transcript,
		VisibleText: candidate.VisibleText, ObservedAt: observedAt}
	metadata, ok := filler.ReadSourceMetadataFS(l.files, candidate.Path)
	if ok {
		signals.Title = metadata.Title
		signals.Description = metadata.Description
		signals.OriginalName = metadata.OriginalName
		signals.UploadDate = metadata.UploadDate
		signals.SourceID = metadata.SourceID
	}
	if signals.SourceID == "" || l.store == nil {
		return signals, nil
	}
	sources, err := l.store.ListFillerSources(ctx)
	if err != nil {
		return fillerenrichment.Signals{}, err
	}
	for _, source := range sources {
		if source.ID != signals.SourceID {
			continue
		}
		geography := source.Geography.Normalize()
		scope := ""
		if geography.Market != "" {
			scope = string(filler.GeographicLocal)
		} else if geography.Country != "" {
			scope = string(filler.GeographicNational)
		}
		signals.Source = fillerenrichment.Geography{Scope: scope, Country: geography.Country, Market: geography.Market}
		break
	}
	return signals, nil
}
