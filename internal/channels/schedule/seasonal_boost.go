package schedule

import (
	"slices"

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
	cal         Calendar // the holidays the channel keeps; loc is its zone, defaulted
	breakFactor float64
	meanMs      float64 // mean programme runtime in the deck, the base of the day cap
	raw         map[int64]*dayPlan
	plans       map[int64]*dayPlan
	base        map[int64][]Slot           // the ring's ordinary-day picks, before neighbour drops
	full        map[int64][]Slot           // what the date features after its holiday-date top-up
	dealtAt     map[int64]windowAirings    // dealtIn's memo
	tiles       map[int64][]Slot           // dealtTile's memo
	farAt       map[int64]bool             // farFromSeason's memo
	lastAt      map[int64]int64            // lastInSeason's memo
	handOffs    map[int64]map[int64][]Slot // handOffRun's memo when its run is not shared
}

// newHolidayPlanner prepares the pass. breakFactor stretches programme runtime by the channel's
// break density so the estimated start times land where the guide will show them.
func newHolidayPlanner(ring *dealtRing, f holidayFeature, window time.Duration, cal Calendar, breakFactor float64) *holidayPlanner {
	zone := cal.Zone
	if zone == nil {
		zone = time.UTC
	}
	deck := ring.d.deck(0)
	p := &holidayPlanner{deck: deck, ring: ring, f: f, window: window, loc: zone, cal: cal, breakFactor: breakFactor,
		raw: map[int64]*dayPlan{}, plans: map[int64]*dayPlan{}, base: map[int64][]Slot{}, full: map[int64][]Slot{}, dealtAt: map[int64]windowAirings{},
		tiles: map[int64][]Slot{}, farAt: map[int64]bool{}, lastAt: map[int64]int64{}, handOffs: map[int64]map[int64][]Slot{}}
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
	for _, h := range p.cal.holidays() {
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
// under per-pass reshuffle drift from it (#1939). The windows after that hand off to the dealer
// (handOff); every other window airs the dealer's tile, which arrange keeps to the spacing.
func (p *holidayPlanner) tile(w int64, dealt []Slot) []Slot {
	if !p.enabled() || p.ring.stride == 0 {
		return dealt
	}
	h, last := p.horizon(), p.lastInSeason(w)
	switch {
	case last < w-h-p.handOffReach():
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
	return p.repairArranged(loopedWindow(prev, p.window.Milliseconds()), tile, p.arrangeTile(tile, w))
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
	for _, w := range UpcomingHolidays(today, 366*24*time.Hour, p.cal) {
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
