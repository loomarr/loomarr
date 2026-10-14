package schedule

import (
	"cmp"
	"fmt"
	"hash/fnv"
	"maps"
	"math"
	"slices"
	"sync"
	"time"
)

// Holiday episodes inside an ordinary series (§6 "Holiday episodes", #1670).
//
// Title-level seasonality (applySeasonal) cannot see that "The Simpsons" holds nine "Treehouse of
// Horror" episodes: the entry's title is not about Halloween, so an October auto channel aired them
// only when the deck happened to deal one. This pass features them. Each civil day of an active
// holiday window has ONE plan (which of the pool's episodes it features, and when), whatever the
// window length; a window airs the plan's features that fall inside it. In season, a window's tile
// leaves the pool out and continues the deck exactly where the window before stopped, so the
// feature never re-airs an ordinary episode. Out of season, with evenings favoured, a holiday
// episode is never benched but is kept out of the evening band.
//
// The deck itself never changes: the boost cannot reshuffle the channel's rotation. Its inputs are
// the deck, the window index and zone, and the policy, so a window arranges identically on every
// reconcile (#1674/#1675), and the cycle cache's key (ClockSignature + window index) stays complete.

const (
	eveningFromHour = 17 // the evening band, channel-local: 17:00 ≤ start < 23:00
	eveningToHour   = 23
	// minHolidayRepeatDays is the in-season exception to EpisodeNoRepeat: a featured holiday
	// episode never airs twice in a day nor on consecutive days, and otherwise rotates evenly
	// through the pool. The channel's ordinary 7-day spacing would cap a nine-episode pool at
	// one a day, at every strength.
	minHolidayRepeatDays = 2
	// holidayShareOfDay caps the feature at 1/holidayShareOfDay of a day's programmes, so the
	// rest of the lineup keeps at least three quarters of the day. Heavy's holiday date is exempt.
	holidayShareOfDay = 4
	// featuredHolidayDayMax is how many a Featured channel airs on the holiday's own date.
	featuredHolidayDayMax = 5
	// seasonLookback bounds the walk back to the first window of a holiday season (the longest
	// built-in window is a month; this leaves room for back-to-back seasons).
	seasonLookback = 120 * 24 * time.Hour
)

// dailyQuota is how many holiday episodes a strength asks for on an ordinary day of the window.
func (s HolidayStrength) dailyQuota() int {
	switch s {
	case HolidayStrengthLight:
		return 1
	case HolidayStrengthHeavy:
		return 5
	default:
		return 3
	}
}

// holidayDayQuota is the holiday's own date (Oct 31): Heavy runs the whole pool as a marathon,
// Featured up to featuredHolidayDayMax, Light keeps its one. Spacing does not apply that night.
func (s HolidayStrength) holidayDayQuota(pool int) int {
	switch s {
	case HolidayStrengthLight:
		return min(1, pool)
	case HolidayStrengthHeavy:
		return pool
	default:
		return min(featuredHolidayDayMax, pool)
	}
}

// HolidayRotation is how a pool of holiday episodes rotates at a strength: perDay of them air on
// an ordinary day of the window, and none airs again sooner than everyDays days. A small pool caps
// the strength (nine episodes: Light 1 every 9 days, Featured 3 every 3, Heavy 4 every 2). A pool
// of one airs every other day. An empty pool airs nothing.
func HolidayRotation(pool int, strength HolidayStrength) (perDay, everyDays int) {
	if pool <= 0 {
		return 0, 0
	}
	q := min(strength.dailyQuota(), pool)
	spacing := max(pool/q, minHolidayRepeatDays)
	if q = min(q, pool/spacing); q == 0 {
		return 1, minHolidayRepeatDays
	}
	return q, pool / q
}

// cappedRotation is HolidayRotation held to dayCap a day (the quarter-of-the-day cap).
func cappedRotation(pool int, strength HolidayStrength, dayCap int) (perDay, everyDays int) {
	perDay, everyDays = HolidayRotation(pool, strength)
	if perDay > dayCap {
		if perDay = max(dayCap, 0); perDay == 0 {
			return 0, 0
		}
		everyDays = max(pool/perDay, minHolidayRepeatDays)
	}
	return perDay, everyDays
}

// rotationPicks returns the pool positions featured on civil day `day`. Consecutive days walk the
// pool as a ring, perDay at a time, so an episode's next airing is at least ⌊pool/perDay⌋ days
// later (≥ everyDays by HolidayRotation's construction).
func rotationPicks(pool, perDay, everyDays int, day int64) []int {
	if pool == 0 || perDay == 0 {
		return nil
	}
	if pool < perDay*everyDays && day%int64(everyDays) != 0 {
		return nil // a pool too small for one a day airs every everyDays days
	}
	start := int((day*int64(perDay))%int64(pool)+int64(pool)) % pool
	picks := make([]int, 0, perDay)
	for k := range perDay {
		picks = append(picks, (start+k)%pool)
	}
	return picks
}

// holidayFeature is what the pass knows about the channel's episodes: which holidays each
// library item's own evidence is about, and the parts of each multi-part unit in the deck.
type holidayFeature struct {
	seasonal ResolvedSeasonal
	matches  map[string][]string // library item id → holiday ids (episodeHolidays)
	expand   map[string][]Slot   // PartGroup → parts (collapseGroups)
}

// unitHolidays is the holidays a (possibly collapsed) unit is about: any part's evidence counts,
// so a two-part Christmas story moves as one.
func (f holidayFeature) unitHolidays(s Slot) []string {
	if !s.IsProgram() {
		return nil
	}
	if parts, ok := f.expand[s.PartGroup]; ok && s.PartGroup != "" {
		var ids []string
		for _, p := range parts {
			for _, id := range f.matches[p.LibraryItemID] {
				if !slices.Contains(ids, id) {
					ids = append(ids, id)
				}
			}
		}
		return ids
	}
	return f.matches[s.LibraryItemID]
}

func unitKey(s Slot) string {
	if s.PartGroup != "" {
		return "group:" + s.PartGroup
	}
	return "item:" + s.LibraryItemID
}

// selected reports whether the channel's holiday subset includes id (empty subset = all).
func (f holidayFeature) selected(id string) bool {
	set := lowerSet(f.seasonal.Holidays)
	if set == nil {
		return true
	}
	_, ok := set[id]
	return ok
}

// holidayPool is the deck's distinct holiday units for any of `ids`, in deck order, which is
// seed-stable, so the rotation is too.
func (f holidayFeature) holidayPool(deck []Slot, ids map[string]bool) []Slot {
	var pool []Slot
	seen := map[string]bool{}
	for _, s := range deck {
		if seen[unitKey(s)] {
			continue
		}
		for _, id := range f.unitHolidays(s) {
			if ids[id] {
				seen[unitKey(s)] = true
				pool = append(pool, s)
				break
			}
		}
	}
	return pool
}

// civilDay numbers a calendar date (days since 1970-01-01), the rotation's day index.
func civilDay(y int, m time.Month, d int) int64 {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Unix() / 86400
}

type holidayAt struct {
	at   time.Time
	unit Slot
}

// dayPlan is one civil day's holiday feature, computed once whatever window asks for it.
type dayPlan struct {
	start, end time.Time // the day's local midnights (23 or 25 hours apart on a DST day)
	pool       []Slot    // the active selected holidays' units, in deck order
	holiday    bool      // an active selected holiday's own date
	features   []holidayAt
}

// holidayPlanner arranges one channel's windows around its per-day holiday plans.
type holidayPlanner struct {
	deck        []Slot     // pass 0's deal: the pool and day plans read it, so they hold across passes
	ring        *dealtRing // what the windows air, pass after pass (simulate and fitted read it)
	f           holidayFeature
	window      time.Duration
	loc         *time.Location
	breakFactor float64
	meanMs      float64 // mean programme runtime in the deck, the base of the day cap
	raw         map[int64]*dayPlan
	plans       map[int64]*dayPlan
	base        map[int64][]Slot        // the ring's ordinary-day picks, before neighbour drops
	full        map[int64][]Slot        // what the date features after its holiday-date top-up
	dealtAt     map[int64]windowAirings // dealtIn's memo
}

// newHolidayPlanner prepares the pass. breakFactor stretches programme runtime by the channel's
// break density so the estimated start times land where the guide will show them.
func newHolidayPlanner(ring *dealtRing, f holidayFeature, window time.Duration, zone *time.Location, breakFactor float64) *holidayPlanner {
	if zone == nil {
		zone = time.UTC
	}
	deck := ring.d.deck(0)
	p := &holidayPlanner{deck: deck, ring: ring, f: f, window: window, loc: zone, breakFactor: breakFactor,
		raw: map[int64]*dayPlan{}, plans: map[int64]*dayPlan{}, base: map[int64][]Slot{}, full: map[int64][]Slot{}, dealtAt: map[int64]windowAirings{}}
	var n int
	var sum int64
	for _, s := range deck {
		if s.IsProgram() {
			n++
			sum += s.DurationMs
		}
	}
	if n > 0 {
		p.meanMs = float64(sum) / float64(n)
	}
	return p
}

// enabled reports whether the pass features anything: auto mode, a wall-clock window, and at
// least one episode with holiday evidence. A window ≤ 0 has no wall-clock anchor.
func (p *holidayPlanner) enabled() bool {
	return p.f.seasonal.Mode == SeasonalAuto && p.window > 0 && len(p.f.matches) > 0 && len(p.deck) > 0
}

func (p *holidayPlanner) runtime(s Slot) time.Duration {
	return time.Duration(float64(s.DurationMs)*p.breakFactor) * time.Millisecond
}

// dayCap is the quarter-of-the-day cap for a day of `length`: ⌊length / (mean runtime × breaks) / 4⌋.
func (p *holidayPlanner) dayCap(length time.Duration) int {
	if p.meanMs <= 0 {
		return len(p.deck)
	}
	return int(float64(length.Milliseconds()) / (p.meanMs * p.breakFactor) / holidayShareOfDay)
}

// rawDay is a date's pool and ring picks, before its neighbours are considered.
func (p *holidayPlanner) rawDay(date time.Time) *dayPlan {
	y, m, d := date.Date()
	key := civilDay(y, m, d)
	if r, ok := p.raw[key]; ok {
		return r
	}
	r := &dayPlan{start: time.Date(y, m, d, 0, 0, 0, 0, p.loc), end: time.Date(y, m, d+1, 0, 0, 0, 0, p.loc)}
	active := map[string]bool{}
	for _, h := range builtinCalendar {
		if p.f.selected(h.id) && holidayActive(h, time.Date(y, m, d, 12, 0, 0, 0, time.UTC)) {
			active[h.id] = true
			if hd := h.day(y); hd.Month() == m && hd.Day() == d {
				r.holiday = true
			}
		}
	}
	if len(active) > 0 {
		r.pool = p.f.holidayPool(p.deck, active)
	}
	perDay, everyDays := cappedRotation(len(r.pool), p.f.seasonal.Strength, p.dayCap(r.end.Sub(r.start)))
	var base []Slot
	for _, i := range rotationPicks(len(r.pool), perDay, everyDays, key) {
		base = append(base, r.pool[i])
	}
	p.raw[key], p.base[key] = r, base
	return r
}

// holidayFull is what a holiday date features: its ring picks topped up to the strength's
// holiday quota, in ring order, preferring units neither neighbour's ring would feature.
func (p *holidayPlanner) holidayFull(date time.Time) []Slot {
	r := p.rawDay(date)
	y, m, d := date.Date()
	key := civilDay(y, m, d)
	if got, ok := p.full[key]; ok {
		return got
	}
	picks := slices.Clone(p.base[key])
	quota := p.f.seasonal.Strength.holidayDayQuota(len(r.pool))
	if p.f.seasonal.Strength != HolidayStrengthHeavy {
		quota = min(quota, p.dayCap(r.end.Sub(r.start)))
	}
	taken := map[string]bool{}
	for _, u := range picks {
		taken[unitKey(u)] = true
	}
	near := map[string]bool{}
	p.rawDay(r.start.AddDate(0, 0, -1))
	p.rawDay(r.start.AddDate(0, 0, 1))
	for _, u := range append(slices.Clone(p.base[key-1]), p.base[key+1]...) {
		near[unitKey(u)] = true
	}
	from := 0
	if len(picks) > 0 {
		from = slices.IndexFunc(r.pool, func(s Slot) bool { return unitKey(s) == unitKey(picks[len(picks)-1]) }) + 1
	}
	for _, avoidNear := range []bool{true, false} {
		for k := 0; k < len(r.pool) && len(picks) < quota; k++ {
			u := r.pool[(from+k)%len(r.pool)]
			if !taken[unitKey(u)] && (!avoidNear || !near[unitKey(u)]) {
				taken[unitKey(u)] = true
				picks = append(picks, u)
			}
		}
	}
	p.full[key] = picks
	return picks
}

// plan is a date's final feature: a holiday date airs its full quota; the day before and after
// drop anything the holiday date airs (compared by unit, since neighbouring pools can differ).
func (p *holidayPlanner) plan(date time.Time) *dayPlan {
	r := p.rawDay(date)
	y, m, d := date.Date()
	key := civilDay(y, m, d)
	if got, ok := p.plans[key]; ok {
		return got
	}
	plan := *r
	var picks []Slot
	if r.holiday {
		picks = p.holidayFull(date)
	} else {
		drop := map[string]bool{}
		for _, nb := range []time.Time{r.start.AddDate(0, 0, -1), r.start.AddDate(0, 0, 1)} {
			if p.rawDay(nb).holiday {
				for _, u := range p.holidayFull(nb) {
					drop[unitKey(u)] = true
				}
			}
		}
		for _, u := range p.base[key] {
			if !drop[unitKey(u)] {
				picks = append(picks, u)
			}
		}
	}
	plan.features = p.placeFeatures(r, picks)
	p.plans[key] = &plan
	return &plan
}

// placeFeatures times a day's picks: evenly through the band (the evening, or the whole day for
// anytime), except Heavy's holiday date, a marathon that airs back to back from 17:00, or early
// enough to end by 23:00 when it is longer than the band (never before the date starts, and only
// what starts before the date ends).
func (p *holidayPlanner) placeFeatures(r *dayPlan, picks []Slot) []holidayAt {
	if len(picks) == 0 {
		return nil
	}
	y, m, d := r.start.Date()
	evening, late := time.Date(y, m, d, eveningFromHour, 0, 0, 0, p.loc), time.Date(y, m, d, eveningToHour, 0, 0, 0, p.loc)
	out := make([]holidayAt, 0, len(picks))
	if r.holiday && p.f.seasonal.Strength == HolidayStrengthHeavy {
		var length time.Duration
		for _, u := range picks {
			length += p.runtime(u)
		}
		at := evening
		if length > late.Sub(evening) {
			at = late.Add(-length)
			if at.Before(r.start) {
				at = r.start
			}
		}
		// Each episode is due when the one before it ends, so a marathon longer than a window
		// carries on into the next; what would start after the date ends does not air.
		for _, u := range picks {
			if !at.Before(r.end) {
				break
			}
			out = append(out, holidayAt{at: at, unit: u})
			at = at.Add(p.runtime(u))
		}
		return out
	}
	from, to := r.start, r.end
	if p.f.seasonal.Evenings {
		from, to = evening, late
	}
	span := to.Sub(from)
	for k, u := range picks {
		out = append(out, holidayAt{at: from.Add(span * time.Duration(2*k+1) / time.Duration(2*len(picks))), unit: u})
	}
	return out
}

// days lists the local dates window w touches.
func (p *holidayPlanner) days(w int64) []time.Time {
	opening := windowOpening(w, p.window, p.loc)
	closing := opening.Add(p.window)
	y, m, d := opening.In(p.loc).Date()
	var out []time.Time
	for day := time.Date(y, m, d, 0, 0, 0, 0, p.loc); day.Before(closing); day = time.Date(day.Year(), day.Month(), day.Day()+1, 0, 0, 0, 0, p.loc) {
		out = append(out, day)
	}
	return out
}

// pooled is the units of every active pool window w touches: in season, they air only where a
// day's plan features them.
func (p *holidayPlanner) pooled(w int64) map[string]bool {
	out := map[string]bool{}
	for _, day := range p.days(w) {
		for _, u := range p.rawDay(day).pool {
			out[unitKey(u)] = true
		}
	}
	return out
}

// features is what window w airs of its days' plans: the features timed inside it, by time.
func (p *holidayPlanner) features(w int64) []holidayAt {
	opening := windowOpening(w, p.window, p.loc)
	closing := opening.Add(p.window)
	var out []holidayAt
	for _, day := range p.days(w) {
		for _, f := range p.plan(day).features {
			if !f.at.Before(opening) && f.at.Before(closing) {
				out = append(out, f)
			}
		}
	}
	slices.SortStableFunc(out, func(a, b holidayAt) int { return a.at.Compare(b.at) })
	return out
}

// tile is window w's ordinary episodes, given the dealer's tile for it. In season, and for the
// horizon of windows after a season, it is the season simulation's tile (§6 "Where it runs"):
// every pick there is decided against what actually aired, never against ring positions, which
// under per-pass reshuffle drift from it (#1939). The horizon after that hands off to the dealer
// (handOff); every other window airs the dealer's tile, which arrange keeps to the spacing.
func (p *holidayPlanner) tile(w int64, dealt []Slot) []Slot {
	if !p.enabled() || p.ring.stride == 0 {
		return dealt
	}
	h := p.horizon()
	last := w // the latest in-season window at or before w
	for ; last >= w-2*h && len(p.pooled(last)) == 0; last-- {
	}
	switch {
	case last < w-2*h:
		return dealt
	case last < w-h:
		return p.handOff(last+h, w, dealt)
	case programRuntime(p.deck) <= p.window.Milliseconds():
		return p.fitted(w, dealt)
	}
	// The anchor and the end are the same for every window of the season, so each window reads
	// the same simulation: two seasons closer than the horizon are one.
	back := int64(seasonLookback / p.window)
	anchor, end := last, last+h
	for v := last - 1; v >= anchor-h-1 && v > w-back; v-- {
		if len(p.pooled(v)) > 0 {
			anchor = v
		}
	}
	for v := last + 1; v <= end && v < w+back; v++ {
		if len(p.pooled(v)) > 0 {
			end = v + h
		}
	}
	return slices.Clone(p.simulate(anchor, end)[w]) // the run is shared: never hand out its slices
}

// fitted is window w's tile for a deck that fits one window: all of it (that window's deal) but
// the pool. The dealer repaired the deal's seam against the window before as it dealt it, but in
// season that window aired less its pool and plus its features, looped, so its loop stopped on a
// different unit: the seam is repaired again against the window before as it really aired, and so
// for the horizon after a season (#1939), in the order arrange airs it (repairArranged).
func (p *holidayPlanner) fitted(w int64, dealt []Slot) []Slot {
	pooled := p.pooled(w)
	var tile []Slot
	for _, s := range dealt {
		if !pooled[unitKey(s)] {
			tile = append(tile, s)
		}
	}
	prev := p.arrange(p.tile(w-1, p.ring.tile(w-1)), w-1)
	return p.repairArranged(loopedWindow(prev, p.window.Milliseconds()), tile, w)
}

// handOff is window w's tile in the horizon after the season simulation, walked from `from`, the
// season's last decided window. The dealer repaired each deal's seam against the deal before it,
// but the season aired its own tiles, so a window is repaired again against the windows before it
// as they really aired while any of them aired otherwise than its deal (#1939). Once the season's
// last tile has passed out of the spacing, windows air their deals as dealt.
//
// The repair reads back over every window the repeat spacing spans, not only the window before: a
// 12-hour spacing on a 6-hour window reaches two windows back (#1960).
func (p *holidayPlanner) handOff(from, w int64, dealt []Slot) []Slot {
	span := p.spanWindows()
	var hist [][]Slot  // the last `span` windows as they aired, oldest first
	var offDeal []bool // whether each aired a tile other than its deal
	for v := from - int64(span) + 1; v <= from; v++ {
		tile := p.tile(v, p.ring.tile(v))
		hist, offDeal = append(hist, p.arrange(tile, v)), append(offDeal, !sameProgrammes(tile, p.ring.tile(v)))
	}
	for v := from + 1; ; v++ {
		deal := dealt
		if v < w {
			deal = p.ring.tile(v)
		}
		tile := deal
		if slices.Contains(offDeal, true) {
			var prev []Slot
			for _, a := range hist {
				prev = append(prev, loopedWindow(a, p.window.Milliseconds())...)
			}
			tile = p.repairArranged(prev, deal, v)
		}
		if v == w {
			return tile
		}
		hist, offDeal = append(hist[1:], p.arrange(tile, v)), append(offDeal[1:], !sameProgrammes(tile, deal))
	}
}

// spanWindows is how many windows the repeat spacing reaches back over, one at least.
func (p *holidayPlanner) spanWindows() int {
	w := p.window.Milliseconds()
	return max(int((p.ring.d.spacing.Milliseconds()+w-1)/w), 1)
}

// repairArranged is window w's `tile` with its seam repaired against `prev`, what aired before it
// (each window looped as playout airs it). Arrange does not only delay a tile's units: in an
// evening it defers an off-season holiday episode past the band, which brings the units behind it
// forward, and so brought a unit held back from the seam to the window's head, next to where the
// window before closed on it (#1939). The seam is repaired on the order arrange airs the tile in
// (its features left out); a unit arrange does not air keeps its place at the end.
func (p *holidayPlanner) repairArranged(prev, tile []Slot, w int64) []Slot {
	inTile := map[string]bool{}
	for _, s := range tile {
		inTile[unitKey(s)] = true
	}
	order := make([]Slot, 0, len(tile))
	for _, s := range p.arrange(tile, w) {
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
		prev = record(p.arrange(p.ring.tile(v), v))
	}
	run := map[int64][]Slot{}
	var owed []Slot // dealt in season but cut for the features, in deal order
	owing := map[string]bool{}
	for v := anchor; v <= end; v++ {
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
// most-rested units it has rather than a near-empty tile looped all day (#1939).
func (p *holidayPlanner) settle(tile []Slot, need, v, end, a, spacing int64, prev, pooled map[string]bool, owed []Slot, airedAt map[string]int64) []Slot {
	dropped, kept := map[string]bool{}, map[string]bool{}
	for {
		mine := map[string]bool{}
		for _, s := range tile {
			if s.IsProgram() {
				mine[unitKey(s)] = true
			}
		}
		var i int64
		soon := ""
		for _, s := range p.arrange(tile, v) {
			if !s.IsProgram() {
				continue
			}
			key := unitKey(s)
			if last, ok := airedAt[key]; ok && mine[key] && !kept[key] && a+i-last < spacing {
				soon = key
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
			kept[soon] = true
			continue
		}
		tile = without
	}
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
// the dealer airs it); owed; the farthest from both.
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
		tier       int // 0: rested both ways; 1: rested since, and its next deal is simulated; 2: neither
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
		c := candidate{s: s, dist: min(past, future), near: prev[key] || next[key], owed: i < len(owed), tier: 2}
		switch {
		case past >= spacing && future >= spacing:
			c.tier = 0
		case past >= spacing && dealt < simulated:
			// Too soon for its next deal, which the simulation will skip: a skip, not a repeat.
			c.tier = 1
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
// outside its spacing then re-aired inside it. So an episode is deferred only when every unit it
// brings forward still rests the dealer's spacing from its last airing (or no less than its deal
// gave it), and the episode still rests it before its own next one (costsSpacing); otherwise it
// airs in place, as it does when the window closes before the band does. Arrange never moves an ordinary unit later on such a channel, so a unit's
// last airing is never later than its dealt one, and checking against the deal is exact.
//
// In order, the dealer promises no spacing: an episode the window cannot defer past the band is
// moved, with any others, to just before the band (beforeBand). Only a window wholly inside the
// band airs it there.
func (p *holidayPlanner) arrange(tile []Slot, w int64) []Slot {
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
	spacing := p.ring.d.spacing
	inPlace := map[string]bool{} // off-season episodes that air where they are
	for {
		out, deferred := p.place(tile, w, pooled, func(s Slot) bool { return offSeason(s) && !inPlace[unitKey(s)] })
		switch {
		case spacing <= 0:
			return p.beforeBand(out, deferred, opening, offSeason)
		case len(deferred) > 0:
			// The window closes inside the band: what it deferred would only move to its end.
			for _, s := range deferred {
				inPlace[unitKey(s)] = true
			}
			continue
		}
		key := p.costsSpacing(tile, out, w, pooled, offSeason, inPlace)
		if key == "" {
			return out
		}
		inPlace[key] = true
	}
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
// the unit back a slot.
func (p *holidayPlanner) costsSpacing(tile, out []Slot, w int64, pooled map[string]bool, offSeason func(Slot) bool, inPlace map[string]bool) string {
	before, _ := p.place(tile, w, pooled, func(Slot) bool { return false })
	dealt, airs := p.airedIn(before, w), p.airedIn(out, w)
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
		if was, ok := p.nextGap(key, w, dealt); ok {
			if now, _ := p.nextGap(key, w, airs); now < min(spacing, was) {
				return key
			}
		}
		deferred = append(deferred, key)
	}
	for _, s := range before {
		key := unitKey(s)
		if !moved(s) || airs.first(key) >= dealt.first(key) {
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

// dealtIn is window v airing its deal unmoved. Out of season arrange never moves an ordinary unit
// later on a reshuffled channel, so a unit's real airing there is never later than this.
func (p *holidayPlanner) dealtIn(v int64) windowAirings {
	if got, ok := p.dealtAt[v]; ok {
		return got
	}
	got := p.airedIn(p.ring.tile(v), v)
	p.dealtAt[v] = got
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
// last airing in window w (as `in` airs it), within the spacing's reach.
func (p *holidayPlanner) nextGap(key string, w int64, in windowAirings) (int64, bool) {
	at := in.at[key]
	gap := in.n - at[len(at)-1]
	for v := w + 1; v <= w+p.spacingWindows(); v++ {
		d := p.dealtIn(v)
		if next := d.at[key]; len(next) > 0 {
			return gap + next[0], true
		}
		gap += d.n
	}
	return 0, false
}

// beforeBand inserts the units the window could not air outside the evening, as one run, just
// before the band the window ends in: at the last position where every one of them still starts
// before 17:00. What does not fit before the band (a window wholly inside it has no room at all)
// airs in place. An out-of-season unit after the insertion joins the run, since the insertion
// would push it into the band.
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

// HolidayEpisodes summarises one selected holiday's episode feature on a channel: how many holiday
// episodes its series hold and how the current strength rotates them. It is the backend's answer to
// the seasonal control's "what will this do" line, so the page never counts episodes itself.
type HolidayEpisodes struct {
	Holiday   string // built-in holiday id
	Episodes  int    // distinct holiday episodes (a multi-part story counts once)
	PerDay    int    // featured on an ordinary day of the window
	EveryDays int    // the soonest one of them airs again
	Active    bool   // the holiday's window is open on the channel's date at the evaluated instant
	// Capped is true when the pool or the quarter-of-the-day cap holds the strength back: fewer a
	// day than it asks for, or not every day.
	Capped bool
	// SameAs lists the other strengths that rotate this pool identically, so the page can say a
	// choice would change nothing.
	SameAs []HolidayStrength
}

var holidayStrengths = []HolidayStrength{HolidayStrengthLight, HolidayStrengthFeatured, HolidayStrengthHeavy}

// summary lists the channel's selected holidays, open or soonest first, as of the channel's
// date at `now`.
func (p *holidayPlanner) summary(now time.Time) []HolidayEpisodes {
	if p.f.seasonal.Mode != SeasonalAuto || now.IsZero() {
		return nil
	}
	today := civilNoon(now, p.loc)
	dayCap := p.dayCap(24 * time.Hour)
	var out []HolidayEpisodes
	for _, w := range UpcomingHolidays(today, 366*24*time.Hour) {
		if !p.f.selected(w.ID) {
			continue
		}
		n := len(p.f.holidayPool(p.deck, map[string]bool{w.ID: true}))
		perDay, every := cappedRotation(n, p.f.seasonal.Strength, dayCap)
		h := HolidayEpisodes{
			Holiday: w.ID, Episodes: n, PerDay: perDay, EveryDays: every, Active: !w.Start.After(today),
			Capped: n > 0 && (perDay < p.f.seasonal.Strength.dailyQuota() || n < perDay*every),
		}
		for _, s := range holidayStrengths {
			if s == p.f.seasonal.Strength {
				continue
			}
			if pd, ev := cappedRotation(n, s, dayCap); pd == perDay && ev == every {
				h.SameAs = append(h.SameAs, s)
			}
		}
		out = append(out, h)
	}
	return out
}
