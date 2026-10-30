package playout

import (
	"math"
	"testing"
)

// ParseFrameRate serves both the live ffprobe path and the format rebuilt from stored inventory
// facts, so the same string must give the same fps from either.
func TestParseFrameRate(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want float64
	}{
		{"24000/1001", 24000.0 / 1001},
		{"25/1", 25},
		{"25", 25},
		{" 30000 / 1001 ", 30000.0 / 1001},
		{"0/0", 0},  // ffprobe's "no framerate"
		{"25/0", 0}, // a zero denominator is unknown, not +Inf
		{"", 0},
		{"abc", 0},
		{"abc/1", 0},
		{"1/abc", 0},
	} {
		if got := ParseFrameRate(tc.in); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("ParseFrameRate(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
