package schedule

import (
	"strings"
	"time"
)

// Seasonality (programming-design §6): holiday-aware programming. Detection is
// deterministic and data-driven — an entry is seasonal for holiday H if it matches
// H's keyword set. Evaluation is against the container wall-clock `now` at reconcile
// time; the periodic sweep naturally rolls channels in/out of windows, so
// seasonality needs no scheduler of its own.
//
// Two symmetric behaviors, because knowing what October wants implies knowing what
// July doesn't: in `auto` mode, in-window seasonal items are BOOSTED and
// out-of-window detected-seasonal items are BENCHED (the half viewers notice —
// Christmas episodes in July break the spell). A zero `now` disables all of it.

// holiday is one entry in the built-in calendar (§6). Windows + keyword sets ship
// as DATA, not code; custom/regional holidays are future work.
type holiday struct {
	id       string
	keywords []string
	// window reports the holiday's [start, end] for a given year (some windows
	// straddle a year boundary, e.g. New Year).
	window func(year int) (start, end time.Time)
}

// builtinCalendar is the v1 holiday set (§6). Keyword matching is case-insensitive
// substring against an entry's TITLE — never its genres (see isSeasonal for why).
var builtinCalendar = []holiday{
	{
		id:       "halloween",
		keywords: []string{"halloween", "spooky", "haunted", "ghost", "witch", "horror"},
		window:   fixedWindow(time.October, 1, time.October, 31),
	},
	{
		id:       "thanksgiving",
		keywords: []string{"thanksgiving", "turkey day"},
		window:   fixedWindow(time.November, 15, time.November, 30),
	},
	{
		id:       "christmas",
		keywords: []string{"christmas", "xmas", "santa", "holiday", "noel", "yuletide"},
		window:   fixedWindow(time.December, 1, time.December, 26),
	},
	{
		id:       "newyear",
		keywords: []string{"new year", "new year's", "hogmanay"},
		window:   newYearWindow,
	},
	{
		id:       "valentines",
		keywords: []string{"valentine", "valentine's"},
		window:   fixedWindow(time.February, 1, time.February, 14),
	},
}

// fixedWindow builds a same-year [start, end] window (inclusive of the end day).
func fixedWindow(startMonth time.Month, startDay int, endMonth time.Month, endDay int) func(int) (time.Time, time.Time) {
	return func(year int) (time.Time, time.Time) {
		start := time.Date(year, startMonth, startDay, 0, 0, 0, 0, time.UTC)
		end := time.Date(year, endMonth, endDay, 23, 59, 59, 0, time.UTC)
		return start, end
	}
}

// newYearWindow straddles the year boundary: Dec 27 (year) → Jan 2 (year+1). For a
// given evaluation year we return the window that could contain a late-Dec or
// early-Jan date; inWindow checks both the current and prior year's window.
func newYearWindow(year int) (time.Time, time.Time) {
	start := time.Date(year, time.December, 27, 0, 0, 0, 0, time.UTC)
	end := time.Date(year+1, time.January, 2, 23, 59, 59, 0, time.UTC)
	return start, end
}

// activeHolidays returns the calendar holidays whose window contains `now`, limited
// to the channel's selected subset (empty subset = all built-in holidays).
func activeHolidays(now time.Time, selected []string) []holiday {
	if now.IsZero() {
		return nil
	}
	sel := lowerSet(selected)
	var active []holiday
	for _, h := range builtinCalendar {
		if len(sel) > 0 {
			if _, ok := sel[h.id]; !ok {
				continue
			}
		}
		if holidayActive(h, now) {
			active = append(active, h)
		}
	}
	return active
}

// holidayActive reports whether now falls in h's window, checking both the current
// and prior evaluation year so a year-straddling window (New Year) matches on both
// sides of the boundary.
func holidayActive(h holiday, now time.Time) bool {
	for _, y := range []int{now.Year(), now.Year() - 1} {
		start, end := h.window(y)
		if !now.Before(start) && !now.After(end) {
			return true
		}
	}
	return false
}

// isSeasonal reports whether an entry matches ANY holiday's keyword set (detection,
// §6). Matching is case-insensitive substring against the entry's TITLE.
//
// ⚠ GENRES ARE DELIBERATELY NOT MATCHED, and this is a correction. Haying over
// title + genres conflated "this title is ABOUT a holiday" with "this title is in a
// genre that correlates with one" — different claims, and only the first is something
// a holiday window should act on.
//
// The failure was total, not cosmetic: `horror` is in the Halloween keyword set, so on
// a year-round horror channel EVERY entry detected as seasonal and `auto` mode benched
// all of them `out_of_season` for the eleven months outside October. The channel was
// legal to configure, reported `live`, and aired nothing.
//
// A genre is what a channel IS; a holiday keyword is what a title is ABOUT. §6's own
// example — "Christmas episodes in July break the spell" — is a title, and this
// restores that reading. Dropping the one genre-shaped keyword would have fixed this
// channel and left the mechanism to re-fire on the next collision (Romance near
// Valentine's, Family near Christmas).
func isSeasonal(e LineupEntry, holidays []holiday) bool {
	hay := strings.ToLower(e.Title)
	for _, h := range holidays {
		for _, kw := range h.keywords {
			if strings.Contains(hay, kw) {
				return true
			}
		}
	}
	return false
}

// isSeasonalForAny reports whether an entry is seasonal for ANY built-in holiday
// (regardless of window) — used to decide the OUT-of-window bench: a detected
// seasonal item that matches no ACTIVE holiday is benched (§6).
func isSeasonalForAny(e LineupEntry) bool {
	return isSeasonal(e, builtinCalendar)
}

// applySeasonal enforces the seasonal policy (§6) over the eligible entries at the
// given clock. It returns the surviving entries (out-of-window seasonal items
// benched) and a report of what was benched. A zero clock or SeasonalOff ⇒ no-op.
//
//   - off:        no detection, no bench.
//   - auto:       bench out-of-window seasonal items; in-window seasonal items are
//     kept (a scheduling BOOST is applied by duplicating them lightly so
//     the deck weights them up — tasteful, not wall-to-wall).
//   - exclusive:  in-window, keep ONLY items seasonal for an active holiday; out of
//     window, run the offSeason fallback (loop the scope w/o the filter,
//     or dark). The dark case returns an empty set.
func applySeasonal(entries []LineupEntry, rp ResolvedPolicy, now time.Time) ([]LineupEntry, ExclusionReport) {
	var report ExclusionReport
	if now.IsZero() || rp.Seasonal.Mode == SeasonalOff {
		return entries, report
	}
	active := activeHolidays(now, rp.Seasonal.Holidays)
	inWindow := len(active) > 0

	switch rp.Seasonal.Mode {
	case SeasonalExclusive:
		if !inWindow {
			if rp.Seasonal.OffSeason == OffSeasonDark {
				// Bench everything — the channel goes dark out of window.
				for _, e := range entries {
					report.add(e, "out_of_season")
				}
				return nil, report
			}
			// loop: run the full scope with no seasonal filter.
			return entries, report
		}
		// In window: keep only items seasonal for an active holiday.
		kept := entries[:0:0]
		for _, e := range entries {
			if isSeasonal(e, active) {
				kept = append(kept, e)
			} else {
				report.add(e, "out_of_season")
			}
		}
		return kept, report

	default: // SeasonalAuto (and any resolved default)
		kept := make([]LineupEntry, 0, len(entries))
		for _, e := range entries {
			// Bench a detected-seasonal item that is NOT for an active holiday
			// (Christmas in July). A non-seasonal item always passes.
			if isSeasonalForAny(e) && !isSeasonal(e, active) {
				report.add(e, "out_of_season")
				continue
			}
			kept = append(kept, e)
			// Boost: an in-window seasonal item gets a light weighting bump so the
			// deck features it (§6 "tasteful, not wall-to-wall") — one extra copy.
			if inWindow && isSeasonal(e, active) {
				kept = append(kept, e)
			}
		}
		return kept, report
	}
}

// lowerSet builds a lowercased set from a slice, or nil for an empty slice.
func lowerSet(ss []string) map[string]struct{} {
	if len(ss) == 0 {
		return nil
	}
	m := make(map[string]struct{}, len(ss))
	for _, s := range ss {
		m[strings.ToLower(s)] = struct{}{}
	}
	return m
}
