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

// tokens is how many attempts key could make now: a key never seen has its whole burst. It spends
// nothing.
func (r *RateLimiter) tokens(key string) float64 {
	now := r.now()
	r.mu.Lock()
	l, ok := r.limiters[key]
	r.mu.Unlock()
	if !ok {
		return float64(r.burst)
	}
	return l.TokensAt(now)
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
//
// Allow spends all three buckets on every attempt, for callers with no credential to fail (a
// request that emails the account holder). A sign-in or a redemption uses Admit instead, which
// spends the account bucket only on a failed credential: anyone who knows a username can otherwise
// keep that bucket empty with one attempt per refill and lock its owner out. An address that has
// signed in as the account is not held by the account bucket (it is still held by the source and
// pair buckets), so the owner gets in while the account is under guessing; a new address during an
// attack waits for a refill.
type SourceLimiter struct {
	source, pair, account *RateLimiter

	mu       sync.Mutex
	inflight map[string]int       // account -> admitted attempts not yet resolved
	signedIn map[string]time.Time // source|account -> when it last signed in
}

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

		inflight: map[string]int{}, signedIn: map[string]time.Time{},
	}
}

// signedInSourceTTL is how long an address that signed in as an account is exempt from its
// account bucket. It is memory only: a restart forgets it, and the exemption is then earned again by
// the next sign-in.
const signedInSourceTTL = 30 * 24 * time.Hour

// maxSignedInSources bounds that memory. A household has a handful of addresses; at the bound the
// oldest are dropped, which only withdraws an exemption.
const maxSignedInSources = 1024

// Admission is one attempt Admit let through, to be resolved exactly once with the outcome of its
// credential check: Failed (a guess, which spends the account bucket), Succeeded (not a guess, and
// the address is remembered), or Abandon (it never reached a credential check, or failed for a
// reason that is not a guess). Resolving it again does nothing. The zero value and the admission of
// a key with no account are safe to resolve.
type Admission struct {
	l       *SourceLimiter
	source  string
	account string
	counted bool // holds a slot in inflight
	once    sync.Once
}

// Admit is Allow for an attempt whose outcome is not known yet: it takes the source and pair
// buckets, and lets the attempt through only if the account bucket has a token to spare, without
// spending it. The attempt's slot is held until it is resolved, so concurrent attempts cannot all
// be let through on the one token that is left.
func (l *SourceLimiter) Admit(key string) (*Admission, bool) {
	source, account, _ := strings.Cut(key, "|")
	account = normalizeAccountKey(account)
	if !l.source.Allow(source) || !l.pair.Allow(source+"|"+account) {
		return nil, false
	}
	adm := &Admission{l: l, source: source, account: account}
	if account == "" {
		return adm, true
	}
	if l.signedInSource(source, account) {
		return adm, true // not held by the account bucket; its failure is still charged to it
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.account.tokens(account)-float64(l.inflight[account]) < 1 {
		return nil, false
	}
	l.inflight[account]++
	adm.counted = true
	return adm, true
}

func (a *Admission) resolve(spend, remember bool) {
	if a == nil || a.l == nil {
		return
	}
	a.once.Do(func() {
		l := a.l
		if a.account != "" && spend {
			l.account.Allow(a.account)
		}
		// The slot is released after the token is spent, so no attempt sees both gone.
		if a.counted {
			l.mu.Lock()
			if l.inflight[a.account]--; l.inflight[a.account] <= 0 {
				delete(l.inflight, a.account)
			}
			l.mu.Unlock()
		}
		if a.account != "" && remember {
			l.rememberSignedIn(a.source, a.account)
		}
	})
}

// Failed records that the attempt's credential was wrong: one guess, spent from the account bucket.
func (a *Admission) Failed() { a.resolve(true, false) }

// Succeeded records that the attempt signed in. It spends nothing, and the address is remembered as
// one that signs in as this account.
func (a *Admission) Succeeded() { a.resolve(false, true) }

// Abandon records an attempt that was not a guess at the account's credential.
func (a *Admission) Abandon() { a.resolve(false, false) }

func (l *SourceLimiter) signedInSource(source, account string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	at, ok := l.signedIn[source+"|"+account]
	if !ok {
		return false
	}
	if l.account.now().Sub(at) >= signedInSourceTTL {
		delete(l.signedIn, source+"|"+account)
		return false
	}
	return true
}

func (l *SourceLimiter) rememberSignedIn(source, account string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.account.now()
	if _, known := l.signedIn[source+"|"+account]; !known && len(l.signedIn) >= maxSignedInSources {
		l.pruneSignedIn(now)
	}
	l.signedIn[source+"|"+account] = now
}

// pruneSignedIn drops the lapsed entries, then the oldest, until there is room for one more.
// Callers hold l.mu.
func (l *SourceLimiter) pruneSignedIn(now time.Time) {
	for key, at := range l.signedIn {
		if now.Sub(at) >= signedInSourceTTL {
			delete(l.signedIn, key)
		}
	}
	for len(l.signedIn) >= maxSignedInSources {
		oldest, oldestAt := "", now.Add(time.Hour)
		for key, at := range l.signedIn {
			if at.Before(oldestAt) {
				oldest, oldestAt = key, at
			}
		}
		delete(l.signedIn, oldest)
	}
}

func (l *SourceLimiter) signedInCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.signedIn)
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
