package app

import (
	"encoding/binary"
	"encoding/json"
	"hash/fnv"
	"sync"
	"time"

	"github.com/mantonx/loomarr/internal/schedule"
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
// The cost of the choice is that the fingerprint must cover every inpu ComputeDesiredAt reads.
// fingerprintChannel below is that list, and it is the one thing to update when the scheduler
// grows a new input — see the note there.
//
// # Why here and not in channels.Engine
//
// Engine.CyclePreview is shared with playoutResolver.AiringNow, which is what ffmpeg streams
// (§9.1's one-source rule: the guide and the encoder must agree). Caching inside the Engine
// would put this on the live broadcast path, where a stale entry means a viewer watching the
// wrong programme. Sitting in the resolver's GUIDE methods keeps AiringNow on the live
// computation: a bug here degrades a grid, never a broadcast.
//
// Draft previews are never cached — CyclePreviewDraft with a non-nil draft is an unsaved
// what-if whose whole purpose is to reflect the edit in hand.

// cycleCacheTTL bounds how long an arranged cycle may be reused.
//
// The fingerprint already covers channel STATE, so this bounds only the inputs a fingerprint
// cannot see: engine-level availability (e.avail moves as titles land) and wall-clock drift
// within a bucket. Sixty seconds is comfortably shorter than the guide's own thirty-second
// re-render feeling stale, and short enough that a title landing shows up promptly.
const cycleCacheTTL = 60 * time.Second

// cycleBucket is how coarsely `at` is quantised into the key.
//
// `at` reaches ComputeDesiredAt, ActiveRuleAt and ResolveWindow, so a curation rule that
// switches the channel at 21:00 genuinely changes the answer. Quantising means two requests a
// few seconds apart share an entry instead of each paying a full arrangement — the guide's
// window start moves with every poll, so an exact-instant key would never hit.
//
// One minute is deliberately finer than any rule boundary the rule engine can express (rules
// switch on the hour, not the second), so a bucket cannot straddle one by more than the bucket
// itself. That bounded skew is the SAME limitation BroadcastsBetween already documents for a
// window spanning a rule boundary — this does not introduce a new class of staleness.
const cycleBucket = time.Minute

// timeVarying reports whether this channel's arrangement can change with `at` alone — with no
// edit, no acquisition, nothing but the clock moving.
//
// # Why this exists
//
// Bucketing `at` into the key is what makes the cache correct for a channel whose lineup really
// does change at 21:00. It is also what makes the cache MISS every sixty seconds for a channel
// whose lineup does not: the guide's window start advances with the wall clock, so a new bucket
// arrives every minute and re-pays a full arrangement for a byte-identical answer. Measured on
// the dev install, arrangements were identical across +0/+6h/+1d/+3d on every channel, while the
// endpoint's p99 sat at 90ms against a p50 of 21ms — that spread was this cache missing on the
// bucket, not real work.
//
// So the bucket is now CONDITIONAL: a channel that cannot vary with time keys on its inputs
// alone and stays cached until something actually edits it.
//
// # The predicate, and why it is deliberately pessimistic
//
// `at` reaches the arrangement by exactly two routes:
//
//   - pickRule (via ComputeDesiredAt, ActiveRuleAt and ResolveWindow) — which can only select a
//     different rule if there ARE rules. Over an empty slice it returns the same "no match" for
//     every instant.
//   - applySeasonal — which returns entries untouched only when the RESOLVED mode is SeasonalOff.
//
// ⚠ The seasonal half is the trap. The resolved default is SeasonalAuto, not Off (policy.go
// defaultSeasonalMode), and activeHolidays walks the whole built-in calendar when no holidays are
// explicitly selected — so a channel with an entirely EMPTY seasonal policy is still time-varying,
// and will bench or unbench items as a holiday window opens. A predicate that read "no holidays
// configured ⇒ invariant" would look right, pass every test written against a rule-less channel,
// and silently serve a Christmas lineup in January.
//
// Hence: time-varying UNLESS seasonality is explicitly switched off AND there are no rules. False
// negatives here cost a cache miss; false positives cost a wrong lineup. The asymmetry decides
// the default.
func timeVarying(policy schedule.ChannelPolicy) bool {
	if len(policy.Rules) > 0 {
		return true
	}
	// Only an EXPLICIT off is safe to treat as time-invariant — an unset mode resolves to
	// SeasonalAuto, which is time-varying.
	return policy.Seasonal.Mode != schedule.SeasonalOff
}

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

// cycleCache memoises arranged cycles for the guide's read path.
//
// Bounded by channel count (one live entry per channel per bucket, pruned on write), so it
// cannot grow with request volume the way a per-window key would.
type cycleCache struct {
	mu      sync.Mutex
	entries map[uint64]cycleEntry
	now     func() time.Time
}

func newCycleCache(now func() time.Time) *cycleCache {
	if now == nil {
		now = time.Now
	}
	return &cycleCache{entries: map[uint64]cycleEntry{}, now: now}
}

// get returns a live entry for this key, if one exists.
func (c *cycleCache) get(key uint64) ([]schedule.Slot, time.Duration, bool) {
	if c == nil {
		return nil, 0, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || c.now().Sub(e.stored) > cycleCacheTTL {
		return nil, 0, false
	}
	return e.slots, e.window, true
}

// put stores an arrangement, and prunes anything that has aged out.
//
// Pruning on WRITE rather than on a timer keeps this free of a background goroutine: the cache
// is only touched by requests, so a cache nobody reads costs nothing rather than ticking.
func (c *cycleCache) put(key uint64, slots []schedule.Slot, window time.Duration) {
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
	c.entries[key] = cycleEntry{slots: slots, window: window, stored: now}
}

// fingerprintChannel hashes everything ComputeDesiredAt's answer depends on.
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
// staleness at one minute is a deliberate trade, not an oversight. A change to any of them shows
// up within a TTL rather than instantly.
// The `bucket` argument is the caller's quantised `at`, and is folded in ONLY for a channel
// whose arrangement can actually vary with time (see timeVarying). For an invariant channel it
// is deliberately ignored, so every instant shares one entry and the cache survives until the
// lineup or policy changes — which is the whole point of fingerprinting the inputs.
func fingerprintChannel(
	channelID string,
	lineup []schedule.LineupEntry,
	policy schedule.ChannelPolicy,
	bucket int64,
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
	writeNum := func(v int64) {
		binary.LittleEndian.PutUint64(num[:], uint64(v))
		_, _ = h.Write(num[:])
	}
	if timeVarying(policy) {
		writeNum(bucket)
	}
	return h.Sum64(), true
}
