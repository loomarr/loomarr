package filler

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/mantonx/loomarr/internal/llm"
	"github.com/mantonx/loomarr/internal/taxonomy"
)

// The TAG stage (§10 V51b): classify the clip from its TEXT signals and write what grounds.
//
// The classification itself is `Classify` + `validateTags`, untouched — the model is served the
// taxonomy vocabulary and its answer is kept only where the clip's own text supports it. What this
// rung changes is when it happens: after `transcribe`, so the transcript is one of the signals it
// grounds against, rather than on an unrelated cron tick that could land ten minutes earlier.
//
// ⚠ **It ADDS knowledge and never replaces it, and that is why `Rewind` does not clear tags.** The
// scalar fields gap-fill (an era already grounded is never overwritten), the taxonomy leaves are
// UNIONed, and the derived category is re-derived from the merged set. A re-tag that first blanked
// the clip would destroy an operator's hand-edits and their confirmed eras.

// TagClipStore is the slice of the store the tag stage needs.
type TagClipStore interface {
	// ListTaxa returns the taxonomy graph — the vocabulary the tagger SERVES to the model and
	// GROUNDS its answer against (§10 V45a).
	ListTaxa(ctx context.Context) ([]taxonomy.Taxon, error)
	// GetClipTags returns a clip's asserted LEAF tags — what a fresh classification is UNIONed
	// with (leavesOnly=true; keyed by hash).
	GetClipTags(ctx context.Context, clipHash string, leavesOnly bool) ([]string, error)
	// SetClipTags REPLACES a clip's tags with the rollup expansion of the given LEAVES.
	SetClipTags(ctx context.Context, clipHash string, leaves []string) error
	// UpdateClipClassification writes non-taxonomy classifier facts (hash-keyed).
	UpdateClipClassification(ctx context.Context, id string, era int, audience string, suggestedEra int, aiTagged bool, updatedAt time.Time) error
	// SetClipBrand records a GROUNDED advertiser (path-keyed, unlike the tag write).
	SetClipBrand(ctx context.Context, path, brand string, at time.Time) error
	// SetClipConfidence persists the grounding-capped score (§10 V51a).
	SetClipConfidence(ctx context.Context, path string, confidence int, at time.Time) error
}

// TagStage classifies a clip from its text signals.
type TagStage struct {
	provider llm.Provider
	store    TagClipStore
	// drop is the drop-folder as an fs.FS, for the info-JSON sidecar. nil ⇒ filename-only
	// tagging, which degrades the tag rather than failing it.
	drop fs.FS
	now  func() time.Time
}

// NewTagStage builds the stage. A nil provider makes it permanently inapplicable — an install with
// no LLM tags nothing, and every ladder says so rather than showing an empty rung.
func NewTagStage(provider llm.Provider, store TagClipStore, drop fs.FS, now func() time.Time) *TagStage {
	if now == nil {
		now = time.Now
	}
	return &TagStage{provider: provider, store: store, drop: drop, now: now}
}

func (s *TagStage) ID() StageID { return StageTag }

// Cost: cheap on the budget, which is about SCARCITY rather than effort. A text completion is a
// local model turn or an ordinary API call — it costs neither a GPU transcode slot, whisper
// minutes, nor a per-image vision charge, and `MaxClips` already bounds how many run in a pass.
func (s *TagStage) Cost() StageCost { return CostCheap }

// Applies to an untagged COMMERCIAL, and nothing else.
//
// ⚠ The kind filter is the old work list's, kept verbatim: bumpers, station-ids and PSAs serve
// their bookend role without era/audience/category, so spending an LLM call on them buys nothing
// pod matching can use.
func (s *TagStage) Applies(_ context.Context, c StoreClip) (bool, string) {
	if s.provider == nil {
		return false, "no language model is configured"
	}
	if c.Kind != Commercial {
		return false, "only commercials need match tags"
	}
	if c.Tagged() {
		return false, "already fully tagged"
	}
	return true, ""
}

// Run classifies the clip and persists what grounded.
func (s *TagStage) Run(ctx context.Context, c StoreClip) (StageResult, error) {
	taxa, err := s.store.ListTaxa(ctx)
	if err != nil {
		return StageResult{}, err
	}
	forest := taxonomy.New(taxa)
	reportProgress(ctx, StageTag, NoMeasurement)

	// The text signals: the clip's ORIGINAL filename (from the sidecar — after intake renames a
	// clip to its hash, that is the only surviving copy of `Frosted Flakes 1993.mp4`, and §8 grounds
	// an era only where the year appears literally in the text), its sidecar description, and its
	// persisted transcript.
	sug, err := Classify(ctx, s.provider, forest, s.displayName(c), sourceSignals(s.sidecarText(c), c.Transcript))
	if err != nil {
		return StageResult{}, fmt.Errorf("classify %s: %w", c.Path, err)
	}

	// Gap-fill the scalars: a classification only fills what is missing, never clears a set field.
	era := c.Era
	if era == 0 {
		era = sug.Era
	}
	audience := string(c.Audience)
	if audience == "" {
		audience = string(sug.Audience)
	}
	brand := c.Brand
	if brand == "" {
		brand = sug.Brand
	}

	// ⚠ UNION the taxonomy leaves, then RE-DERIVE the category from the merged set — so the stored
	// shadow always equals PrimaryProductLeaf(the persisted leaves), the invariant the design turns
	// on. Unlike the scalars this is not a gap-fill: a re-tag adds newly-grounded leaves to
	// whatever the clip already had.
	existingLeaves, err := s.store.GetClipTags(ctx, c.Hash, true)
	if err != nil {
		return StageResult{}, err
	}
	mergedLeaves := unionLeaves(existingLeaves, sug.Tags)
	category := forest.PrimaryProductLeaf(mergedLeaves)

	// An UNGROUNDED era rides along as a suggestion, and only while the clip has no era at all.
	suggestedEra := 0
	if era == 0 {
		suggestedEra = sug.SuggestedEra
	}

	newBrand := c.Brand == "" && brand != ""
	gainedLeaves := len(mergedLeaves) > len(existingLeaves)
	if era == 0 && audience == "" && category == "" && suggestedEra == 0 && !newBrand && !gainedLeaves {
		// Nothing usable. Not a failure and not a reject — the model read the signals and they did
		// not say what this is. The score rung decides what that means for the clip.
		return StageResult{Verdict: VerdictContinue, Note: "the text signals did not say what this is"}, nil
	}

	// ⚠ Hash, not Path. `UpdateClipClassification` is keyed `WHERE hash = ?` while brand and confidence
	// writers below are path-keyed — the same operation legitimately needs both, and getting it
	// wrong is silent: a hash-keyed call handed a path matches nothing and reports not-found.
	if err := s.store.UpdateClipClassification(ctx, c.Hash, era, audience, suggestedEra, true, s.now().UTC()); err != nil {
		return StageResult{}, err
	}
	if gainedLeaves {
		if err := s.store.SetClipTags(ctx, c.Hash, mergedLeaves); err != nil {
			return StageResult{}, err
		}
	}
	if newBrand {
		if err := s.store.SetClipBrand(ctx, c.Path, brand, s.now().UTC()); err != nil {
			return StageResult{}, err
		}
	}
	// ⚠ **The confidence is written HERE as well as at the score rung, and the duplication is the
	// point.** This is the only place the model's own self-report exists — `Score` lets it LOWER
	// the grounded ceiling (never raise it), so a model that is unsure about a clip whose tags all
	// verify still gets surfaced to a human. The score rung recomputes the ceiling from the
	// persisted row, where that self-report is no longer available, and takes the lower of the two.
	// Drop this write and the lowering half of `Score` would quietly stop working.
	if err := s.store.SetClipConfidence(ctx, c.Path, sug.Confidence, s.now().UTC()); err != nil {
		return StageResult{}, err
	}

	updated := c
	updated.Era = era
	updated.Audience = Audience(audience)
	updated.Category = category
	updated.SuggestedEra = suggestedEra
	updated.Brand = brand
	updated.AITagged = true
	updated.Confidence = sug.Confidence
	if len(mergedLeaves) > 0 {
		updated.Tags = mergedLeaves
	}

	if sug.Complete() {
		return StageResult{Clip: updated, Verdict: VerdictContinue}, nil
	}
	return StageResult{Clip: updated, Verdict: VerdictContinue, Note: "tagged, but not on every axis"}, nil
}

// sidecarText reads the info-JSON beside a clip and renders its text signals.
//
// ⚠ Read by PATH. Clip identity is a content hash and intake files every clip as
// `a3/f9/<hash>.mp4`, so the old walk comparing normalised basenames would match nothing and every
// filed clip would silently lose its sidecar text.
func (s *TagStage) sidecarText(c StoreClip) string {
	if s.drop == nil || c.Path == "" {
		return ""
	}
	return SidecarText(s.drop, c.Path)
}

// displayName is the text the tagger reasons about — the clip's ORIGINAL filename where one was
// recorded, falling back to its catalog name.
//
// ⚠ **This is what keeps era grounding alive after the rename.** §8 accepts an era only when the
// year appears literally in a text signal, and the filename is one of them. Once intake files
// `Frosted Flakes 1993.mp4` as `a3f9….mp4` the path carries no year, so without reading
// `originalName` back out of the sidecar every filename-grounded era would silently become a
// suggestion.
func (s *TagStage) displayName(c StoreClip) string {
	if s.drop != nil && c.Path != "" {
		if tags, ok := ReadSidecarTagsFS(s.drop, c.Path); ok && tags.OriginalName != "" {
			return strings.TrimSuffix(tags.OriginalName, filepath.Ext(tags.OriginalName))
		}
	}
	return c.Name
}
