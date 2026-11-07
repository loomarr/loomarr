package suggest

import (
	"testing"

	"github.com/loomarr/loomarr/internal/proposals"
)

func TestIntentHashInvalidatesPriorPlannerContract(t *testing.T) {
	// This is the v4/v4 cache identity for this normalized intent. The planner's
	// collection contract now participates in the cache key, so a completed
	// proposal from the former contract cannot be cloned during the cache TTL.
	const priorHash = "092e0efa2de7b81bc3185e8c37176494a8013de5ba7ffb92e3eafe61ac237ba6"
	if got := IntentHash(proposals.Intent{Description: "90s action"}); got == priorHash {
		t.Fatal("planner contract change reused the prior cache identity")
	}
}

func TestIntentHashSeparatesMembershipPolicyFromNormalizedDescription(t *testing.T) {
	if IntentHash(proposals.Intent{Description: "qfun"}) == IntentHash(proposals.Intent{Description: "QFUN"}) {
		t.Fatal("a named-set request must not reuse a generic cached proposal")
	}
}

func TestIntentHashPreservesCaseSensitiveReferenceIdentity(t *testing.T) {
	upper := proposals.Intent{Description: "Use https://example.com/RosterA for this programming block"}
	lower := proposals.Intent{Description: "Use https://example.com/rostera for this programming block"}
	if IntentHash(upper) == IntentHash(lower) {
		t.Fatal("distinct case-sensitive reference resources shared a cache identity")
	}
}
