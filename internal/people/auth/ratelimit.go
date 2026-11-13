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
// throttle source (ThrottleSource) and the username, trimmed and lower-cased so "Admin" and
// " admin " are one account.
func LoginRateKey(clientIP, username string) string {
	return ThrottleSource(clientIP) + "|" + normalizeAccountKey(username)
}

func normalizeAccountKey(username string) string { return strings.ToLower(strings.TrimSpace(username)) }

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

// SourceLimiter throttles attempts keyed by LoginRateKey on three buckets, from the narrowest
// cause to the widest:
//
//   - source: one client address (an IPv6 /64) across every username it tries, so rotating
//     usernames cannot buy a fresh burst per name;
//   - pair: one account from one source, a person retyping a password;
//   - account: one account from every source together, so rotating sources (the /64s of an IPv6
//     delegation, an IPv4 pool) cannot buy a fresh burst per address. The cost of a guess no longer
//     depends on how much address space the guesser has.
//
// The account bucket is asked last, so an attempt the source or the pair has already refused
// spends nothing from it: one address cannot lock an account out of everyone else's reach.
type SourceLimiter struct{ source, pair, account *RateLimiter }

// NewSourceLimiter allows pairBurst attempts per LoginRateKey (refilling at pairPerSec),
// sourceBurst per client address across all usernames (sourcePerSec), and accountBurst per account
// across every source (accountPerSec).
func NewSourceLimiter(
	pairPerSec float64, pairBurst int, sourcePerSec float64, sourceBurst int, accountPerSec float64, accountBurst int,
) *SourceLimiter {
	return &SourceLimiter{
		source:  NewRateLimiter(sourcePerSec, sourceBurst),
		pair:    NewRateLimiter(pairPerSec, pairBurst),
		account: NewRateLimiter(accountPerSec, accountBurst),
	}
}

// Allow reports whether an attempt for key (a LoginRateKey) may proceed now. A key with no username
// has no account to throttle and is held by its source alone.
func (l *SourceLimiter) Allow(key string) bool {
	source, account, _ := strings.Cut(key, "|")
	if !l.source.Allow(source) || !l.pair.Allow(source+"|"+normalizeAccountKey(account)) {
		return false
	}
	account = normalizeAccountKey(account)
	return account == "" || l.account.Allow(account)
}

// The sign-in throttle. A pair allows a short burst (a person retyping a password); a source allows
// more across every username (a household behind one address signing in on several devices), and
// no more; an account allows a larger burst across all sources, refilling slowly, which is what
// stops an attacker who rotates addresses. A real owner locked out by that waits one refill.
const (
	loginSourceBurst  = 20
	loginAccountBurst = 20
	// loginAccountRefill is the time one account-wide attempt takes to come back.
	loginAccountRefill = 30 * time.Second
)

// NewLoginLimiter returns the sign-in throttle.
func NewLoginLimiter() *SourceLimiter {
	return NewSourceLimiter(0.2, 5, 0.5, loginSourceBurst, 1/loginAccountRefill.Seconds(), loginAccountBurst)
}
