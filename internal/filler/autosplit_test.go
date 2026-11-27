package filler_test

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/split"
)

func policy(on bool, minConf int, maxDur time.Duration) *filler.AutoSplitPolicy {
	return &filler.AutoSplitPolicy{
		Enabled:       func() bool { return on },
		MinConfidence: func() int { return minConf },
		MaxDuration:   func() time.Duration { return maxDur },
	}
}

const floor = 10 * time.Second

// oneSpot is a cut the look saw as one whole advert, with a clean sound, on edges a single
// detector found (65): only the boundary threshold decides it.
func oneSpot() split.Segment {
	return split.Segment{
		StartMs: 0, EndMs: 30_000, Looked: true, BoundaryConfidence: 65,
		Whole: split.WholeOne, LookedRole: split.RoleCommercial,
		Sound: &split.CutSound{MedianDb: -22, StartDb: -40, EndDb: -40, InsideDb: -6},
	}
}

// `filler.autosplit.min_confidence` is the boundary threshold of the unattended cut: lowering it
// admits a single-detector edge, raising it holds the same cut back.
func TestAutoCuttable_ThresholdChangesTheAnswer(t *testing.T) {
	p := filler.SplitProposal{Segments: []split.Segment{oneSpot()}}

	if got := filler.AutoCuttable(p, policy(true, 60, 120*time.Second), floor); len(got.Confirm) != 1 {
		t.Errorf("min=60 dropped a 65-confidence cut: %+v", got)
	}
	got := filler.AutoCuttable(p, policy(true, 85, 120*time.Second), floor)
	if len(got.Confirm) != 0 || len(got.Hold) != 1 || got.Hold[0].HoldReason != string(filler.RejectBoundaryUncertain) {
		t.Errorf("min=85 = %+v, want the cut held as %q", got, filler.RejectBoundaryUncertain)
	}
}

// A threshold the policy cannot read is the registry's 85, and one out of range is the strictest.
func TestAutoCuttable_ThresholdDefaultsAndClamps(t *testing.T) {
	cut := oneSpot()
	cut.BoundaryConfidence = 90
	p := filler.SplitProposal{Segments: []split.Segment{cut}}

	unread := &filler.AutoSplitPolicy{Enabled: func() bool { return true }}
	if got := filler.AutoCuttable(p, unread, floor); len(got.Confirm) != 1 {
		t.Errorf("no threshold reader dropped a 90-confidence cut: %+v", got)
	}
	for _, minConf := range []int{0, 101} {
		if got := filler.AutoCuttable(p, policy(true, minConf, 120*time.Second), floor); len(got.Confirm) != 0 {
			t.Errorf("min=%d cut a 90-confidence edge; an out-of-range threshold is the strictest (95)", minConf)
		}
	}
}

func TestAutoCuttable_RefusesAnEmptyProposal(t *testing.T) {
	if got := filler.AutoCuttable(filler.SplitProposal{}, policy(true, 85, 120*time.Second), floor); got.Reject != filler.RejectNoSegments {
		t.Errorf("an empty proposal = %+v, want %q", got, filler.RejectNoSegments)
	}
}
