package channels

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/schedule"
)

// A holiday turns at the channel's local midnight. In a :30 zone that is not an hour top of the
// cursor's (UTC) clock, so the forecast must probe the local midnight itself (#1930 review N3).
func TestNextClockChange_HalfHourZoneTurnsAtLocalMidnight(t *testing.T) {
	kolkata, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Skip("no tzdata:", err)
	}
	policy := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Seasonal: schedule.SeasonalPolicy{Mode: schedule.SeasonalAuto}}}
	after := time.Date(2026, time.October, 31, 18, 0, 0, 0, time.UTC) // 23:30 Oct 31 in Kolkata
	got := nextClockChange(policy, schedule.Calendar{Zone: kolkata}, after, after.Add(2*time.Hour))
	if want := time.Date(2026, time.November, 1, 0, 0, 0, 0, kolkata); !got.Equal(want) {
		t.Fatalf("next clock change = %v, want Kolkata's midnight %v", got, want.UTC())
	}
}
