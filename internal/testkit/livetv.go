package testkit

import (
	"context"
	"sync"
)

// LiveTV is the shared library.LiveTV test double (CLAUDE.md: one mock per
// capability). It is an in-memory media-server Live TV surface that records
// registered tuners/providers and guide-refresh pokes, so setup tests can assert
// the idempotent enumerate-first behavior (§6 second-call-no-op) without the
// real /LiveTv/* HTTP shapes (those are pinned via the Phase-10 live capture and
// exercised by the library adapter's own fixture tests).
type LiveTV struct {
	mu        sync.Mutex
	tuners    map[string]bool // by M3U url
	listings  map[string]bool // by XMLTV url
	Refreshes int

	// Injectable failures (nil = success).
	AddTunerErr error
	RefreshErr  error
}

// NewLiveTV builds an empty in-memory Live TV surface.
func NewLiveTV() *LiveTV {
	return &LiveTV{tuners: map[string]bool{}, listings: map[string]bool{}}
}

func (l *LiveTV) TunerRegistered(_ context.Context, m3u string) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.tuners[m3u], nil
}

func (l *LiveTV) ListingRegistered(_ context.Context, xmltv string) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.listings[xmltv], nil
}

func (l *LiveTV) AddTuner(_ context.Context, m3u string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.AddTunerErr != nil {
		return l.AddTunerErr
	}
	l.tuners[m3u] = true
	return nil
}

func (l *LiveTV) AddListingProvider(_ context.Context, xmltv string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.listings[xmltv] = true
	return nil
}

func (l *LiveTV) RefreshGuide(_ context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.RefreshErr != nil {
		return l.RefreshErr
	}
	l.Refreshes++
	return nil
}

// TunerCount / ListingCount are race-safe accessors for assertions.
func (l *LiveTV) TunerCount() int   { l.mu.Lock(); defer l.mu.Unlock(); return len(l.tuners) }
func (l *LiveTV) ListingCount() int { l.mu.Lock(); defer l.mu.Unlock(); return len(l.listings) }

// Note: this mock deliberately does NOT reference library.LiveTV to avoid a
// testkit→library import cycle (library's internal tests import testkit). It
// satisfies the interface structurally; the assertion lives at the use site
// (setup.NewLiveTVConnector takes a library.LiveTV).
