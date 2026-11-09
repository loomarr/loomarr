package filler

import (
	"testing"

	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/media/tools"
)

func TestMediaQualityVerdict(t *testing.T) {
	tests := []struct {
		name   string
		q      tools.MediaQuality
		want   pipelinestate.Verdict
		reason pipelinestate.RejectReason
	}{
		{"clean", tools.MediaQuality{DurationMs: 30_000}, pipelinestate.VerdictContinue, ""},
		{"near-total black", tools.MediaQuality{DurationMs: 30_000, Black: []tools.Interval{{StartMs: 0, EndMs: 27_000}}}, pipelinestate.VerdictReject, pipelinestate.ReasonBlackContent},
		{"near-total silence", tools.MediaQuality{DurationMs: 30_000, Silence: []tools.Interval{{StartMs: 1000, EndMs: 29_000}}}, pipelinestate.VerdictReject, pipelinestate.ReasonSilentContent},
		{"long black is reviewed", tools.MediaQuality{DurationMs: 30_000, Black: []tools.Interval{{StartMs: 0, EndMs: 5_000}}}, pipelinestate.VerdictReview, ""},
		{"long freeze is reviewed", tools.MediaQuality{DurationMs: 30_000, Freeze: []tools.Interval{{StartMs: 10_000, EndMs: 20_000}}}, pipelinestate.VerdictReview, ""},
		{"ordinary end slate proceeds", tools.MediaQuality{DurationMs: 30_000, Freeze: []tools.Interval{{StartMs: 26_000, EndMs: 30_000}}}, pipelinestate.VerdictContinue, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason, detail := EvaluateMediaQuality(tt.q)
			if got != tt.want || reason != tt.reason {
				t.Fatalf("verdict/reason = %v/%q (%s), want %v/%q", got, reason, detail, tt.want, tt.reason)
			}
		})
	}
}
