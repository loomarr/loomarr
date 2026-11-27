package split

import "testing"

// levelsFor lays out 200 ms windows from 0: loud at -20 dB except where quiet marks a window.
func levelsFor(n int, quiet ...int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = -20
	}
	for _, q := range quiet {
		out[q] = -60
	}
	return out
}

func TestCutSound(t *testing.T) {
	// A 30 s cut from 2000 ms to 32000 ms: windows 10 and 160 sit on its edges.
	const start, end = 2_000, 32_000
	for _, tc := range []struct {
		name   string
		levels []float64
		clean  bool
	}{
		{name: "one spot: quiet at both edges, sound throughout", levels: levelsFor(170, 10, 160), clean: true},
		{name: "a window either side of the edge still counts", levels: levelsFor(170, 9, 161), clean: true},
		{name: "part of a spot: the sound runs through an edge", levels: levelsFor(170, 10), clean: false},
		{name: "several spots: quiet where two meet", levels: levelsFor(170, 10, 85, 160), clean: false},
		// A pause close to an edge belongs to the edge's own fade, not to a join.
		{name: "a dip within the margin of an edge is not inside", levels: levelsFor(170, 10, 15, 160), clean: true},
		{name: "no sound at all", levels: make170(SoundSilentDb), clean: false},
		{name: "no windows", levels: nil, clean: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := MeasureCutSound(tc.levels, 0, start, end)
			if got := c.Clean(); got != tc.clean {
				t.Errorf("clean = %v, want %v (%+v)", got, tc.clean, c)
			}
		})
	}
	var unmeasured *CutSound
	if unmeasured.Clean() {
		t.Error("an unmeasured cut reads as clean")
	}
}

func make170(db float64) []float64 {
	out := make([]float64, 170)
	for i := range out {
		out[i] = db
	}
	return out
}
