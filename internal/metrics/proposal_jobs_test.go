package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestProposalJobObservedBoundsResultLabels(t *testing.T) {
	successBefore := testutil.ToFloat64(proposalJobOutcomes.WithLabelValues("success"))
	genericBefore := testutil.ToFloat64(proposalJobOutcomes.WithLabelValues("generation_failed"))

	ProposalJobObserved(250*time.Millisecond, "success")
	ProposalJobObserved(time.Second, `upstream said api-key suffix 1234`)

	if got := testutil.ToFloat64(proposalJobOutcomes.WithLabelValues("success")); got != successBefore+1 {
		t.Fatalf("success counter = %v, want %v", got, successBefore+1)
	}
	if got := testutil.ToFloat64(proposalJobOutcomes.WithLabelValues("generation_failed")); got != genericBefore+1 {
		t.Fatalf("unknown result should normalize to generation_failed: got %v want %v", got, genericBefore+1)
	}
	if got := testutil.ToFloat64(proposalJobOutcomes.WithLabelValues(`upstream said api-key suffix 1234`)); got != 0 {
		t.Fatalf("raw diagnostic became a metric label: %v", got)
	}
}
