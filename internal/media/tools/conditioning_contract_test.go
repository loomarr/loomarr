package tools_test

import (
	"testing"

	"github.com/loomarr/loomarr/internal/media/tools"
)

func TestConditioningRequestHasOneParentAndIndexedCutStreams(t *testing.T) {
	req := tools.ConditioningRequest{
		Path:       "child.mp4",
		ParentPath: "parent.mp4",
		IntendedCuts: []tools.Interval{
			{StartMs: 1_000, EndMs: 2_000},
			{StartMs: 3_000, EndMs: 4_000},
		},
	}
	if len(req.IntendedCuts) != 2 {
		t.Fatalf("intended cuts = %d, want 2", len(req.IntendedCuts))
	}
	stream := tools.ConditioningCutStream{Kind: tools.StreamAudio, Index: 2}
	if stream.Index != 2 {
		t.Fatalf("stream index = %d, want 2", stream.Index)
	}
	peak := tools.ConditioningTruePeak{State: tools.TruePeakNegativeInfinity}
	if peak.State != tools.TruePeakNegativeInfinity {
		t.Fatalf("true-peak state = %q", peak.State)
	}
}
