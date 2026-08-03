package filler

import (
	"context"
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

// sidecarText reads the info-JSON beside a clip and renders its text signals. Returns "" when
// there is no clip folder, no sidecar, or nothing useful in it — tagging then proceeds on the
// name alone, the honest fallback for a clip that never had a sidecar.
//
// ⚠ **Read by PATH, which V38c made both possible and necessary.** This used to walk the whole
// folder comparing NORMALIZED BASENAMES, because clip identity was a display name that Tunarr's
// scan might have tidied and no reliable name→path mapping existed. Intake now files every clip
// as `a3/f9/<hash>.mp4`, so that walk compares "frostedflakes1993" against "a3f9…" and matches
// nothing: every filed clip would silently lose its sidecar text and fall back to filename-only
// tagging. The row carries the path, so guessing was replaced by reading.
func (t *Tagger) sidecarText(clip StoreClip) string {
	if t.drop == nil || clip.Path == "" {
		return ""
	}
	return SidecarText(t.drop, clip.Path)
}

// displayName is the text the tagger reasons about — the clip's ORIGINAL filename where one was
// recorded, falling back to its catalog name.
//
// ⚠ **This is what keeps era grounding alive after the rename** (§10 V38c). §8's rule accepts an
// era only when the year appears literally in the clip's text signals, and the filename is one of
// them (`Frosted Flakes 1993.mp4`). Once intake files that as `a3f9….mp4` the path carries no
// year at all, so without reading `originalName` back out of the sidecar every clip whose era
// came from its filename would become ungrounded — the model's guess would be capped to a
// suggestion and no one would ever be told why.
//
// The catalog `Name` is the fallback rather than the primary because it is DERIVED from the same
// sidecar field by the scan, and a clip whose sidecar has gone missing keeps whatever name it was
// last catalogued with.
func (t *Tagger) displayName(clip StoreClip) string {
	if t.drop != nil && clip.Path != "" {
		if tags, ok := ReadSidecarTagsFS(t.drop, clip.Path); ok && tags.OriginalName != "" {
			return strings.TrimSuffix(tags.OriginalName, filepath.Ext(tags.OriginalName))
		}
	}
	return clip.Name
}

// (`normalizeForMatch` lived here until V38c. It reduced a filename to a comparable form so the
// sidecar walk above could survive Tunarr's display-name tidying. Both went together: with clips
// filed under a content hash there is nothing to fuzzily match, and a helper whose only caller is
// gone is dead weight that reads as a capability.)

// TagStore is the slice of the store the tagging job needs.
type TagStore interface {
	// ListUntaggedCommercials returns commercials missing match tags (the work list).
	ListUntaggedCommercials(ctx context.Context) ([]StoreClip, error)
	UpdateClipTags(ctx context.Context, id string, era int, audience, category string, suggestedEra int, aiTagged bool, updatedAt time.Time) error
	// SetClipsHeld files a clip into the catalog (§10 V38). The tagger calls it only to FILE
	// (held=false, autoFiled=true) — sending a clip back for review is a human's decision,
	// made from Incoming, never the job's.
	SetClipsHeld(ctx context.Context, paths []string, held, autoFiled bool, at time.Time) (int, error)
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
	// autoFile decides whether a freshly-tagged HELD clip is filed without a human (§10 V38).
	// nil ⇒ the tagger tags but never files, which is the safe default for a caller that has
	// not opted in: every held clip waits for a person.
	autoFile *AutoFilePolicy
}

// NewTagger builds the tagging job. `drop` is the drop-folder FS for sidecar reads;
// pass nil to tag from filenames alone.
func NewTagger(store TagStore, provider llm.Provider, drop fs.FS, now func() time.Time, log *slog.Logger) *Tagger {
	if now == nil {
		now = time.Now
	}
	return &Tagger{store: store, provider: provider, drop: drop, log: log, now: now}
}

// AutoFilePolicy decides whether a freshly-tagged HELD clip is filed without a human (§10 V38).
//
// ⚠ A closure read at RUN time, not a value captured at construction, so changing the threshold
// in Settings takes effect on the next run rather than at the next restart — the same hot-apply
// contract `WithEnabled` uses for the source switch.
type AutoFilePolicy struct {
	// Enabled reports whether auto-filing is on at all.
	//
	// ⚠ Must be backed by a FAIL-CLOSED read (`boolv`, not `boolOn`). When the settings service
	// cannot answer, the safe answer here is "don't file" — failing open would publish unreviewed
	// clips to live channels precisely when the install is degraded, which is the worst moment to
	// be making unattended decisions. This is the opposite polarity from the folder-scan switch,
	// where failing open merely keeps scanning.
	Enabled func() bool
	// MinConfidence is the score a clip must reach. Bounded 50-95 by the registry; see
	// MaxAutoFileConfidence for why the ceiling is load-bearing.
	MinConfidence func() int
}

// WithAutoFile attaches the auto-filing policy. Absent, the tagger tags but never files —
// every held clip waits for a human, which is the safe default for a caller that has not
// opted in.
func (t *Tagger) WithAutoFile(p AutoFilePolicy) *Tagger {
	t.autoFile = &p
	return t
}

// shouldAutoFile reports whether a clip scoring `score` may be filed unattended.
//
// ⚠ The threshold is clamped to MaxAutoFileConfidence here as well as in the registry. The
// registry validates what an operator TYPES; this guards what the code READS — an env pin, a
// migration, or a hand-edited database row can all put an out-of-range value in front of this,
// and a threshold of 0 would file everything including ungrounded eras.
func (t *Tagger) shouldAutoFile(score int) bool {
	if t.autoFile == nil || !t.autoFile.Enabled() {
		return false
	}
	min := t.autoFile.MinConfidence()
	if min <= 0 || min > MaxAutoFileConfidence {
		min = MaxAutoFileConfidence
	}
	return score >= min
}

// TagResult reports what a tagging run did.
type TagResult struct {
	Considered int // untagged commercials examined
	Tagged     int // clips that got a complete classification and were written
	Partial    int // clips the model tagged partially (some field dropped) — still written
	Skipped    int // clips the model couldn't classify at all
	// AutoFiled counts HELD clips this run filed into the catalog without a human (§10 V38).
	// Reported so the operator can see what happened unattended — an unattended decision that
	// leaves no trace is not one an appliance gets to make.
	AutoFiled int
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
		// Text signals: the clip's ORIGINAL filename + the info-JSON sidecar beside it (§10).
		//
		// This used to pass `clip.Source` — a PROVENANCE enum — so every prompt read
		// "Source description: tunarr-local", feeding the classifier a misleading constant
		// while the real title/description sat unread in the sidecar.
		//
		// ⚠ And it must be `displayName`, not `clip.Name`, since V38c: the era can only be
		// GROUNDED if the year appears literally in a text signal, and after intake renames a
		// clip to its hash the only surviving copy of `Frosted Flakes 1993.mp4` is the
		// sidecar's `originalName`.
		sug, err := Classify(ctx, t.provider, t.displayName(clip), t.sidecarText(clip))
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
		// An UNGROUNDED era (§10, V34) is never written to era — it rides along as
		// a suggestion for the operator, and only while the clip has no era at all
		// (a known era needs no suggestion). A suggestion alone is still worth the
		// write: it is the difference between "the model guessed" and silence.
		suggestedEra := 0
		if era == 0 {
			suggestedEra = sug.SuggestedEra
		}
		if era == 0 && audience == "" && category == "" && suggestedEra == 0 {
			res.Skipped++
			continue // nothing usable
		}
		if err := t.store.UpdateClipTags(ctx, clip.Path, era, audience, category, suggestedEra, true, t.now()); err != nil {
			return res, err
		}
		// The filing decision (§10 V38). Only HELD clips are candidates: a clip already in the
		// catalog was filed by a human or by an earlier run, and re-filing it would flip its
		// `auto_filed` attribution — rewriting the answer to "did anyone look at this?".
		//
		// ⚠ The score is `sug.Confidence`, which is grounding-CAPPED (TagSuggestion.Score), not
		// the model's self-report. An ungrounded era cannot reach any settable threshold, so the
		// fabrication class this section guards stays with a person however this is configured.
		if clip.Held && t.shouldAutoFile(sug.Confidence) {
			if _, err := t.store.SetClipsHeld(ctx, []string{clip.Path}, false, true, t.now()); err != nil {
				return res, err
			}
			res.AutoFiled++
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
