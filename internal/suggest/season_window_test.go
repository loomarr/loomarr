package suggest

import (
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
)

// clampSeasonWindow is the §8 grounding gate for a model-proposed AIRING season
// window: series-only, sane bounds, inverted/non-positive dropped so a bad value
// can never make an empty channel. These are the rules "Mudpuddles Classics → 1–10"
// relies on, and the guardrails against the model emitting nonsense.
func TestClampSeasonWindow(t *testing.T) {
	cases := []struct {
		name             string
		mt               provision.MediaType
		min, max         int
		wantMin, wantMax int
		wantOK           bool
	}{
		{name: "classic mudpuddles 1-10", mt: provision.Series, min: 1, max: 10, wantMin: 1, wantMax: 10, wantOK: true},
		{name: "single lower bound (11 onward)", mt: provision.Series, min: 11, max: 0, wantMin: 11, wantMax: 0, wantOK: true},
		{name: "single upper bound (through 10)", mt: provision.Series, min: 0, max: 10, wantMin: 0, wantMax: 10, wantOK: true},
		{name: "no window proposed", mt: provision.Series, min: 0, max: 0, wantMin: 0, wantMax: 0, wantOK: false},
		{name: "inverted range dropped", mt: provision.Series, min: 10, max: 1, wantMin: 0, wantMax: 0, wantOK: false},
		{name: "negative bounds cleared to none", mt: provision.Series, min: -3, max: -1, wantMin: 0, wantMax: 0, wantOK: false},
		{name: "negative min, positive max → upper only", mt: provision.Series, min: -1, max: 8, wantMin: 0, wantMax: 8, wantOK: true},
		{name: "movie ignores any window", mt: provision.Movie, min: 1, max: 10, wantMin: 0, wantMax: 0, wantOK: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			min, max, ok := clampSeasonWindow(c.mt, c.min, c.max)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if ok && (min != c.wantMin || max != c.wantMax) {
				t.Errorf("window = [%d,%d], want [%d,%d]", min, max, c.wantMin, c.wantMax)
			}
		})
	}
}
