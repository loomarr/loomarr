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

// sourceLimiterClock puts every bucket of l on one fake clock.
func sourceLimiterClock(l *SourceLimiter, now *time.Time) {
	clock := func() time.Time { return *now }
	l.source.now, l.pair.now, l.account.now = clock, clock, clock
}

// The throttle keys on the source, and a source can be rotated: an IPv6 delegation of /56 is 256
// /64s, an IPv4 attacker has a pool. Each fresh source got a fresh source burst and a fresh
// per-account burst, so the cost of a password guess depended on how much address space the
// attacker had (probe: 50 of 50 allowed from 2001:db8:1:{1..50}::1). The account has a bucket no
// source can refill.
func TestLoginLimiterCapsOneAccountAcrossSources(t *testing.T) {
	sources := map[string]func(i int) string{
		"IPv6 /64s inside one /56": func(i int) string { return "2001:db8:1:" + strconv.Itoa(i) + "::1" },
		"IPv6 /64s across /48s":    func(i int) string { return "2001:db8:" + strconv.Itoa(i) + "::1" },
		"IPv4 pool":                func(i int) string { return "192.0.2." + strconv.Itoa(i) },
		"IPv4 and IPv6 mixed": func(i int) string {
			if i%2 == 0 {
				return "198.51.100." + strconv.Itoa(i)
			}
			return "2001:db8:2:" + strconv.Itoa(i) + "::1"
		},
	}
	for name, source := range sources {
		t.Run(name, func(t *testing.T) {
			limiter := NewLoginLimiter()
			allowed := 0
			for i := 1; i <= 50; i++ {
				if limiter.Allow(LoginRateKey(source(i), "admin")) {
					allowed++
				}
			}
			if allowed != loginAccountBurst {
				t.Fatalf("one account got %d attempts from 50 rotating sources, want %d", allowed, loginAccountBurst)
			}
		})
	}
}

// The account is one account however its name is cased or padded: "Admin" and " admin " are the
// same sign-in, so they share the bucket.
func TestLoginLimiterAccountBucketIgnoresCaseAndPadding(t *testing.T) {
	limiter := NewLoginLimiter()
	names := []string{"admin", "Admin", "ADMIN", " admin", "admin ", "\tAdMiN\n"}
	allowed := 0
	for i := 1; i <= 60; i++ {
		if limiter.Allow(LoginRateKey("192.0.2."+strconv.Itoa(i), names[i%len(names)])) {
			allowed++
		}
	}
	if allowed != loginAccountBurst {
		t.Fatalf("spellings of one account got %d attempts, want %d", allowed, loginAccountBurst)
	}
}

// The account-wide bucket is per account: spraying one does not spend another's, and a source that
// is itself refused never reaches it, so one address cannot lock an account out for everyone.
func TestLoginLimiterAccountBucketIsPerAccountAndOutOfReachOfOneSource(t *testing.T) {
	limiter := NewLoginLimiter()
	for i := 1; i <= 50; i++ {
		limiter.Allow(LoginRateKey("192.0.2."+strconv.Itoa(i), "admin"))
	}
	if limiter.Allow(LoginRateKey("198.51.100.200", "admin")) {
		t.Fatal("a fresh source reached an account whose bucket was spent")
	}
	if !limiter.Allow(LoginRateKey("198.51.100.200", "ada")) {
		t.Fatal("spending one account's bucket throttled a different account")
	}

	// One source hammering one account: it is stopped by its own buckets, having spent no more of
	// the account's than its per-account burst, and the account's owner still gets in from elsewhere.
	limiter = NewLoginLimiter()
	for range 1000 {
		limiter.Allow(LoginRateKey("203.0.113.9", "admin"))
	}
	owner := 0
	for i := 1; i <= 50; i++ {
		if limiter.Allow(LoginRateKey("198.51.100."+strconv.Itoa(i), "admin")) {
			owner++
		}
	}
	if owner == 0 || owner < loginAccountBurst-5 {
		t.Fatalf("after one source hammered the account, others got %d attempts, want %d: one address locked the account out",
			owner, loginAccountBurst-5)
	}
}

// The bucket refills, slowly: a locked account is not locked forever, and it is not forgiven early.
func TestLoginLimiterAccountBucketRefills(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	limiter := NewLoginLimiter()
	sourceLimiterClock(limiter, &now)
	n := 0
	next := func() bool { n++; return limiter.Allow(LoginRateKey("2001:db8:1:"+strconv.Itoa(n)+"::1", "admin")) }
	for range loginAccountBurst {
		if !next() {
			t.Fatal("refused inside the burst")
		}
	}
	if next() {
		t.Fatal("allowed past the burst")
	}
	now = now.Add(loginAccountRefill / 2)
	if next() {
		t.Fatal("forgiven before a token refilled")
	}
	now = now.Add(loginAccountRefill)
	if !next() {
		t.Fatal("still refused after a token refilled")
	}
	if next() {
		t.Fatal("one refilled token allowed two attempts")
	}
}

// Attempts with no username have no account to throttle: they are held by their source alone.
func TestSourceLimiterNoAccountBucketForAnEmptyUsername(t *testing.T) {
	limiter := NewLoginLimiter()
	allowed := 0
	for i := 1; i <= 200; i++ {
		if limiter.Allow(LoginRateKey("192.0.2."+strconv.Itoa(i), "")) {
			allowed++
		}
	}
	if allowed != 200 {
		t.Fatalf("empty-username attempts from 200 sources got %d allowed, want all: they share no account", allowed)
	}
}

func TestLoginRateKeyNormalizesTheUsername(t *testing.T) {
	for _, tc := range []struct{ ip, user, want string }{
		{"192.0.2.10", "Admin", "192.0.2.10|admin"},
		{"192.0.2.10", "  admin\t", "192.0.2.10|admin"},
		{"::ffff:192.0.2.10", "admin", "192.0.2.10|admin"},
		{"2001:db8:1:2::1", "admin", "2001:db8:1:2::/64|admin"},
		{"2001:db8:1:2:ffff:ffff:ffff:ffff", "admin", "2001:db8:1:2::/64|admin"},
		{"2001:db8:1:3::1", "admin", "2001:db8:1:3::/64|admin"},
		{"192.0.2.10", "", "192.0.2.10|"},
	} {
		if got := LoginRateKey(tc.ip, tc.user); got != tc.want {
			t.Errorf("LoginRateKey(%q, %q) = %q, want %q", tc.ip, tc.user, got, tc.want)
		}
	}
}
