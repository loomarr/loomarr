package accountdelivery

import (
	"fmt"
	"testing"

	"github.com/loomarr/loomarr/internal/people/auth"
)

// One account reached from many addresses must not get a fresh recovery burst per address: each
// accepted request emails the account holder, so rotating sources (the /64s of an IPv6 delegation,
// an IPv4 pool) would otherwise be an email bomb with no ceiling.
func TestRecoveryRequestLimiterBoundsOneAccountAcrossSources(t *testing.T) {
	for name, source := range map[string]func(int) string{
		"IPv6 /64s inside a /56": func(i int) string { return fmt.Sprintf("2001:db8:1:%d::1", i) },
		"IPv4 pool":              func(i int) string { return fmt.Sprintf("192.0.2.%d", i) },
	} {
		t.Run(name, func(t *testing.T) {
			l := newRecoveryRequestLimiter()
			allowed := 0
			for i := 1; i <= 50; i++ {
				if l.Allow(auth.LoginRateKey(source(i), "ada")) {
					allowed++
				}
			}
			if allowed != recoveryAccountBurst {
				t.Fatalf("one account got %d recovery requests from 50 sources, want %d", allowed, recoveryAccountBurst)
			}
			if !l.Allow(auth.LoginRateKey("198.51.100.4", "grace")) {
				t.Fatal("another account was throttled by the first one's requests")
			}
		})
	}
}

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
