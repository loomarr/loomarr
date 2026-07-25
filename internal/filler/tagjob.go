package filler

import (
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/mantonx/loomarr/internal/llm"
)

// The AI-tagging job (§10): classify untagged commercials in the catalog via the
// LLM (text signals only) and write the validated tags. It's an opt-in
// (FILLER_AI_TAGGING, §15) batch over the store's untagged work-list — the same
// grounding rule as the suggester (can only reference clips that actually exist,
// which is inherent here: we hand the model real catalog clips) plus the
// enum-validation in Classify.

// sidecarText finds the info-JSON sidecar for a clip and renders its text signals.
// Returns "" when there is no drop-folder, no sidecar, or nothing useful in it —
// tagging then proceeds on the filename alone, which is the honest fallback for a
// clip that was hand-copied into the drop folder and never had a sidecar.
//
// Matching is by BASENAME rather than a constructed path: Tunarr's scan supplies the
// display name, and it may normalize what the file was called. Guessing a
// name→path transformation would fail silently; scanning and comparing normalized
// basenames either finds the right sidecar or cleanly finds nothing.
func (t *Tagger) sidecarText(clip StoreClip) string {
	if t.drop == nil {
		return ""
	}
	want := normalizeForMatch(clip.Name)
	if want == "" {
		return ""
	}
	var found string
	_ = fs.WalkDir(t.drop, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || found != "" {
			return nil //nolint:nilerr // an unreadable subtree just means no sidecar here
		}
		if !strings.HasSuffix(path, ".info.json") {
			return nil
		}
		base := strings.TrimSuffix(filepath.Base(path), ".info.json")
		if normalizeForMatch(base) == want {
			found = path
		}
		return nil
	})
	if found == "" {
		return ""
	}
	raw, err := fs.ReadFile(t.drop, found)
	if err != nil {
		return ""
	}
	var info sidecarInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return ""
	}
	return info.text()
}

// normalizeForMatch reduces a filename or display name to a comparable form:
// lowercase, with punctuation and whitespace collapsed away. "Toy Ad (1994).mp4" and
// "toy_ad_1994" both become "toyad1994", so Tunarr's display-name tidying doesn't
// break the match.
func normalizeForMatch(s string) string {
	s = strings.TrimSuffix(s, filepath.Ext(s))
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

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
	// drop is the filler drop-folder (FILLER_DIR, §15) as an fs.FS, used to read the
	// info-JSON sidecars ingest writes beside each clip. nil ⇒ tagging falls back to
	// filename-only, which is what a drop-folder clip (hand-copied, no sidecar) gets
	// anyway — so a missing or unreadable folder degrades the tag, never fails it.
	drop fs.FS
}

// NewTagger builds the tagging job. `drop` is the drop-folder FS for sidecar reads;
// pass nil to tag from filenames alone.
func NewTagger(store TagStore, provider llm.Provider, drop fs.FS, now func() time.Time, log *slog.Logger) *Tagger {
	if now == nil {
		now = time.Now
	}
	return &Tagger{store: store, provider: provider, drop: drop, log: log, now: now}
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
		// Text signals: the clip name + the info-JSON sidecar ingest wrote beside it
		// (§10). This used to pass `clip.Source` — a PROVENANCE enum — so every prompt
		// read "Source description: tunarr-local", feeding the classifier a misleading
		// constant while the real title/description sat unread in the sidecar.
		sug, err := Classify(ctx, t.provider, clip.Name, t.sidecarText(clip))
		if err != nil {
			if t.log != nil {
				t.log.Warn("clip classify failed", "clip", clip.Path, "err", err)
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
		if err := t.store.UpdateClipTags(ctx, clip.Path, era, audience, category, true, t.now()); err != nil {
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
