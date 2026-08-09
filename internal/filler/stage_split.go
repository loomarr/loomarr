package filler

import (
	"context"
	"fmt"
	"time"
)

// The SPLIT stage (§10 V51b): a compilation is cut into the adverts it holds.
//
// V43's `SplitRunner` logic, minus the sweep. Detection is `Splitter.Propose` and the gate is
// `AutoConfirmable`, both unchanged — what moves is who decides a clip is a candidate (the probe
// rung, which now sets `is_composite`) and what happens to the pieces (they are SPAWNED, so each
// segment runs the whole ladder for itself instead of waiting for six cron jobs to notice it).
//
// ⚠ **`filler.autosplit.enabled` now defaults ON** (maintainer decision, V51b). The gate it turns
// on is strict and unchanged: the whole reel qualifies or none of it does, `SuggestedEra > 0`
// disqualifies at every threshold, and `MaxAutoFileConfidence` remains the ceiling. A reel that
// fails the gate keeps its proposal and appears in Incoming exactly as it does today — with
// `AutoSplitReject`'s reason recorded, so "why did this not auto-cut?" is answerable without
// reading the log.

// SplitStageStore is the slice of the store the split stage reads.
//
// ⚠ One read of the pending queue rather than a per-clip existence check — the queue is a review
// backlog a human is expected to clear, so it is small by construction, and re-detecting over a
// proposal an operator is halfway through editing is the thing being prevented.
type SplitStageStore interface {
	ListSplitProposals(ctx context.Context) ([]SplitProposal, error)
}

// SplitStage proposes (and where allowed, confirms) the cuts inside a compilation.
type SplitStage struct {
	splitter *Splitter
	store    SplitStageStore
	// autoConfirm is the opt-in gate. nil ⇒ propose only, every reel waits for a human.
	autoConfirm *AutoSplitPolicy
	// minClipDuration is `filler.min_duration`, the SAME floor the scan boundary rejects on.
	minClipDuration func() time.Duration
}

// NewSplitStage builds the stage. Without `WithAutoConfirm` it PROPOSES ONLY, which is the safe
// default: proposing writes no clips and consumes no file, so the waiting is removed without the
// deciding.
func NewSplitStage(splitter *Splitter, store SplitStageStore) *SplitStage {
	return &SplitStage{splitter: splitter, store: store}
}

// WithAutoConfirm attaches the auto-confirm policy and the clip floor it must respect.
func (s *SplitStage) WithAutoConfirm(pol AutoSplitPolicy, minClipDuration func() time.Duration) *SplitStage {
	s.autoConfirm = &pol
	s.minClipDuration = minClipDuration
	return s
}

func (s *SplitStage) ID() StageID     { return StageSplit }
func (s *SplitStage) Cost() StageCost { return CostSplit }

// Applies to a composite, and nothing else.
//
// ⚠ **The candidate decision is the probe rung's, not this one's.** Probe measures the file and
// sets `is_composite`; this rung acts on that mark. Splitting the question that way is what lets
// the expensive boundary scan be gated on duration in ONE place, and it is why a 16-minute
// recording is excluded from pods the moment it is measured rather than only once someone splits
// it — the exact bug §10 V45 describes.
func (s *SplitStage) Applies(_ context.Context, c StoreClip) (bool, string) {
	if s.splitter == nil {
		return false, "splitting is not available on this install"
	}
	if !c.IsComposite {
		return false, "not a compilation"
	}
	return true, ""
}

// Run detects the cuts and either makes them or leaves them for a human.
func (s *SplitStage) Run(ctx context.Context, c StoreClip) (StageResult, error) {
	// Already review-pending: re-detecting would replace a proposal an operator may be halfway
	// through editing. ⚠ Keyed by HASH on both sides — the identity `Propose` takes.
	pending, err := s.store.ListSplitProposals(ctx)
	if err != nil {
		return StageResult{}, err
	}
	for _, p := range pending {
		if p.ClipHash == c.Hash {
			return StageResult{Verdict: VerdictReview, Note: "its cuts are waiting for you to review"}, nil
		}
	}

	p, err := s.splitter.Propose(ctx, c.Hash)
	if err != nil {
		return StageResult{}, err
	}

	// ⚠ Auto-confirm is decided HERE, not inside `Propose`. The manual path runs the same Propose
	// from a button an operator just pressed — a human is already present and about to review, so
	// confirming under them would take the decision they came to make.
	if reject := AutoConfirmable(*p, s.autoConfirm, s.minClipFloor()); reject != AutoSplitOK {
		// ⚠ The reason is RECORDED on the ladder, not just logged. "It didn't auto-confirm" is
		// unactionable for an operator deciding whether to lower a threshold.
		return StageResult{Verdict: VerdictReview, Note: string(reject)}, nil
	}

	spawned, err := s.splitter.Confirm(ctx, p.ID, p.Segments)
	if err != nil {
		// The proposal survives, so the operator can still review it by hand — a failed
		// auto-confirm degrades to the manual path rather than losing the detection work. Returned
		// as an error so the runner retries with backoff; `split` is not fatal, so exhausting the
		// retries leaves the reel in review, which is where a failed cut belongs.
		return StageResult{}, err
	}
	reportProgress(ctx, StageSplit, 100)

	// ⚠ The segments are SPAWNED, so each one is enrolled at `probe` and runs the whole ladder for
	// itself — transcoded, heard, transcribed, tagged and scored like any other arrival. Before
	// this, a freshly cut segment had to wait for whichever of six sweeps happened to reach it.
	note := fmt.Sprintf("cut into %d adverts", len(spawned))
	if len(spawned) == 1 {
		note = "cut into one advert"
	}
	return StageResult{Verdict: VerdictContinue, Spawned: spawned, Note: note}, nil
}

// minClipFloor resolves the clip-duration floor, defaulting to zero (no floor check).
func (s *SplitStage) minClipFloor() time.Duration {
	if s.minClipDuration == nil {
		return 0
	}
	return s.minClipDuration()
}
