package schedule

import (
	"fmt"
	"hash/fnv"
	"slices"
	"sort"
	"sync"
	"time"
)

// This file deals a channel's shuffled deck one PASS at a time (#1676, programming-design §5).
//
// A pass is one trip of the rolling window around the catalog: window w starts at program
// w*stride of the ring, so the ring wraps every nProg/stride windows. Until #1676 the ring was a
// single deck, so every pass replayed the last one pair for pair. Now each pass is dealt under
// `seed XOR pass`, so successive passes differ and each is still reproducible.
//
// Passes are counted from the channel's DealOrigin, not from the Unix epoch. That makes the count
// small (a channel's age in passes, not thousands), which is what lets every seam be repaired
// against the deck that really aired before it: see dealChain.

// A deal is a full constraint-aware arrangement (backtrackArrange): measured 5-22 ms for a pool of
// 36-500 episodes across several series. That cost, not the repair, is what bounds how many passes
// may be dealt to answer one question, so the bounds below are in deals.
//
// There are two regimes.
//
//   - Tail-stable (tailStable): the repair moves a unit at most 2*spacing + 2*longestUnit into the
//     deck, so when that is short of the tail the next seam reads, the repaired tail IS the dealt
//     tail, and pass p needs only the deals of p-1 and p. Exact, and the same cost at any age.
//     Every pool large enough to hold three spacings is here, and so is a pool that fits one window.
//   - Chained: a pool of one to six windows, whose spacing is raised to two windows (see
//     repeatSpacing), pushes units into the tail, so each seam needs the repaired deck before it and
//     the walk runs from an anchor (dealChain). It is bounded by chainBudget.
//
// chainBudget is the most slot-deals (passes x pool size) one walk may cost. At ~100 us a slot-deal
// (BenchmarkDealChain: a 6-year-old channel with a 60-episode pool walks in ~0.2 s) that is the
// worst a homelab box pays, cold, once per process and pool. A walk starts at the latest multiple of
// chainBound(n) passes at or before the wanted pass, so the answer stays a pure function of
// (inputs, pass) however old the channel is. This is the ONLY place the seam guarantee relaxes: the
// first pass after an anchor is repaired against its predecessor as dealt rather than as aired,
// which can miss a unit the earlier repairs displaced. A pool of 27 films meets it every 64 passes,
// and a pool in the tail-stable regime never does.
const (
	chainBudget = 2048
	// checkpointEvery is how often a walk keeps the deck it reached, so asking for an earlier pass
	// (the guide looks backwards as well as forwards) resumes from a nearby checkpoint, not the anchor.
	// chainBound is always a multiple of it.
	checkpointEvery = 16
	maxChainBound   = 4096
)

// chainBound is the number of passes between anchors for a pool of n programmes.
func chainBound(n int) int64 {
	return min(max(int64(chainBudget/max(n, 1))/checkpointEvery*checkpointEvery, checkpointEvery), maxChainBound)
}

// maxChains bounds the process-wide memo of chains: one per distinct (pool, policy, seed, window).
const maxChains = 64

// ResolveDealOrigin is the instant a channel's passes are counted from: the channel's creation time,
// else its playout anchor, else zero, which ComputeDesiredAt turns into a value derived from the
// channel id. All three are stable for the life of the channel, so a pass keeps its number.
func ResolveDealOrigin(createdAt, playoutAnchor time.Time) time.Time {
	if !createdAt.IsZero() {
		return createdAt
	}
	return playoutAnchor
}

// dealOriginOf is the channel's DealOrigin, or a deterministic stand-in when none was resolved: a
// day of 2025 chosen by the channel id, so two such channels do not count passes in step. It is
// placed before Loomarr had any channel, so no real clock reads a pass before it (a pass before the
// origin is dealt but not repaired, since nothing had aired).
func dealOriginOf(ch Channel) time.Time {
	if !ch.DealOrigin.IsZero() {
		return ch.DealOrigin
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(ch.ID))
	return time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, int(h.Sum64()%365))
}

// channelSeed is the seed a channel shuffles with. A channel made through the binder stores 0, and
// every such channel shuffled the same pool into the same order. 0 therefore means "derive it from
// the channel id": stable for the life of the channel, different between channels, and needing no
// stored value, so a channel that already exists is fixed without a data migration.
func channelSeed(ch Channel) int64 {
	if ch.Shuffle.Seed != 0 {
		return ch.Shuffle.Seed
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(ch.ID))
	if s := int64(h.Sum64()); s != 0 {
		return s
	}
	return 1
}

// passSeed is `seed XOR pass`, with the pass spread by the golden-ratio constant so neighbouring
// passes do not seed neighbouring generator states. Pass 0 is the channel seed itself.
func passSeed(seed, pass int64) int64 {
	return seed ^ int64(uint64(pass)*0x9E3779B97F4A7C15)
}

// deckDealer deals the ordered deck for any pass of one channel at one moment.
type deckDealer struct {
	slots     []Slot
	rp        ResolvedPolicy
	seed      int64
	window    time.Duration
	spacing   time.Duration // how long a unit rests before airing again (see repeatSpacing)
	reshuffle bool          // false: every pass is the pass-0 deck (sequential keeps its order)
	whole     bool          // the whole catalog fits one window, so a window is a pass (see deck)
	stable    bool          // tail-stable: see the regimes above
	bound     int64         // passes between anchors of a chained walk
	chain     *dealChain    // shared across calls; nil unless reshuffling
	bases     map[int64]dealt
}

type dealt struct {
	deck    []Slot
	applied []AppliedRelaxation
}

func newDeckDealer(slots []Slot, rp ResolvedPolicy, seed int64, window time.Duration) *deckDealer {
	d := &deckDealer{
		slots: slots, rp: rp, seed: seed, window: window,
		reshuffle: rp.Ordering == OrderShuffle || rp.Ordering == OrderSyndication,
		bases:     map[int64]dealt{},
	}
	if !d.reshuffle {
		return d
	}
	var programs int
	var runtime, longest int64
	for _, s := range slots {
		if s.IsProgram() {
			programs++
			runtime += s.DurationMs
			longest = max(longest, s.DurationMs)
		}
	}
	d.whole = window > 0 && runtime > 0 && runtime <= window.Milliseconds()
	d.spacing = d.repeatSpacing()
	d.stable = 3*d.spacing.Milliseconds()+2*longest <= runtime
	d.bound = chainBound(programs)
	d.chain = chainFor(d)
	return d
}

// deal is the seeded, unrepaired deal of one pass.
func (d *deckDealer) deal(pass int64) dealt {
	deck, applied := slotWithRelaxation(d.slots, d.rp, passSeed(d.seed, pass))
	return dealt{deck, applied}
}

func (d *deckDealer) base(pass int64) dealt {
	if !d.reshuffle {
		pass = 0
	}
	if b, ok := d.bases[pass]; ok {
		return b
	}
	b := d.deal(pass)
	d.bases[pass] = b
	return b
}

// applied is the relaxation record for the pass's deck.
func (d *deckDealer) applied(pass int64) []AppliedRelaxation { return d.base(pass).applied }

// deck is the pass's deck as it airs: the seeded deal, repaired so nothing the previous pass ended
// on airs again inside the repeat spacing. The slice is shared with the chain memo: read it, never
// modify it.
//
// A catalog that fits inside one window has no ring to wrap: it airs whole in every window, and
// before #1676 aired the same deck in the same order every day. There a window is the pass, so each
// window deals a fresh permutation, repaired against the previous window's like any other seam.
func (d *deckDealer) deck(pass int64) []Slot {
	switch {
	case !d.reshuffle:
		return d.base(0).deck
	case pass <= 0: // pass 0 has no predecessor, and before the channel existed nothing aired
		return d.base(pass).deck
	}
	return d.chain.deckAt(d, pass)
}

// repeatSpacing is how long a unit must rest between two airings across a pass boundary: the
// episode spacing for a pool of episodes, the film spacing for a pool of films, the longer of the
// two for a mix. Two bounds apply:
//
//   - A third of the pool's runtime at most, less the slack (two longest units) that keeps the
//     repair out of the tail the next seam reads (the tail-stable regime above). A pool shorter
//     than the spacing can never honour it (the relaxation ladder, §7, owns that shortfall), and a
//     spacing near the pool's runtime would pin every unit to its old place, a replay not a reshuffle.
//   - Two windows at least, as far as the pool allows (it must keep one window free). Windows do
//     not start where passes do, so a unit airing near the end of one pass can land in the very
//     next window of the following one unless it rests for two windows (#1694). This floor is what
//     takes a pool of one to six windows out of the tail-stable regime.
func (d *deckDealer) repeatSpacing() time.Duration {
	var episodes, films bool
	var longest int64
	for _, s := range d.slots {
		if !s.IsProgram() {
			continue
		}
		longest = max(longest, s.DurationMs)
		if s.Season > 0 || s.SeriesTitle != "" {
			episodes = true
		} else {
			films = true
		}
	}
	var spacing time.Duration
	if episodes {
		spacing = d.rp.Sep.EpisodeNoRepeat
	}
	if films && d.rp.Sep.MovieNoRepeat > spacing {
		spacing = d.rp.Sep.MovieNoRepeat
	}
	pool := time.Duration(programRuntime(d.slots)) * time.Millisecond
	spacing = min(spacing, (pool-2*time.Duration(longest)*time.Millisecond)/3)
	return max(spacing, min(2*d.window, pool-d.window))
}

// repairSeam reorders `next` so that every unit airs at least `spacing` after it aired in `prev`.
// A unit that started `s` into prev (of a pass `total` long) may start no earlier than
// s+spacing-total into next: that is its release time, and only units from the end of prev have a
// positive one. `next` is scanned in order; a unit not yet released is held back, and goes out as
// soon as the deck reaches its release time, ahead of the next unit in line. Every other unit
// keeps the place the shuffle gave it, relative to the others.
//
// `prev` must be the deck as it aired, not as it was dealt: a held-back unit can end up anywhere
// up to `spacing` into the deck, so on a pool only a few windows wide it lands in the tail the
// following pass measures its own seam against. That dependence is why decks are walked pass by
// pass (dealChain) rather than repaired independently.
//
// Holding back never idles the deck while the spacing fits the pool (units released by position y
// number at least y+1), so every unit is placed at or after its release time. A spacing the pool
// cannot honour is not this function's to fix: the leftovers go out earliest-release first.
func repairSeam(prev, next []Slot, spacing time.Duration) []Slot {
	if spacing <= 0 || len(prev) == 0 {
		return next
	}
	limit := spacing.Milliseconds()

	var total int64
	for _, s := range prev {
		if s.IsProgram() {
			total += s.DurationMs
		}
	}
	release := map[string]int64{} // library item id → earliest start in next, for units that have one
	var started int64
	for _, s := range prev {
		if !s.IsProgram() {
			continue
		}
		if r := started + limit - total; r > 0 {
			release[s.LibraryItemID] = r
		}
		started += s.DurationMs
	}
	if len(release) == 0 {
		return next
	}

	type held struct {
		slot Slot
		at   int64
	}
	var queue []held // held-back units, earliest release first (ties: scan order)
	out := make([]Slot, 0, len(next))
	var at int64
	place := func(s Slot) {
		out = append(out, s)
		if s.IsProgram() {
			at += s.DurationMs
		}
	}
	for i := 0; i < len(next) || len(queue) > 0; {
		switch {
		case len(queue) > 0 && (queue[0].at <= at || i == len(next)):
			place(queue[0].slot)
			queue = queue[1:]
		default:
			s := next[i]
			i++
			if r, ok := release[s.LibraryItemID]; ok && s.IsProgram() && r > at {
				pos := sort.Search(len(queue), func(k int) bool { return queue[k].at > r })
				queue = slices.Insert(queue, pos, held{s, r})
				continue
			}
			place(s)
		}
	}
	return out
}

// dealChain is the memo of one pool's repaired decks, shared by every call that deals that pool
// (a reconcile, the guide, a preview). A pass's repaired deck needs the previous pass's repaired
// deck, so a cold walk deals every pass from the walk's anchor; the memo makes that a once-per-
// process cost, and each later pass one deal more.
type dealChain struct {
	mu       sync.Mutex
	decks    map[int64][]Slot // checkpoints, plus the furthest pass walked
	furthest int64            // -1: nothing walked yet
}

var (
	chainsMu sync.Mutex
	chains   = map[uint64]*dealChain{}
	chainLRU []uint64 // oldest first
)

// chainFor returns the shared chain for the dealer's inputs. The key is a hash of everything a deal
// reads, so an edit to the lineup, the policy, the seed or the window lands on a different chain
// and the old one is simply unreachable. That restart IS the "an edited lineup deals afresh" rule.
// A pool carrying airing history (never the case while the ring rotates: ComputeDesiredAt clears
// it for the guarded slice) is not shared, since history is not part of the key.
func chainFor(d *deckDealer) *dealChain {
	if len(d.rp.LastAired) > 0 {
		return newDealChain()
	}
	h := fnv.New64a()
	rp := d.rp
	rp.LastAired = nil
	_, _ = fmt.Fprintf(h, "%d|%d|%+v|", d.seed, d.window, rp)
	for _, s := range d.slots {
		_, _ = fmt.Fprintf(h, "%v\x00", s)
	}
	key := h.Sum64()

	chainsMu.Lock()
	defer chainsMu.Unlock()
	if c, ok := chains[key]; ok {
		return c
	}
	if len(chainLRU) >= maxChains {
		delete(chains, chainLRU[0])
		chainLRU = chainLRU[1:]
	}
	c := newDealChain()
	chains[key] = c
	chainLRU = append(chainLRU, key)
	return c
}

func newDealChain() *dealChain { return &dealChain{decks: map[int64][]Slot{}, furthest: -1} }

// stableDecks bounds what a tail-stable pool remembers: the few passes a window straddles.
const stableDecks = 16

// deckAt is the repaired deck of `pass` (>= 1). A tail-stable pool is repaired against the deal
// before it, which is exact. Otherwise the walk resumes from the nearest deck already walked inside
// the pass's segment, else from the segment's anchor, and walks forward.
func (c *dealChain) deckAt(d *deckDealer, pass int64) []Slot {
	c.mu.Lock()
	defer c.mu.Unlock()
	if deck, ok := c.decks[pass]; ok {
		return deck
	}
	if d.stable {
		deck := repairSeam(d.base(pass-1).deck, d.base(pass).deck, d.spacing)
		if len(c.decks) >= stableDecks {
			clear(c.decks)
		}
		c.decks[pass] = deck
		return deck
	}

	anchor := pass - pass%d.bound
	at, cur := anchor, []Slot(nil)
	switch {
	case c.furthest >= anchor && c.furthest <= pass:
		at, cur = c.furthest, c.decks[c.furthest]
	default:
		for p := pass - pass%checkpointEvery; p >= anchor; p -= checkpointEvery {
			if deck, ok := c.decks[p]; ok {
				at, cur = p, deck
				break
			}
		}
	}
	if cur == nil {
		switch {
		case anchor == 0:
			cur = d.base(0).deck
		default:
			// The segment's first pass: its predecessor is a different segment, so repair against it as
			// dealt (the regimes' doc says why this is the one imperfect seam).
			cur = repairSeam(d.deal(anchor-1).deck, d.deal(anchor).deck, d.spacing)
		}
		c.decks[anchor] = cur
	}

	for p := at + 1; p <= pass; p++ {
		cur = repairSeam(cur, d.deal(p).deck, d.spacing)
		if p%checkpointEvery == 0 {
			c.decks[p] = cur
		}
	}
	// Only checkpoints and the head of the walk are kept; an earlier pass asked for out of order
	// leaves the head where it was.
	if pass > c.furthest {
		if c.furthest%checkpointEvery != 0 {
			delete(c.decks, c.furthest)
		}
		c.decks[pass], c.furthest = cur, pass
	}
	return cur
}

// windowSliceDealt is windowSlice over a ring of dealt passes: the window starts at program
// index*stride of the ring, the ring being pass after pass, and a slice that runs off the end of
// one pass carries on into the head of the next (skipping any unit it has already taken from the
// pass before, so a unit never airs twice in one window). It reports the pass the window starts in.
func windowSliceDealt(d *deckDealer, window time.Duration, index int64) ([]Slot, int64) {
	if d.whole {
		return d.deck(index), index // nothing to rotate: the window is the pass
	}
	first := d.deck(0)
	if window <= 0 || len(first) == 0 {
		return first, 0
	}
	nProg := 0
	var total int64
	for _, s := range first {
		if s.IsProgram() {
			nProg++
			total += s.DurationMs
		}
	}
	budget := window.Milliseconds()
	if nProg == 0 || total <= budget || total == 0 {
		return first, 0 // nothing to rotate, and nothing to deal a window at a time (see deck)
	}
	stride := max(programsPerWindow(first, budget), 1)

	ring, n := index*int64(stride), int64(nProg)
	pass, start := ring/n, ring%n
	if start < 0 {
		pass, start = pass-1, start+n
	}
	if !d.reshuffle {
		pass = 0
	}

	deck := d.deck(pass)
	pos := 0 // the slot holding the start-th program
	for seen := int64(0); pos < len(deck); pos++ {
		if deck[pos].IsProgram() {
			if seen == start {
				break
			}
			seen++
		}
	}

	out := make([]Slot, 0, len(deck))
	took := map[string]bool{}     // programs taken from the pass being walked
	tookPrev := map[string]bool{} // programs taken from earlier passes of this slice
	var acc int64
	for steps, walking := 0, pass; steps < 2*len(deck); steps++ {
		if pos == len(deck) {
			walking++
			deck, pos = d.deck(walking), 0
			for id := range took {
				tookPrev[id] = true
			}
			took = map[string]bool{}
		}
		s := deck[pos]
		pos++
		if s.IsProgram() {
			if tookPrev[s.LibraryItemID] {
				continue
			}
			took[s.LibraryItemID] = true
			acc += s.DurationMs
		}
		out = append(out, s)
		if s.IsProgram() && acc >= budget {
			break
		}
	}
	return out, pass
}
