package auth

import (
	"fmt"
	"testing"
)

// An IPv6 client holds a whole /64, so addresses inside it are one source: rotating them buys no
// fresh burst. A neighbouring /64 is another source.
func TestLoginLimiterTreatsAnIPv6SlashSixtyFourAsOneSource(t *testing.T) {
	limiter := NewLoginLimiter()
	allowed := 0
	for i := range 200 {
		if limiter.Allow(LoginRateKey(fmt.Sprintf("2001:db8:1:2::%x", i+1), "user")) {
			allowed++
		}
	}
	if allowed > 5 {
		t.Fatalf("one /64 rotating addresses got %d sign-in attempts on one account, want at most 5", allowed)
	}
	if !limiter.Allow(LoginRateKey("2001:db8:1:3::1", "user")) {
		t.Fatal("a neighbouring /64 was throttled by the first one's attempts")
	}
}

func TestThrottleSource(t *testing.T) {
	for in, want := range map[string]string{
		"203.0.113.9":                "203.0.113.9",
		"2001:db8:1:2:aaaa:bbbb:c:d": "2001:db8:1:2::/64",
		"::ffff:203.0.113.9":         "203.0.113.9",
		"not an address":             "not an address",
		"fe80::1":                    "fe80::/64",
	} {
		if got := ThrottleSource(in); got != want {
			t.Errorf("ThrottleSource(%q) = %q, want %q", in, got, want)
		}
	}
}
