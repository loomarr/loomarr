package filler

import (
	"context"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/taxonomy"
)

// The AI-tagging job (§10): classify untagged commercials in the catalog via the
// LLM (text signals only) and write the validated tags. It's an opt-in
// (FILLER_AI_TAGGING, §15) batch over the store's untagged work-list — the same
// grounding rule as the suggester (can only reference clips that actually exist,
// which is inherent here: we hand the model real catalog clips) plus the
// enum-validation in Classify.

// (`sidecarText` and `displayName` lived here until V51b. They now sit on `TagStage`, because the
// per-clip tagging logic does — this sweep and the pipeline's tag rung share ONE implementation,
// and a second copy of the sidecar read is how the two would drift on the thing that keeps era
// grounding alive: after intake renames a clip to its hash, the sidecar's `originalName` is the
// only surviving copy of `Frosted Flakes 1993.mp4`.)

// (`normalizeForMatch` lived here until V38c. It reduced a filename to a comparable form so the
// sidecar walk above could survive Tunarr's display-name tidying. Both went together: with clips
// filed under a content hash there is nothing to fuzzily match, and a helper whose only caller is
// gone is dead weight that reads as a capability.)

// sourceSignals joins the sidecar text and the clip's persisted transcript into the one
// `sourceText` string Classify grounds against (§10 V44). Both are optional and either may be
// empty — a drop-folder clip has no sidecar, a wordless or not-yet-transcribed clip has no
// transcript — so it drops the empties rather than emitting blank lines that would read as signal.
//
// ⚠ The transcript is a GROUNDING signal, not decoration. `validateTags` accepts a brand or an era
// only when it appears literally in exactly this text; a cereal advert with an empty sidecar
// grounds its "Kellogg's" here or nowhere. Joined with a newline so a year or a name that ends one
// signal and abuts the next cannot fuse into a token that matches neither.
func sourceSignals(sidecar, transcript string) string {
	parts := make([]string, 0, 2)
	if s := strings.TrimSpace(sidecar); s != "" {
		parts = append(parts, s)
	}
	if tr := strings.TrimSpace(transcript); tr != "" {
		parts = append(parts, tr)
	}
	return strings.Join(parts, "\n")
}

// unionLeaves merges two leaf-slug sets into a de-duplicated, sorted set (§10 V45a) — the tagger's
// "add knowledge, never replace" rule for the taxonomy tags. Sorted so a re-tag that gains nothing
// produces the identical slice (the churn-avoidance guard compares lengths, and stable order keeps
// SetClipTags reproducible).
func unionLeaves(existing, fresh []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(existing)+len(fresh))
	for _, s := range existing {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, s := range fresh {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// TagStore is the slice of the store the tagging job needs.
type TagStore interface {
	// ListUntaggedCommercials returns commercials missing match tags (the work list).
	ListUntaggedCommercials(ctx context.Context) ([]StoreClip, error)
	// ListTaxa returns the taxonomy graph — the vocabulary the tagger SERVES to the model and GROUNDS
	// its answer against (§10 V45a). Loaded once per run and built into a Forest,
	// so a graph edit takes effect on the next run without a restart.
	ListTaxa(ctx context.Context) ([]taxonomy.Taxon, error)
	// GetClipTags returns a clip's asserted LEAF tags — what a fresh classification is UNIONed with, so
	// a re-tag adds knowledge rather than replacing it (leavesOnly=true; keyed by hash).
	GetClipTags(ctx context.Context, clipHash string, leavesOnly bool) ([]string, error)
	// SetClipTags REPLACES a clip's tags with the rollup expansion of the given LEAVES (§10 V45a) — the
	// per-clip taxonomy writer. The tagger calls it with the union of existing + newly-grounded leaves.
	SetClipTags(ctx context.Context, clipHash string, leaves []string) error
	// UpdateClipClassification writes the non-taxonomy classifier facts. The category shadow belongs
	// to SetClipTags so a caller cannot persist tags and a category from different graph generations.
	UpdateClipClassification(ctx context.Context, id string, era int, audience string, suggestedEra int, aiTagged bool, updatedAt time.Time) error
	// SetClipBrand records a GROUNDED advertiser from the TEXT tagger (§10 V44). Separate from
	// UpdateClipClassification because brand has a different key and a different writer story: it is
	// PATH-keyed (like the transcript/vision writers it sits beside), while classification is
	// hash-keyed. Only a brand `Classify` already grounded reaches here — the store call writes
	// what it is given, the grounding lives in validateTags.
	SetClipBrand(ctx context.Context, path, brand string, at time.Time) error
	// SetClipConfidence persists the grounding-capped score (§10 V38) — PATH-keyed, beside
	// SetClipBrand. ⚠ Added in V51a because the score had NO writer: it was computed here, used
	// once for the auto-file comparison, and dropped, so `confidence` was 0 for every clip in
	// every catalog and the Incoming meter never rendered.
	SetClipConfidence(ctx context.Context, path string, confidence int, at time.Time) error
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

// Allows reports whether a clip scoring `score` may be filed unattended.
//
// ⚠ The threshold is clamped to MaxAutoFileConfidence here as well as in the registry. The
// registry validates what an operator TYPES; this guards what the code READS — an env pin, a
// migration, or a hand-edited database row can all put an out-of-range value in front of this,
// and a threshold of 0 would file everything including ungrounded eras.
//
// ⚠ **On the policy, not on the Tagger**, so the pipeline's score stage and the tagger apply the
// SAME clamp rather than each carrying a copy. Two implementations of a safety ceiling is how one
// of them ends up missing the guard — and the guard is the only thing keeping a fabricated era out
// of a live channel.
func (p *AutoFilePolicy) Allows(score int) bool {
	if p == nil || p.Enabled == nil || !p.Enabled() {
		return false
	}
	min := MaxAutoFileConfidence
	if p.MinConfidence != nil {
		min = p.MinConfidence()
	}
	if min <= 0 || min > MaxAutoFileConfidence {
		min = MaxAutoFileConfidence
	}
	return score >= min
}

// shouldAutoFile defers to the policy so there is one clamp.
func (t *Tagger) shouldAutoFile(score int) bool { return t.autoFile.Allows(score) }

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
	// ⚠ **ONE implementation of per-clip tagging, shared with the ingest pipeline's tag rung**
	// (§10 V51b). This sweep is the operator's "tag everything now" action; the rung is the
	// unattended path that runs as a clip arrives. They were separate code paths for about an hour
	// during V51b, which is exactly long enough to notice that a second copy of the grounding
	// merge is how one of them ends up missing a guard — the union rule, the gap-fill rule and the
	// hash-vs-path split are all easy to get subtly wrong twice.
	stage := NewTagStage(t.provider, t.store, t.drop, t.now)
	res := TagResult{Considered: len(work)}
	for _, clip := range work {
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		default:
		}
		// ⚠ `Applies` is asked even here, where the work list has already filtered. The list answers
		// "is this an untagged commercial?" from SQL; the rung answers "is there a model to ask?".
		// A sweep that skipped the second question would report every clip as Skipped on an install
		// with no LLM and give no reason for it.
		if ok, _ := stage.Applies(ctx, clip); !ok {
			res.Skipped++
			continue
		}
		out, err := stage.Run(ctx, clip)
		if err != nil {
			// ⚠ Per-clip tolerance, unlike the rung. A sweep that aborted on one unreadable clip
			// would leave the rest of the catalog untagged and report a failure the operator
			// cannot act on; the pipeline can afford to fail one clip because it retries it.
			if t.log != nil {
				t.log.Warn("clip tagging failed", "clip", clip.Path, "err", err)
			}
			res.Skipped++
			continue
		}
		if out.Clip.Hash == "" {
			res.Skipped++
			continue // the rung found nothing usable and wrote nothing
		}
		// The filing decision (§10 V38). Only HELD clips are candidates: a clip already in the
		// catalog was filed by a human or by an earlier run, and re-filing it would flip its
		// `auto_filed` attribution — rewriting the answer to "did anyone look at this?".
		//
		// ⚠ The score is the rung's grounding-CAPPED confidence, not the model's self-report. An
		// ungrounded era cannot reach any settable threshold, so the fabrication class this guards
		// stays with a person however this install is configured.
		//
		// ⚠ It lives here rather than in the rung because the two drivers file at different
		// moments: the pipeline files at the SCORE rung, after vision has had its turn, while this
		// sweep has no later rung to defer to. Filing inside the tag rung would file a clip before
		// the tier that exists for wordless spots had run.
		if clip.Held && t.shouldAutoFile(out.Clip.Confidence) {
			if _, err := t.store.SetClipsHeld(ctx, []string{clip.Path}, false, true, t.now()); err != nil {
				return res, err
			}
			res.AutoFiled++
		}
		if out.Clip.Tagged() {
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
