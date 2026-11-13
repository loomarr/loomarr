package schedule

import (
	"cmp"
	"fmt"
	"hash/fnv"
	"maps"
	"math"
	"slices"
	"sync"
)

// handOff is window w's tile, in the order it airs, in the windows after the season simulation,
// walked from `from`, the season's last decided window. The dealer repaired each deal's seam
// against the deal before it, but the season aired its own tiles, so while any window the spacing
// reaches aired otherwise than the dealer alone would have aired it, a window's deal is arranged
// and then repaired against those windows as they really aired (#1939): each unit is held to the
// rest the dealer's arrangement gives it (owedRest), held back until it has rested
// (holdUntilRested), and swapped out where it cannot rest before the window closes (substitute).
//
// The repaired order is what airs, and arrange leaves it alone (handingOff): arranging it again let
// a swap bring a unit the repair held back into its window's evening, checked against the windows
// around as dealt, which here they are not (#2001). Once every window the spacing reaches has
// aired as the dealer alone would, the hand-off is done, and every window after it airs as the
// dealer arranges it. That can take longer than the horizon (up to 15 windows on a daily deck), so
// the hand-off runs until then, not for a fixed horizon (handOffReach bounds it).
//
// The repair reads back over every window arrange's checks reach, not only the window before: a
// 12-hour spacing on a 6-hour window reaches two windows back (#1960).
func (p *holidayPlanner) handOff(from, w int64, dealt []Slot) []Slot {
	if tile, ok := p.handOffRun(from)[w]; ok {
		return slices.Clone(tile) // the run is shared: never hand out its slices
	}
	return p.arrangeTile(dealt, w)
}

// handOffRun walks the hand-off from `from` and returns each window's tile until the dealer airs
// alone. Like the season simulation, one walk serves every window of it and is shared across calls
// (handOffRuns).
func (p *holidayPlanner) handOffRun(from int64) map[int64][]Slot {
	key, shared := p.runKey(from, from)
	if shared {
		if run, ok := handOffRuns.get(key); ok {
			return run
		}
	} else if run, ok := p.handOffs[from]; ok {
		return run
	}
	reach := p.spacingWindows()
	// The last `reach` windows as they aired and as the dealer alone airs them, oldest first, and
	// whether the two differ.
	var aired, dealt [][]Slot
	var offDeal []bool
	for v := from - reach + 1; v <= from; v++ {
		deal := p.ring.tile(v)
		a, d := p.arrange(p.tile(v, deal), v), p.arrangeTile(deal, v)
		aired, dealt, offDeal = append(aired, a), append(dealt, d), append(offDeal, !sameProgrammes(a, d))
	}
	looped := func(windows [][]Slot) []Slot {
		var out []Slot
		for _, w := range windows {
			out = append(out, loopedWindow(w, p.window.Milliseconds())...)
		}
		return out
	}
	run := map[int64][]Slot{}
	for v := from + 1; v <= from+p.handOffReach() && slices.Contains(offDeal, true); v++ {
		arranged := p.arrangeTile(p.ring.tile(v), v)
		prev, owed := looped(aired), p.owedRest(looped(dealt), arranged)
		tile := p.substitute(prev, p.holdUntilRested(prev, arranged, owed), v, owed)
		run[v] = tile
		aired, dealt, offDeal = append(aired[1:], tile), append(dealt[1:], arranged), append(offDeal[1:], !sameProgrammes(tile, arranged))
	}
	if shared {
		handOffRuns.put(key, run)
	} else {
		p.handOffs[from] = run
	}
	return run
}

// handOffRuns memoises hand-off walks as seasonRuns does season simulations.
var handOffRuns = &runMemo{runs: map[uint64]map[int64][]Slot{}}

// substitute is window v's tile with each unit that would still air sooner after its last airing
// in prev (the windows before, as they aired) than it is owed swapped, in its own slot, for a unit
// of the pass that has rested the spacing since its last airing (an off-season holiday episode is
// never swapped out). holdUntilRested can only hold a
// unit back, and one that cannot rest before the window closes goes out at its end anyway, while
// a window cannot simply skip it: one unit short, playout loops its head, which aired a window ago
// (#2001). The swap moves nothing else, so it is a skip, as at the dealer's seams. A unit that also
// rests the spacing before its next deal goes in first, the most rested such; else the one
// farthest from its next deal, which that window then holds back or skips in turn. With none, the
// unit airs where it is.
func (p *holidayPlanner) substitute(prev, tile []Slot, v int64, owed map[string]int64) []Slot {
	spacing := p.spacingProgrammes()
	airedAt := map[string]int64{}
	var n int64
	for _, s := range prev {
		if s.IsProgram() {
			airedAt[unitKey(s)], n = n, n+1
		}
	}
	out := slices.Clone(tile)
	in := map[string]bool{}
	for _, s := range out {
		in[unitKey(s)] = true
	}
	pass, _ := p.ring.locate(v * p.ring.stride)
	var i int64
	for k, s := range out {
		if !s.IsProgram() {
			continue
		}
		at := n + i
		i++
		// A holiday episode is never benched out of season (§6): held back, it still airs.
		last, ok := airedAt[unitKey(s)]
		if !ok || at-last >= owedOr(owed, unitKey(s), spacing) || slices.ContainsFunc(p.f.unitHolidays(s), p.f.selected) {
			airedAt[unitKey(s)] = at
			continue
		}
		// The slot's ring position, as dealt: arrange and the hold move a few units, so it only
		// ranks the candidates by how far their next deal is.
		u := v*p.ring.stride + i - 1
		var best Slot
		bestTier, bestDist := 2, int64(-1)
		for _, c := range p.ring.pass(pass) {
			key := unitKey(c)
			if in[key] || slices.ContainsFunc(p.f.unitHolidays(c), p.f.selected) {
				continue
			}
			past := int64(math.MaxInt64)
			if last, ok := airedAt[key]; ok {
				past = at - last
			}
			if past < spacing {
				continue
			}
			future := int64(math.MaxInt64)
			for _, q := range p.ring.airings(key, u+1) {
				if q > u {
					future = min(future, q-u)
				}
			}
			tier, dist := 1, future
			if future >= spacing {
				tier, dist = 0, min(past, future)
			}
			if tier < bestTier || tier == bestTier && dist > bestDist {
				best, bestTier, bestDist = c, tier, dist
			}
		}
		if bestTier == 2 {
			airedAt[unitKey(s)] = at
			continue
		}
		delete(in, unitKey(s))
		out[k], in[unitKey(best)], airedAt[unitKey(best)] = best, true, at
	}
	return out
}

// owedRest is the rest each unit of `arranged` is owed at its first airing in it: the spacing, or
// less where the dealer alone gives it less (arrange keeps a moved unit no closer than its deal,
// §6), measured from its last airing in dealtPrev, the windows before as the dealer alone airs
// them. Holding every unit to the full spacing instead made the hand-off move units the dealer's
// own arrangement rests less, so it never settled (#2001).
func (p *holidayPlanner) owedRest(dealtPrev, arranged []Slot) map[string]int64 {
	spacing := p.spacingProgrammes()
	lastAt := map[string]int64{}
	var n int64
	for _, s := range dealtPrev {
		if s.IsProgram() {
			lastAt[unitKey(s)], n = n, n+1
		}
	}
	out := map[string]int64{}
	var i int64
	for _, s := range loopedWindow(arranged, p.window.Milliseconds()) {
		if !s.IsProgram() {
			continue
		}
		if _, seen := out[unitKey(s)]; !seen {
			out[unitKey(s)] = spacing
			if last, ok := lastAt[unitKey(s)]; ok {
				out[unitKey(s)] = min(spacing, n+i-last)
			}
		}
		i++
	}
	return out
}

// owedOr is owed's rest for key, or def for a unit it does not hold.
func owedOr(owed map[string]int64, key string, def int64) int64 {
	if r, ok := owed[key]; ok {
		return r
	}
	return def
}

// holdUntilRested holds each unit of tile that would air sooner after its last airing in prev
// than it is owed back to the slot where it has rested, as the dealer holds a unit back at a seam
// (holdBack), until none would. A unit held past the window's close goes out at its end.
func (p *holidayPlanner) holdUntilRested(prev, tile []Slot, owed map[string]int64) []Slot {
	spacing := p.spacingProgrammes()
	airedAt := map[string]int64{}
	var n int64
	for _, s := range prev {
		if s.IsProgram() {
			airedAt[unitKey(s)], n = n, n+1
		}
	}
	for range len(tile) {
		var starts []int64 // ms into the tile where each programme slot starts
		var ms int64
		for _, s := range tile {
			if s.IsProgram() {
				starts = append(starts, ms)
				ms += s.DurationMs
			}
		}
		release := map[string]int64{}
		var i int64
		for _, s := range tile {
			if !s.IsProgram() {
				continue
			}
			if last, ok := airedAt[unitKey(s)]; ok {
				if slot := last + owedOr(owed, unitKey(s), spacing) - n; slot > i {
					release[s.LibraryItemID] = ms + 1 // past the tile: out at its end
					if slot < int64(len(starts)) {
						release[s.LibraryItemID] = starts[slot]
					}
				}
			}
			i++
		}
		held := holdBack(tile, release)
		if len(release) == 0 || sameProgrammes(held, tile) {
			return held
		}
		tile = held
	}
	return tile
}

// handOffReach is how many windows past the simulation's horizon the hand-off may run before the
// dealer airs alone: four horizons. Over 120 deal origins of the decks the spacing tests walk, the
// hand-off settled within 15 windows (a daily 400-episode deck, whose horizon is 8), and it stops
// as soon as it settles, so the bound costs nothing until it is needed.
func (p *holidayPlanner) handOffReach() int64 {
	return 4 * p.horizon()
}

// lastInSeason is the latest window at or before w that is in season, or one before the earliest
// window the simulation and the hand-off can reach back from w when none is.
func (p *holidayPlanner) lastInSeason(w int64) int64 {
	if got, ok := p.lastAt[w]; ok {
		return got
	}
	floor := w - p.horizon() - p.handOffReach()
	last := w
	for ; last >= floor && len(p.pooled(last)) == 0; last-- {
	}
	p.lastAt[w] = last
	return last
}

// handingOff reports whether window w is in the hand-off after a season (handOff), whose tile is
// already in the order it airs. On a channel with a viewing history the guard (tradeRecent) may
// still trade that tile, and its order then airs as traded: the guard holds it to the spacing,
// which wins over the evening (#1960).
func (p *holidayPlanner) handingOff(w int64) bool {
	if !p.enabled() || p.ring.stride == 0 {
		return false
	}
	h, last := p.horizon(), p.lastInSeason(w)
	return last < w-h && last >= w-h-p.handOffReach()
}

// repairArranged is `tile` with its seam repaired against `prev`, what aired before it (each window
// looped as playout airs it), given `arranged`, the order arrange airs it in. Arrange does not only
// delay a tile's units: in an evening it defers an off-season holiday episode past the band, which
// brings the units behind it forward, and so brought a unit held back from the seam to the window's
// head, next to where the window before closed on it (#1939). The seam is repaired on the arranged
// order (its features left out); a unit arrange does not air keeps its place at the end.
func (p *holidayPlanner) repairArranged(prev, tile, arranged []Slot) []Slot {
	inTile := map[string]bool{}
	for _, s := range tile {
		inTile[unitKey(s)] = true
	}
	order := make([]Slot, 0, len(tile))
	for _, s := range arranged {
		if inTile[unitKey(s)] {
			order = append(order, s)
			delete(inTile, unitKey(s))
		}
	}
	for _, s := range tile {
		if inTile[unitKey(s)] {
			order = append(order, s)
		}
	}
	return repairSeam(prev, order, p.ring.d.spacing)
}

// sameProgrammes reports whether a and b air the same programmes in the same order.
func sameProgrammes(a, b []Slot) bool {
	a = slices.DeleteFunc(slices.Clone(a), func(s Slot) bool { return !s.IsProgram() })
	b = slices.DeleteFunc(slices.Clone(b), func(s Slot) bool { return !s.IsProgram() })
	return slices.EqualFunc(a, b, func(x, y Slot) bool { return unitKey(x) == unitKey(y) })
}

// horizon is how many windows after a season the simulation still runs: twice the windows the
// repeat spacing spans and one, so what the season aired late rests its spacing before a
// dealer-only window can deal it again, and the trades it causes settle before the hand-off.
func (p *holidayPlanner) horizon() int64 {
	return 2 * ((p.spacingProgrammes()+p.ring.stride-1)/p.ring.stride + 1)
}

// simulate runs windows anchor..end in order and returns each one's tile (before arrange). It
// keeps airedAt, each unit's last airing as an index over every programme aired (features
// included), seeded with the dealer's arranged windows before the anchor: each window takes its
// dealer tile less the pool and anything that rested less than the spacing, keeps what fits
// beside its features (what is cut is owed), and tops up a short tile (topUp). One run serves the
// whole season, and is shared across calls (seasonRuns).
func (p *holidayPlanner) simulate(anchor, end int64) map[int64][]Slot {
	key, shared := p.runKey(anchor, end)
	if shared {
		if run, ok := seasonRuns.get(key); ok {
			return run
		}
	}
	spacing := p.spacingProgrammes()
	stride := p.ring.stride
	airedAt := map[string]int64{}
	var a int64
	// A window is recorded as playout airs it: a tile short of its window loops, so its head airs
	// again at the close, next to the following window's head (#1939).
	record := func(arranged []Slot) map[string]bool {
		aired := map[string]bool{}
		for _, s := range loopedWindow(arranged, p.window.Milliseconds()) {
			if s.IsProgram() {
				airedAt[unitKey(s)], aired[unitKey(s)] = a, true
				a++
			}
		}
		return aired
	}
	var prev map[string]bool
	for v := anchor - (spacing+stride-1)/stride - 1; v < anchor; v++ {
		prev = record(p.arrangeTile(p.ring.tile(v), v))
	}
	run := map[int64][]Slot{}
	var owed []Slot // dealt in season but cut for the features, in deal order
	owing := map[string]bool{}
	for v := anchor; v <= end; v++ {
		p.ring.d.checkCancel()
		pooled := p.pooled(v)
		need := p.window.Milliseconds()
		for _, f := range p.features(v) {
			need -= f.unit.DurationMs
		}
		var tile, later []Slot
		taken := map[string]bool{}
		var acc, n int64
		keep := func(s Slot) {
			taken[unitKey(s)] = true
			acc += s.DurationMs
			tile = append(tile, s)
			n++
		}
		for _, s := range p.ring.tile(v) {
			if !s.IsProgram() {
				tile = append(tile, s)
				continue
			}
			key := unitKey(s)
			if pooled[key] || taken[key] {
				continue
			}
			if last, ok := airedAt[key]; ok && a+n-last < spacing {
				later = append(later, s) // too soon here; it may have rested by the window's end
				continue
			}
			if acc >= need {
				if !owing[key] {
					owed, owing[key] = append(owed, s), true
				}
				continue
			}
			keep(s)
		}
		// A deal that is too soon where it falls airs later in the window once it has rested: the
		// dealer's own spacing works the same way, by order.
		for _, s := range later {
			if acc < need && !taken[unitKey(s)] && a+n-airedAt[unitKey(s)] >= spacing && p.restsBeyond(unitKey(s), v, end, spacing) {
				keep(s)
			}
		}
		if acc < need {
			tile = p.topUp(tile, taken, need-acc, v, end, a+n, spacing, prev, pooled, owed, airedAt)
		}
		tile = p.settle(tile, need, v, end, a, spacing, prev, pooled, owed, airedAt)
		run[v] = tile
		prev = record(p.arrange(tile, v))
		owed = slices.DeleteFunc(owed, func(s Slot) bool { return prev[unitKey(s)] })
		for k := range prev {
			delete(owing, k)
		}
	}
	if shared {
		seasonRuns.put(key, run)
	}
	return run
}

// restsBeyond reports whether a unit aired at the end of window v rests the spacing before its next
// deal past the simulation's last window, which only the dealer airs. A deal inside the simulation
// is checked there, against what aired.
func (p *holidayPlanner) restsBeyond(key string, v, end, spacing int64) bool {
	from := (v + 1) * p.ring.stride
	for _, q := range p.ring.airings(key, from) {
		if q >= (end+1)*p.ring.stride && q-from < spacing {
			return false
		}
	}
	return true
}

// settle checks a window's tile where it really airs. The tile is chosen by deal order from index a,
// but arrange moves it: features go in, and an off-season holiday episode leaves the evening, so a
// unit after one airs earlier than its place in the tile. A unit that, arranged, airs less than the
// spacing after its last airing is dropped for the window, and the tile is topped up again without
// it; each pass drops one unit for good, so it ends. A short window is looped by playout, so its
// head re-airs as many programmes later as the window holds: a unit no rested one can replace
// stays when the window without it would loop sooner than the spacing. The window then airs the
// most-rested units it has rather than a near-empty tile looped all day (#1939). Such a unit is
// first held back in the tile until it has rested, and stays put only once it cannot be (#2001).
func (p *holidayPlanner) settle(tile []Slot, need, v, end, a, spacing int64, prev, pooled map[string]bool, owed []Slot, airedAt map[string]int64) []Slot {
	dropped, kept := map[string]bool{}, map[string]bool{}
	held := 0 // units held back: each moves one later, so a window's worth bounds the loop
	for {
		mine := map[string]bool{}
		for _, s := range tile {
			if s.IsProgram() {
				mine[unitKey(s)] = true
			}
		}
		var i, short int64
		soon := ""
		for _, s := range p.arrange(tile, v) {
			if !s.IsProgram() {
				continue
			}
			key := unitKey(s)
			if last, ok := airedAt[key]; ok && mine[key] && !kept[key] && a+i-last < spacing {
				soon, short = key, spacing-(a+i-last)
				break
			}
			i++
		}
		if soon == "" {
			return tile
		}
		dropped[soon] = true
		without := slices.DeleteFunc(slices.Clone(tile), func(s Slot) bool { return s.IsProgram() && unitKey(s) == soon })
		var acc, n int64
		taken := maps.Clone(dropped)
		for _, s := range without {
			if s.IsProgram() {
				acc += s.DurationMs
				taken[unitKey(s)] = true
				n++
			}
		}
		if acc < need {
			// Its first estimate, like simulate's: the next pass checks where the top-up really airs.
			without = p.topUp(without, taken, need-acc, v, end, a+n, spacing, prev, pooled, owed, airedAt)
		}
		if aired := programRuntime(without); aired < need && aired < programRuntime(tile) && programmesIn(p.arrange(without, v)) < spacing {
			// No rested unit can take its place, so it waits in the window until it has rested, as the
			// dealer holds a unit back at a seam (holdBack), and stays put only once it cannot (#2001).
			if later, ok := holdLater(tile, soon, short); ok && held < len(tile) {
				tile, held = later, held+1
				continue
			}
			kept[soon] = true
			continue
		}
		tile = without
	}
}

// holdLater is tile with key's unit moved `by` programmes later, the units it passes each moving
// one earlier, or false when it is already the last programme.
func holdLater(tile []Slot, key string, by int64) ([]Slot, bool) {
	i := slices.IndexFunc(tile, func(s Slot) bool { return s.IsProgram() && unitKey(s) == key })
	if i < 0 {
		return nil, false
	}
	at := i
	for j := i + 1; j < len(tile) && by > 0; j++ {
		if tile[j].IsProgram() {
			at, by = j, by-1
		}
	}
	if at == i {
		return nil, false
	}
	out := slices.Delete(slices.Clone(tile), i, i+1)
	return slices.Insert(out, at, tile[i]), true
}

// programmesIn counts the programmes among slots.
func programmesIn(slots []Slot) int64 {
	var n int64
	for _, s := range slots {
		if s.IsProgram() {
			n++
		}
	}
	return n
}

// topUp fills a short tile with `short` of ordinary runtime, unavoidable when a window's features
// air fewer pool episodes than its deal held, or when the deal re-airs what rested too little. at
// is the index the first top-up would air at (a later one airs later still, so its past distance is
// understated, never overstated). Owed units go first, then any ordinary unit of the pass, and the
// order of preference is: not aired by the window before nor dealt to the next; at least the
// spacing from its last airing (airedAt) and its next dealt airing (the ring: past the horizon only
// the hand-off airs it); rested since its last airing, before one that has not; owed; the farthest
// from both.
func (p *holidayPlanner) topUp(tile []Slot, taken map[string]bool, short, v, end, at, spacing int64, prev, pooled map[string]bool, owed []Slot, airedAt map[string]int64) []Slot {
	stride := p.ring.stride
	next := map[string]bool{}
	for u := (v + 1) * stride; u < (v+2)*stride; u++ {
		next[unitKey(p.ring.at(u))] = true
	}
	type candidate struct {
		s          Slot
		dist       int64
		near, owed bool
		tier       int // 0: rested both ways; 1: rested since, its next deal simulated; 2: rested since; 3: neither
	}
	var cands []candidate
	pass, _ := p.ring.locate(v * stride)
	from, simulated := (v+1)*stride, (end+1)*stride
	for i, s := range append(slices.Clone(owed), p.ring.pass(pass)...) {
		key := unitKey(s)
		if pooled[key] || taken[key] || slices.ContainsFunc(p.f.unitHolidays(s), p.f.selected) {
			continue
		}
		taken[key] = true // owed units recur in the pass
		past, future, dealt := int64(math.MaxInt64), int64(math.MaxInt64), int64(math.MaxInt64)
		if last, ok := airedAt[key]; ok {
			past = at - last
		}
		for _, q := range p.ring.airings(key, from) {
			if q >= from {
				future, dealt = min(future, q-from), min(dealt, q)
			}
		}
		c := candidate{s: s, dist: min(past, future), near: prev[key] || next[key], owed: i < len(owed), tier: 3}
		switch {
		case past >= spacing && future >= spacing:
			c.tier = 0
		case past >= spacing && dealt < simulated:
			// Too soon for its next deal, which the simulation will skip: a skip, not a repeat.
			c.tier = 1
		case past >= spacing:
			// Too soon for its next deal past the simulation, which the hand-off holds back or skips
			// against what aired: a skip, where a unit that has not rested would be a repeat (#2001).
			c.tier = 2
		}
		cands = append(cands, c)
	}
	slices.SortStableFunc(cands, func(a, b candidate) int {
		switch {
		case a.tier != b.tier:
			return cmp.Compare(a.tier, b.tier)
		case a.near != b.near:
			return cmpBool(a.near, b.near)
		case a.owed != b.owed:
			return cmpBool(b.owed, a.owed)
		}
		return cmp.Compare(b.dist, a.dist)
	})
	for _, c := range cands {
		if short <= 0 {
			break
		}
		short -= c.s.DurationMs
		tile = append(tile, c.s)
	}
	return tile
}

// spacingProgrammes is the rest the simulation gives an ordinary unit, in programmes: the dealer's
// repeat spacing in programmes of the deck's mean runtime when it deals every pass afresh. One fixed
// deck has no dealer spacing; its bound is half the deck less its holiday episodes (§6). The
// dealer's spacing is capped at what the pool can honour at every airing: its ordinary units (their
// last airings are distinct, so the most rested one rested at least that many). A pool barely
// wider than its window has its dealer spacing capped at the whole pool, holiday episodes
// included, so every ordinary unit would be too soon all season and the window would air nearly
// empty, looped (#1939).
func (p *holidayPlanner) spacingProgrammes() int64 {
	var holiday int64
	for _, s := range p.ring.pass(0) {
		if slices.ContainsFunc(p.f.unitHolidays(s), p.f.selected) {
			holiday++
		}
	}
	if !p.ring.d.reshuffle {
		return max(p.ring.nProg/2-holiday, 1)
	}
	if p.meanMs <= 0 {
		return 0
	}
	spacing := int64(math.Ceil(float64(p.ring.d.spacing.Milliseconds()) / p.meanMs))
	return max(min(spacing, p.ring.nProg-holiday), 2)
}

// maxSeasonRuns bounds the season simulations remembered: a few seasons of a few channels' lineups.
const maxSeasonRuns = 32

// seasonRuns memoises season simulations by everything one reads (runKey), like the deal chain,
// so a guide request that computes a window at a time simulates the season once. An edited
// lineup or policy lands on a new key, and the old run ages out.
var seasonRuns = &runMemo{runs: map[uint64]map[int64][]Slot{}}

type runMemo struct {
	mu    sync.Mutex
	runs  map[uint64]map[int64][]Slot
	order []uint64 // oldest first
}

func (m *runMemo) get(key uint64) (map[int64][]Slot, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.runs[key]
	if ok {
		i := slices.Index(m.order, key) // a hit is a use: it goes to the back
		m.order = append(slices.Delete(m.order, i, i+1), key)
	}
	return run, ok
}

func (m *runMemo) put(key uint64, run map[int64][]Slot) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.runs[key]; ok {
		return
	}
	if len(m.order) >= maxSeasonRuns {
		delete(m.runs, m.order[0])
		m.order = m.order[1:]
	}
	m.runs[key] = run
	m.order = append(m.order, key)
}

// runKey hashes what a simulation of windows anchor..end reads: the dealer's inputs (dealerKey),
// the ring's origin, the window, zone and break density, and the holiday evidence. A pool with
// airing history deals differently by it, so it is not shared.
func (p *holidayPlanner) runKey(anchor, end int64) (uint64, bool) {
	if len(p.ring.d.rp.LastAired) > 0 {
		return 0, false
	}
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%d|%d|%d|%s|%g|%d|%d|", dealerKey(p.ring.d), p.ring.origin, p.window, p.loc, p.breakFactor, anchor, end)
	for _, id := range slices.Sorted(maps.Keys(p.f.matches)) {
		_, _ = fmt.Fprintf(h, "%s=%v\x00", id, p.f.matches[id])
	}
	for _, g := range slices.Sorted(maps.Keys(p.f.expand)) {
		_, _ = fmt.Fprintf(h, "%s=%v\x00", g, p.f.expand[g])
	}
	return h.Sum64(), true
}
