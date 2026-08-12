package filler

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// The split-review sweep (§10 V54): a reel whose leftover cuts nobody reviewed eventually expires,
// and its recording is reclaimed.
//
// ⚠ **It exists because partial confirm leaves a residue by design.** Every reel now files its
// confident cuts and keeps the doubtful ones back, so the review queue accumulates small proposals
// nobody will ever get to — and each one pins a 1–2 GB recording on disk. Without an expiry the
// feature that shrinks the operator's work grows their storage instead.
//
// ⚠ **A naive sweep is WORSE than none, and this is the trap it is written around.** Deleting the
// proposal alone does not finish the job: the composite is still `is_composite` and still on the
// belt, so the split rung re-detects it on the next pass — propose → partly confirm → leftovers →
// sweep → re-propose, burning a boundary scan every cycle, forever. The reel must also be taken
// OFF the belt, which is what `MarkPipelineFiled` does here.
//
// ⚠ **This is the first thing in Loomarr that deletes an operator's media**, and that is a
// deliberate reversal of the rule `fillerbulk.go` states ("nothing in Loomarr deletes an operator's
// media"). Three constraints keep it defensible, and all three are enforced below or in the store:
// only a reel that ALREADY PRODUCED CLIPS is eligible (a reel that yielded nothing is the
// operator's only copy of that content); only after a window they set; and the catalog ROW survives
// so lineage still resolves.

// SweepStore is the slice of the store the sweep needs.
type SweepStore interface {
	ListSweepableSplitProposals(ctx context.Context, before time.Time) ([]SweepableProposal, error)
	DeleteSplitProposal(ctx context.Context, id string) error
	MarkClipReaped(ctx context.Context, hash string, at time.Time) error
	MarkPipelineFiled(ctx context.Context, hash string, at time.Time) error
}

// SweepableProposal mirrors the store's row — a reel the sweep may retire.
type SweepableProposal struct {
	ProposalID string
	ClipHash   string
	ClipPath   string
	Segments   int
}

// SplitSweeper retires expired split proposals and reclaims their recordings.
type SplitSweeper struct {
	store   SweepStore
	clipDir string
	// window is `filler.split.review_window`, read live so a change takes effect next run.
	window func() time.Duration
	now    func() time.Time
	log    *slog.Logger
}

// NewSplitSweeper builds the sweeper. A nil `window` disables the sweep entirely — the safe
// default, since the alternative is deleting recordings on a schedule nobody configured.
func NewSplitSweeper(store SweepStore, clipDir string, window func() time.Duration, now func() time.Time, log *slog.Logger) *SplitSweeper {
	if now == nil {
		now = time.Now
	}
	return &SplitSweeper{store: store, clipDir: clipDir, window: window, now: now, log: log}
}

// SweepResult is what one pass retired.
type SweepResult struct {
	Retired    int
	ReapedMs   int64 // bytes reclaimed, named for the log line
	ReapedFail int
}

// Run retires every proposal past the window.
func (sw *SplitSweeper) Run(ctx context.Context) (SweepResult, error) {
	var res SweepResult
	if sw.store == nil || sw.window == nil {
		return res, nil
	}
	window := sw.window()
	if window <= 0 {
		// ⚠ Zero means OFF, and it must: an operator who has not chosen an expiry has not agreed
		// to have their recordings deleted. Same three-state encoding the rest of §10 uses.
		return res, nil
	}

	due, err := sw.store.ListSweepableSplitProposals(ctx, sw.now().UTC().Add(-window))
	if err != nil {
		return res, err
	}
	for _, p := range due {
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		if err := sw.retire(ctx, p); err != nil {
			// One bad reel must not stop the sweep — the next pass retries it.
			if sw.log != nil {
				sw.log.Warn("split sweep: could not retire a reel", "clip", p.ClipHash, "err", err)
			}
			res.ReapedFail++
			continue
		}
		res.Retired++
	}
	if res.Retired > 0 && sw.log != nil {
		sw.log.Info("split sweep: retired expired reels",
			"retired", res.Retired, "failed", res.ReapedFail, "windowHours", window.Hours())
	}
	return res, nil
}

// retire is the ORDER-SENSITIVE part, and the order is the whole correctness story.
func (sw *SplitSweeper) retire(ctx context.Context, p SweepableProposal) error {
	// 1. Off the belt FIRST. If anything below fails, a reel that is merely filed is a recoverable
	//    state; a reel whose file is gone while it is still claimable is not — the rung would pick
	//    it up and fail on a missing file every pass.
	if err := sw.store.MarkPipelineFiled(ctx, p.ClipHash, sw.now().UTC()); err != nil {
		return fmt.Errorf("take off the belt: %w", err)
	}
	// 2. The tombstone, BEFORE the unlink. `DeleteClipsNotIn` keys on this, so a sync landing
	//    between the unlink and the mark would prune the row and dangle every child's
	//    `parent_hash`. Marking first makes that window empty.
	if err := sw.store.MarkClipReaped(ctx, p.ClipHash, sw.now().UTC()); err != nil {
		return fmt.Errorf("mark reaped: %w", err)
	}
	// 3. The proposal. After this the leftover cuts are gone for good — which is the decision the
	//    window encodes.
	if err := sw.store.DeleteSplitProposal(ctx, p.ProposalID); err != nil {
		return fmt.Errorf("delete proposal: %w", err)
	}
	// 4. The bytes, LAST and best-effort. Every durable fact above is already recorded, so a failed
	//    unlink leaves a file the operator can remove by hand rather than a half-swept row.
	//    ⚠ `ClipPath` is joined onto the drop dir here and nowhere else — the same containment
	//    boundary `Propose` and `Confirm` use, so a path from the row can never escape FILLER_DIR.
	full := filepath.Join(sw.clipDir, filepath.FromSlash(p.ClipPath))
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		if sw.log != nil {
			sw.log.Warn("split sweep: the recording could not be removed; its row is already retired",
				"clip", p.ClipHash, "path", full, "err", err)
		}
	}
	return nil
}
