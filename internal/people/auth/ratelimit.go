package auth

import (
	"net/netip"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// sweepEvery is how often a RateLimiter forgets the keys whose bucket has refilled.
const sweepEvery = time.Minute

// RateLimiter gates attempts per key, in-memory (§11: per-instance is acceptable v1). A small burst
// absorbs a user retyping a password; sustained attempts are throttled.
//
// A key whose bucket is full again is indistinguishable from a key never seen, so at most once a
// minute the limiter drops those. That forgives nothing early, and it keeps a stream of one-off
// keys (a new username per attempt) from growing the map without bound.
type RateLimiter struct {
	mu        sync.Mutex
	limiters  map[string]*rate.Limiter
	perSec    rate.Limit
	burst     int
	now       func() time.Time
	lastSweep time.Time
}

// NewRateLimiter allows `burst` immediate attempts, refilling at perSec/sec.
func NewRateLimiter(perSec float64, burst int) *RateLimiter {
	return &RateLimiter{limiters: map[string]*rate.Limiter{}, perSec: rate.Limit(perSec), burst: burst, now: time.Now}
}

// Allow reports whether an attempt for key may proceed now.
func (r *RateLimiter) Allow(key string) bool {
	now := r.now()
	r.mu.Lock()
	if now.Sub(r.lastSweep) >= sweepEvery {
		r.sweep(now)
	}
	l, ok := r.limiters[key]
	if !ok {
		l = rate.NewLimiter(r.perSec, r.burst)
		r.limiters[key] = l
	}
	r.mu.Unlock()
	return l.AllowN(now, 1)
}

// sweep drops every key whose bucket has refilled to the full burst. Callers hold r.mu.
func (r *RateLimiter) sweep(now time.Time) {
	r.lastSweep = now
	for key, l := range r.limiters {
		if l.TokensAt(now) >= float64(r.burst) {
			delete(r.limiters, key)
		}
	}
}

func (r *RateLimiter) size() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.limiters)
}

// LoginRateKey is the key a sign-in or account-recovery attempt is throttled on: the client's
// throttle source (ThrottleSource) and the username.
func LoginRateKey(clientIP, username string) string { return ThrottleSource(clientIP) + "|" + username }

// ThrottleSource is the source a client address is throttled as. An IPv4 address is itself. An IPv6
// address is its /64, because a client is routinely handed the whole prefix and could otherwise take
// a fresh address, and a fresh allowance, per attempt. Anything else is kept as given.
func ThrottleSource(clientIP string) string {
	addr, err := netip.ParseAddr(clientIP)
	if err != nil {
		return clientIP
	}
	addr = addr.WithZone("").Unmap()
	if addr.Is4() {
		return addr.String()
	}
	prefix, _ := addr.Prefix(64)
	return prefix.String()
}

// SourceLimiter throttles attempts keyed by LoginRateKey on two buckets: one per key (one account
// from one source), and one per client address across every username it tries. Rotating usernames
// therefore cannot buy a fresh burst per name.
type SourceLimiter struct{ account, source *RateLimiter }

// NewSourceLimiter allows keyBurst attempts per LoginRateKey (refilling at keyPerSec) and
// sourceBurst per client address across all usernames (refilling at sourcePerSec).
func NewSourceLimiter(keyPerSec float64, keyBurst int, sourcePerSec float64, sourceBurst int) *SourceLimiter {
	return &SourceLimiter{
		account: NewRateLimiter(keyPerSec, keyBurst),
		source:  NewRateLimiter(sourcePerSec, sourceBurst),
	}
}

// Allow reports whether an attempt for key (a LoginRateKey) may proceed now. The source bucket is
// asked first, so an attempt it refuses spends nothing from the account's.
func (l *SourceLimiter) Allow(key string) bool {
	source, _, _ := strings.Cut(key, "|")
	return l.source.Allow(source) && l.account.Allow(key)
}

// The sign-in throttle. An account allows a short burst from one source (a person retyping a
// password); a source allows more across every username (a household behind one address signing in
// on several devices), and no more.
const loginSourceBurst = 20

// NewLoginLimiter returns the sign-in throttle.
func NewLoginLimiter() *SourceLimiter { return NewSourceLimiter(0.2, 5, 0.5, loginSourceBurst) }
