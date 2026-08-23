package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// LLMTokens splits prompt vs completion and skips zero (a provider that reports
// no usage must not add a phantom sample).
func TestLLMTokens(t *testing.T) {
	pb := testutil.ToFloat64(llmTokens.WithLabelValues("prompt"))
	cb := testutil.ToFloat64(llmTokens.WithLabelValues("completion"))

	LLMTokens(120, 40)
	LLMTokens(0, 0) // no usage reported → no-op

	if got := testutil.ToFloat64(llmTokens.WithLabelValues("prompt")); got != pb+120 {
		t.Errorf("prompt tokens = %v, want %v", got, pb+120)
	}
	if got := testutil.ToFloat64(llmTokens.WithLabelValues("completion")); got != cb+40 {
		t.Errorf("completion tokens = %v, want %v", got, cb+40)
	}
}

// FillerPodAssembled records the ladder rung reached.
func TestFillerPodAssembled(t *testing.T) {
	before := testutil.ToFloat64(fillerPods.WithLabelValues("widened"))
	FillerPodAssembled("widened")
	if got := testutil.ToFloat64(fillerPods.WithLabelValues("widened")); got != before+1 {
		t.Errorf("filler pods{widened} = %v, want %v", got, before+1)
	}
}

func TestFillerRotationAired(t *testing.T) {
	cases := []struct {
		name                   string
		repeated, relaxed, pin bool
		repeat, cooldown       string
	}{
		{"fresh ready", false, false, false, "fresh", "ready"},
		{"depleted repeat", true, true, false, "repeat", "relaxed"},
		{"pinned repeat", true, false, true, "repeat", "override"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			counter := fillerRotationAirings.WithLabelValues(tc.repeat, tc.cooldown)
			before := testutil.ToFloat64(counter)
			FillerRotationAired(tc.repeated, tc.relaxed, tc.pin)
			if got := testutil.ToFloat64(counter); got != before+1 {
				t.Fatalf("rotation counter = %v, want %v", got, before+1)
			}
		})
	}
}

// SlotSubstitutions adds the count and no-ops on zero (a clean reconcile).
func TestSlotSubstitutions(t *testing.T) {
	before := testutil.ToFloat64(slotSubstitutions)
	SlotSubstitutions(3)
	SlotSubstitutions(0)
	if got := testutil.ToFloat64(slotSubstitutions); got != before+3 {
		t.Errorf("slot substitutions = %v, want %v", got, before+3)
	}
}
