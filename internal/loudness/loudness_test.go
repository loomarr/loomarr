package loudness

import (
	"math"
	"testing"
)

func peak(v float64) *float64 { return &v }

// The rows are the household clips that motivated #2032: a beer ad at -12.2 LUFS that aired 11 dB
// hot under the old ±6 dB clamp, and a snack ad at -33.7 that no gain can rescue.
func TestCorrection(t *testing.T) {
	for _, tc := range []struct {
		name      string
		measured  float64
		peak      *float64
		gain      float64
		reachable bool
	}{
		{"on target", -23, peak(-9), 0, true},
		{"hot clip is cut all the way", -12.2, peak(-1.5), -10.8, true},
		{"absurd measurement is bounded", 10, nil, -MaxCutDB, true},
		{"quiet clip is raised", -27, peak(-12), 4, true},
		{"boost stops at the peak ceiling", -27, peak(-3), 2, true},
		{"a clip already at the ceiling is not raised", -27, peak(-0.5), 0, true},
		{"unmeasured peak still boosts", -27, nil, 4, true},
		{"too quiet to rescue", -33.7, peak(-12), MaxBoostDB, false},
		{"exactly the boost limit is reachable", -29, peak(-20), MaxBoostDB, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gain, reachable := Correction(-23, tc.measured, tc.peak)
			if math.Abs(gain-tc.gain) > 1e-9 || reachable != tc.reachable {
				t.Fatalf("Correction(-23, %v) = %v, %v; want %v, %v", tc.measured, gain, reachable, tc.gain, tc.reachable)
			}
		})
	}
}
