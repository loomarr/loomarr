// Package logchange keeps a repeating per-item condition from flooding the log.
//
// A background pass that revisits the same items every few minutes and warns about each failing
// one writes the same lines forever: in production one filler pass logged ~25 identical per-clip
// warnings every two minutes, which — with River's chatter — rotated the container logs away
// after ~18 hours. The condition was worth one warning, not 375.
//
// A Throttle remembers, per key, the last state it let through. A record passes when the key is
// new, when its state differs from the last one, or when the long interval has elapsed (so a
// condition that never clears still resurfaces occasionally). Everything else is demoted to
// DEBUG rather than dropped, so it stays reachable when someone turns the level up.
package logchange

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const defaultMaxKeys = 10_000

type entry struct {
	state string
	at    time.Time
}

// Throttle is safe for concurrent use. The zero value is not usable; call New. A nil *Throttle
// passes everything through, so an optional dependency needs no guard at the call site.
type Throttle struct {
	mu       sync.Mutex
	seen     map[string]entry
	interval time.Duration
	maxKeys  int
	now      func() time.Time
}

// Option customises a Throttle.
type Option func(*Throttle)

// WithClock injects the time source (tests).
func WithClock(now func() time.Time) Option { return func(t *Throttle) { t.now = now } }

// WithMaxKeys bounds how many keys are remembered. Past the bound the memory is reset, which at
// worst repeats each live condition's warning once — cheaper than an unbounded map.
func WithMaxKeys(n int) Option { return func(t *Throttle) { t.maxKeys = n } }

// New returns a Throttle that lets an unchanged condition through again after interval.
func New(interval time.Duration, opts ...Option) *Throttle {
	t := &Throttle{seen: map[string]entry{}, interval: interval, maxKeys: defaultMaxKeys, now: time.Now}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// Changed reports whether (key, state) should be logged loudly, and records that it was.
func (t *Throttle) Changed(key, state string) bool {
	if t == nil {
		return true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	if prev, ok := t.seen[key]; ok && prev.state == state && now.Sub(prev.at) < t.interval {
		return false
	}
	if _, ok := t.seen[key]; !ok && len(t.seen) >= t.maxKeys {
		clear(t.seen)
	}
	t.seen[key] = entry{state: state, at: now}
	return true
}

// Forget re-arms key, so its next occurrence warns again. Call it when the condition clears.
func (t *Throttle) Forget(key string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	delete(t.seen, key)
	t.mu.Unlock()
}

// Len is the number of keys currently remembered.
func (t *Throttle) Len() int {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.seen)
}

// Warn logs msg at WARN when (key, state) is new or changed, and at DEBUG otherwise. args are the
// usual slog attributes (usually the error). state is what "unchanged" means for this key —
// typically the error text.
func (t *Throttle) Warn(log *slog.Logger, key, state, msg string, args ...any) {
	if log == nil {
		return
	}
	level := slog.LevelDebug
	if t.Changed(key, state) {
		level = slog.LevelWarn
	}
	log.Log(context.Background(), level, msg, args...)
}
