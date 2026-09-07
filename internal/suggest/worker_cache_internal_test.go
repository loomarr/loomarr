package suggest

import "testing"

func TestIntentHashInvalidatesPriorPlannerContract(t *testing.T) {
	// This is the v4/v4 cache identity for this normalized intent. The planner's
	// collection contract now participates in the cache key, so a completed
	// proposal from the former contract cannot be cloned during the cache TTL.
	const priorHash = "092e0efa2de7b81bc3185e8c37176494a8013de5ba7ffb92e3eafe61ac237ba6"
	if got := IntentHash(Intent{Description: "90s action"}); got == priorHash {
		t.Fatal("planner contract change reused the prior cache identity")
	}
}
