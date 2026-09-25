package app

import (
	"encoding/binary"
	"encoding/json"
	"hash/fnv"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/schedule"
)

// The arranged-cycle cache — the guide's read path only (V13b follow-up).
//
// GET /v1/guide resolves EVERY channel's timeline, and each one re-runs the whole
// schedule.ComputeDesiredAt for that channel. Profiled on the maintainer's dev install:
// backtrackArrange was 53% of the request's CPU (newCandidateOrder 29%, the candidate sort 20%)
// with GC a further 21% from the allocation churn that arrangement produces. The store was
// 0.66% — this path is CPU, not I/O.
//
// A previous round made the channels resolve CONCURRENTLY (see api.channelGuide), which turned
// a sum into a max and took ~490ms to ~100ms. That is the floor concurrency can reach: you
// cannot parallelize below the slowest single channel. The remaining cost is REDUNDANT rather
// than slow — the same channel, unchanged, is re-arranged on every poll of a grid that refetches
// on an SSE frame and re-renders every thirty seconds.
//
// # Why a fingerprint key and not an event
//
// This cache is keyed on a HASH OF THE INPUTS ComputeDesiredAt actually reads, not on a
// channel-changed notification. store.Channel carries no version or updated-at column, so an
// event key would mean trusting ChannelChanged to fire on every mutation — and a missed frame
// would serve a stale lineup indefinitely, which is a correctness bug that looks like a UI
// glitch. Fingerprinting inverts that: a changed lineup, policy or channel field yields a
// DIFFERENT key, so a stale entry is unreachable rather than merely unlikely. The store read
// that makes it possible is already on this path and costs ~0.7% of the request.
//
// The cost of the choice is that the fingerprint must cover every input ComputeDesiredAt reads.
// fingerprintChannel below is that list, and it is the one thing to update when the scheduler
// grows a new input — see the note there.
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
// change. It used to be quantised to the wall-clock minute, which made nearly every request a new
// key: a navigated window always missed, and so did any visit more than a minute after the last.
//
// But `at` reaches the arrangement by exactly three routes — the active rule, the active holidays,
// and the rolling-window index — so the key carries THOSE, not the time. schedule.ClockSignature
// covers the first two; the rolling-window index is added per lookup from the window length the
// cache learned when it stored the entry (see cycleCache). Two windows inside one rolling window,
// under the same rule and holiday state, share one arrangement however far apart they are.

// cycleCacheTTL bounds how long an arranged cycle may be reused.
//
// The fingerprint covers channel STATE and the clock signature covers the CLOCK, so this bounds
// only what neither can see: engine-level availability (e.avail moves as titles land) and the
// hot-applied break-density / default-window settings. Those move slowly, and a forecast that is
// a few minutes behind them is the same class of staleness BroadcastsBetween already documents;
// the window containing NOW is served from the persisted Desired cycle, not from here.
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

// windowKey folds the rolling-window index of `at` into the base key.
func windowKey(base uint64, at time.Time, window time.Duration) uint64 {
	h := fnv.New64a()
	var num [8]byte
	binary.LittleEndian.PutUint64(num[:], base)
	_, _ = h.Write(num[:])
	binary.LittleEndian.PutUint64(num[:], uint64(schedule.WindowIndex(at, window)))
	_, _ = h.Write(num[:])
	return h.Sum64()
}

// get returns a live entry for this input set at `at`, if one exists. A base key never stored has
// no known window length, so it simply misses.
func (c *cycleCache) get(base uint64, at time.Time) ([]schedule.Slot, time.Duration, bool) {
	if c == nil {
		return nil, 0, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	note, ok := c.windows[base]
	if !ok || c.now().Sub(note.stored) > cycleCacheTTL {
		return nil, 0, false
	}
	e, ok := c.entries[windowKey(base, at, note.window)]
	if !ok || c.now().Sub(e.stored) > cycleCacheTTL {
		return nil, 0, false
	}
	return e.slots, e.window, true
}

// put stores an arrangement, and prunes anything that has aged out.
//
// Pruning on WRITE rather than on a timer keeps this free of a background goroutine: the cache
// is only touched by requests, so a cache nobody reads costs nothing rather than ticking.
func (c *cycleCache) put(base uint64, at time.Time, slots []schedule.Slot, window time.Duration) {
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
	c.entries[windowKey(base, at, window)] = cycleEntry{slots: slots, window: window, stored: now}
}

// fingerprintChannel hashes everything ComputeDesiredAt's answer depends on, other than the
// rolling-window index (the cache adds that per lookup).
//
// ⚠ THIS FUNCTION IS THE CACHE'S CORRECTNESS. An input the scheduler reads but this does not
// hash is an input whose change cannot evict the entry — the arrangement would silently keep the
// old answer. When the scheduler grows a new input, it belongs here in the same PR.
//
// It hashes the lineup and policy WHOLE, via their JSON encoding, rather than enumerating
// fields. That is a deliberate defence against exactly the rot above. Both structs have grown
// repeatedly and are far wider than the fields an arrangement obviously depends on:
// LineupEntry alone carries OfficialRating, Genres, Year, RuntimeSec and CollectionID, each
// feeding enforcement or ordering, and CollectionID in particular is HEALED by a later reconcile
// — a field-by-field hash that missed it would pin the channel to its pre-heal arrangement.
// A whole-struct hash rots CLOSED: a new field changes the hash and costs a cache miss, where a
// hand-maintained field list rots OPEN and costs a wrong lineup.
//
// JSON specifically because policy_json is how the store already round-trips ChannelPolicy, so
// the encoding is faithful to what persistence considers meaningful. Encoding errors fall back
// to a zero fingerprint, which forces a miss — the safe direction.
//
// # What the fingerprint deliberately does NOT cover
//
// Three inputs live inside channels.Engine and are invisible from here: e.avail (availability,
// which moves as titles land), the hot-applied e.breaksPerHour / e.defaultWindow settings, and
// hasFillerPool (a live e.pods.HasPool probe — note the engine OVERWRITES the channel's stored
// BreaksPerHour from it, so hashing the stored field would be worse than useless: it would look
// like coverage while tracking a value the arrangement ignores).
//
// Reaching into the engine to recompute them here would duplicate CyclePreviewDraft's assembly
// and give the system two answers to one question — the §10 shared-assembler mistake in a new
// place. Instead they are covered by cycleCacheTTL: all three are slow-moving (an operator
// changing a setting, a filler pool emptying, an acquisition completing), so bounding their
// staleness is a deliberate trade, not an oversight. A change to any of them shows up within a
// TTL rather than instantly.
//
// `clock` is schedule.ClockSignature(policy, at): the rule and holiday state at `at`. It is
// folded in for EVERY channel — for one whose policy cannot vary with time (seasonality off, no
// rules) it is constant, so that channel keeps one entry across every instant.
func fingerprintChannel(
	channelID string,
	lineup []schedule.LineupEntry,
	policy schedule.ChannelPolicy,
	clock uint64,
) (uint64, bool) {
	h := fnv.New64a()
	_, _ = h.Write([]byte(channelID))

	// Lineup and policy, whole and in order — entry order is itself an input to the arrangement,
	// and json.Marshal preserves slice order.
	lineupJSON, err := json.Marshal(lineup)
	if err != nil {
		return 0, false
	}
	_, _ = h.Write(lineupJSON)

	policyJSON, err := json.Marshal(policy)
	if err != nil {
		return 0, false
	}
	_, _ = h.Write(policyJSON)

	var num [8]byte
	binary.LittleEndian.PutUint64(num[:], clock)
	_, _ = h.Write(num[:])
	return h.Sum64(), true
}
