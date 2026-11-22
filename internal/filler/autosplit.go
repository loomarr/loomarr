package filler

import (
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/filler/split"
)

// The policy, reasons and partition of the unattended cut behind `filler.autosplit.enabled`. The
// gate itself is `AutoCuttable` (autocut.go).
//
// ⚠ **Each segment qualifies on its own evidence.** Boundary confidence (§10 V34) is per-segment
// evidence, so dropping five cuts out of 52 is routing rather than an arbitrary split of one
// decision.

// AutoSplitPolicy is what the gate reads, resolved per call so a settings change takes effect on
// the next run rather than at the next restart.
type AutoSplitPolicy struct {
	// Enabled reports whether auto-confirm is on at all.
	//
	// Backed by the `filler.autosplit.enabled` handle, so a reader that cannot answer reads the
	// setting's declared default (on). Without a store there is no catalog for the gate to cut.
	Enabled func() bool
	// MinConfidence is the score every segment must reach. Bounded 50-95 by the registry.
	MinConfidence func() int
	// MaxDuration is the longest a segment may be and still be advert-shaped.
	MaxDuration func() time.Duration
}

// MaxAutoSplitConfidence is the strictest operator-selectable boundary threshold.
const MaxAutoSplitConfidence = 95

// AutoSplitReject names why a proposal was not auto-confirmed. Returned rather than logged so
// the caller can report it — an unattended decision that leaves no trace is not one an appliance
// gets to make (§10), and "it just didn't" is unactionable for an operator tuning the threshold.
type AutoSplitReject string

const (
	AutoSplitOK AutoSplitReject = ""
	// RejectDisabled is the ordinary state: the operator has not opted in.
	RejectDisabled   AutoSplitReject = "auto-split is off"
	RejectNoSegments AutoSplitReject = "the proposal has no segments"
	// RejectUnsplittable is the signal this gate exists for. The detector already knows it
	// failed on this span; auto-confirming around that admission is the 69% failure.
	RejectUnsplittable AutoSplitReject = "a segment could not be split and needs a human"
	RejectTooLong      AutoSplitReject = "a segment is longer than one advert"
	RejectTooShort     AutoSplitReject = "a segment is shorter than the clip floor"
	// RejectDuplicate is defense in depth for callers that invoke the gate without the split
	// stage's deterministic curation. The automated path discards the match before this point.
	RejectDuplicate AutoSplitReject = "a segment already exists in the catalog"
	// RejectDedupUnchecked holds back a segment whose comparison with the catalog could not be
	// made. Filler is a cache: a stalled auto-split recovers on the next pass once the dedup tooling
	// is back, whereas a duplicate that has aired cannot be un-aired. A person can still confirm it.
	RejectDedupUnchecked AutoSplitReject = "a segment could not be checked for duplicates"
	// RejectBoundaryUncertain is the CONFIDENCE outcome, and the only one of these that is a
	// threshold rather than a refusal (§10 V34).
	RejectBoundaryUncertain AutoSplitReject = "Loomarr is not sure this was cut in the right place"
)

// minConfidence resolves the operator's threshold, defaulting to the registry's 85 when the
// policy carries no reader. Out of range is clamped to MaxAutoSplitConfidence, the strictest
// threshold an operator can pick.
func minConfidence(pol *AutoSplitPolicy) int {
	if pol == nil || pol.MinConfidence == nil {
		return 85
	}
	n := pol.MinConfidence()
	if n <= 0 || n > MaxAutoSplitConfidence {
		return MaxAutoSplitConfidence
	}
	return n
}

// SplitPartition is what the gate decided, segment by segment.
type SplitPartition struct {
	// Confirm is cut unattended.
	Confirm []split.Segment
	// Hold is dropped, each carrying the reason it was not cut.
	Hold []split.Segment
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

// HoldSummary reports the reason the most held-back segments share, how many share it, and how
// many are held in total — so a caller can say "36 of 37" rather than implying unanimity.
//
// ⚠ **The first held segment's reason is not the summary.** Measured 2026-08-13: 36 of 37 segments
// were refused for one reason and 1 as boundary-uncertain, but the doubtful one sorted first, so a
// summary from `Hold[0]` told the operator to lower a threshold that would have released nothing.
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
// ⚠ It states the SHARE, not just the reason. "a segment could not be classified" reads as a
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
