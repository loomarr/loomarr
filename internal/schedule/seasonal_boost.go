package schedule

import (
	"slices"
	"time"
)

// Holiday episodes inside an ordinary series (§6 "Holiday episodes", #1670).
//
// Title-level seasonality (applySeasonal) cannot see that "The Simpsons" holds nine "Treehouse of
// Horror" episodes: the entry's title is not about Halloween, so an October auto channel aired them
// only when the deck happened to deal one. This pass features them. During an active holiday window
// it takes the window's tile of the deck, pulls every holiday episode out of it, and puts back a
// ROTATING few per day, timed into the evening band (or spread across the day). Out of season, with
// evenings favoured, a holiday episode is never benched but is kept out of the evening band.
//
// It runs on the window's tile, AFTER the deck is dealt and sliced, so the deck itself never
// changes: the boost cannot reshuffle the channel's rotation. Its inputs are the deck, the tile,
// the window index and zone, and the policy, so a window arranges identically on every reconcile
// (#1674/#1675), and the cycle cache's key (ClockSignature + window index) stays complete.

const (
	eveningFromHour = 17 // the evening band, channel-local: 17:00 ≤ start < 23:00
	eveningToHour   = 23
	// minHolidayRepeatDays is the in-season exception to EpisodeNoRepeat: a featured holiday
	// episode never airs twice in a day nor on consecutive days, and otherwise rotates evenly
	// through the pool. The channel's ordinary 7-day spacing would cap a nine-episode pool at
	// one a day, at every strength.
	minHolidayRepeatDays = 2
	// holidayShareOfDay caps the feature at 1/holidayShareOfDay of a day's programmes, so the
	// rest of the lineup keeps at least three quarters of the day.
	holidayShareOfDay = 4
	// featuredHolidayDayMax is how many a Featured channel airs on the holiday's own date.
	featuredHolidayDayMax = 5
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

// rotationPicks returns the pool positions featured on civil day `day`. Consecutive days walk the
// pool as a ring, perDay at a time, so an episode's next airing is at least ⌊pool/perDay⌋ days
// later (≥ everyDays by HolidayRotation's construction).
func rotationPicks(pool, perDay, everyDays int, day int64, holidayDay int) []int {
	if pool == 0 {
		return nil
	}
	n := perDay
	if holidayDay > 0 {
		n = holidayDay
	} else if pool < perDay*everyDays && day%int64(everyDays) != 0 {
		return nil // a pool too small for one a day airs every everyDays days
	}
	start := int((day*int64(perDay))%int64(pool)+int64(pool)) % pool
	picks := make([]int, 0, n)
	for k := range n {
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

// featureHolidayEpisodes rearranges one window's tile (see the file comment). breakFactor
// stretches programme runtime by the channel's break density so the estimated start times land
// where the guide will show them. A window ≤ 0 has no wall-clock anchor, so it is left alone.
func featureHolidayEpisodes(deck, slice []Slot, f holidayFeature, window time.Duration, windowIdx int64, zone *time.Location, breakFactor float64) []Slot {
	if f.seasonal.Mode != SeasonalAuto || window <= 0 || len(f.matches) == 0 || len(slice) == 0 {
		return slice
	}
	loc := zone
	if loc == nil {
		loc = time.UTC
	}
	opening := windowOpening(windowIdx, window, zone)
	closing := opening.Add(window)
	advance := func(t time.Time, s Slot) time.Time {
		return t.Add(time.Duration(float64(s.DurationMs)*breakFactor) * time.Millisecond)
	}

	// Programmes per civil date in the tile as dealt: the base for the 1/4-of-the-day cap.
	perDate := map[int64]int{}
	for t, i := opening, 0; i < len(slice); i++ {
		if slice[i].IsProgram() {
			y, m, d := t.In(loc).Date()
			perDate[civilDay(y, m, d)]++
			t = advance(t, slice[i])
		}
	}

	type feature struct {
		at   time.Time
		unit Slot
	}
	var features []feature
	pooled := map[string]bool{} // in an active pool: airs only where the rotation features it
	y, m, d := opening.In(loc).Date()
	for day := time.Date(y, m, d, 0, 0, 0, 0, loc); day.Before(closing); day = time.Date(day.Year(), day.Month(), day.Day()+1, 0, 0, 0, 0, loc) {
		y, m, d := day.Date()
		noon := time.Date(y, m, d, 12, 0, 0, 0, time.UTC)
		active, holidayDay := map[string]bool{}, false
		for _, h := range builtinCalendar {
			if f.selected(h.id) && holidayActive(h, noon) {
				active[h.id] = true
				if hd := h.day(y); hd.Month() == m && hd.Day() == d {
					holidayDay = true
				}
			}
		}
		if len(active) == 0 {
			continue
		}
		pool := f.holidayPool(deck, active)
		for _, u := range pool {
			pooled[unitKey(u)] = true
		}
		perDay, everyDays := HolidayRotation(len(pool), f.seasonal.Strength)
		onHoliday := 0
		if holidayDay {
			onHoliday = f.seasonal.Strength.holidayDayQuota(len(pool))
		}
		picks := rotationPicks(len(pool), perDay, everyDays, civilDay(y, m, d), onHoliday)
		picks = picks[:min(len(picks), max(1, perDate[civilDay(y, m, d)]/holidayShareOfDay))]

		from, to := day, time.Date(y, m, d+1, 0, 0, 0, 0, loc)
		if f.seasonal.Evenings {
			from, to = time.Date(y, m, d, eveningFromHour, 0, 0, 0, loc), time.Date(y, m, d, eveningToHour, 0, 0, 0, loc)
		}
		if from.Before(opening) {
			from = opening
		}
		if to.After(closing) {
			to = closing
		}
		if !to.After(from) || len(picks) == 0 {
			continue
		}
		// Evenly spaced through the band: Featured's three land early, mid and late evening.
		span := to.Sub(from)
		for k, p := range picks {
			features = append(features, feature{at: from.Add(span * time.Duration(2*k+1) / time.Duration(2*len(picks))), unit: pool[p]})
		}
	}

	// Out of season (a selected holiday whose window is closed), a holiday episode keeps its
	// turn but not the evening: with evenings favoured it is deferred past the band.
	offSeason := func(s Slot) bool {
		if !f.seasonal.Evenings || pooled[unitKey(s)] {
			return false
		}
		for _, id := range f.unitHolidays(s) {
			if f.selected(id) {
				return true
			}
		}
		return false
	}
	if len(pooled) == 0 && !slices.ContainsFunc(slice, offSeason) {
		return slice
	}

	var queue []Slot
	inTile := map[string]bool{}
	for _, s := range slice {
		inTile[unitKey(s)] = true
		if !pooled[unitKey(s)] {
			queue = append(queue, s)
		}
	}
	// Top-up: when pulling holiday episodes shortens the tile, continue from where it ended.
	next := 0
	for i, s := range deck {
		if s.IsProgram() && inTile[unitKey(s)] {
			next = i + 1
		}
	}
	topUp := func() (Slot, bool) {
		for ; next < len(deck)+len(deck); next++ {
			s := deck[next%len(deck)]
			if s.IsProgram() && !pooled[unitKey(s)] && !inTile[unitKey(s)] {
				inTile[unitKey(s)] = true
				next++
				return s, true
			}
		}
		return Slot{}, false
	}
	inEvening := func(t time.Time) bool {
		h := t.In(loc).Hour()
		return h >= eveningFromHour && h < eveningToHour
	}

	out := make([]Slot, 0, len(slice)+len(features))
	budget := window.Milliseconds()
	t, fi := opening, 0
	var acc int64
	emit := func(s Slot) {
		out = append(out, s)
		if s.IsProgram() {
			acc += s.DurationMs
			t = advance(t, s)
		}
	}
	for acc < budget {
		if fi < len(features) && !t.Before(features[fi].at) {
			emit(features[fi].unit)
			fi++
			continue
		}
		if len(queue) == 0 {
			s, ok := topUp()
			if !ok {
				if fi < len(features) {
					emit(features[fi].unit)
					fi++
					continue
				}
				break
			}
			queue = append(queue, s)
		}
		pick := 0
		if inEvening(t) {
			eligible := func(s Slot) bool { return !offSeason(s) }
			i := slices.IndexFunc(queue, eligible)
			for i < 0 {
				s, ok := topUp()
				if !ok {
					break
				}
				if queue = append(queue, s); eligible(s) {
					i = len(queue) - 1
				}
			}
			pick = max(i, 0)
		}
		emit(queue[pick])
		queue = slices.Delete(queue, pick, pick+1)
	}
	return out
}

// HolidayEpisodes summarises one selected holiday's episode feature on a channel: how many holiday
// episodes its series hold and how the current strength rotates them. It is the backend's answer to
// the seasonal control's "what will this do" line, so the page never counts episodes itself.
type HolidayEpisodes struct {
	Holiday   string // built-in holiday id
	Episodes  int    // distinct holiday episodes (a multi-part story counts once)
	PerDay    int    // featured on an ordinary day of the window
	EveryDays int    // the soonest one of them airs again
	Active    bool   // the holiday's window is open at the evaluated instant
}

// holidayEpisodeSummary lists the channel's selected holidays, open or soonest first.
func holidayEpisodeSummary(deck []Slot, f holidayFeature, now time.Time) []HolidayEpisodes {
	if f.seasonal.Mode != SeasonalAuto || now.IsZero() {
		return nil
	}
	var out []HolidayEpisodes
	for _, w := range UpcomingHolidays(now, 366*24*time.Hour) {
		if !f.selected(w.ID) {
			continue
		}
		n := len(f.holidayPool(deck, map[string]bool{w.ID: true}))
		perDay, every := HolidayRotation(n, f.seasonal.Strength)
		out = append(out, HolidayEpisodes{
			Holiday: w.ID, Episodes: n, PerDay: perDay, EveryDays: every, Active: !w.Start.After(now),
		})
	}
	return out
}
