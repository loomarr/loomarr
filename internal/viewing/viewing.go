// Package viewing tracks who is watching which channel right now, from the players' own playlist
// polls (#1662).
//
// A player on a live HLS stream re-reads its media playlist every segment for as long as it plays,
// so a person is watching a channel exactly while their player keeps polling it. The signal is the
// server's own observation, which needs nothing from the clients and can't drift from what is
// really streaming. Two rules turn polls into viewings:
//
//   - MinPolls. The channel warmer fetches a neighbour's playlist once so a surf lands fast; one
//     fetch is not a viewer, a second within the TTL is.
//   - TTL. A player that stops polling (closed tab, TV off, network gone) has stopped watching.
//
// A device watches one channel at a time: when it polls a new channel past MinPolls, that
// replaces its old one at once rather than showing it on both until the old one times out.
//
// The tracker is in memory. One replica is the supported topology (deployment.md), and a restart
// loses nothing a single poll interval doesn't rebuild.
package viewing

import (
	"slices"
	"sync"
	"time"
)

const (
	// TTL is how long after its last poll a viewing ends. Several segment intervals, so one slow
	// or dropped poll doesn't flicker a viewer out.
	TTL = 30 * time.Second
	// MinPolls is how many playlist polls within the TTL make a viewing.
	MinPolls = 2
)

// Viewer is who is behind a stream: the person, and the device they watch on.
type Viewer struct {
	UserID string
	// DeviceKey tells one person's devices apart: a paired device's identity, or the browser.
	DeviceKey string
	// DeviceLabel is what the household calls the device ("Living room TV", "Firefox on macOS").
	DeviceLabel string
}

// Watch is one person watching one channel on one device.
type Watch struct {
	Viewer
	ChannelID string
	// Since is the first poll of this viewing.
	Since    time.Time
	LastSeen time.Time
}

type key struct{ user, device, channel string }

type entry struct {
	label       string
	first, last time.Time
	polls       int
}

// Tracker records playlist polls and answers who is watching. Safe for concurrent use.
type Tracker struct {
	mu      sync.Mutex
	entries map[key]*entry
}

// New returns an empty tracker.
func New() *Tracker { return &Tracker{entries: make(map[key]*entry)} }

// Observe records one media-playlist poll by v on channelID at `at`.
func (t *Tracker) Observe(v Viewer, channelID string, at time.Time) {
	if v.UserID == "" || channelID == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneLocked(at)
	k := key{v.UserID, v.DeviceKey, channelID}
	e := t.entries[k]
	if e == nil {
		e = &entry{first: at, last: at}
		t.entries[k] = e
	}
	e.polls++
	if at.After(e.last) {
		e.last = at
	}
	e.label = v.DeviceLabel
}

// pruneLocked drops every entry whose last poll is older than the TTL, so a viewing that pauses
// past the TTL starts afresh (new Since, MinPolls again) and the map stays bounded by live players.
func (t *Tracker) pruneLocked(at time.Time) {
	for k, e := range t.entries {
		if at.Sub(e.last) > TTL {
			delete(t.entries, k)
		}
	}
}

// Watching returns every live viewing at `at`, one per device, oldest first.
func (t *Tracker) Watching(at time.Time) []Watch {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneLocked(at)
	type device struct{ user, device string }
	current := make(map[device]Watch)
	for k, e := range t.entries {
		if e.polls < MinPolls {
			continue
		}
		d := device{k.user, k.device}
		if w, ok := current[d]; ok && !e.last.After(w.LastSeen) {
			continue
		}
		current[d] = Watch{Viewer: Viewer{UserID: k.user, DeviceKey: k.device, DeviceLabel: e.label},
			ChannelID: k.channel, Since: e.first, LastSeen: e.last}
	}
	out := make([]Watch, 0, len(current))
	for _, w := range current {
		out = append(out, w)
	}
	slices.SortFunc(out, func(a, b Watch) int {
		if c := a.Since.Compare(b.Since); c != 0 {
			return c
		}
		if a.UserID != b.UserID {
			if a.UserID < b.UserID {
				return -1
			}
			return 1
		}
		if a.DeviceKey < b.DeviceKey {
			return -1
		}
		if a.DeviceKey > b.DeviceKey {
			return 1
		}
		return 0
	})
	return out
}
