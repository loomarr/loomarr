package suggest

import (
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
)

// clampSeasonWindow is the §8 grounding gate for a model-proposed AIRING season
// window: series-only, sane bounds, inverted/non-positive dropped so a bad value
// can never make an empty channel. These are the rules "Simpsons Classics → 1–10"
// relies on, and the guardrails against the model emitting nonsense.
func TestClampSeasonWindow(t *testing.T) {
	cases := []struct {
		name             string
		mt               provision.MediaType
		min, max         int
		wantMin, wantMax int
		wantOK           bool
	}{
		{"classic simpsons 1-10", provision.Series, 1, 10, 1, 10, true},
		{"single lower bound (11 onward)", provision.Series, 11, 0, 11, 0, true},
		{"single upper bound (through 10)", provision.Series, 0, 10, 0, 10, true},
		{"no window proposed", provision.Series, 0, 0, 0, 0, false},
		{"inverted range dropped", provision.Series, 10, 1, 0, 0, false},
		{"negative bounds cleared to none", provision.Series, -3, -1, 0, 0, false},
		{"negative min, positive max → upper only", provision.Series, -1, 8, 0, 8, true},
		{"movie ignores any window", provision.Movie, 1, 10, 0, 0, false},
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
