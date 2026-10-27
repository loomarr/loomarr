package schedule

import (
	"sort"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/holidayvocab"
)

// Seasonality (programming-design §6): holiday-aware programming. Detection is
// holidayvocab's judgment of whether an entry is about holiday H (`about`) or only
// fits its mood (`adjacent`). Evaluation is against the container wall-clock `now` at reconcile
// time; the periodic sweep naturally rolls channels in/out of windows, so
// seasonality needs no scheduler of its own.
//
// Two symmetric behaviors, because knowing what October wants implies knowing what
// July doesn't: in `auto` mode, in-window seasonal items are BOOSTED and
// out-of-window detected-seasonal items are BENCHED (the half viewers notice —
// Christmas episodes in July break the spell). A zero `now` disables all of it.

// holiday is one holiday as the household's country keeps it (§6, #2030). Its dates are data in
// holidayvocab's per-country table, beside the words that detect it.
type holiday struct {
	id string
	// window reports the holiday's [start, end] for a given year (some windows
	// straddle a year boundary, e.g. New Year).
	window func(year int) (start, end time.Time)
	// day is the holiday's own date in a given year (the night a Heavy channel runs its
	// holiday-episode marathon, §6 "Holiday episodes"). Only month and day are read.
	day func(year int) time.Time
	// vocab is the definitions the holiday came from, which judge whether a title is about it.
	vocab holidayvocab.Vocabulary
}

// Calendar is the household's holiday calendar: the holiday definitions (Holidays, nil = the
// shipped ones), the ones its country keeps (filler.home_country, "" = the US), on its wall clock
// (guide.timezone, nil = UTC). Programming, the on-screen logo and filler all date and judge
// holidays through one, so they agree on what and when a holiday is. holidayvocab judges
// membership; a genre is never evidence on its own (see isSeasonal).
type Calendar struct {
	Zone     *time.Location
	Country  string
	Holidays *holidayvocab.Vocabulary
}

// Vocab is the calendar's holiday definitions.
func (c Calendar) Vocab() holidayvocab.Vocabulary {
	if c.Holidays == nil {
		return holidayvocab.Default()
	}
	return *c.Holidays
}

func (c Calendar) holidays() []holiday {
	v := c.Vocab()
	kept := v.Calendar(c.Country)
	out := make([]holiday, len(kept))
	for i, o := range kept {
		out[i] = holiday{id: o.ID, window: o.Window, day: o.Day, vocab: v}
	}
	return out
}

// HolidayWindow is one built-in holiday's window, inclusive of End.
type HolidayWindow struct {
	ID         string
	Start, End time.Time
}

// UpcomingHolidays lists the calendar's holidays whose window is on at `now` or starts within
// `horizon`, soonest first (#1665 channel ideas). It reads the same calendar the seasonal policy
// does, so an idea never announces a holiday the seasonal policy wouldn't honour.
func UpcomingHolidays(now time.Time, horizon time.Duration, cal Calendar) []HolidayWindow {
	var out []HolidayWindow
	for _, h := range cal.holidays() {
		for _, y := range []int{now.Year() - 1, now.Year(), now.Year() + 1} {
			start, end := h.window(y)
			if !end.Before(now) && !start.After(now.Add(horizon)) {
				out = append(out, HolidayWindow{ID: h.id, Start: start, End: end})
				break
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out
}

// GenericHolidayTag is the seasonal taxon for a clip that names no one holiday ("happy holidays"):
// it is in season whenever a winter holiday, Thanksgiving through New Year, is (#2030).
const GenericHolidayTag = "holiday"

// civilNoon is noon UTC of `now`'s calendar date on zone's wall clock (nil = UTC): the instant a
// holiday window, which is a run of whole dates, is evaluated at. Evaluating the instant itself
// would end Halloween at 8 pm on Oct 31 in New York.
func civilNoon(now time.Time, zone *time.Location) time.Time {
	if zone == nil {
		zone = time.UTC
	}
	y, m, d := now.In(zone).Date()
	return time.Date(y, m, d, 12, 0, 0, 0, time.UTC)
}

// HonouredHolidays lists the calendar holidays a channel honours, soonest first, each with its
// window that is on at `now` or comes next (#1659 W2c, the On-screen logo's holiday rows). A channel
// whose seasonal mode is off (Ignore holidays) honours none; a chosen subset limits the list. Like
// activeHolidays it dates `now` on the calendar's wall clock, so a row is on exactly while
// ActiveHolidayAt names it.
func HonouredHolidays(now time.Time, seasonal SeasonalPolicy, cal Calendar) []HolidayWindow {
	if seasonal.Mode == SeasonalOff {
		return nil
	}
	now = civilNoon(now, cal.Zone)
	sel := lowerSet(seasonal.Holidays)
	var out []HolidayWindow
	for _, h := range cal.holidays() {
		if _, ok := sel[h.id]; len(sel) > 0 && !ok {
			continue
		}
		for _, y := range []int{now.Year() - 1, now.Year(), now.Year() + 1} {
			if start, end := h.window(y); !end.Before(now) {
				out = append(out, HolidayWindow{ID: h.id, Start: start, End: end})
				break
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out
}

// ActiveHolidayAt is the calendar holiday a channel honours at t, or "": the holiday the scheduler
// boosts, through the same calendar, selection and channel-local evaluation, so the on-screen
// logo's holiday version (WatermarkHoliday) flips at the same local instant as the programming. A
// channel whose seasonal mode is off (Ignore holidays) honours none.
func ActiveHolidayAt(t time.Time, seasonal SeasonalPolicy, cal Calendar) string {
	if seasonal.Mode == SeasonalOff {
		return ""
	}
	if active := activeHolidays(t, cal, seasonal.Holidays); len(active) > 0 {
		return active[0].id
	}
	return ""
}

// activeHolidays returns the calendar holidays whose window contains `now`'s date on the
// calendar's wall clock, limited to the channel's selected subset (empty = all it keeps).
func activeHolidays(now time.Time, cal Calendar, selected []string) []holiday {
	if now.IsZero() {
		return nil
	}
	now = civilNoon(now, cal.Zone)
	sel := lowerSet(selected)
	var active []holiday
	for _, h := range cal.holidays() {
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

// isSeasonal reports whether an entry is ABOUT any of the holidays (detection, §6):
// holidayvocab's `about` verdict over the entry's title. Bench and boost act on this
// alone, because a false bench silently removes programming: a mood word such as
// "ghost" never benches (an action film named "Ghost …" airs all year), even with
// genres that make it `adjacent`.
//
// ⚠ A GENRE IS NEVER EVIDENCE ON ITS OWN, and this is a correction: genres only
// corroborate a mood word into `adjacent`, which bench and boost ignore. Haying over
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
	for _, h := range holidays {
		if judgeEntry(e, h).Verdict == holidayvocab.VerdictAbout {
			return true
		}
	}
	return false
}

// fitsSeason reports whether an entry belongs on an exclusive holiday channel in
// window: about the holiday, or `adjacent` (a mood word the genres corroborate).
func fitsSeason(e LineupEntry, holidays []holiday) bool {
	for _, h := range holidays {
		if judgeEntry(e, h).Fits() {
			return true
		}
	}
	return false
}

func judgeEntry(e LineupEntry, h holiday) holidayvocab.Judgment {
	return h.vocab.Judge(h.id, holidayvocab.Evidence{Title: e.Title, Genres: e.Genres})
}

// benchesForAny reports whether an entry benches for ANY holiday the calendar keeps
// (regardless of window) — used to decide the OUT-of-window bench: a detected
// seasonal item that matches no ACTIVE holiday is benched (§6). A name alone does not
// bench for a holiday whose name is also an ordinary title (Judgment.Benches, #2030),
// and a holiday the household's country does not keep never benches anything.
func benchesForAny(e LineupEntry, cal Calendar) bool {
	for _, h := range cal.holidays() {
		if judgeEntry(e, h).Benches() {
			return true
		}
	}
	return false
}

// aboutAnyHoliday reports whether an entry is about any built-in holiday, kept by any country: a
// title that is itself a holiday's, whose episodes are therefore not featured one by one.
func aboutAnyHoliday(e LineupEntry, v holidayvocab.Vocabulary) bool {
	for _, d := range v.Definitions() {
		if v.Judge(d.ID, holidayvocab.Evidence{Title: e.Title, Genres: e.Genres}).Verdict == holidayvocab.VerdictAbout {
			return true
		}
	}
	return false
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
	return applySeasonalWithTrace(entries, rp, now, Calendar{}, nil)
}

func applySeasonalWithTrace(entries []LineupEntry, rp ResolvedPolicy, now time.Time, cal Calendar, trace *scheduleTraceBuilder) ([]LineupEntry, ExclusionReport) {
	var report ExclusionReport
	if now.IsZero() || rp.Seasonal.Mode == SeasonalOff {
		return entries, report
	}
	active := activeHolidays(now, cal, rp.Seasonal.Holidays)
	inWindow := len(active) > 0

	switch rp.Seasonal.Mode {
	case SeasonalExclusive:
		if !inWindow {
			if rp.Seasonal.OffSeason == OffSeasonDark {
				// Bench everything — the channel goes dark out of window.
				for _, e := range entries {
					report.add(e, "out_of_season")
					trace.add(hardFilterFact(e, OutcomeExcluded, ReasonOutOfSeason))
				}
				return nil, report
			}
			// loop: run the full scope with no seasonal filter.
			return entries, report
		}
		// In window: keep only items seasonal for an active holiday.
		kept := entries[:0:0]
		for _, e := range entries {
			// A series title is not evidence about each of its episodes. The
			// concrete exclusive filter runs after episode safety and grouping.
			if e.Key.IsSeries() || fitsSeason(e, active) {
				kept = append(kept, e)
			} else {
				report.add(e, "out_of_season")
				trace.add(hardFilterFact(e, OutcomeExcluded, ReasonOutOfSeason))
			}
		}
		return kept, report

	default: // SeasonalAuto (and any resolved default)
		kept := make([]LineupEntry, 0, len(entries))
		for _, e := range entries {
			// Bench a detected-seasonal item that is NOT for an active holiday
			// (Christmas in July). A non-seasonal item always passes.
			if benchesForAny(e, cal) && !isSeasonal(e, active) {
				report.add(e, "out_of_season")
				trace.add(hardFilterFact(e, OutcomeExcluded, ReasonOutOfSeason))
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

func exclusiveEpisodeHolidays(rp ResolvedPolicy, now time.Time, cal Calendar) map[string]bool {
	if now.IsZero() || rp.Seasonal.Mode != SeasonalExclusive {
		return nil
	}
	active := activeHolidays(now, cal, rp.Seasonal.Holidays)
	if len(active) == 0 {
		return nil
	}
	ids := make(map[string]bool, len(active))
	for _, h := range active {
		ids[h.id] = true
	}
	return ids
}

func filterExclusiveEpisodes(e LineupEntry, episodes []ResolvedProgram, editorialUnavailable bool, holidays map[string]bool, v holidayvocab.Vocabulary, report *ExclusionReport, trace *scheduleTraceBuilder) []ResolvedProgram {
	if len(holidays) == 0 {
		return episodes
	}
	var selected []ResolvedProgram
	if !editorialUnavailable {
		selected = matchingHolidayEpisodes(episodes, holidays, v)
	}
	kept := make(map[resolvedProgramID]int, len(selected))
	for _, ep := range selected {
		kept[resolvedProgramIdentity(ep)]++
	}
	for _, ep := range episodes {
		id := resolvedProgramIdentity(ep)
		if kept[id] > 0 {
			kept[id]--
			continue
		}
		trace.add(episodeFact(e, ep, StageHardFilter, OutcomeExcluded, ReasonOutOfSeason))
		if report != nil {
			report.Items = append(report.Items, ExcludedItem{Key: e.Key, Title: episodeLabel(e, ep), Reason: "out_of_season"})
		}
	}
	return selected
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
