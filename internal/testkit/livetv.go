package testkit

import (
	"context"
	"strconv"
	"strings"
	"sync"
)

// LiveTV is the shared library.LiveTV test double (AGENTS.md: one mock per
// capability). It is an in-memory media-server Live TV surface that records
// registered tuners/providers and guide-refresh pokes, so setup tests can assert
// the idempotent enumerate-first behavior (§6 second-call-no-op) without the
// real /LiveTv/* HTTP shapes (those are pinned via the Phase-10 live capture and
// exercised by the library adapter's own fixture tests).
type LiveTV struct {
	mu        sync.Mutex
	tuners    map[string]fakeTuner // by id
	listings  map[string]string    // id → XMLTV url
	nextID    int
	Refreshes int
	Rescans   int // tuner re-scans (§9 new-channel discovery)
	calls     []string

	// Injectable failures (nil = success).
	AddTunerErr      error
	AddListingErr    error
	RemoveTunerErr   error
	RemoveListingErr error
	StaleListingsErr error
	RefreshErr       error
	RescanErr        error
}

// fakeTuner models a media-server tuner host with the fields the URL-change
// reconcile keys on: the URL and the FriendlyName that marks Loomarr ownership.
type fakeTuner struct {
	url          string
	friendlyName string
}

// loomarrFriendlyName mirrors library.tunerFriendlyName — the tag AddTuner stamps
// so the reconcile can tell Loomarr's tuners from hand-added ones.
const loomarrFriendlyName = "loomarr"

// NewLiveTV builds an empty in-memory Live TV surface.
func NewLiveTV() *LiveTV {
	return &LiveTV{tuners: map[string]fakeTuner{}, listings: map[string]string{}}
}

// SeedTuner registers a tuner with an explicit FriendlyName (test setup): use it to
// pre-load a Loomarr-owned tuner at an OLD url (friendlyName "loomarr") or a
// hand-added one (any other name) to drive the URL-change reconcile tests.
func (l *LiveTV) SeedTuner(url, friendlyName string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.nextID++
	l.tuners[l.idLocked()] = fakeTuner{url: url, friendlyName: friendlyName}
}

func (l *LiveTV) idLocked() string { return "tuner-" + strconv.Itoa(l.nextID) }

func (l *LiveTV) TunerRegistered(_ context.Context, m3u string) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, "check-tuner:"+m3u)
	for _, t := range l.tuners {
		if t.url == m3u {
			return true, nil
		}
	}
	return false, nil
}

func (l *LiveTV) ListingRegistered(_ context.Context, xmltv string) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, "check-listing:"+xmltv)
	for _, u := range l.listings {
		if u == xmltv {
			return true, nil
		}
	}
	return false, nil
}

func (l *LiveTV) AddTuner(_ context.Context, m3u string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, "add-tuner:"+m3u)
	if l.AddTunerErr != nil {
		return l.AddTunerErr
	}
	l.nextID++
	// Real AddTuner stamps FriendlyName "loomarr"; the fake mirrors that so the
	// URL-change reconcile can distinguish Loomarr's tuners from hand-added ones.
	l.tuners[l.idLocked()] = fakeTuner{url: m3u, friendlyName: loomarrFriendlyName}
	return nil
}

func (l *LiveTV) AddListingProvider(_ context.Context, xmltv string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, "add-listing:"+xmltv)
	if l.AddListingErr != nil {
		return l.AddListingErr
	}
	l.nextID++
	l.listings["listing-"+strconv.Itoa(l.nextID)] = xmltv
	return nil
}

// StaleLoomarrTuners returns ids of Loomarr-owned tuners whose url != desired.
func (l *LiveTV) StaleLoomarrTuners(_ context.Context, desiredM3U string) ([]string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, "stale-tuners:"+desiredM3U)
	var ids []string
	for id, t := range l.tuners {
		if t.friendlyName == loomarrFriendlyName && t.url != desiredM3U {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// LoomarrTuners returns ids of ALL Loomarr-owned tuners (for a forced re-wire).
func (l *LiveTV) LoomarrTuners(_ context.Context) ([]string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var ids []string
	for id, t := range l.tuners {
		if t.friendlyName == loomarrFriendlyName {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// StaleLoomarrListings mirrors library.LiveTV's ownership rule: either Tunarr's
// XMLTV path or Loomarr's internal-playout guide path is managed, including when
// the latter carries its device-token query string.
func (l *LiveTV) StaleLoomarrListings(_ context.Context, desiredXMLTV string) ([]string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, "stale-listings:"+desiredXMLTV)
	if l.StaleListingsErr != nil {
		return nil, l.StaleListingsErr
	}
	var ids []string
	for id, u := range l.listings {
		if isLoomarrListing(u) && u != desiredXMLTV {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func isLoomarrListing(raw string) bool {
	path := strings.TrimRight(raw, "/")
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	return strings.HasSuffix(path, "/api/xmltv.xml") || strings.HasSuffix(path, "/playout/guide.xml")
}

func (l *LiveTV) RemoveTuner(_ context.Context, id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, "remove-tuner:"+id)
	if l.RemoveTunerErr != nil {
		return l.RemoveTunerErr
	}
	delete(l.tuners, id)
	return nil
}

func (l *LiveTV) RemoveListingProvider(_ context.Context, id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, "remove-listing:"+id)
	if l.RemoveListingErr != nil {
		return l.RemoveListingErr
	}
	delete(l.listings, id)
	return nil
}

func (l *LiveTV) RefreshGuide(_ context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, "refresh-guide")
	if l.RefreshErr != nil {
		return l.RefreshErr
	}
	l.Refreshes++
	return nil
}

func (l *LiveTV) RescanTuner(_ context.Context, m3u string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, "rescan-tuner:"+m3u)
	if l.RescanErr != nil {
		return l.RescanErr
	}
	l.Rescans++
	return nil
}

// TunerCount / ListingCount are race-safe accessors for assertions.
func (l *LiveTV) TunerCount() int   { l.mu.Lock(); defer l.mu.Unlock(); return len(l.tuners) }
func (l *LiveTV) ListingCount() int { l.mu.Lock(); defer l.mu.Unlock(); return len(l.listings) }

// HasTuner reports whether any tuner host currently targets the given url —
// lets URL-change reconcile tests assert which tuner survived without exposing ids.
func (l *LiveTV) HasTuner(url string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, t := range l.tuners {
		if t.url == url {
			return true
		}
	}
	return false
}

// Calls returns the ordered media-server operations observed by this double.
// It lets transition tests prove target registration precedes stale retirement
// without inventing a private mock for the library.LiveTV capability.
func (l *LiveTV) Calls() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.calls...)
}

// Note: this mock deliberately does NOT reference library.LiveTV to avoid a
// testkit→library import cycle (library's internal tests import testkit). It
// satisfies the interface structurally; the assertion lives at the use site
// (setup.NewLiveTVConnector takes a library.LiveTV).
