package api

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

func TestIncomingPreparationDTOStates(t *testing.T) {
	now := time.Date(2026, time.September, 17, 12, 0, 0, 0, time.UTC)
	base := filler.ClipPipeline{
		PreparationAttempt: 2, PreparationStartedAt: now.Add(-time.Minute),
		PreparationStartReason: filler.PreparationStartedByEnrollment, PreparationProgress: 42,
		Disposition: filler.DispositionRunning, Status: filler.StatusRunning, UpdatedAt: now,
	}
	tests := []struct {
		name     string
		edit     func(*filler.ClipPipeline)
		estimate filler.ReadyEstimate
		state    string
		percent  *int
	}{
		{name: "estimating", edit: func(*filler.ClipPipeline) {}, state: "estimating", percent: intPointer(42)},
		{name: "estimated", edit: func(*filler.ClipPipeline) {}, estimate: filler.ReadyEstimate{Lower: time.Minute, Upper: 3 * time.Minute}, state: "estimated", percent: intPointer(42)},
		{name: "retrying", edit: func(row *filler.ClipPipeline) { row.Status = filler.StatusFailed; row.NextRun = now.Add(time.Minute) }, state: "retrying", percent: intPointer(42)},
		{name: "waiting", edit: func(row *filler.ClipPipeline) { row.Disposition = filler.DispositionReview }, state: "waiting", percent: intPointer(42)},
		{name: "restarted", edit: func(row *filler.ClipPipeline) {
			row.PreparationStartReason = filler.PreparationStartedByRestart
			row.PreparationProgress = 0
		}, state: "restarted", percent: intPointer(0)},
		{name: "ready", edit: func(row *filler.ClipPipeline) {
			row.Disposition = filler.DispositionReady
			row.PreparationProgress = 91
		}, state: "ready", percent: intPointer(100)},
		{name: "unavailable", edit: func(row *filler.ClipPipeline) { row.PreparationAttempt = 0; row.PreparationProgress = -1 }, state: "unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := base
			tt.edit(&row)
			got := incomingPreparationDTO(row, now, tt.estimate)
			if got.State != tt.state || !equalIntPointer(got.Percent, tt.percent) {
				t.Fatalf("projection = %+v, want state %q percent %v", got, tt.state, tt.percent)
			}
			if tt.state == "estimated" {
				if got.ReadyIn == nil || got.ReadyIn.LowerSeconds != 60 || got.ReadyIn.UpperSeconds != 180 {
					t.Fatalf("ReadyIn = %+v, want 60–180 seconds", got.ReadyIn)
				}
			} else if got.ReadyIn != nil {
				t.Fatalf("state %q exposed estimate %+v", tt.state, got.ReadyIn)
			}
		})
	}
}

func intPointer(value int) *int { return &value }

func equalIntPointer(left, right *int) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}
