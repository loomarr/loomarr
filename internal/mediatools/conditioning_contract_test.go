package mediatools_test

import (
	"testing"

	"github.com/loomarr/loomarr/internal/mediatools"
)

func TestConditioningRequestHasOneParentAndIndexedCutStreams(t *testing.T) {
	req := mediatools.ConditioningRequest{
		Path:       "child.mp4",
		ParentPath: "parent.mp4",
		IntendedCuts: []mediatools.Interval{
			{StartMs: 1_000, EndMs: 2_000},
			{StartMs: 3_000, EndMs: 4_000},
		},
	}
	if len(req.IntendedCuts) != 2 {
		t.Fatalf("intended cuts = %d, want 2", len(req.IntendedCuts))
	}
	stream := mediatools.ConditioningCutStream{Kind: mediatools.StreamAudio, Index: 2}
	if stream.Index != 2 {
		t.Fatalf("stream index = %d, want 2", stream.Index)
	}
	peak := mediatools.ConditioningTruePeak{State: mediatools.TruePeakNegativeInfinity}
	if peak.State != mediatools.TruePeakNegativeInfinity {
		t.Fatalf("true-peak state = %q", peak.State)
	}
}
