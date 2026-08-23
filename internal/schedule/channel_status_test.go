package schedule_test

import (
	"testing"

	"github.com/loomarr/loomarr/internal/schedule"
)

func TestChannelStatusReconcilable(t *testing.T) {
	tests := []struct {
		name   string
		status schedule.ChannelStatus
		want   bool
	}{
		{name: "zero", status: "", want: true},
		{name: "building", status: schedule.StatusBuilding, want: true},
		{name: "live", status: schedule.StatusLive, want: true},
		{name: "drifted", status: schedule.StatusDrifted, want: true},
		{name: "empty", status: schedule.StatusEmpty, want: true},
		{name: "paused", status: schedule.StatusPaused, want: false},
		{name: "detached", status: schedule.StatusDetached, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.Reconcilable(); got != tt.want {
				t.Fatalf("Reconcilable() = %v, want %v", got, tt.want)
			}
		})
	}
}
