package schedule_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/schedule"
)

// UpcomingHolidays reads the same calendar the seasonal policy enforces (#1665 channel ideas):
// the household country's windows on now or starting within the horizon, soonest first, across
// the year boundary.
func TestUpcomingHolidays(t *testing.T) {
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 12, 0, 0, 0, time.UTC) }
	for _, tc := range []struct {
		now     time.Time
		country string
		want    string
	}{
		{day(2026, time.September, 28), "", "[halloween@2026-10-01]"},                                         // 3 days ahead
		{day(2026, time.October, 10), "", "[halloween@2026-10-01 thanksgiving@2026-11-15]"},                   // one on, one ahead
		{day(2026, time.December, 25), "", "[christmas@2026-12-01 newyear@2026-12-27 valentines@2027-02-01]"}, // across the year
		{day(2027, time.January, 1), "", "[newyear@2026-12-27 valentines@2027-02-01]"},                        // the straddling window, from its far side
		{day(2026, time.June, 1), "", "[fathers-day@2026-06-08 independence-day@2026-06-27]"},                 // the US summer
		// Canada's Thanksgiving is the second Monday of October, so its window opens in September.
		{day(2026, time.September, 28), "CA", "[thanksgiving@2026-09-29 halloween@2026-10-01]"},
		{day(2026, time.June, 1), "GB", "[fathers-day@2026-06-08]"}, // no Fourth of July in Britain
	} {
		var got []string
		for _, h := range schedule.UpcomingHolidays(tc.now, 6*7*24*time.Hour, schedule.Calendar{Country: tc.country}) {
			got = append(got, fmt.Sprintf("%s@%s", h.ID, h.Start.Format("2006-01-02")))
		}
		if fmt.Sprint(got) != tc.want {
			t.Errorf("UpcomingHolidays(%s, %q) = %v, want %s", tc.now.Format("2006-01-02"), tc.country, got, tc.want)
		}
	}
}
