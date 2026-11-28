package playout

import (
	"testing"

	"github.com/loomarr/loomarr/internal/playout/admission"
)

// A heavy source prices its rungs from the heavy table and an SDR title up to 1080p from the SDR
// one: the step-up bar on keyframes-only is the class's own cost ratio.
func TestRungCostsFor_PicksTheSourcesTable(t *testing.T) {
	if got := RungCostsFor(testSources()["hevc-4k-hdr-dv"]); got != heavyRungCosts {
		t.Errorf("4K HDR costs = %v, want the heavy table %v", got, heavyRungCosts)
	}
	if got := RungCostsFor(testSources()["hevc10-1080p"]); got != admission.SDRRungCosts {
		t.Errorf("1080p SDR costs = %v, want the SDR table %v", got, admission.SDRRungCosts)
	}
}
