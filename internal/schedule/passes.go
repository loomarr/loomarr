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
//     Every pool wider than a window and large enough to hold three spacings is here. A pool that
//     fits one window never is: it loops inside the window, so the tail the next seam reads can be
//     any part of the deck, repaired or not.
//   - Chained: a pool of one to six windows, whose spacing is raised to two windows and a unit (see
//     repeatSpacing), pushes units into the tail, so each seam needs the repaired deck before it and
//     the walk runs from an anchor (dealChain). It is bounded by chainBudget.
//   - Pinned: a pool whose slack (its runtime beyond the spacing) is shorter than its shortest unit
//     can only hold every unit back to its old place, so every pass replays pass 0, exactly.
//
// chainBudget is the most slot-deals (passes x pool size) one walk's segment may cost. At ~100 us a
// slot-deal (BenchmarkDealChain: a 6-year-old channel with a 60-episode pool walks in ~0.25 s) that
// is the worst a homelab box pays, cold, once per process and pool. A walk's anchor is the latest
// multiple of chainBound(n) passes at or before the wanted pass, so the answer stays a pure function
// of (inputs, pass) however old the channel is. The anchor's predecessor is the last pass of the
// segment before, so the walk deals warmUpPasses passes ahead of the anchor to reach it: a walk
// from any start forgets it within that, so the anchor is repaired against the deck that aired
// (#1965; it used to be repaired against its predecessor as dealt, and the window crossing it
// re-aired what closed the window before, an episode back to back on 80 a day).
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
	pinned    bool          // every pass replays pass 0: see the regimes above
	bound     int64         // passes between anchors of a chained walk
	warmUp    int64         // passes a chained walk deals before its anchor (see warmUpPasses)
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
	shortest := int64(-1)
	for _, s := range slots {
		if s.IsProgram() {
			programs++
			runtime += s.DurationMs
			longest = max(longest, s.DurationMs)
			if shortest < 0 || s.DurationMs < shortest {
				shortest = s.DurationMs
			}
		}
	}
	d.whole = window > 0 && runtime > 0 && runtime <= window.Milliseconds()
	d.spacing = d.repeatSpacing()
	d.stable = !d.whole && 3*d.spacing.Milliseconds()+2*longest <= runtime
	d.pinned = !d.whole && runtime-d.spacing.Milliseconds() < shortest
	d.bound = chainBound(programs)
	d.warmUp = d.warmUpPasses(runtime)
	d.chain = chainFor(d)
	return d
}

// warmUpPasses is how many passes before its anchor a chained walk starts dealing, so that the
// anchor is repaired against the deck that really aired before it (#1965).
//
// A walk started from a pass as dealt forgets that start: a unit is held back to where it started
// in the pass before less the slack (the seam's runtime beyond the spacing), so a unit the start
// placed differently from the walk before it airs about one slack earlier each pass, until it
// leaves the tail the next seam reads and both walks deal the same deck from there on. That takes
// about spacing/slack passes: measured 2 to 13 on the pools the seam tests use (BenchmarkDealChain's
// shapes, 80 episodes in 6 to 24 h windows, 27 films a day), and 67 on a pool within half an hour
// of pinned (68 22-minute episodes a day). The warm-up is twice that and six passes more, and
// TestDealChain_AnchorSeamIsExact checks it reaches the deck the walk from pass 0 deals. A cold
// walk therefore deals up to chainBound plus this many passes. A pool with no slack worth a unit
// is pinned instead, and never walked.
func (d *deckDealer) warmUpPasses(runtime int64) int64 {
	seam := runtime
	if d.whole {
		seam = d.window.Milliseconds() // the deck loops until the window closes (see repair)
	}
	spacing := d.spacing.Milliseconds()
	slack := seam - spacing
	if d.pinned || slack <= 0 {
		return 0
	}
	return min(2*((spacing+slack-1)/slack)+6, maxChainBound)
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
	case d.pinned:
		return d.base(0).deck
	}
	return d.chain.deckAt(d, pass)
}

// repeatSpacing is how long a unit must rest between two airings across a pass boundary: the
// episode spacing for a pool of episodes, the film spacing for a pool of films, the longer of the
// two for a mix. Bounds apply, the floors winning over the cap, and none exceeding the pool:
//
//   - A third of the pool's runtime at most, less the slack (two longest units) that keeps the
//     repair out of the tail the next seam reads (the tail-stable regime above). A pool shorter
//     than the spacing can never honour it (the relaxation ladder, §7, owns that shortfall), and a
//     spacing near the pool's runtime would pin every unit to its old place, a replay not a reshuffle.
//   - Two longest units at least, so a unit never airs twice in a row across a seam: a pool that
//     fits one window loops inside it, and the cap above leaves a pool of five or fewer nothing.
//   - For a pool wider than a window, two windows and a unit as far as the pool allows (it must keep
//     one window free), and never less than a window and a unit. A window airs a stride of
//     programmes, its last one running past the close, so two consecutive windows span up to two
//     windows and a unit: two windows flat let a unit open one window and close the next (#1676,
//     a 24 h window of 22-minute episodes). Windows do not start where passes do,
//     so a unit airing near the end of one pass can land in the very next window of the following
//     one (#1694), or, rested less than a window, in the same window twice; the window would skip
//     it, read past its stride, and the next window would air those units again. This floor is
//     what takes a pool of one to six windows out of the tail-stable regime.
func (d *deckDealer) repeatSpacing() time.Duration {
	var episodes, films bool
	var longest int64
	var programs int
	for _, s := range d.slots {
		if !s.IsProgram() {
			continue
		}
		programs++
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
	unit := time.Duration(longest) * time.Millisecond
	spacing = min(spacing, (pool-2*unit)/3)
	var floor time.Duration
	if programs >= 2 {
		floor = 2 * unit
	}
	if !d.whole {
		floor = max(floor, min(2*d.window+unit, pool-d.window), d.window+unit)
	}
	return max(spacing, min(floor, pool))
}

// repair is repairSeam against what really aired before `next`. A pool wider than a window airs
// its decks back to back, so that is `prev` itself. A pool that fits one window loops its deck
// until the window closes, `elapsed % total` as playout does, so the units that closed the window
// are wherever the loop stopped: a deck repaired against its own end would let the unit that
// really closed the window open the next one.
func (d *deckDealer) repair(prev, next []Slot) []Slot {
	if d.whole {
		prev = loopedWindow(prev, d.window.Milliseconds())
	}
	return repairSeam(prev, next, d.spacing)
}

// loopedWindow is `deck`'s programs looped until `window` ms have aired, the last one cut at the
// close, so its runtime is the window's.
func loopedWindow(deck []Slot, window int64) []Slot {
	var out []Slot
	for at := int64(0); at < window; {
		before := at
		for _, s := range deck {
			if !s.IsProgram() || s.DurationMs <= 0 || at >= window {
				continue
			}
			s.DurationMs = min(s.DurationMs, window-at)
			out = append(out, s)
			at += s.DurationMs
		}
		if at == before {
			break
		}
	}
	return out
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
	return holdBack(next, release)
}

// holdBack scans `next` in order and holds a unit back until the deck reaches its release time (ms
// into next, by library item id), placing it ahead of the next unit in line; every other unit keeps
// its place relative to the others. Leftovers go out earliest-release first.
func holdBack(next []Slot, release map[string]int64) []Slot {
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
	key := dealerKey(d)

	chainsMu.Lock()
	defer chainsMu.Unlock()
	if c, ok := chains[key]; ok {
		if i := slices.Index(chainLRU, key); i >= 0 { // a hit is a use: it goes to the back
			chainLRU = append(slices.Delete(chainLRU, i, i+1), key)
		}
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

// dealerKey hashes everything a deal reads but the airing history: the seed, the window, the policy
// and the slots. The deal chain and the season simulation (seasonal_boost.go) share pools by it.
func dealerKey(d *deckDealer) uint64 {
	h := fnv.New64a()
	rp := d.rp
	rp.LastAired = nil
	_, _ = fmt.Fprintf(h, "%d|%d|%+v|", d.seed, d.window, rp)
	for _, s := range d.slots {
		_, _ = fmt.Fprintf(h, "%v\x00", s)
	}
	return h.Sum64()
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
		deck := d.repair(d.base(pass-1).deck, d.base(pass).deck)
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
		switch anchor {
		case 0:
			cur = d.base(0).deck
		default:
			// The segment's first pass: its predecessor is the last of a different segment, so walk up
			// to it from a warm-up start (warmUpPasses says why that reaches the deck that aired).
			cur = d.repair(d.warmedUp(anchor), d.deal(anchor).deck)
		}
		c.decks[anchor] = cur
	}

	for p := at + 1; p <= pass; p++ {
		cur = d.repair(cur, d.deal(p).deck)
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

// warmedUp is the deck of the pass before `anchor`, walked from warmUp passes before the anchor as
// dealt, or from pass 0 when that is nearer: the deck the walk before the anchor reached.
func (d *deckDealer) warmedUp(anchor int64) []Slot {
	from := anchor - max(d.warmUp, 1)
	var cur []Slot
	if from <= 0 {
		from, cur = 0, d.base(0).deck
	} else {
		cur = d.deal(from).deck
	}
	for p := from + 1; p < anchor; p++ {
		cur = d.repair(cur, d.deal(p).deck)
	}
	return cur
}

// windowSliceDealt keeps a ROTATING ~window-of-runtime slice of a ring of dealt passes, advancing
// its start by the window index and wrapping the catalog (§6.5), so over a full cycle every
// program airs rather than the head of the deck every window. The window starts at program
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

// dealtRing is the channel's ring of dealt passes in absolute window coordinates, so the holiday
// planner (§6) walks in season exactly the ring windowSliceDealt tiles out of season: window w
// starts at programme w*stride, counted from the channel's DealOrigin window. Without it the
// planner walked one fixed deck while the windows around the season aired dealt passes, and the
// season's seams re-aired the window before.
type dealtRing struct {
	d      *deckDealer
	window time.Duration
	origin int64 // window index of the channel's DealOrigin
	stride int64 // programmes per window, as windowSliceDealt counts them
	nProg  int64
	progs  map[int64][]Slot           // pass → its deck's programmes, in order
	index  map[int64]map[string]int64 // pass → unit → its programme in progs
}

func newDealtRing(d *deckDealer, window time.Duration, origin int64) *dealtRing {
	r := &dealtRing{d: d, window: window, origin: origin, progs: map[int64][]Slot{}, index: map[int64]map[string]int64{}}
	first := d.deck(0)
	for _, s := range first {
		if s.IsProgram() {
			r.nProg++
		}
	}
	if window > 0 {
		r.stride = int64(max(programsPerWindow(first, window.Milliseconds()), 1))
	}
	return r
}

// tile is window w's out-of-season tile.
func (r *dealtRing) tile(w int64) []Slot {
	t, _ := windowSliceDealt(r.d, r.window, w-r.origin)
	return t
}

// at is programme u of the ring, where window w starts at u = w*stride.
func (r *dealtRing) at(u int64) Slot {
	pass, k := r.locate(u)
	return r.pass(pass)[k]
}

// airings are the ring positions where the unit keyed key airs in the pass holding u and the
// passes either side: under per-pass reshuffle a unit sits anywhere in each pass, so this, not an
// offset along one deck, is how far an airing near u is from its others.
func (r *dealtRing) airings(key string, u int64) []int64 {
	at, _ := r.locate(u)
	var out []int64
	for pass := at - 1; pass <= at+1; pass++ {
		r.pass(pass)
		if k, ok := r.index[r.dealt(pass)][key]; ok {
			out = append(out, r.origin*r.stride+pass*r.nProg+k)
		}
	}
	return out
}

// locate splits ring position u into its pass and its programme within that pass.
func (r *dealtRing) locate(u int64) (pass, k int64) {
	rel := u - r.origin*r.stride
	pass, k = rel/r.nProg, rel%r.nProg
	if k < 0 {
		pass, k = pass-1, k+r.nProg
	}
	return pass, k
}

// dealt is the deck pass airs: its own deal under reshuffle, pass 0's otherwise.
func (r *dealtRing) dealt(pass int64) int64 {
	if !r.d.reshuffle {
		return 0
	}
	return pass
}

// pass is pass's programmes in order, indexed by unit (a multi-part unit by its first part).
func (r *dealtRing) pass(pass int64) []Slot {
	pass = r.dealt(pass)
	progs, ok := r.progs[pass]
	if !ok {
		// Held here: the chain keeps only checkpoints, so asking it again for an earlier pass walks.
		index := map[string]int64{}
		for _, s := range r.d.deck(pass) {
			if s.IsProgram() {
				if _, seen := index[unitKey(s)]; !seen {
					index[unitKey(s)] = int64(len(progs))
				}
				progs = append(progs, s)
			}
		}
		r.progs[pass], r.index[pass] = progs, index
	}
	return progs
}
