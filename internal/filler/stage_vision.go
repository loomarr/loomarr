package filler

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/filler/research"
	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/filler/taxonomy"
	"github.com/loomarr/loomarr/internal/media/tools"
	"github.com/loomarr/loomarr/internal/platform/llm"
)

// The VISION stage (§10 V51b) — the expensive last tier, unchanged from V44 except for what drives
// it.
//
// It reads keyframes and asks a multimodal model what is on them, and it earns its cost only where
// the cheaper tiers left a gap. The spot it exists for is the WORDLESS one: a silent visual advert
// whose only signal is an on-screen logo or a "1987" burned into the corner — no dialogue for
// Whisper, often no sidecar text either, so the frame is the only place its brand or era can be
// grounded.
//
// ⚠ **The grounding rule is untouched and must stay that way.** `groundVisionTags` keeps a brand,
// era or category ONLY when it appears literally in the on-screen text the model claims to have
// read. Nothing in this phase relaxes that — a pipeline that made a fabricated brand easier to
// persist would have traded the whole point of §8 for a tidier runner.

// VisionClipStore is the slice of the store the vision stage needs.
type VisionClipStore interface {
	// ListTaxa is the vocabulary the vision tier grounds its category against (§10 V45a).
	ListTaxa(ctx context.Context) ([]taxonomy.Taxon, error)
	// ApplyClipVision records what the pass GROUNDED. Facts and taxonomy projections commit as one
	// semantic write. The rung's done record on the ladder, not the clip, says vision already read
	// it.
	ApplyClipVision(ctx context.Context, hash, path, brand, visibleText string, era int, leaves []string, at time.Time) error
	// RecordFillerEraObservation keeps an era the frames only suggest as an uncited Context
	// suggestion (#2323).
	RecordFillerEraObservation(ctx context.Context, clipHash string, era int, source research.ObservationSource, at time.Time) error
}

// VisionStage reads a clip's frames.
type VisionStage struct {
	tools    tools.MediaTools
	provider llm.VisionProvider
	store    VisionClipStore
	clipDir  string
	enabled  func() bool
	now      func() time.Time
}

// NewVisionStage builds the stage. A nil provider is the un-opted-in default — an install with no
// vision model (the common case) pays nothing, and its ladder says so on every clip.
func NewVisionStage(tools tools.MediaTools, provider llm.VisionProvider, store VisionClipStore, clipDir string, enabled func() bool, now func() time.Time) *VisionStage {
	if now == nil {
		now = time.Now
	}
	return &VisionStage{tools: tools, provider: provider, store: store, clipDir: clipDir, enabled: enabled, now: now}
}

func (s *VisionStage) ID() pipelinestate.StageID { return pipelinestate.StageVision }
func (s *VisionStage) Cost() StageCost           { return CostVision }

// Applies when vision is configured, on, and this clip still has a gap the frames might close.
//
// ⚠ The clip must still be short on tags after the cheaper tiers: a clip the text tiers already
// resolved has nothing for vision to add and a real cost to avoid. Nothing here asks whether vision
// already read it — the ladder does. A done rung is never re-run, and a Rewind that resets it is a
// deliberate request to look again.
func (s *VisionStage) Applies(_ context.Context, c cliplibrary.StoreClip) (bool, string) {
	if s.tools == nil || s.provider == nil {
		return false, "no vision model is configured"
	}
	if s.enabled == nil || !s.enabled() {
		return false, "vision tagging is off"
	}
	if c.Tagged() {
		return false, "the cheaper signals already tagged it"
	}
	return true, ""
}

// VisionObservation is the exact grounded result of one bounded frame pass.
type VisionObservation struct {
	Brand, VisibleText string
	Era, SuggestedEra  int
	Tags               []string
}

// Observe extracts keyframes and grounds the answer without persisting it. Progressive catch-up
// uses this half so its raw signal, accepted axes, and completion identity commit atomically.
func (s *VisionStage) Observe(ctx context.Context, c cliplibrary.StoreClip) (StageResult, VisionObservation, error) {
	// ⚠ Loaded per clip rather than cached on the stage, and the vision BUDGET is what makes that
	// affordable: at most `MaxVision` clips reach this rung in a pass. The alternative — a forest
	// cached for the process — goes stale the moment an operator edits the taxonomy, and a
	// category silently failing to resolve is far more expensive to diagnose than a small query.
	taxa, err := s.store.ListTaxa(ctx)
	if err != nil {
		return StageResult{}, VisionObservation{}, err
	}
	forest := taxonomy.New(taxa)

	file := filepath.Join(s.clipDir, filepath.FromSlash(c.Path))
	frames, err := s.tools.VisionKeyframesIn(ctx, file, 0, c.DurationMs, split.VisionKeyframes)
	if err != nil {
		return StageResult{}, VisionObservation{}, fmt.Errorf("keyframes for %s: %w", c.Path, err)
	}
	if len(frames) == 0 {
		return StageResult{}, VisionObservation{}, fmt.Errorf("no keyframes could be extracted from %s", c.Path)
	}
	pipelinestate.ReportProgress(ctx, pipelinestate.StageVision, pipelinestate.NoMeasurement)

	resp, err := s.provider.AskAboutImages(llm.WithCallSite(ctx, "filler.vision"), visionPrompt(forest), frames)
	if err != nil {
		return StageResult{}, VisionObservation{}, fmt.Errorf("vision model for %s: %w", c.Path, err)
	}
	var out visionOutput
	// ⚠ Unwrap a code fence before parsing (§10 V44, live-found): a vision model wraps its JSON in
	// ```json … ``` and json.Unmarshal rejects the leading backtick.
	if err := json.Unmarshal([]byte(llm.ExtractJSONObject(resp.Content)), &out); err != nil {
		// The model answered but not in JSON — we learned nothing we can trust, so this is a retry
		// rather than a stamp. Persisting garbage would be worse than paying for the frames twice.
		return StageResult{}, VisionObservation{}, fmt.Errorf("vision output for %s is not JSON: %w", c.Path, err)
	}

	v := groundVisionTags(out, forest)

	// The free frame-heuristic tier: if the model grounded no era, read one off the pixels already
	// decoded. Only ever a SUGGESTION — a one-click-dismissible prompt on a clip that had no era
	// signal at all, never a fact.
	suggestedEra := 0
	if v.Era == 0 {
		suggestedEra = tools.SuggestedEraFrom(tools.AnalyzeFrames(frames))
	}

	pipelinestate.ReportProgress(ctx, pipelinestate.StageVision, 100)

	updated := c
	updated.VisibleText = v.VisibleText
	if v.Brand != "" {
		updated.Brand = v.Brand
	}
	if v.Era > 0 {
		updated.Era = v.Era
	}
	if len(v.Tags) > 0 {
		updated.AssertedTags = unionLeaves(c.AssertedTags, v.Tags)
		updated.Tags = append([]string(nil), c.Tags...)
		for _, leaf := range v.Tags {
			updated.Tags = unionLeaves(updated.Tags, []string{leaf})
			updated.Tags = unionLeaves(updated.Tags, forest.Ancestors(leaf))
		}
		updated.Category = forest.PrimaryProductLeaf(updated.AssertedTags)
	}
	if v.Brand == "" && v.Era == 0 && len(v.Tags) == 0 {
		// Read, but nothing the frames supported. Still stamped — the vision analogue of the
		// wordless transcript sentinel: an outcome recorded precisely so it is never re-paid-for.
		return StageResult{Clip: updated, Verdict: pipelinestate.VerdictContinue, Note: "nothing on the frames could be grounded"},
			VisionObservation{VisibleText: v.VisibleText, SuggestedEra: suggestedEra}, nil
	}
	return StageResult{Clip: updated, Verdict: pipelinestate.VerdictContinue}, VisionObservation{
		Brand: v.Brand, VisibleText: v.VisibleText, Era: v.Era, SuggestedEra: suggestedEra,
		Tags: append([]string(nil), v.Tags...),
	}, nil
}

// Run is the readiness-stage adapter around Observe.
func (s *VisionStage) Run(ctx context.Context, c cliplibrary.StoreClip) (StageResult, error) {
	result, observation, err := s.Observe(ctx, c)
	if err != nil {
		return StageResult{}, err
	}
	if s.store != nil && c.Path != "" {
		at := s.now().UTC()
		if err := s.store.ApplyClipVision(ctx, c.Hash, c.Path, observation.Brand, observation.VisibleText,
			observation.Era, observation.Tags, at); err != nil {
			return StageResult{}, err
		}
		// An era the frames only suggest is a Context observation, never a clip fact. After the
		// visible text, so it binds to the revision that text produced; the store skips it when
		// the clip has a verified era.
		if observation.Era == 0 {
			if err := s.store.RecordFillerEraObservation(ctx, c.Hash, observation.SuggestedEra,
				research.ObservedByVision, at); err != nil {
				return StageResult{}, err
			}
		}
	}
	return result, nil
}

const visionPromptVersion = "filler-vision-grounding-v2"
const splitVisionPromptVersion = "filler-split-vision-grounding-v3"

// VisionPromptVersion is the semantic identity progressive enrichment records for frame catch-up.
const VisionPromptVersion = visionPromptVersion

// visionTags is a grounded vision classification for one clip — the fields ApplyClipVision
// writes.
type visionTags struct {
	Brand       string
	Tags        []string
	Category    string
	Era         int
	VisibleText string
}

// visionOutput is the model's raw multimodal answer (untrusted — grounded before use).
type visionOutput struct {
	// VisibleText is the on-screen text the model says it can literally SEE — a logo, a product
	// name, a year burned into the frame. It is BOTH persisted (the auditable record of what
	// vision read) AND the grounding signal every other field here is checked against.
	VisibleText string `json:"visibleText"`
	// ProposedTitle is requested only by split grounding. It remains untrusted until the exact
	// segment's VisibleText grounds its identifying phrase.
	ProposedTitle string   `json:"proposedTitle"`
	Brand         string   `json:"brand"`
	Tags          []string `json:"tags"`
	// Category accepts the retired one-value answer shape so an older configured model response
	// degrades safely during upgrade. New prompts request Tags and all values still resolve through
	// the live forest.
	Category string `json:"category"`
	Era      int    `json:"era"`
	// Role is a per-segment semantic judgement used only by the source-structure reducer. It is
	// not taxonomy and never changes the grounding rules above.
	Role       string `json:"role"`
	RoleReason string `json:"roleReason"`
}

// UnmarshalJSON salvages independently valid fields from a model answer. Vision models
// occasionally use a numeric sentinel for one optional string field (measured live:
// `category: 0`); the default decoder rejected the whole object and discarded valid visible text
// beside it. Dropping only the malformed field is the safe direction because groundVisionTags
// still applies every evidence and taxonomy constraint afterwards.
func (v *visionOutput) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if fields == nil {
		return fmt.Errorf("vision output must be a JSON object")
	}
	v.VisibleText = decodeVisionString(fields["visibleText"])
	v.ProposedTitle = decodeVisionString(fields["proposedTitle"])
	v.Brand = decodeVisionString(fields["brand"])
	v.Tags = decodeVisionStrings(fields["tags"])
	v.Category = decodeVisionString(fields["category"])
	v.Era = decodeVisionInt(fields["era"])
	v.Role = decodeVisionString(fields["role"])
	v.RoleReason = decodeVisionString(fields["roleReason"])
	return nil
}

func decodeVisionStrings(raw json.RawMessage) []string {
	var values []string
	if len(raw) == 0 || json.Unmarshal(raw, &values) != nil {
		return nil
	}
	return values
}

func decodeVisionString(raw json.RawMessage) string {
	var value string
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return ""
	}
	return value
}

func decodeVisionInt(raw json.RawMessage) int {
	var value int
	if len(raw) == 0 {
		return 0
	}
	if err := json.Unmarshal(raw, &value); err == nil {
		return value
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return 0
	}
	value, _ = strconv.Atoi(strings.TrimSpace(text))
	return value
}

// groundVisionTags is the grounding pass for the vision tier: the same rule the text tagger applies
// (a brand or year must appear in the signals it was read from), but the signal a tag must appear
// in is the model's OWN visibleText rather than the clip's text signals (§10 V44).
//
// ⚠ **This is the era rule generalised to pixels, and it must not be weakened.** A vision model
// that reports {brand:"Coca-Cola"} with a visibleText that never spells "Coca-Cola" has INFERRED
// the brand from the imagery — the exact confidently-wrong metadata §8 keeps out — so the brand is
// dropped. A brand/category/era survives ONLY when it appears literally in the visibleText the
// model claims to have read. The asymmetry is safe the same way it is for text: dropping a true
// positive leaves the clip untagged for a later pass or a human, while accepting a fabrication
// corrupts matching silently.
//
// VisibleText itself is NOT grounded against anything — it IS the ground truth here, the record of
// what the pass read off the frame, kept verbatim so a reviewer can see WHY a tag was (or was not)
// grounded.
func groundVisionTags(out visionOutput, forest *taxonomy.Forest) visionTags {
	var v visionTags
	v.VisibleText = strings.TrimSpace(out.VisibleText)
	haystack := strings.ToLower(v.VisibleText)

	// BRAND — case-insensitive, since a logo has a case a year does not. There is deliberately no
	// SuggestedBrand: an ungrounded advertiser is a guess, not a question worth a human.
	if brand := strings.TrimSpace(out.Brand); brand != "" &&
		strings.Contains(haystack, strings.ToLower(brand)) {
		v.Brand = brand
	}
	// ERA — a plausible year that ALSO appears literally in the visibleText. A "1987" read off a
	// corner grounds the era; a decade inferred from the film stock does not. The frame-heuristic
	// tier owns "this LOOKS old" suggestions, so an ungrounded vision year is simply dropped.
	if out.Era >= 1930 && out.Era <= 2035 && strings.Contains(v.VisibleText, strconv.Itoa(out.Era)) {
		v.Era = out.Era
	}
	// TAGS — TAXONOMY-grounded (§10 V45a). Deliberately NOT required to appear in visibleText.
	//
	// ⚠ **It does not have to appear on the frame (V54b).** The brand/era rule — the claim must be
	// visible in `visibleText` — does not fit a field that makes a different kind of claim. A brand
	// or a year is a specific FACT, and asserting one the frame does not show is fabrication that is
	// wrong in a checkable way. A category is a JUDGEMENT ABOUT IMAGERY — calling a toy advert
	// `toys` because it shows toys is the reason a vision tier exists, and demanding the genre be
	// printed on screen does not make the judgement honest, only rare.
	//
	// ⚠ This matters for split auto-confirm: `segmentVerdict` refuses an untagged segment BEFORE it
	// consults boundary confidence, so requiring the word on the frame would drop a correct `psa`
	// against visibleText "WAGA-5/Fox Commercial Breaks (2/5/1995)" and make auto-confirm
	// structurally impossible, while `era` still grounds from the same string because "1995" is in
	// it.
	//
	// ⚠ The TAXONOMY is what keeps this grounded rather than open: an unresolvable category is
	// still dropped, so the vocabulary is the constraint. Resolution still maps `burgers`→`fast_food`.
	if forest != nil {
		rawTags := append([]string(nil), out.Tags...)
		if out.Category != "" {
			rawTags = append(rawTags, out.Category)
		}
		for _, raw := range rawTags {
			if slug, ok := forest.Resolve(raw); ok {
				v.Tags = unionLeaves(v.Tags, []string{slug})
			}
		}
		v.Category = forest.PrimaryProductLeaf(v.Tags)
	}
	return v
}

// visionPrompt asks the multimodal model for exactly the fields groundVisionTags will check, and
// tells it the grounding rule in the same words the text tagger's prompt uses. The model returning
// a value it did not read is not an error we can prevent at the prompt — it is why groundVisionTags
// exists — but asking for the honest answer costs nothing and reduces the drop rate.
func visionPrompt(forest *taxonomy.Forest) string {
	return visionPromptFor(forest, false)
}

func splitVisionPrompt(forest *taxonomy.Forest) string {
	return visionPromptFor(forest, true)
}

func visionPromptFor(forest *taxonomy.Forest, includeTitle bool) string {
	vocab := "(no taxonomy vocabulary is configured)"
	if forest != nil {
		vocab = forest.Vocab()
	}
	titleField := ""
	titleRule := ""
	if includeTitle {
		titleField = `,"proposedTitle":"<short, friendly clip title grounded in visibleText; empty if none>"`
		titleRule = " Give proposedTitle only when its identifying product, brand, programme, or subject is literally present in visibleText; a generic label such as commercial is not a title."
	}
	return fmt.Sprintf(`You are shown a few still frames from one candidate segment in a recording. It may be a commercial, promo, bumper, station ID, PSA, trailer, programme material, other non-filler, ambiguous, or materially unusable.
First read any TEXT visible in the frames — a logo, a product name, a slogan, a year.
Return ONLY this JSON, no prose:
{"visibleText":"<the on-screen text you can read, verbatim; empty if none>"%s,"brand":"<advertiser name or empty>","era":<4-digit year visible in a frame, or 0>,"tags":["<zero or more taxonomy slugs>"],"role":"<commercial|promo|bumper|station_id|psa|trailer|programme_fragment|non_filler|ambiguous|unusable>","roleReason":"<brief visual evidence for that role>"}
Rules: put in visibleText only text you can actually READ in the frames; give brand ONLY when the advertiser's name is among that visible text — never guess it from the imagery or the products; give era ONLY when a 4-digit year is visible in a frame — never infer a decade from the film stock, colour, or style, use 0 otherwise;%s choose tags only from the live vocabulary below. Vocabulary entries written as "child (under parent)" explain hierarchy; return only the slug, never the annotation. A tag may describe what the imagery shows even when its slug is not printed as text. Return an empty tags array when the frames do not support a choice. Judge role independently from tags and the parent recording. Use commercial for a product/service offer, promo for promotion of a programme or network property, bumper for a brief transition into or out of a break, station_id for station identification, psa for a public-service message, trailer for promotion of a film or release, programme_fragment for material dependent on a larger programme, and non_filler for other independently bounded material that must not air as filler. Use ambiguous rather than guessing. Use unusable only when the frames are materially unassessable. Explain the observed visual evidence; never infer role from the generated filename or duration.

Live taxonomy vocabulary:
%s`, titleField, titleRule, vocab)
}
