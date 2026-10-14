package playout

import (
	"math"
	"strings"
	"testing"
	"time"
)

// seconds must never emit exponent notation — ffmpeg would parse "1e-06" as a token rather
// than a duration.
func TestSeconds_NeverUsesExponentNotation(t *testing.T) {
	for _, d := range []time.Duration{
		time.Microsecond, time.Millisecond, time.Second,
		90*time.Minute + 500*time.Millisecond, 0,
	} {
		got := seconds(d)
		if strings.ContainsAny(got, "eE") {
			t.Errorf("seconds(%v) = %q, which ffmpeg cannot parse", d, got)
		}
	}
}

func TestStaticGainDB(t *testing.T) {
	for _, tc := range []struct {
		name             string
		target, measured float64
		want             float64
	}{
		{"already at target", -23, -23.1, 0.1},
		{"quiet clip comes up", -23, -26, 3},
		{"loud clip goes down", -23, -20, -3},
		{"absurd boost is clamped", -23, -60, MaxGainDB},
		{"absurd cut is clamped", -23, 10, -MaxGainDB},
	} {
		if got := StaticGainDB(tc.target, tc.measured); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("%s: StaticGainDB(%v, %v) = %v, want %v", tc.name, tc.target, tc.measured, got, tc.want)
		}
	}
}
