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

// SlotSubstitutions adds the count and no-ops on zero (a clean reconcile).
func TestSlotSubstitutions(t *testing.T) {
	before := testutil.ToFloat64(slotSubstitutions)
	SlotSubstitutions(3)
	SlotSubstitutions(0)
	if got := testutil.ToFloat64(slotSubstitutions); got != before+3 {
		t.Errorf("slot substitutions = %v, want %v", got, before+3)
	}
}
