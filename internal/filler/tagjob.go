package filler

import (
	"context"
	"log/slog"
	"time"

	"github.com/mantonx/loomarr/internal/llm"
)

// The AI-tagging job (§10): classify untagged commercials in the catalog via the
// LLM (text signals only) and write the validated tags. It's an opt-in
// (FILLER_AI_TAGGING, §15) batch over the store's untagged work-list — the same
// grounding rule as the suggester (can only reference clips that actually exist,
// which is inherent here: we hand the model real catalog clips) plus the
// enum-validation in Classify.

// TagStore is the slice of the store the tagging job needs.
type TagStore interface {
	// ListUntaggedCommercials returns commercials missing match tags (the work list).
	ListUntaggedCommercials(ctx context.Context) ([]StoreClip, error)
	UpdateClipTags(ctx context.Context, id string, era int, audience, category string, aiTagged bool, updatedAt time.Time) error
}

// Tagger runs AI classification over untagged clips.
type Tagger struct {
	store    TagStore
	provider llm.Provider
	log      *slog.Logger
	now      func() time.Time
}

// NewTagger builds the tagging job.
func NewTagger(store TagStore, provider llm.Provider, now func() time.Time, log *slog.Logger) *Tagger {
	if now == nil {
		now = time.Now
	}
	return &Tagger{store: store, provider: provider, log: log, now: now}
}

// TagResult reports what a tagging run did.
type TagResult struct {
	Considered int // untagged commercials examined
	Tagged     int // clips that got a complete classification and were written
	Partial    int // clips the model tagged partially (some field dropped) — still written
	Skipped    int // clips the model couldn't classify at all
}

// Run classifies every untagged commercial and writes the validated tags (§10).
// A clip the model can't fully classify keeps whatever valid fields it gave
// (partial tagging still helps matching); a clip it can't classify at all is left
// untagged for a human. Respects ctx cancellation (JOB_TIMEOUT bound by the
// caller). Grounded: only real catalog clips, only enum-valid tags.
func (t *Tagger) Run(ctx context.Context) (TagResult, error) {
	work, err := t.store.ListUntaggedCommercials(ctx)
	if err != nil {
		return TagResult{}, err
	}
	res := TagResult{Considered: len(work)}
	for _, clip := range work {
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		default:
		}
		// Text signals: the clip name + its source provenance (yt-dlp/Archive
		// title/description are preserved as the clip name/source at ingest, §10).
		sug, err := Classify(ctx, t.provider, clip.Name, clip.Source)
		if err != nil {
			if t.log != nil {
				t.log.Warn("clip classify failed", "clip", clip.TunarrProgramID, "err", err)
			}
			res.Skipped++
			continue
		}
		// Merge with any existing partial tags (e.g. an era already seeded from the
		// filename) — the classification only fills gaps, never clears a set field.
		era := clip.Era
		if era == 0 {
			era = sug.Era
		}
		audience := string(clip.Audience)
		if audience == "" {
			audience = string(sug.Audience)
		}
		category := clip.Category
		if category == "" {
			category = sug.Category
		}
		if era == 0 && audience == "" && category == "" {
			res.Skipped++
			continue // nothing usable
		}
		if err := t.store.UpdateClipTags(ctx, clip.TunarrProgramID, era, audience, category, true, t.now()); err != nil {
			return res, err
		}
		if era > 0 && audience != "" && category != "" {
			res.Tagged++
		} else {
			res.Partial++
		}
	}
	if t.log != nil {
		t.log.Info("filler AI tagging run", "considered", res.Considered, "tagged", res.Tagged, "partial", res.Partial, "skipped", res.Skipped)
	}
	return res, nil
}
