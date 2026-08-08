package filler

import (
	"context"
	"log/slog"
	"time"
)

// The scheduled split pass (§10 V43) — detection over the catalog, unattended.
//
// ⚠ **Splitting used to happen ONLY on a button press**, and its result always required a human.
// That made compilations the most manual part of a system whose claim is that it maintains
// itself: the tagger beside it files clips unattended above a threshold, while a 13-minute
// recording holding twenty adverts sat waiting for someone to click, wait minutes for ffmpeg,
// and confirm every cut by hand.

// SplitRunStore is the slice of the store the runner needs.
//
// ⚠ Reuses `ClipQuery` and `ListSplitProposals` rather than declaring a
// `ProposalExistsForClip`. One read of the pending queue answers "which clips already have a
// proposal waiting" for every candidate at once — a per-clip existence check would be a query
// per candidate to learn what one list already says, and the queue is small by construction
// (it is a review backlog a human is expected to clear).
type SplitRunStore interface {
	ListClips(ctx context.Context, f ClipQuery) ([]StoreClip, error)
	ListSplitProposals(ctx context.Context) ([]SplitProposal, error)
}

// SplitRunner proposes cuts for over-long catalog clips on a schedule.
type SplitRunner struct {
	store    SplitRunStore
	splitter *Splitter
	// maxDuration is `filler.autosplit.max_duration`, read per run. A clip LONGER than one
	// advert is a compilation worth detecting; anything shorter is already a clip.
	maxDuration func() time.Duration
	// maxPerRun bounds the work. Detection is minutes per file, so an unbounded first pass over
	// a large catalog would run for hours and hold the job slot the whole time.
	maxPerRun int
	// autoConfirm is the opt-in gate (§10 V43). nil ⇒ every proposal waits for a human, which is
	// the safe default for a caller that has not opted in — the same shape `Tagger.autoFile` uses.
	autoConfirm *AutoSplitPolicy
	// minClipDuration is `filler.min_duration`, the SAME floor the scan boundary rejects on.
	// Read per call so a settings change applies on the next run.
	minClipDuration func() time.Duration
	log             *slog.Logger
}

// WithAutoConfirm attaches the auto-confirm policy and the clip floor it must respect.
//
// ⚠ Absent, the runner PROPOSES ONLY — every reel waits for review. That is the default and the
// safe one: proposing writes no clips and consumes no file, so a caller who has not opted in gets
// the waiting removed without the deciding.
func (r *SplitRunner) WithAutoConfirm(pol AutoSplitPolicy, minClipDuration func() time.Duration) *SplitRunner {
	r.autoConfirm = &pol
	r.minClipDuration = minClipDuration
	return r
}

// minClipFloor resolves the clip-duration floor, defaulting to zero (no floor check) when the
// caller wired none — the gate's other checks still apply.
func (r *SplitRunner) minClipFloor() time.Duration {
	if r.minClipDuration == nil {
		return 0
	}
	return r.minClipDuration()
}

// NewSplitRunner wires the scheduled pass.
func NewSplitRunner(st SplitRunStore, sp *Splitter, maxDuration func() time.Duration, log *slog.Logger) *SplitRunner {
	if log == nil {
		log = slog.Default()
	}
	return &SplitRunner{store: st, splitter: sp, maxDuration: maxDuration, maxPerRun: defaultSplitsPerRun, log: log}
}

// defaultSplitsPerRun bounds one pass.
//
// ⚠ Small on purpose. Detection is MINUTES per file (chapters → blackdetect/silencedetect →
// whisper rescue), so this is not a throughput knob — it is what stops the first run after an
// upgrade from occupying the scheduler for hours on a catalog full of recordings. The next run
// picks up where this one stopped, so a backlog drains over cycles rather than in one sitting.
const defaultSplitsPerRun = 3

// SplitRunResult reports what one pass did.
type SplitRunResult struct {
	Considered int // over-long clips seen
	Proposed   int // proposals created
	Skipped    int // already had a proposal waiting
	Failed     int // detection errored (not fatal to the run)
	// Confirmed counts reels cut with NO human (§10 V43). Reported separately from Proposed
	// because they are different events for an operator: one made work, the other did it.
	Confirmed int
}

// Run proposes cuts for over-long clips that do not already have a proposal waiting.
//
// ⚠ It PROPOSES only — confirming is a separate decision behind `filler.autosplit.enabled`.
// That split is why this job can be ON by default: an unconfirmed proposal writes no clips and
// consumes no file, so the cost of being wrong here is a review the operator ignores.
func (r *SplitRunner) Run(ctx context.Context) (SplitRunResult, error) {
	var res SplitRunResult
	if r == nil || r.splitter == nil || r.store == nil {
		return res, nil
	}

	maxDur := 120 * time.Second
	if r.maxDuration != nil {
		if d := r.maxDuration(); d > 0 {
			maxDur = d
		}
	}

	// ⚠ `IncludeHeld: false` — the CATALOG only. A held clip is waiting for a human to decide
	// whether it belongs at all; detecting cuts inside something nobody has accepted yet spends
	// minutes of ffmpeg on a file that may be about to be dropped.
	clips, err := r.store.ListClips(ctx, ClipQuery{})
	if err != nil {
		return res, err
	}

	// One read of the pending queue, not a query per candidate. Keyed by the clip each proposal
	// belongs to, so re-detection cannot replace a proposal an operator is halfway through
	// editing.
	pending, err := r.store.ListSplitProposals(ctx)
	if err != nil {
		return res, err
	}
	// ⚠ Keyed by HASH, matched against `c.Hash` below — the same identity `Propose` takes. It was
	// path-on-both-sides before V51a, which agreed with itself but not with the clip lookup two
	// lines down; keying the guard on the identity the work is actually done under is what keeps
	// "already pending" and "propose this" talking about the same clip.
	waiting := make(map[string]struct{}, len(pending))
	for _, p := range pending {
		waiting[p.ClipHash] = struct{}{}
	}

	for _, c := range clips {
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		default:
		}
		if res.Proposed >= r.maxPerRun {
			break
		}
		if time.Duration(c.DurationMs)*time.Millisecond <= maxDur {
			continue // already advert-shaped; nothing to split
		}
		res.Considered++

		// Already review-pending: re-detecting would replace a proposal an operator may be
		// halfway through editing.
		if _, pendingReview := waiting[c.Hash]; pendingReview {
			res.Skipped++
			continue
		}

		p, err := r.splitter.Propose(ctx, c.Hash)
		if err != nil {
			// ⚠ NOT fatal to the run. Detection failing on one recording (an unreadable file, no
			// usable boundaries) must not stop the other candidates being proposed — the same
			// per-item tolerance the reconciler and the janitor use.
			res.Failed++
			r.log.Warn("auto-split: detection failed", "clip", c.Path, "err", err)
			continue
		}
		res.Proposed++

		// ⚠ Auto-confirm is decided HERE, not inside `Propose`. The manual path runs the same
		// Propose from a button an operator just pressed — a human is already present and about
		// to review, so confirming under them would take the decision they came to make. Only
		// the unattended path asks this question.
		if reject := AutoConfirmable(*p, r.autoConfirm, r.minClipFloor()); reject != AutoSplitOK {
			// ⚠ Logged at INFO with the REASON. An unattended decision that leaves no trace is
			// not one an appliance gets to make (§10), and "it didn't auto-confirm" is
			// unactionable for an operator deciding whether to lower the threshold.
			r.log.Info("auto-split: left for review", "clip", c.Path, "segments", len(p.Segments), "reason", string(reject))
			continue
		}
		if err := r.splitter.Confirm(ctx, p.ID, p.Segments); err != nil {
			// The proposal survives, so the operator can still review it by hand. A failed
			// auto-confirm degrades to the manual path rather than losing the detection work.
			r.log.Warn("auto-split: confirm failed; the proposal is left for review",
				"clip", c.Path, "proposal", p.ID, "err", err)
			continue
		}
		res.Confirmed++
		r.log.Info("auto-split: cut a compilation unattended",
			"clip", c.Path, "segments", len(p.Segments))
	}
	return res, nil
}
