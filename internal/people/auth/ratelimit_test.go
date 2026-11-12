package auth

import (
	"strconv"
	"testing"
	"time"
)

// A key whose bucket has refilled is the same as a key never seen, so the limiter forgets it: a
// stream of one-off keys cannot grow the map without bound.
func TestRateLimiterForgetsKeysThatHaveRefilled(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	rl := NewRateLimiter(0.2, 5)
	rl.now = func() time.Time { return now }

	for i := range 10_000 {
		rl.Allow("203.0.113.9|user" + strconv.Itoa(i))
	}
	now = now.Add(2 * time.Minute) // 5 tokens at 0.2/s refill in 25s
	rl.Allow("203.0.113.9|one-more")

	if n := rl.size(); n > 1 {
		t.Fatalf("limiter holds %d keys after every bucket refilled, want at most 1", n)
	}
}

// Forgetting a key never forgives it early: a key still short of tokens is kept.
func TestRateLimiterKeepsKeysStillThrottled(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	rl := NewRateLimiter(0.01, 2) // one token per 100s
	rl.now = func() time.Time { return now }

	rl.Allow("k")
	rl.Allow("k")
	now = now.Add(61 * time.Second) // past the sweep interval, but "k" has refilled 0.61 tokens
	rl.Allow("other")

	if rl.Allow("k") {
		t.Fatal("a key short of tokens was forgotten by the sweep and got a fresh burst")
	}
}

// The login key is IP|username, so a single source rotating usernames gets a fresh account bucket
// each time. The source has a bucket of its own that caps it across every username.
func TestLoginLimiterCapsOneSourceAcrossUsernames(t *testing.T) {
	limiter := NewLoginLimiter()
	allowed := 0
	for i := range 200 {
		if limiter.Allow(LoginRateKey("203.0.113.9", "user"+strconv.Itoa(i))) {
			allowed++
		}
	}
	if allowed > loginSourceBurst {
		t.Fatalf("one source got %d attempts across 200 usernames, want at most %d", allowed, loginSourceBurst)
	}
	// Another source is unaffected.
	if !limiter.Allow(LoginRateKey("198.51.100.7", "user0")) {
		t.Fatal("a different source was throttled by the first one's attempts")
	}
}
