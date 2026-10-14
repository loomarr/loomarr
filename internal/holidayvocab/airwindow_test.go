package holidayvocab

import (
	"testing"
	"time"
)

// Only Halloween has mood aliases today, so the other broadcast windows are not reachable through
// Judge yet; their edges, the January wrap and the moving Thanksgiving week are pinned here.
func TestAiredInWindowEdges(t *testing.T) {
	day := func(year int, month time.Month, d int) time.Time {
		return time.Date(year, month, d, 0, 0, 0, 0, time.UTC)
	}
	for _, tt := range []struct {
		id    string
		aired time.Time
		want  bool
	}{
		{"christmas", day(1995, time.December, 1), true},
		{"christmas", day(1995, time.November, 30), false},
		{"christmas", day(1996, time.January, 2), true},
		{"christmas", day(1996, time.January, 3), false},
		{"newyear", day(1999, time.December, 26), true},
		{"newyear", day(2000, time.January, 7), true},
		{"newyear", day(2000, time.January, 8), false},
		{"valentines", day(2004, time.February, 7), true},
		{"valentines", day(2004, time.February, 22), false},
		// Thanksgiving 2001 was Thursday November 22.
		{"thanksgiving", day(2001, time.November, 15), true},
		{"thanksgiving", day(2001, time.November, 14), false},
		{"thanksgiving", day(2001, time.November, 25), true},
		{"thanksgiving", day(2001, time.November, 26), false},
		{"halloween", time.Time{}, false},
		// The time of day is ignored: the end day is inclusive.
		{"halloween", time.Date(1990, time.November, 7, 23, 0, 0, 0, time.UTC), true},
	} {
		d, ok := lookup(tt.id)
		if !ok {
			t.Fatalf("unknown holiday %s", tt.id)
		}
		if got := d.airedInWindow(tt.aired); got != tt.want {
			t.Errorf("%s airedInWindow(%s) = %v, want %v", tt.id, tt.aired.Format(time.DateOnly), got, tt.want)
		}
	}
}
