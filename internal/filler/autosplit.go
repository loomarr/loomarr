package filler

import (
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/filler/split"
)

// Auto-confirm for split proposals — the gate behind `filler.autosplit.enabled`. The gate itself is
// `CertifiedStructureMaterializable` (§10 V67); this file holds the policy it reads and the
// partition it returns.

// AutoSplitPolicy is what the gate reads, resolved per call so a settings change takes effect on
// the next run rather than at the next restart.
type AutoSplitPolicy struct {
	// Enabled reports whether auto-confirm is on at all.
	//
	// Backed by the `filler.autosplit.enabled` handle, so a reader that cannot answer reads the
	// setting's declared default (on). Without a store there is no catalog for the gate to cut.
	Enabled func() bool
	// MaxDuration is the longest a segment may be and still be advert-shaped.
	MaxDuration func() time.Duration
}

// AutoSplitReject names why a proposal was not auto-confirmed. Returned rather than logged so
// the caller can report it — an unattended decision that leaves no trace is not one an appliance
// gets to make (§10), and "it just didn't" is unactionable for an operator.
type AutoSplitReject string

const (
	AutoSplitOK AutoSplitReject = ""
	// RejectDisabled is the ordinary state: the operator has not opted in.
	RejectDisabled   AutoSplitReject = "auto-split is off"
	RejectNoSegments AutoSplitReject = "the proposal has no segments"
	RejectTooLong    AutoSplitReject = "a segment is longer than one advert"
	RejectTooShort   AutoSplitReject = "a segment is shorter than the clip floor"
	// RejectDuplicate is defense in depth for callers that invoke the gate without the split
	// stage's deterministic curation. The automated path discards the match before this point.
	RejectDuplicate AutoSplitReject = "a segment already exists in the catalog"
)

// SplitPartition is what the gate decided, segment by segment.
type SplitPartition struct {
	// Confirm may be cut into held child work items unattended.
	Confirm []split.Segment
	// Hold stays in the proposal, each carrying the reason it was kept back.
	Hold []split.Segment
	// Discard is complete-plan material intentionally omitted with retained structure authority.
	// It is neither publishable nor an unresolved hold.
	Discard []split.Segment
	// Reject is set only for a WHOLE-proposal refusal (auto-split switched off, nothing detected).
	// A per-segment refusal is on the segment, not here.
	Reject AutoSplitReject
}

// String is the operator-facing text of a reject reason.
func (r AutoSplitReject) String() string { return string(r) }

// pluralizeCuts renders "1 cut" / "5 cuts" for the ladder note.
func pluralizeCuts(n int) string {
	if n == 1 {
		return "1 cut"
	}
	return fmt.Sprintf("%d cuts", n)
}

// Verdict reduces a partition to ONE reel-level answer: `AutoSplitOK` when every segment cleared,
// otherwise the reason the MOST held-back segments give.
//
// ⚠ A summary for a ladder note or a log line, never the decision itself. Reading this instead of
// `Confirm`/`Hold` reintroduces the all-or-nothing behaviour V54 removed — one doubtful segment
// would once again speak for 51 good ones.
//
// ⚠ **This returned `Hold[0].HoldReason` — the FIRST held segment's reason — and on a real reel it
// reported the minority.** Measured 2026-08-13: 36 of 37 segments were held for one reason and 1 for
// another, but the minority sorted first, so the ladder sent the operator to a threshold that would
// have changed nothing. A summary that can name the reason for one segment out of thirty-seven is
// worse than no summary, because it is actionable and wrong.
func (sp SplitPartition) Verdict() AutoSplitReject {
	if sp.Reject != AutoSplitOK {
		return sp.Reject
	}
	reason, _, _ := sp.HoldSummary()
	return reason
}

// HoldSummary reports the reason the most held-back segments share, how many share it, and how
// many are held in total — so a caller can say "36 of 37" rather than implying unanimity.
//
// Ties break on the reason text, so one reel always produces one note.
func (sp SplitPartition) HoldSummary() (reason AutoSplitReject, shared, total int) {
	if len(sp.Hold) == 0 {
		return AutoSplitOK, 0, 0
	}
	counts := map[AutoSplitReject]int{}
	for _, seg := range sp.Hold {
		counts[AutoSplitReject(seg.HoldReason)]++
	}
	for r, n := range counts {
		if n > shared || (n == shared && r < reason) {
			reason, shared = r, n
		}
	}
	return reason, shared, len(sp.Hold)
}

// holdNote renders the ladder note for a reel where nothing cleared the gate.
//
// ⚠ It states the SHARE, not just the reason. "a segment is shorter than the clip floor" reads as a
// property of the reel; "36 of 37 cuts: …" tells the operator both what to fix and how much of the
// reel it accounts for — and, when the reasons are mixed, that fixing this one will not release
// everything.
func holdNote(sp SplitPartition) string {
	reason, shared, total := sp.HoldSummary()
	switch {
	case total == 0:
		return string(AutoSplitOK)
	case shared == total:
		return fmt.Sprintf("%s: %s", pluralizeCuts(total), reason)
	default:
		return fmt.Sprintf("%d of %s: %s", shared, pluralizeCuts(total), reason)
	}
}
