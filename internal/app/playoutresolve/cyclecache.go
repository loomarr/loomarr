package playoutresolve

import (
	"encoding/binary"
	"hash/fnv"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
)

// The arranged-cycle cache — the guide's read path only (V13b follow-up).
//
// GET /v1/guide resolves EVERY channel's timeline, and each one re-runs the whole
// schedule.ComputeDesiredAt for that channel. Profiled on the maintainer's dev install:
// backtrackArrange was 53% of the request's CPU (newCandidateOrder 29%, the candidate sort 20%)
// with GC a further 21% from the allocation churn that arrangement produces. The store was
// 0.66% — this path is CPU, not I/O.
//
// The channels already resolve CONCURRENTLY (see api.channelGuide), which turns a sum into a max
// (~490ms to ~100ms). That is the floor concurrency can reach: you cannot parallelize below the
// slowest single channel. The remaining cost is REDUNDANT rather
// than slow — the same channel, unchanged, is re-arranged on every poll of a grid that refetches
// on an SSE frame and re-renders every thirty seconds.
//
// # Why a fingerprint key and not an event
//
// This cache is keyed on a HASH OF THE INPUTS the forecast reads, not on a channel-changed
// notification or the row's revision. An event key would mean trusting ChannelChanged to fire on
// every mutation, and a missed frame would serve a stale lineup indefinitely. The row's Revision
// and UpdatedAt move on every reconcile save, deadlines included, so keying on them would empty
// the cache at every sweep. The input hash makes a stale entry unreachable rather than merely
// unlikely, and survives a sweep that changes nothing. The store read that makes it possible is
// already on this path and costs ~0.7% of the request.
//
// channels.Engine.ForecastKey owns the hash, over the same settings snapshot the forecast reads,
// and its test classifies every channel field, so a new scheduler input cannot be missed here.
//
// # Why here and not in channels.Engine
//
// Engine.CyclePreview forecasts future guide windows. AiringNow instead reads the persisted
// accepted Desired cycle directly, and the guide substitutes that same snapshot for the window
// containing now. Keeping this cache on forecast computation means an expiry can make future
// listings slower or stale, never change the actual broadcast or its current guide block.
//
// Draft previews are never cached — CyclePreviewDraft with a non-nil draft is an unsaved
// what-if whose whole purpose is to reflect the edit in hand.
//
// # How `at` enters the key (#1397)
//
// `at` is the Guide window's `from`, so it moves with every poll, every ±1h step and every day
// change. Keying on the wall-clock minute would make nearly every request a new key: a navigated
// window would always miss, and so would any visit more than a minute after the last.
//
// But `at` reaches the arrangement by exactly three routes — the active rule, the active holidays,
// and the rolling-window index — so the key carries THOSE, not the time. schedule.ClockSignature
// covers the first two; the rolling-window index is added per lookup from the window length the
// cache learned when it stored the entry (see cycleCache). Two windows inside one rolling window,
// under the same rule and holiday state, share one arrangement however far apart they are.

// cycleCacheTTL bounds how long an arranged cycle may be reused.
//
// The key covers channel STATE, the install settings and the CLOCK, so this bounds only what the
// row cannot show: availability (titles land), the filler pool (clips land or retire), airing
// history and natural-break measurements. Those move slowly, and a forecast that is a few minutes
// behind them is the same class of staleness BroadcastsBetween already documents; the window
// containing NOW is served from the persisted Desired cycle, not from here.
const cycleCacheTTL = 5 * time.Minute

// cycleCacheMaxEntries bounds the cache against a client paging through many windows. One channel
// under one rule/holiday state has one entry per rolling window visited.
const cycleCacheMaxEntries = 512

// cycleEntry is one arranged cycle plus the rolling-window horizon it resolved to.
//
// The window rides along because it comes from the SAME CyclePreview answer and the guide's
// segmentation needs it to know where the deck rotates (segmentedBroadcasts). Caching it avoids
// re-deriving the rule > channel > default precedence at the call site and getting it subtly
// different from what the arrangement actually used.
type cycleEntry struct {
	slots  []schedule.Slot
	window time.Duration
	stored time.Time
}

// windowNote remembers the rolling-window length an input set resolved to. It is what lets a
// lookup compute the window index BEFORE it has an arrangement: the length is a function of the
// rule/channel/engine default, all of which are already pinned by the base key (or, for the
// engine default, bounded by cycleCacheTTL).
type windowNote struct {
	window time.Duration
	stored time.Time
}

// cycleCache memoises arranged cycles for the guide's read path.
//
// Entries are keyed by (base fingerprint, rolling-window index). Bounded by cycleCacheMaxEntries
// and pruned on write, so it cannot grow with request volume.
type cycleCache struct {
	mu      sync.Mutex
	entries map[uint64]cycleEntry
	windows map[uint64]windowNote
	now     func() time.Time
}

func newCycleCache(now func() time.Time) *cycleCache {
	if now == nil {
		now = time.Now
	}
	return &cycleCache{entries: map[uint64]cycleEntry{}, windows: map[uint64]windowNote{}, now: now}
}

// windowKey folds the rolling-window index of `at`, on the grid's wall clock `zone`, into the base
// key.
func windowKey(base uint64, at time.Time, window time.Duration, zone *time.Location) uint64 {
	h := fnv.New64a()
	var num [8]byte
	binary.LittleEndian.PutUint64(num[:], base)
	_, _ = h.Write(num[:])
	binary.LittleEndian.PutUint64(num[:], uint64(schedule.WindowIndex(at, window, zone)))
	_, _ = h.Write(num[:])
	return h.Sum64()
}

// get returns a live entry for this input set at `at`, if one exists. A base key never stored has
// no known window length, so it simply misses.
func (c *cycleCache) get(base uint64, at time.Time, zone *time.Location) ([]schedule.Slot, time.Duration, bool) {
	if c == nil {
		return nil, 0, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	note, ok := c.windows[base]
	if !ok || c.now().Sub(note.stored) > cycleCacheTTL {
		return nil, 0, false
	}
	e, ok := c.entries[windowKey(base, at, note.window, zone)]
	if !ok || c.now().Sub(e.stored) > cycleCacheTTL {
		return nil, 0, false
	}
	return e.slots, e.window, true
}

// put stores an arrangement, and prunes anything that has aged out.
//
// Pruning on WRITE rather than on a timer keeps this free of a background goroutine: the cache
// is only touched by requests, so a cache nobody reads costs nothing rather than ticking.
func (c *cycleCache) put(base uint64, at time.Time, zone *time.Location, slots []schedule.Slot, window time.Duration) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	for k, e := range c.entries {
		if now.Sub(e.stored) > cycleCacheTTL {
			delete(c.entries, k)
		}
	}
	for k, n := range c.windows {
		if now.Sub(n.stored) > cycleCacheTTL {
			delete(c.windows, k)
		}
	}
	for len(c.entries) >= cycleCacheMaxEntries {
		var oldest uint64
		var oldestAt time.Time
		for k, e := range c.entries {
			if oldestAt.IsZero() || e.stored.Before(oldestAt) {
				oldest, oldestAt = k, e.stored
			}
		}
		delete(c.entries, oldest)
	}
	c.windows[base] = windowNote{window: window, stored: now}
	c.entries[windowKey(base, at, window, zone)] = cycleEntry{slots: slots, window: window, stored: now}
}
