package filler

import "testing"

func TestMediaQualityVerdict(t *testing.T) {
	tests := []struct {
		name   string
		q      MediaQuality
		want   Verdict
		reason RejectReason
	}{
		{"clean", MediaQuality{DurationMs: 30_000}, VerdictContinue, ""},
		{"near-total black", MediaQuality{DurationMs: 30_000, Black: []Interval{{StartMs: 0, EndMs: 27_000}}}, VerdictReject, ReasonBlackContent},
		{"near-total silence", MediaQuality{DurationMs: 30_000, Silence: []Interval{{StartMs: 1000, EndMs: 29_000}}}, VerdictReject, ReasonSilentContent},
		{"long black is reviewed", MediaQuality{DurationMs: 30_000, Black: []Interval{{StartMs: 0, EndMs: 5_000}}}, VerdictReview, ""},
		{"long freeze is reviewed", MediaQuality{DurationMs: 30_000, Freeze: []Interval{{StartMs: 10_000, EndMs: 20_000}}}, VerdictReview, ""},
		{"ordinary end slate proceeds", MediaQuality{DurationMs: 30_000, Freeze: []Interval{{StartMs: 26_000, EndMs: 30_000}}}, VerdictContinue, ""},
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
