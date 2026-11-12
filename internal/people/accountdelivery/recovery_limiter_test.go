package accountdelivery

import (
	"fmt"
	"testing"

	"github.com/loomarr/loomarr/internal/people/auth"
)

// One address rotating usernames must not get a fresh recovery burst per name: each accepted
// request can email an account holder.
func TestRecoveryRequestLimiterBoundsOneSourceAcrossUsernames(t *testing.T) {
	l := newRecoveryRequestLimiter()
	allowed := 0
	for i := range 50 {
		if l.Allow(auth.LoginRateKey("203.0.113.9", fmt.Sprintf("user%d", i))) {
			allowed++
		}
	}
	if allowed != 6 {
		t.Fatalf("one source rotating usernames got %d recovery requests, want 6", allowed)
	}
	if !l.Allow(auth.LoginRateKey("198.51.100.4", "user0")) {
		t.Fatal("a different source was throttled by the first one's attempts")
	}
}
