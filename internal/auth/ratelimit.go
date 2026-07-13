package auth

import (
	"sync"

	"golang.org/x/time/rate"
)

// RateLimiter gates login attempts per key (IP+username), in-memory (§11: login
// only; per-instance is acceptable v1). A small burst absorbs a user retyping a
// password; sustained attempts are throttled.
type RateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	perSec   rate.Limit
	burst    int
}

// NewRateLimiter allows `burst` immediate attempts, refilling at perSec/sec.
func NewRateLimiter(perSec float64, burst int) *RateLimiter {
	return &RateLimiter{limiters: map[string]*rate.Limiter{}, perSec: rate.Limit(perSec), burst: burst}
}

// Allow reports whether an attempt for key may proceed now.
func (r *RateLimiter) Allow(key string) bool {
	r.mu.Lock()
	l, ok := r.limiters[key]
	if !ok {
		l = rate.NewLimiter(r.perSec, r.burst)
		r.limiters[key] = l
	}
	r.mu.Unlock()
	return l.Allow()
}
