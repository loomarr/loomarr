package schedule

import (
	"math"
	"slices"
	"time"
)

// cmpBool orders false before true.
func cmpBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}

// arrange times window w's tile around its features. A feature airs as soon as the clock reaches
// it. Out of season, with evenings favoured, a holiday episode the clock reaches inside the band is
// deferred to the first moment outside it.
//
// On a channel that deals every pass afresh, the repeat spacing wins over the evening (#1960).
// Deferring an episode brings the units behind it forward, and a unit the dealer placed just
// outside its spacing then re-aired inside it. So an episode first swaps places with one ordinary
// unit after the band or, far from a season, before it (swapOut), which moves no other unit;
// failing that, it is deferred only when every unit it brings forward still rests the dealer's
// spacing from its last airing (or no less than its deal gave it), and the episode still rests it
// before its own next one (costsSpacing); otherwise it airs in place, as it does when the window
// closes before the band does.
//
// The check reads the windows around as dealt, without arranging them. Away from a season that is
// exact (swapOut says why). Near a season the simulation and the hand-off air other tiles, and
// check what really aired.
//
// In order, the dealer promises no spacing: an episode the window cannot defer past the band is
// moved, with any others, to just before the band (beforeBand); what does not fit there airs at
// the window's end.
//
// A window in the hand-off after a season (handOff) is already in the order it airs, and stays so.
func (p *holidayPlanner) arrange(tile []Slot, w int64) []Slot {
	if p.handingOff(w) {
		return tile
	}
	return p.arrangeTile(tile, w)
}

// arrangeTile is arrange for a window outside the hand-off.
func (p *holidayPlanner) arrangeTile(tile []Slot, w int64) []Slot {
	if !p.enabled() {
		return tile
	}
	// A marathon can fill a window: its tile is then empty, and the window airs the features alone.
	if len(tile) == 0 && len(p.features(w)) == 0 {
		return tile
	}
	pooled := p.pooled(w)
	opening := windowOpening(w, p.window, p.loc)
	offSeason := func(s Slot) bool {
		if !p.f.seasonal.Evenings || pooled[unitKey(s)] {
			return false
		}
		return slices.ContainsFunc(p.f.unitHolidays(s), p.f.selected)
	}
	if len(pooled) == 0 && !slices.ContainsFunc(tile, offSeason) {
		return tile
	}
	if p.ring.d.spacing <= 0 {
		out, deferred := p.place(tile, w, pooled, offSeason)
		return p.beforeBand(out, deferred, opening, offSeason)
	}
	before, _ := p.place(tile, w, pooled, never)
	dealt := p.airedIn(before, w)
	inPlace := map[string]bool{} // off-season episodes that air where they are
	for {
		swapped, early := p.swapOut(tile, w, pooled, offSeason, inPlace, before, dealt)
		out, deferred := p.place(swapped, w, pooled, func(s Slot) bool { return offSeason(s) && !inPlace[unitKey(s)] && !early[unitKey(s)] })
		if len(deferred) > 0 {
			// The window closes inside the band: what it deferred would only move to its end.
			for _, s := range deferred {
				inPlace[unitKey(s)] = true
			}
			continue
		}
		key := p.costsSpacing(before, dealt, out, w, offSeason, inPlace)
		if key == "" {
			return out
		}
		inPlace[key] = true
	}
}

func never(Slot) bool { return false }

// beforeBandSwaps is always on in production. Off, a window arranges as before #1971, the oracle a
// test checks the swaps against near a season.
var beforeBandSwaps = true

// swapOut is window w's tile with each off-season episode the clock reaches in the evening band
// swapped with one ordinary unit (#1960, #1971). The swap moves only those two, so the units
// between keep their places in the programme count the spacing is promised in. An episode first
// tries a unit after the band, which moves the episode later and the unit earlier, as deferring it
// would. Failing that, and only far from a season (farFromSeason), it tries a unit before the band:
// the episode moves earlier, which widens the gap to its own next airing (the gap that stops most
// swaps after the band), and the unit moves later. Either swap is taken only when the episode then
// starts outside the band, both still air, no other unit airs later than dealt, and no unit (this
// swap's or an earlier one's) comes closer to another airing than the spacing and its deal allow
// (costsSpacing). A swap before the band must also keep the window's programme count and every
// other unit's side of the band. Of the units that qualify, the one with the most slack over that
// floor takes the episode's place: since its last airing after the band, before its next one before
// it. An episode no unit qualifies for is left to arrange's deferral. A multi-part story is never
// split.
//
// Why the checks stay exact while they read the windows around as dealt. Take two consecutive
// airings of a unit, a in window v and b in a later window w. The gap that airs is the dealt gap,
// less how far a moved later, plus how far b moved later, so it shrinks only when a moves later or
// b earlier:
//   - An ordinary unit moves later only as a before-band swap's unit, and v checks its next gap to
//     nextGapLo, the earliest w can bring b. In w an ordinary unit moves earlier only by deferrals
//     of episodes dealt at or after the band ahead of it, never past the first of them, or by an
//     after-band swap into one of their places.
//   - Otherwise w checks an ordinary unit it brings forward against a as dealt, and a aired no
//     later.
//   - An episode moves earlier than w's deferral reserve for it (deferrableAhead) only by a
//     before-band swap, and w checks its last gap from lastGapHi, the latest v can air a: as dealt
//     when dealt before the band, where nothing is deferred or swapped later and no swap carries a
//     unit across the band, else v's last programme.
//   - Within the reserve, v left that room when it moved a later (costsSpacing); otherwise w
//     checks it against a as dealt.
//   - Every other unit keeps its programme index, and the window its programme count.
//
// Near a season the simulation and the hand-off air other tiles and read what aired, and no window
// within the spacing of them swaps earlier.
func (p *holidayPlanner) swapOut(tile []Slot, w int64, pooled map[string]bool, offSeason func(Slot) bool, inPlace map[string]bool,
	before []Slot, dealt windowAirings) (out []Slot, early map[string]bool) {
	count := map[string]int{}
	for _, s := range tile {
		count[unitKey(s)]++
	}
	ordinary := func(s Slot) bool {
		key := unitKey(s)
		return s.IsProgram() && count[key] == 1 && !pooled[key] && !slices.ContainsFunc(p.f.unitHolidays(s), p.f.selected)
	}
	spacing := p.spacingProgrammes()
	out, early = tile, map[string]bool{}
	tried, moved := map[string]bool{}, map[string]bool{} // episodes tried, and those swapped out
	dealtStarts := p.startsIn(before, w)
	for {
		placed, _ := p.place(out, w, pooled, never)
		starts := p.startsIn(placed, w)
		e := slices.IndexFunc(out, func(s Slot) bool {
			key := unitKey(s)
			return s.IsProgram() && offSeason(s) && count[key] == 1 && !inPlace[key] && !tried[key] && p.inEvening(starts[key])
		})
		if e < 0 {
			return out, early
		}
		episode := unitKey(out[e])
		tried[episode] = true
		var best []Slot
		var bestSlack int64
		for u := e + 1; u < len(out); u++ {
			key := unitKey(out[u])
			// A unit past the window's close would air only by the swap, and the episode not at all.
			if !ordinary(out[u]) || p.inEvening(starts[key]) || len(dealt.at[key]) == 0 {
				continue
			}
			candidate := slices.Clone(out)
			candidate[e], candidate[u] = candidate[u], candidate[e]
			placed, _ := p.place(candidate, w, pooled, never)
			airs := p.airedIn(placed, w)
			if len(airs.at[episode]) == 0 || p.backInEvening(p.startsIn(placed, w), episode, moved) || laterThanDealt(placed, dealt, airs, offSeason, "") ||
				p.costsSpacing(before, dealt, placed, w, offSeason, inPlace) != "" {
				continue
			}
			slack := int64(math.MaxInt64) // no airing within the spacing's reach
			if was, ok := p.lastGap(key, w, dealt); ok {
				now, _ := p.lastGap(key, w, airs)
				slack = now - min(spacing, was)
			}
			if best == nil || slack > bestSlack {
				best, bestSlack = candidate, slack
			}
		}
		if best == nil && beforeBandSwaps && p.farFromSeason(w) {
			aired := p.airedIn(placed, w)
			last := func(key string) int64 { return aired.at[key][len(aired.at[key])-1] }
			for u := 0; u < e; u++ {
				key := unitKey(out[u])
				if !ordinary(out[u]) || !p.beforeTheBand(starts[key]) || len(dealt.at[key]) == 0 {
					continue
				}
				// Most units fail on the gaps alone, judged on the places the two trade.
				if p.episodeTooEarly(episode, w, aired.first(key), dealt, offSeason) ||
					p.unitTooLate(key, w, aired.first(episode), aired.n-last(episode), dealt, offSeason) {
					continue
				}
				candidate := slices.Clone(out)
				candidate[e], candidate[u] = candidate[u], candidate[e]
				placed, _ := p.place(candidate, w, pooled, never)
				airs := p.airedIn(placed, w)
				newStarts := p.startsIn(placed, w)
				if len(airs.at[episode]) == 0 || len(airs.at[key]) == 0 || airs.n != dealt.n || !p.beforeTheBand(newStarts[episode]) ||
					p.backInEvening(newStarts, episode, moved) || laterThanDealt(placed, dealt, airs, offSeason, key) ||
					p.bandChanged(placed, newStarts, dealtStarts, episode, key) || p.costsSpacing(before, dealt, placed, w, offSeason, inPlace) != "" {
					continue
				}
				slack := int64(math.MaxInt64) // no airing within the spacing's reach
				if was, _, _, ok := p.nextGap(key, w, dealt); ok {
					at := airs.at[key]
					now, _ := p.nextGapLo(key, w, airs.n-at[len(at)-1], offSeason)
					slack = now - min(spacing, was)
				}
				if best == nil || slack > bestSlack {
					best, bestSlack = candidate, slack
				}
			}
			if best != nil {
				early[episode] = true
			}
		}
		if best != nil {
			out, moved[episode] = best, true
		}
	}
}

// beforeTheBand reports whether t starts before its day's evening band.
func (p *holidayPlanner) beforeTheBand(t time.Time) bool {
	return t.In(p.loc).Hour() < eveningFromHour
}

// bandChanged reports whether any unit of `placed` but the swapped two, a and b, starts on another
// side of the band (before it, in it, or after it) than dealt.
func (p *holidayPlanner) bandChanged(placed []Slot, starts, dealtStarts map[string]time.Time, a, b string) bool {
	for _, s := range placed {
		key := unitKey(s)
		if !s.IsProgram() || key == a || key == b {
			continue
		}
		if d, ok := dealtStarts[key]; ok && (p.inEvening(d) != p.inEvening(starts[key]) || p.beforeTheBand(d) != p.beforeTheBand(starts[key])) {
			return true
		}
	}
	return false
}

// farFromSeason reports whether every window the spacing reaches either side of w airs its deal,
// as tile hands it to arrange: no window there is in season, in the horizon after one, or in the
// hand-off after that (handOffReach).
func (p *holidayPlanner) farFromSeason(w int64) bool {
	if got, ok := p.farAt[w]; ok {
		return got
	}
	reach := p.spacingWindows()
	far := true
	for v := w - reach - p.horizon() - p.handOffReach(); v <= w+reach && far; v++ {
		far = len(p.pooled(v)) == 0
	}
	p.farAt[w] = far
	return far
}

// laterThanDealt reports whether `placed` airs any unit but an off-season episode and `except` (a
// before-band swap's unit) later than its deal. A swap keeps the units between in their places, but
// its time shift can carry one past a feature's start, which then airs first; the checks read
// neighbours as dealt only because no other ordinary unit moves later.
func laterThanDealt(placed []Slot, dealt, airs windowAirings, offSeason func(Slot) bool, except string) bool {
	for _, s := range placed {
		key := unitKey(s)
		if key == except {
			continue
		}
		if s.IsProgram() && !offSeason(s) && len(dealt.at[key]) > 0 && len(airs.at[key]) > 0 && airs.first(key) > dealt.first(key) {
			return true
		}
	}
	return false
}

// backInEvening reports whether the episode, or any one swapped out before it, starts in the band.
func (p *holidayPlanner) backInEvening(starts map[string]time.Time, episode string, moved map[string]bool) bool {
	if p.inEvening(starts[episode]) {
		return true
	}
	for key := range moved {
		if p.inEvening(starts[key]) {
			return true
		}
	}
	return false
}

// startsIn is when each unit in window w's `placed` tile first starts.
func (p *holidayPlanner) startsIn(placed []Slot, w int64) map[string]time.Time {
	out := map[string]time.Time{}
	t := windowOpening(w, p.window, p.loc)
	for _, s := range placed {
		if _, seen := out[unitKey(s)]; !seen {
			out[unitKey(s)] = t
		}
		if s.IsProgram() {
			t = t.Add(p.runtime(s))
		}
	}
	return out
}

// place times tile around window w's features, deferring each unit `deferOut` names out of the
// evening band to the first moment outside it, and returns what it could not defer before the
// window closed.
func (p *holidayPlanner) place(tile []Slot, w int64, pooled map[string]bool, deferOut func(Slot) bool) (out, deferred []Slot) {
	features := p.features(w)
	var queue []Slot
	for _, s := range tile {
		if !pooled[unitKey(s)] {
			queue = append(queue, s)
		}
	}
	out = make([]Slot, 0, len(queue)+len(features))
	t := windowOpening(w, p.window, p.loc)
	emit := func(s Slot) {
		out = append(out, s)
		if s.IsProgram() {
			t = t.Add(p.runtime(s))
		}
	}
	for {
		switch {
		case len(features) > 0 && (!t.Before(features[0].at) || len(queue) == 0 && (len(deferred) == 0 || p.inEvening(t))):
			emit(features[0].unit)
			features = features[1:]
		case len(deferred) > 0 && !p.inEvening(t):
			emit(deferred[0])
			deferred = deferred[1:]
		case len(queue) > 0:
			s := queue[0]
			queue = queue[1:]
			if deferOut(s) && p.inEvening(t) {
				deferred = append(deferred, s)
			} else {
				emit(s)
			}
		default:
			return out, deferred
		}
	}
}

func (p *holidayPlanner) inEvening(t time.Time) bool {
	h := t.In(p.loc).Hour()
	return h >= eveningFromHour && h < eveningToHour
}

// costsSpacing is the off-season episode whose deferral in `out` costs the repeat spacing, or ""
// when none does. Spacing is counted in programmes, as the dealer promises it, and a move costs it
// when it leaves a unit closer to another airing than both the spacing and its deal did: the
// episode itself, closer to its next dealt airing, or else the nearest episode deferred from ahead
// of a unit it brought forward, closer to that unit's last airing. Keeping that one in place moves
// the unit back a slot. Far from a season, a swap before the band also costs it when its episode
// comes too close to its last airing (episodeTooEarly) or its unit to its next (unitTooLate), and
// that episode is the one kept in place. `before` is the tile placed unmoved, and `dealt` where it
// airs.
func (p *holidayPlanner) costsSpacing(before []Slot, dealt windowAirings, out []Slot, w int64, offSeason func(Slot) bool, inPlace map[string]bool) string {
	airs := p.airedIn(out, w)
	spacing := p.spacingProgrammes()
	var deferred []string        // deferred episodes, in deal order
	moved := func(s Slot) bool { // airs in both, so both say where
		return s.IsProgram() && len(dealt.at[unitKey(s)]) > 0 && len(airs.at[unitKey(s)]) > 0
	}
	for _, s := range before {
		key := unitKey(s)
		if !moved(s) || !offSeason(s) || inPlace[key] || airs.first(key) <= dealt.first(key) {
			continue
		}
		// Its next window can bring its next airing forward a slot for each off-season episode it
		// defers from ahead of it, so the episode leaves room for those too.
		if was, v, next, ok := p.nextGap(key, w, dealt); ok {
			if now, _, _, _ := p.nextGap(key, w, airs); now-p.deferrableAhead(v, next, offSeason) < min(spacing, was) {
				return key
			}
		}
		deferred = append(deferred, key)
	}
	for _, s := range before {
		key := unitKey(s)
		if !moved(s) {
			continue
		}
		// A swap before the band moves an ordinary unit later and an episode earlier (swapOut).
		if p.farFromSeason(w) {
			at := airs.at[key]
			if offSeason(s) && p.episodeTooEarly(key, w, airs.first(key), dealt, offSeason) {
				return key
			}
			if !offSeason(s) && p.unitTooLate(key, w, airs.first(key), airs.n-at[len(at)-1], dealt, offSeason) {
				if e := unitAt(out, dealt.first(key)); offSeason(e) && !inPlace[unitKey(e)] {
					return unitKey(e) // the episode that took its place
				}
			}
		}
		if airs.first(key) >= dealt.first(key) {
			continue
		}
		was, ok := p.lastGap(key, w, dealt)
		if now, _ := p.lastGap(key, w, airs); !ok || now >= min(spacing, was) {
			continue
		}
		blame := ""
		for _, d := range deferred {
			if dealt.first(d) < dealt.first(key) {
				blame = d
			}
		}
		if blame != "" {
			return blame
		}
		// No deferral brought the episode forward, so a swap before the band did.
		if offSeason(s) && !inPlace[key] && p.farFromSeason(w) {
			return key
		}
	}
	return ""
}

// windowAirings is where each unit airs in one window, by programme index, looped like playout
// when the tile is shorter than the window, and how many programmes the window airs.
type windowAirings struct {
	at map[string][]int64
	n  int64
}

func (a windowAirings) first(key string) int64 { return a.at[key][0] }

// airedIn is window w airing `arranged`.
func (p *holidayPlanner) airedIn(arranged []Slot, w int64) windowAirings {
	out := windowAirings{at: map[string][]int64{}}
	opening := windowOpening(w, p.window, p.loc)
	closing := opening.Add(p.window)
	for t := opening; t.Before(closing); {
		before := t
		for _, s := range arranged {
			if !s.IsProgram() || !t.Before(closing) {
				continue
			}
			out.at[unitKey(s)] = append(out.at[unitKey(s)], out.n)
			out.n++
			t = t.Add(p.runtime(s))
		}
		if t.Equal(before) {
			break
		}
	}
	return out
}

// dealtIn is window v airing its deal unmoved. Away from a season, the checks bound how far arrange
// can move a unit from it on a reshuffled channel (swapOut).
func (p *holidayPlanner) dealtIn(v int64) windowAirings {
	if got, ok := p.dealtAt[v]; ok {
		return got
	}
	got := p.airedIn(p.dealtTile(v), v)
	p.dealtAt[v] = got
	return got
}

// dealtTile is window v's deal, read only: the checks read each neighbour's many times.
func (p *holidayPlanner) dealtTile(v int64) []Slot {
	if got, ok := p.tiles[v]; ok {
		return got
	}
	got := p.ring.tile(v)
	p.tiles[v] = got
	return got
}

// spacingWindows is how many windows either side the repeat spacing can reach, a window's last
// unit running past its close.
func (p *holidayPlanner) spacingWindows() int64 {
	return int64(p.ring.d.spacing/p.window) + 2
}

// lastGap is how many programmes key's first airing in window w (as `in` airs it) comes after its
// last dealt airing in the windows before, within the spacing's reach.
func (p *holidayPlanner) lastGap(key string, w int64, in windowAirings) (int64, bool) {
	gap := in.first(key)
	for v := w - 1; v >= w-p.spacingWindows(); v-- {
		d := p.dealtIn(v)
		if at := d.at[key]; len(at) > 0 {
			return gap + d.n - at[len(at)-1], true
		}
		gap += d.n
	}
	return 0, false
}

// nextGap is how many programmes key's next dealt airing, in the windows after, comes after its
// last airing in window w (as `in` airs it), within the spacing's reach, and the window and
// programme index of that airing.
func (p *holidayPlanner) nextGap(key string, w int64, in windowAirings) (gap, v, next int64, ok bool) {
	at := in.at[key]
	return p.nextGapFrom(key, w, in.n-at[len(at)-1])
}

// nextGapFrom is nextGap for a last airing `rest` programmes before window w closes.
func (p *holidayPlanner) nextGapFrom(key string, w, rest int64) (gap, v, next int64, ok bool) {
	gap = rest
	for v = w + 1; v <= w+p.spacingWindows(); v++ {
		d := p.dealtIn(v)
		if at := d.at[key]; len(at) > 0 {
			return gap + at[0], v, at[0], true
		}
		gap += d.n
	}
	return 0, 0, 0, false
}

// unitTooLate reports whether ordinary unit key, airing first at programme `first` of window w and
// last `rest` programmes before it closes, airs later than dealt and closer to its next airing than
// the spacing and its deal allow, wherever that window moves it (nextGapLo).
func (p *holidayPlanner) unitTooLate(key string, w, first, rest int64, dealt windowAirings, offSeason func(Slot) bool) bool {
	if first <= dealt.first(key) {
		return false
	}
	was, _, _, ok := p.nextGap(key, w, dealt)
	if !ok {
		return false
	}
	now, _ := p.nextGapLo(key, w, rest, offSeason)
	return now < min(p.spacingProgrammes(), was)
}

// episodeTooEarly reports whether off-season episode key, airing first at programme `first` of
// window w, comes closer to its last airing than the spacing and its deal allow, wherever that
// window moved it (lastGapHi).
func (p *holidayPlanner) episodeTooEarly(key string, w, first int64, dealt windowAirings, offSeason func(Slot) bool) bool {
	if first >= dealt.first(key)-p.deferrableAhead(w, dealt.first(key), offSeason) {
		return false
	}
	was, ok := p.lastGap(key, w, dealt)
	return ok && p.lastGapHi(key, w, first) < min(p.spacingProgrammes(), was)
}

// nextGapLo is nextGapFrom with the next airing as early as its window v can air it: at the first
// off-season episode v deals at or after the band ahead of it, if any, else as dealt.
func (p *holidayPlanner) nextGapLo(key string, w, rest int64, offSeason func(Slot) bool) (int64, bool) {
	gap, v, next, ok := p.nextGapFrom(key, w, rest)
	if !ok {
		return 0, false
	}
	t := windowOpening(v, p.window, p.loc)
	var i int64
	for _, s := range p.dealtTile(v) {
		if !s.IsProgram() {
			continue
		}
		if i >= next {
			break
		}
		if offSeason(s) && !p.beforeTheBand(t) {
			return gap - (next - i), true
		}
		t = t.Add(p.runtime(s))
		i++
	}
	return gap, true
}

// lastGapHi is how many programmes programme `first` of window w comes after key's last dealt
// airing, with that airing as late as its window v can air it: as dealt when dealt before the
// band, else at v's last programme.
func (p *holidayPlanner) lastGapHi(key string, w, first int64) int64 {
	gap := first
	for v := w - 1; v >= w-p.spacingWindows(); v-- {
		d := p.dealtIn(v)
		if at := d.at[key]; len(at) > 0 {
			last := at[len(at)-1]
			if !p.beforeTheBand(p.dealtStart(v, last)) {
				last = d.n - 1
			}
			return gap + d.n - last
		}
		gap += d.n
	}
	return math.MaxInt64 // no airing within the spacing's reach
}

// dealtStart is when window v's deal starts its programme i, looped like playout.
func (p *holidayPlanner) dealtStart(v, i int64) time.Time {
	t := windowOpening(v, p.window, p.loc)
	tile := p.dealtTile(v)
	for n := int64(0); ; {
		looped := false
		for _, s := range tile {
			if !s.IsProgram() {
				continue
			}
			if n == i {
				return t
			}
			t, n, looped = t.Add(p.runtime(s)), n+1, true
		}
		if !looped {
			return t
		}
	}
}

// unitAt is the unit `out` airs as its programme i.
func unitAt(out []Slot, i int64) Slot {
	var n int64
	for _, s := range out {
		if !s.IsProgram() {
			continue
		}
		if n == i {
			return s
		}
		n++
	}
	return Slot{}
}

// deferrableAhead is how many off-season episodes window v deals ahead of its programme `next`:
// the most slots deferring them can bring that programme forward.
func (p *holidayPlanner) deferrableAhead(v, next int64, offSeason func(Slot) bool) int64 {
	var n, i int64
	for _, s := range p.dealtTile(v) {
		if !s.IsProgram() {
			continue
		}
		if i >= next {
			break
		}
		if offSeason(s) {
			n++
		}
		i++
	}
	return n
}

// beforeBand inserts the units the window could not air outside the evening, as one run, just
// before the band the window ends in: at the last position where every one of them still starts
// before 17:00. What does not fit before the band (a window wholly inside it has no room at all)
// airs at the window's end. An out-of-season unit after the insertion joins the run, since the
// insertion would push it into the band.
func (p *holidayPlanner) beforeBand(out, deferred []Slot, opening time.Time, offSeason func(Slot) bool) []Slot {
	if len(deferred) == 0 {
		return out
	}
	end := opening // the band the arrangement ends in
	for _, s := range out {
		if s.IsProgram() {
			end = end.Add(p.runtime(s))
		}
	}
	y, m, d := end.In(p.loc).Date()
	band := time.Date(y, m, d, eveningFromHour, 0, 0, 0, p.loc)
	var tail []Slot
	for {
		var lead time.Duration // the run's length up to its last unit's start
		for _, s := range deferred[:len(deferred)-1] {
			lead += p.runtime(s)
		}
		at, t := len(out), opening
		for i, o := range out {
			if o.IsProgram() && t.Add(lead).Before(band) {
				at = i
			}
			if o.IsProgram() {
				t = t.Add(p.runtime(o))
			}
		}
		if at == len(out) && len(deferred) > 1 {
			// Not all of them fit before the band: fit what does, the rest air in place.
			tail = append([]Slot{deferred[len(deferred)-1]}, tail...)
			deferred = deferred[:len(deferred)-1]
			continue
		}
		if at == len(out) || !slices.ContainsFunc(out[at:], offSeason) {
			return append(slices.Insert(out, at, deferred...), tail...)
		}
		i := at + slices.IndexFunc(out[at:], offSeason)
		deferred = append(deferred, out[i])
		out = slices.Delete(out, i, i+1)
	}
}
