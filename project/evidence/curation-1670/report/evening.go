package main

import "time"

// eveningCut learns where the household's day turns over from its own viewing: the local hour
// with the least time watched across the span (#4140, "learned evening"). An evening is then the
// stretch from that hour to the same hour the next day, so it ends where nobody is watching and
// never splits one sitting in two. Ties go to the first quiet hour after the busiest one, so the
// cut follows the evening rather than preceding it.
//
// With no viewings there is nothing to learn from, and the cut is midnight; learned reports which.
func eveningCut(viewings []Viewing, from, to time.Time, loc *time.Location) (hour int, learned bool) {
	var watched [24]time.Duration
	for _, v := range viewings {
		start, end := v.Started, v.Ended
		if start.Before(from) {
			start = from
		}
		if end.After(to) {
			end = to
		}
		for t := start; t.Before(end); {
			local := t.In(loc)
			next := time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), 0, 0, 0, loc).Add(time.Hour)
			if next.After(end) {
				next = end
			}
			watched[local.Hour()] += next.Sub(t)
			t = next
		}
	}
	busiest := 0
	for h := range watched {
		if watched[h] > watched[busiest] {
			busiest = h
		}
	}
	if watched[busiest] == 0 {
		return 0, false
	}
	hour = (busiest + 1) % 24
	for i := 1; i < 24; i++ {
		if h := (busiest + i) % 24; watched[h] < watched[hour] {
			hour = h
		}
	}
	return hour, true
}

// evenings splits the household's watched commercial-break airings into viewing evenings, the
// unit of #3838 decision 1 ("no ad repeats within one evening"). Airings arrive sorted by time and
// come from every channel: rotation is household-wide, so an ad seen on one channel and again on
// another in the same evening is a repeat. An evening runs from the cut hour to the same hour the
// next day (eveningCut).
//
// Each returned evening holds its airings in time order; an empty input returns no evenings.
func evenings(airings []ClipAiring, loc *time.Location, cut int) [][]ClipAiring {
	var out [][]ClipAiring
	var current time.Time
	for _, a := range airings {
		day := eveningOf(a.AiredAt, loc, cut)
		if len(out) == 0 || !day.Equal(current) {
			out = append(out, nil)
			current = day
		}
		out[len(out)-1] = append(out[len(out)-1], a)
	}
	return out
}

// eveningOf is the local date, at midnight, of the evening t falls in.
func eveningOf(t time.Time, loc *time.Location, cut int) time.Time {
	local := t.In(loc)
	if local.Hour() < cut {
		local = local.AddDate(0, 0, -1)
	}
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
}
