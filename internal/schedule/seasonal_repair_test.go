package schedule_test

import (
	"fmt"
	"maps"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/testkit/libraryfixture"
)

// The #1930 review's findings, one test each: a holiday episode never airs twice in a civil day
// whatever the window length, consecutive windows never re-air each other's episodes, an
// out-of-season holiday episode is never benched, and the holiday's own date never shares an
// episode with the day before or after.

// special is one holiday episode of seriesLike: its id prefix names the holiday.
type special struct{ id, title, overview string }

func christmasSpecials(n int) []special {
	var out []special
	for i := range n {
		out = append(out, special{fmt.Sprintf("xmas-%02d", i), fmt.Sprintf("Winter story %d", i+1), "The family's Christmas special."})
	}
	return out
}

func thanksgivingSpecials(n int) []special {
	var out []special
	for i := range n {
		out = append(out, special{fmt.Sprintf("turkey-%02d", i), fmt.Sprintf("Harvest story %d", i+1), "The family's Thanksgiving special."})
	}
	return out
}

// seriesLike is `ordinary` plain episodes with the specials scattered evenly through the run.
func seriesLike(ordinary int, specials ...special) libraryfixture.Availability {
	var eps []schedule.ResolvedProgram
	k := 0
	for i := range ordinary {
		eps = append(eps, schedule.ResolvedProgram{
			LibraryItemID: fmt.Sprintf("ep-%03d", i), Title: fmt.Sprintf("Pip's day %d", i),
			Season: i/20 + 1, Episode: i%20 + 1, DurationMs: episodeMs,
		})
		for ; k < len(specials) && k*ordinary/len(specials) <= i; k++ {
			s := specials[k]
			eps = append(eps, schedule.ResolvedProgram{LibraryItemID: s.id, Title: s.title, Overview: s.overview,
				Season: i/20 + 1, Episode: 40 + k, DurationMs: episodeMs})
		}
	}
	return libraryfixture.Availability{Series: map[provision.Key]schedule.EpisodeResolution{mudpuddlesKey: {Programs: eps}}}
}

func channelWithWindow(w time.Duration, zone *time.Location) schedule.Channel {
	ch := mudpuddlesChannel()
	ch.DefaultWindow, ch.WindowZone = w, zone
	return ch
}

func isHoliday(id string) bool {
	return strings.HasPrefix(id, "thoh-") || strings.HasPrefix(id, "xmas-") || strings.HasPrefix(id, "turkey-") || strings.HasPrefix(id, "newyr-")
}

// airedOn groups the airings of every window touching [from, to) by channel-local civil date.
func airedOn(t *testing.T, ch schedule.Channel, avail libraryfixture.Availability, policy schedule.ChannelPolicy, from, to time.Time) map[string][]airing {
	t.Helper()
	zone := ch.WindowZone
	if zone == nil {
		zone = time.UTC
	}
	out := map[string][]airing{}
	for at := schedule.WindowStart(from, ch.DefaultWindow, ch.WindowZone); at.Before(to); at = schedule.NextWindowStart(at, ch.DefaultWindow, ch.WindowZone) {
		for _, a := range arrange(t, ch, avail, policy, at) {
			if !a.start.Before(from) && a.start.Before(to) {
				out[a.start.In(zone).Format(time.DateOnly)] = append(out[a.start.In(zone).Format(time.DateOnly)], a)
			}
		}
	}
	return out
}

func ids(as []airing) map[string]bool {
	out := map[string]bool{}
	for _, a := range as {
		out[a.id] = true
	}
	return out
}

func assertNoRepeatInADay(t *testing.T, label string, byDay map[string][]airing) {
	t.Helper()
	for date, as := range byDay {
		seen := map[string]bool{}
		for _, a := range as {
			if seen[a.id] {
				t.Fatalf("%s: %s aired twice on %s", label, a.id, date)
			}
			seen[a.id] = true
		}
	}
}

// Finding 1: a sub-day window used to place the whole day's picks inside every window it saw.
func TestHolidayEpisodes_SubDayWindowsNeverRepeatInADay(t *testing.T) {
	for _, w := range []time.Duration{12 * time.Hour, 6 * time.Hour} {
		for _, placement := range []schedule.HolidayPlacement{schedule.HolidayPlacementAnytime, schedule.HolidayPlacementEvenings} {
			byDay := airedOn(t, channelWithWindow(w, nil), mudpuddlesLike(9), seasonalPolicy(schedule.SeasonalPolicy{Placement: placement}),
				day(time.October, 13, 0), day(time.October, 14, 0))
			assertNoRepeatInADay(t, fmt.Sprintf("%s window, %s", w, placement), byDay)
			if n := len(holidayAirings(byDay["2026-10-13"])); n != 3 {
				t.Fatalf("%s window, %s: %d holiday episodes on Oct 13, want Featured's 3", w, placement, n)
			}
		}
	}
}

func halloweenSpecials(n int) []special {
	var out []special
	for i := range n {
		out = append(out, special{fmt.Sprintf("thoh-%02d", i), fmt.Sprintf("Puddlehouse of Horror %d", i+1), "The family's annual Halloween special."})
	}
	return out
}

// Finding 2: the top-up continued into the next window's tile, so its head aired twice. Each deck
// holds more than two windows of ordinary episodes, so seasonality off never repeats across a seam.
func TestHolidayEpisodes_ConsecutiveWindowsShareNothing(t *testing.T) {
	light := seasonalPolicy(schedule.SeasonalPolicy{Strength: schedule.HolidayStrengthLight})
	for _, tc := range []struct {
		window time.Duration
		avail  libraryfixture.Availability
	}{
		{24 * time.Hour, mudpuddlesLike(40)},
		{72 * time.Hour, seriesLike(600, halloweenSpecials(40)...)},
	} {
		w := tc.window
		ch := channelWithWindow(w, nil)
		at := schedule.WindowStart(day(time.October, 2, 0), w, nil)
		prev := ids(arrange(t, ch, tc.avail, light, at))
		// Through the season's end (#1930 re-review N2): the first windows out of season share
		// nothing with the last in season either.
		for at = at.Add(w); at.Before(day(time.November, 5, 0)); at = at.Add(w) {
			cur := arrange(t, ch, tc.avail, light, at)
			for _, a := range cur {
				if prev[a.id] {
					t.Fatalf("%s window opening %s re-airs %s from the window before", w, at.Format(time.DateOnly), a.id)
				}
			}
			prev = ids(cur)
		}
	}
	assertNoRepeatInADay(t, "3-day window", airedOn(t, channelWithWindow(72*time.Hour, nil), seriesLike(600, halloweenSpecials(40)...), light,
		day(time.October, 4, 0), day(time.October, 25, 0)))
}

// Finding 3: out of season, a short window with no time after the evening band benched the
// holiday episodes the deck dealt it. Never benched, in any order; in order (no dealer spacing for
// a move to cost) those reached in the band move to just before it.
func TestHolidayEpisodes_OutOfSeasonShortWindowsNeverBench(t *testing.T) {
	for _, order := range []schedule.OrderingMode{schedule.OrderSequential, schedule.OrderShuffle} {
		t.Run(string(order), func(t *testing.T) {
			assertShortWindowsNeverBench(t, order)
		})
	}
}

func assertShortWindowsNeverBench(t *testing.T, order schedule.OrderingMode) {
	ch := channelWithWindow(5*time.Hour, nil)
	evenings := seasonalPolicy(schedule.SeasonalPolicy{})
	evenings.Ordering = order
	off := seasonalPolicy(schedule.SeasonalPolicy{Mode: schedule.SeasonalOff})
	off.Ordering = order
	dealt := 0
	for at := schedule.WindowStart(day(time.November, 5, 0), ch.DefaultWindow, nil); at.Before(day(time.November, 21, 0)); at = at.Add(ch.DefaultWindow) {
		all := arrange(t, ch, mudpuddlesLike(30), evenings, at)
		got, want := holidayAirings(all), holidayAirings(arrange(t, ch, mudpuddlesLike(30), off, at))
		dealt += len(want)
		if len(got) != len(want) {
			t.Fatalf("window opening %s: %d holiday episodes air where the deck deals %d", at.Format(time.Stamp), len(got), len(want))
		}
		if order != schedule.OrderSequential {
			continue
		}
		// One may air in the band only when the window's time before the band is already all
		// holiday episodes (a window wholly inside the band has none).
		for _, a := range got {
			if !inEvening(a, nil) {
				continue
			}
			band := time.Date(a.start.Year(), a.start.Month(), a.start.Day(), 17, 0, 0, 0, time.UTC)
			for _, b := range all {
				if b.start.Before(band) && !strings.HasPrefix(b.id, "thoh-") {
					t.Fatalf("window opening %s: out-of-season %s airs at %s, in the evening, though %s airs at %s",
						at.Format(time.Stamp), a.id, a.start.Format(time.Kitchen), b.id, b.start.Format(time.Kitchen))
				}
			}
		}
	}
	if dealt == 0 {
		t.Fatal("the deck dealt no holiday episode in two weeks; the fixture proves nothing")
	}
}

// The out-of-season band is the channel's evening, not UTC's (in order, where every one leaves it).
func TestHolidayEpisodes_OutOfSeasonEveningIsChannelLocal(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no tzdata")
	}
	ch := channelWithWindow(24*time.Hour, ny)
	policy := seasonalPolicy(schedule.SeasonalPolicy{})
	policy.Ordering = schedule.OrderSequential
	seen := 0
	for d := 2; d <= 12; d++ {
		for _, a := range holidayAirings(arrange(t, ch, mudpuddlesLike(30), policy, time.Date(2026, 11, d, 9, 0, 0, 0, ny))) {
			seen++
			if inEvening(a, ny) {
				t.Fatalf("Nov %d: out-of-season %s airs at %s New York time, in the evening", d, a.id, a.start.In(ny).Format(time.Kitchen))
			}
		}
	}
	if seen == 0 {
		t.Fatal("no holiday episode aired; the fixture proves nothing")
	}
}

// Finding 4: the holiday's own date shared episodes with the day after.
func TestHolidayEpisodes_HolidayDateNeighboursShareNothing(t *testing.T) {
	avail := seriesLike(200, append(christmasSpecials(9), thanksgivingSpecials(9)...)...)
	for _, s := range []schedule.HolidayStrength{schedule.HolidayStrengthFeatured, schedule.HolidayStrengthHeavy} {
		policy := seasonalPolicy(schedule.SeasonalPolicy{Strength: s})
		for _, tc := range []struct {
			around time.Time
			prefix string
		}{{day(time.November, 26, 0), "turkey-"}, {day(time.December, 25, 0), "xmas-"}} {
			around := tc.around
			var prev map[string]bool
			for d := -2; d <= 2; d++ {
				cur, featured := map[string]bool{}, 0
				for _, a := range arrange(t, channelWithWindow(24*time.Hour, nil), avail, policy, around.AddDate(0, 0, d).Add(9*time.Hour)) {
					if isHoliday(a.id) {
						if prev[a.id] {
							t.Fatalf("%s: %s airs on %s and the day before", s, a.id, around.AddDate(0, 0, d).Format(time.DateOnly))
						}
						cur[a.id] = true
					}
					if strings.HasPrefix(a.id, tc.prefix) {
						featured++
					}
				}
				if d == 0 && s == schedule.HolidayStrengthHeavy && featured != 9 {
					t.Fatalf("Heavy on %s: %d holiday episodes, want the whole pool of 9", around.Format(time.DateOnly), featured)
				}
				prev = cur
			}
		}
	}
}

// Heavy's holiday night is a marathon: the whole pool back to back in the evening.
func TestHolidayEpisodes_HeavyHolidayNightIsBackToBack(t *testing.T) {
	got := arrange(t, mudpuddlesChannel(), mudpuddlesLike(9), seasonalPolicy(schedule.SeasonalPolicy{Strength: schedule.HolidayStrengthHeavy}), day(time.October, 31, 9))
	first, run := -1, 0
	for i, a := range got {
		if strings.HasPrefix(a.id, "thoh-") {
			if first < 0 {
				first = i
			}
			if i != first+run {
				t.Fatalf("the marathon is interleaved: %s at %s is not right after the one before", a.id, a.start.Format(time.Kitchen))
			}
			run++
		}
	}
	if run != 9 || !inEvening(got[first], nil) {
		t.Fatalf("marathon = %d episodes from %v, want 9 starting in the evening", run, got[first].start)
	}
}

// A two-part holiday story features whole: both parts, adjacent and in order.
func TestHolidayEpisodes_TwoPartStoryStaysWhole(t *testing.T) {
	var eps []schedule.ResolvedProgram
	for i := range 200 {
		eps = append(eps, schedule.ResolvedProgram{LibraryItemID: fmt.Sprintf("ep-%03d", i), Title: fmt.Sprintf("Pip's day %d", i),
			Season: i/20 + 1, Episode: i%20 + 1, DurationMs: episodeMs})
	}
	for k := range 4 {
		eps = append(eps, schedule.ResolvedProgram{LibraryItemID: fmt.Sprintf("xmas-%02d", k), Title: fmt.Sprintf("Winter story %d", k),
			Overview: "The family's Christmas special.", Season: 20, Episode: k + 1, DurationMs: episodeMs})
	}
	eps = append(eps,
		schedule.ResolvedProgram{LibraryItemID: "xmas-carol-1", Title: "A Carol (1)", Overview: "A Christmas Eve ghost story.", Season: 20, Episode: 10, DurationMs: episodeMs},
		schedule.ResolvedProgram{LibraryItemID: "xmas-carol-2", Title: "A Carol (2)", Overview: "The ghosts return.", Season: 20, Episode: 11, DurationMs: episodeMs})
	avail := libraryfixture.Availability{Series: map[provision.Key]schedule.EpisodeResolution{mudpuddlesKey: {Programs: eps}}}
	aired := 0
	for d := 1; d <= 25; d++ {
		got := arrange(t, mudpuddlesChannel(), avail, seasonalPolicy(schedule.SeasonalPolicy{}), day(time.December, d, 9))
		for i, a := range got {
			if a.id == "xmas-carol-1" {
				aired++
				if i+1 >= len(got) || got[i+1].id != "xmas-carol-2" {
					t.Fatalf("Dec %d: part 1 of the two-parter is not followed by part 2", d)
				}
			}
			if a.id == "xmas-carol-2" && (i == 0 || got[i-1].id != "xmas-carol-1") {
				t.Fatalf("Dec %d: part 2 of the two-parter airs without part 1 before it", d)
			}
		}
	}
	if aired == 0 {
		t.Fatal("the two-part Christmas story never featured in December")
	}
}

// A 25-hour day (Europe's clocks go back on Oct 25, 2026) arranges like any other.
func TestHolidayEpisodes_DSTDayArrangesCleanly(t *testing.T) {
	london, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Skip("no tzdata")
	}
	ch := channelWithWindow(24*time.Hour, london)
	policy := seasonalPolicy(schedule.SeasonalPolicy{Strength: schedule.HolidayStrengthLight})
	var prev map[string]bool
	for d := 23; d <= 27; d++ {
		got := arrange(t, ch, mudpuddlesLike(40), policy, time.Date(2026, 10, d, 9, 0, 0, 0, london))
		for _, a := range got {
			if prev[a.id] {
				t.Fatalf("Oct %d (London): %s re-aired from the day before", d, a.id)
			}
			if isHoliday(a.id) && !inEvening(a, london) {
				t.Fatalf("Oct %d (London): %s starts %s, outside the evening", d, a.id, a.start.In(london).Format(time.Kitchen))
			}
		}
		prev = ids(got)
	}
}

// The holiday window is a channel-local date: 9 pm New York on Oct 31 is still Halloween.
func TestHolidayEpisodes_HolidayWindowIsChannelLocal(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no tzdata")
	}
	ch := channelWithWindow(24*time.Hour, ny)
	d := schedule.ComputeDesiredAt(ch, []schedule.LineupEntry{{Key: mudpuddlesKey, Title: "The Mudpuddles"}},
		mudpuddlesLike(9), schedule.PodFill, seasonalPolicy(schedule.SeasonalPolicy{}), time.Date(2026, 10, 31, 21, 0, 0, 0, ny))
	if len(d.HolidayEpisodes) == 0 || d.HolidayEpisodes[0].Holiday != "halloween" || !d.HolidayEpisodes[0].Active {
		t.Fatalf("9 pm New York on Oct 31: summary %+v, want Halloween active", d.HolidayEpisodes)
	}
}

// Finding 5: when the pool makes two strengths rotate identically, the summary says so.
func TestHolidayEpisodes_SummaryNamesIdenticalStrengths(t *testing.T) {
	summary := func(pool int, s schedule.HolidayStrength) schedule.HolidayEpisodes {
		t.Helper()
		d := schedule.ComputeDesiredAt(mudpuddlesChannel(), []schedule.LineupEntry{{Key: mudpuddlesKey, Title: "The Mudpuddles"}},
			mudpuddlesLike(pool), schedule.PodFill, seasonalPolicy(schedule.SeasonalPolicy{Strength: s}), day(time.October, 13, 9))
		if len(d.HolidayEpisodes) == 0 || d.HolidayEpisodes[0].Holiday != "halloween" {
			t.Fatalf("pool %d: no Halloween summary", pool)
		}
		return d.HolidayEpisodes[0]
	}
	if got := summary(5, schedule.HolidayStrengthFeatured); !slices.Contains(got.SameAs, schedule.HolidayStrengthHeavy) || !got.Capped {
		t.Fatalf("pool 5, Featured: %+v, want capped and the same as Heavy", got)
	}
	for _, pool := range []int{1, 2} {
		if got := summary(pool, schedule.HolidayStrengthLight); len(got.SameAs) != 2 {
			t.Fatalf("pool %d, Light: %+v, want the same as both other strengths", pool, got)
		}
	}
	if got := summary(30, schedule.HolidayStrengthFeatured); got.Capped || len(got.SameAs) != 0 {
		t.Fatalf("pool 30, Featured: %+v, want uncapped and distinct", got)
	}
}

// #1930 re-review N1: Heavy's holiday-night marathon longer than the window used to leave the
// window with no ordinary room, and the window then arranged nothing at all. Every window of the
// date airs something, the marathon airs whole across the windows, and no episode airs twice.
func TestHolidayEpisodes_MarathonLongerThanTheWindowNeverEmpties(t *testing.T) {
	heavy := seasonalPolicy(schedule.SeasonalPolicy{Strength: schedule.HolidayStrengthHeavy})
	for _, w := range []time.Duration{time.Hour, 2 * time.Hour} {
		ch := channelWithWindow(w, nil)
		for at := day(time.October, 31, 0); at.Before(day(time.November, 1, 0)); at = at.Add(w) {
			if len(arrange(t, ch, mudpuddlesLike(9), heavy, at)) == 0 {
				t.Fatalf("%s window at %s arranges nothing", w, at.Format(time.Kitchen))
			}
		}
		byDay := airedOn(t, ch, mudpuddlesLike(9), heavy, day(time.October, 31, 0), day(time.November, 1, 0))
		assertNoRepeatInADay(t, fmt.Sprintf("%s window", w), byDay)
		if n := len(holidayAirings(byDay["2026-10-31"])); n != 9 {
			t.Fatalf("%s window: the marathon aired %d of 9", w, n)
		}
	}
	// A pool longer than the whole day fills the date with holiday episodes, back to back.
	got := arrange(t, mudpuddlesChannel(), seriesLike(400, halloweenSpecials(80)...), heavy, day(time.October, 31, 9))
	var n int
	for _, a := range got {
		if isHoliday(a.id) {
			n++
		}
	}
	if n < 60 {
		t.Fatalf("an 80-episode marathon on a daily window airs %d holiday episodes (%d slots), want the day full", n, len(got))
	}
}

func newYearSpecials(n int) []special {
	var out []special
	for i := range n {
		out = append(out, special{fmt.Sprintf("newyr-%02d", i), fmt.Sprintf("Countdown story %d", i+1), "The family's New Year's Eve special."})
	}
	return out
}

// #1930 re-review N2: in season the walk passes more of the pool than it features, so it runs
// ahead of windowSlice; the first window out of season went back to windowSlice's position and
// re-aired the episodes the season's last window had just aired, minutes apart. The walk is now
// capped where windowSlice starts the next window, and the airtime it cannot fill comes from half
// a lap away: a seam only skips, and an in-season re-air is about half a cycle from either airing.
func TestHolidayEpisodes_SeasonEndOnlySkips(t *testing.T) {
	for _, seam := range seamCases {
		for _, pool := range seamPools {
			for _, order := range []schedule.OrderingMode{schedule.OrderSequential, schedule.OrderShuffle, schedule.OrderSyndication} {
				for _, strength := range holidayStrengths {
					t.Run(fmt.Sprintf("%s/%d episodes/%s window/%s/%s", seam.name, pool.ordinary, pool.window, order, strength), func(t *testing.T) {
						assertSeamGuarantee(t, channelWithWindow(pool.window, nil), seam, pool, order, strength)
					})
				}
			}
		}
	}
}

// #1960: TestHolidayEpisodes_SeasonEndOnlySkips held only because of where its channel counts
// passes from. A channel's passes start at its creation (its DealOrigin), so which evening a
// pass seam lands on, and how far a season's end falls into a pass, differs per household. Over
// the 365 origins of a year, an 80-episode deck in a 6-hour window broke the spacing on 30 the
// day after Halloween (31 programmes against 33) and on 45 on an ordinary March day; a 9-episode
// deck in an hour's window on 78. Out of season, deferring a holiday episode out of the evening
// brought the units behind it forward. This samples origins, seeds, orderings and strengths, and
// asserts the guarantee at every seam, including a date far from any season.
func TestHolidayEpisodes_SeamSpacingHoldsForEveryDealOrigin(t *testing.T) {
	rng := rand.New(rand.NewPCG(1960, 1676))
	year := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	for range 24 { // 120 seam walks, ~40 s under -race on four cores; main fails 6 of them
		ch := channelWithWindow(0, nil)
		ch.DealOrigin = year.AddDate(0, 0, rng.IntN(365))
		ch.Shuffle.Seed = []int64{42, 7, 1960, 0}[rng.IntN(4)] // 0 derives the seed from the channel id
		order := []schedule.OrderingMode{schedule.OrderShuffle, schedule.OrderSyndication}[rng.IntN(2)]
		strength := holidayStrengths[rng.IntN(len(holidayStrengths))]
		pool := seamPools[rng.IntN(len(seamPools))]
		ch.DefaultWindow = pool.window
		for _, seam := range seamCases {
			t.Run(fmt.Sprintf("origin %s/seed %d/%s/%d episodes/%s window/%s/%s",
				ch.DealOrigin.Format(time.DateOnly), ch.Shuffle.Seed, seam.name, pool.ordinary, pool.window, order, strength), func(t *testing.T) {
				t.Parallel()
				assertSeamGuarantee(t, ch, seam, pool, order, strength)
			})
		}
	}
}

// #2001: the season simulation hands a channel back to the dealer over the windows after a season
// ends, past the two days the seam tests walk. On an 80-episode daily deck the windows of 6–13
// November re-aired ordinary episodes inside the spacing at most deal origins. This walks every
// window from the season's last day until well past the hand-off (24 windows, more than twice the
// simulation's horizon and the spacing on every deck here) at sampled origins, and holds each unit
// to the spacing, or to its closest gap as dealt with the season off where the dealer gives it less.
// A holiday episode's in-season airings follow the rotation, not the spacing, so only gaps that
// open after the season count.
func TestHolidayEpisodes_SpacingHoldsAfterTheSeasonEnds(t *testing.T) {
	year := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	origins := rand.New(rand.NewPCG(2001, 1967)).Perm(365)[:120]
	end := day(time.November, 1, 0) // Halloween's season is Oct 1–31
	for _, pool := range []seamPool{{9, time.Hour}, {80, 6 * time.Hour}, {80, 12 * time.Hour}, {80, 24 * time.Hour}, {400, 24 * time.Hour}} {
		for _, order := range []schedule.OrderingMode{schedule.OrderShuffle, schedule.OrderSyndication} {
			t.Run(fmt.Sprintf("%d episodes/%s window/%s", pool.ordinary, pool.window, order), func(t *testing.T) {
				t.Parallel() // origins share one chain, so a case walks them in turn
				specials := max(3, pool.ordinary/20)
				avail := seriesLike(pool.ordinary, halloweenSpecials(specials)...)
				spacing := repeatSpacingProgrammes(pool.ordinary+specials, pool.window)
				policy := seasonalPolicy(schedule.SeasonalPolicy{Strength: schedule.HolidayStrengthFeatured})
				policy.Ordering = order
				off := seasonalPolicy(schedule.SeasonalPolicy{Mode: schedule.SeasonalOff})
				off.Ordering = order
				stretch := [2]time.Time{end.Add(-max(24*time.Hour, pool.window)), end.Add(24 * pool.window)}
				short, worst, first := 0, spacing, ""
				for _, o := range origins {
					ch := channelWithWindow(pool.window, nil)
					ch.DealOrigin = year.AddDate(0, 0, o)
					if gap, at := closestAfter(t, ch, avail, policy, off, stretch, end, spacing); at != "" {
						short++
						if gap < worst {
							worst = gap
						}
						if first == "" {
							first = fmt.Sprintf("origin %s: %s", ch.DealOrigin.Format(time.DateOnly), at)
						}
					}
				}
				if short > 0 {
					t.Errorf("%d of %d origins air a unit inside its spacing (%d programmes) after the season, worst %d (first: %s)", short, len(origins), spacing, worst, first)
				}
			})
		}
	}
}

// closestAfter walks the windows of stretch as playout airs them, on the seasonal policy and on
// `off`, and returns the shortest gap that opens at or after `from` and is less than the spacing
// and the closest the unit airs as dealt with the season off, with where it happened ("" if none).
func closestAfter(t *testing.T, ch schedule.Channel, avail libraryfixture.Availability, policy, off schedule.ChannelPolicy, stretch [2]time.Time, from time.Time, spacing int) (int, string) {
	t.Helper()
	dealt := map[string]int{}
	for id, at := range programmeIndices(t, ch, avail, off, stretch) {
		dealt[id] = spacing
		for k := 1; k < len(at); k++ {
			dealt[id] = min(dealt[id], at[k]-at[k-1])
		}
	}
	closest, where := spacing, ""
	last := map[string]airing{}
	at := map[string]int{}
	n := 0
	for open := stretch[0]; open.Before(stretch[1]); open = open.Add(ch.DefaultWindow) {
		for _, a := range airedLooped(t, ch, avail, policy, open) {
			if k, ok := at[a.id]; ok && n-k < min(spacing, dealt[a.id]) && !a.start.Before(from) && (!isHoliday(a.id) || !last[a.id].start.Before(from)) && n-k < closest {
				closest, where = n-k, fmt.Sprintf("%s at %s and again %d programmes later at %s", a.id, last[a.id].start.Format(time.Stamp), n-k, a.start.Format(time.Stamp))
			}
			last[a.id], at[a.id] = a, n
			n++
		}
	}
	return closest, where
}

// programmeIndices walks the windows of [stretch[0], stretch[1]) as playout airs them and returns,
// for each unit, the programme indices it airs at.
func programmeIndices(t *testing.T, ch schedule.Channel, avail libraryfixture.Availability, policy schedule.ChannelPolicy, stretch [2]time.Time) map[string][]int {
	t.Helper()
	out := map[string][]int{}
	n := 0
	for open := stretch[0]; open.Before(stretch[1]); open = open.Add(ch.DefaultWindow) {
		for _, a := range airedLooped(t, ch, avail, policy, open) {
			out[a.id] = append(out[a.id], n)
			n++
		}
	}
	return out
}

var holidayStrengths = []schedule.HolidayStrength{schedule.HolidayStrengthLight, schedule.HolidayStrengthFeatured, schedule.HolidayStrengthHeavy}

type seamCase struct {
	name     string
	at       time.Time
	specials func(int) []special
}

// seamCases are the seams the seam tests walk the two days either side of: two season ends, the
// day after one, the middle of a season, and a date far from any season.
var seamCases = []seamCase{
	{"Halloween's end", day(time.November, 1, 0), halloweenSpecials},
	{"first day after Halloween", day(time.November, 2, 0), halloweenSpecials},
	{"New Year's end", time.Date(2027, time.January, 3, 0, 0, 0, 0, time.UTC), newYearSpecials},
	// #1930 review B1: mid-season, a deck under about two windows re-aired an episode back to back.
	{"mid-season", day(time.October, 16, 0), halloweenSpecials},
	// #1960: out of season an evening defers the holiday episodes it is dealt, which brings the
	// units behind them forward, so a pass seam on any evening is a seam the planner reorders.
	{"out of season", day(time.March, 10, 0), halloweenSpecials},
}

type seamPool struct {
	ordinary int
	window   time.Duration
}

var seamPools = []seamPool{{9, time.Hour}, {80, 6 * time.Hour}, {80, 12 * time.Hour}, {80, 24 * time.Hour}, {30, 24 * time.Hour}, {400, 24 * time.Hour}}

// assertSeamGuarantee walks the seam on ch (whose window is pool's) and asserts what the ordering
// guarantees there (§6 "Holiday episodes"). Consecutive windows share nothing when the deck holds
// the window before, this one and the next; on a shorter deck they must share, and spacing is the
// guarantee. In order airs one fixed deck, so an episode aired again sooner than half the deck less
// its pool (the in-season minimum on a small deck) is a seam repeat. Shuffled and syndication deal
// every pass afresh, so the fixed deck's geometry is gone and the guarantee is the dealer's repeat
// spacing. A deck that fits its window loops all day, so the loop, not the deck, sets where the
// window closes: the guarantee across the seam is the dealer's, never back to back. Holiday
// episodes promise to keep the dealer's spacing, and the dealer keeps it at every seam, its chain's
// anchors included (#1965), so it is asserted outright.
func assertSeamGuarantee(t *testing.T, ch schedule.Channel, seam seamCase, pool seamPool, order schedule.OrderingMode, strength schedule.HolidayStrength) {
	t.Helper()
	specials := max(3, pool.ordinary/20)
	avail := seriesLike(pool.ordinary, seam.specials(specials)...)
	deck := time.Duration(pool.ordinary+specials) * episodeMs * time.Millisecond
	spacing := (pool.ordinary+specials)/2 - specials
	switch {
	case deck < pool.window:
		spacing = 2
	case order != schedule.OrderSequential:
		spacing = repeatSpacingProgrammes(pool.ordinary+specials, pool.window)
	}
	policy := seasonalPolicy(schedule.SeasonalPolicy{Strength: strength})
	policy.Ordering = order
	assertSeamOnlySkips(t, ch, avail, policy, seam.at, spacing, deck >= 3*pool.window)
}

// #1939: on a reshuffled 6-hour channel the first window out of Halloween (Nov 1 00:00) re-aired
// ep-026 from Halloween night's last window, because the trade read ring positions, not airings.
func TestHolidayEpisodes_SeasonEndNeverReairsHalloweenNight(t *testing.T) {
	ch := channelWithWindow(6*time.Hour, nil)
	avail := seriesLike(80, halloweenSpecials(4)...)
	for _, order := range []schedule.OrderingMode{schedule.OrderShuffle, schedule.OrderSyndication} {
		policy := seasonalPolicy(schedule.SeasonalPolicy{Strength: schedule.HolidayStrengthHeavy})
		policy.Ordering = order
		night := ids(arrange(t, ch, avail, policy, day(time.October, 31, 18)))
		for _, a := range arrange(t, ch, avail, policy, day(time.November, 1, 0)) {
			if !isHoliday(a.id) && night[a.id] {
				t.Fatalf("%s: Nov 1 00:00 re-airs %s from Halloween night", order, a.id)
			}
		}
	}
}

// BenchmarkHolidayPlanner_OffSeasonEvening prices one out-of-season evening window of an 80-episode
// shuffled deck with evenings favoured, where arrange checks each deferral against the spacing
// (#1960): the regime every window outside a season is in, all year.
func BenchmarkHolidayPlanner_OffSeasonEvening(b *testing.B) {
	ch := channelWithWindow(6*time.Hour, nil)
	avail := seriesLike(80, halloweenSpecials(4)...)
	policy := seasonalPolicy(schedule.SeasonalPolicy{})
	policy.Ordering = schedule.OrderShuffle
	lineup := []schedule.LineupEntry{{Key: mudpuddlesKey, Title: "The Mudpuddles"}}
	for b.Loop() {
		for d := 1; d <= 7; d++ {
			schedule.ComputeDesiredAt(ch, lineup, avail, schedule.PodFill, policy, day(time.March, d, 18))
		}
	}
}

// BenchmarkHolidayPlanner_ShuffledSeason prices one in-season window of a 400-episode shuffled
// deck: the planner replays the season from its anchor and tops every window up with the
// most-rested units of the pass (topUp), so this is the regime #1939's real-distance pick has to
// keep cheap.
func BenchmarkHolidayPlanner_ShuffledSeason(b *testing.B) {
	ch := channelWithWindow(24*time.Hour, nil)
	avail := seriesLike(400, halloweenSpecials(20)...)
	policy := seasonalPolicy(schedule.SeasonalPolicy{Strength: schedule.HolidayStrengthFeatured})
	policy.Ordering = schedule.OrderShuffle
	at := day(time.October, 30, 0)
	lineup := []schedule.LineupEntry{{Key: mudpuddlesKey, Title: "The Mudpuddles"}}
	for b.Loop() {
		schedule.ComputeDesiredAt(ch, lineup, avail, schedule.PodFill, policy, at)
	}
}

// repeatSpacingProgrammes is the dealer's repeat spacing (repeatSpacing in passes.go) for a deck of
// n 22-minute episodes under the default seven-day episode separation, in whole programmes: a third
// of the pool less two units, or two windows and a unit as far as the pool allows and never less
// than a window and a unit, the floors winning.
func repeatSpacingProgrammes(n int, window time.Duration) int {
	unit := time.Duration(episodeMs) * time.Millisecond
	pool := time.Duration(n) * unit
	spacing := (pool - 2*unit) / 3
	floor := 2 * unit
	if pool > window {
		floor = max(floor, min(2*window+unit, pool-window), window+unit)
	}
	return int(max(spacing, min(floor, pool)) / unit)
}

// assertSeamOnlySkips walks the windows of the two days either side of seam: none is empty, none
// re-airs an ordinary episode from the window before (when disjoint), and no ordinary episode airs
// twice within `spacing` programmes.
func assertSeamOnlySkips(t *testing.T, ch schedule.Channel, avail libraryfixture.Availability, policy schedule.ChannelPolicy, seam time.Time, spacing int, disjoint bool) {
	t.Helper()
	last := map[string]airing{}
	at := map[string]int{}
	var before map[string]bool
	n := 0
	for open := seam.Add(-48 * time.Hour); open.Before(seam.Add(48 * time.Hour)); open = open.Add(ch.DefaultWindow) {
		got := arrange(t, ch, avail, policy, open)
		if len(got) == 0 {
			t.Fatalf("the window opening %s airs nothing", open.Format(time.Stamp))
		}
		// Spacing is measured on what airs: a deck shorter than its window loops (#1939).
		for _, a := range airedLooped(t, ch, avail, policy, open) {
			n++
			if isHoliday(a.id) {
				continue
			}
			if disjoint && before[a.id] {
				t.Fatalf("the window opening %s re-airs %s from the window before", open.Format(time.Stamp), a.id)
			}
			if k, ok := at[a.id]; ok && n-k < spacing {
				t.Fatalf("%s airs at %s and again %d programmes later at %s, want at least %d apart",
					a.id, last[a.id].start.Format(time.Stamp), n-k, a.start.Format(time.Stamp), spacing)
			}
			last[a.id], at[a.id] = a, n
		}
		before = ids(got)
	}
}

// #1939 final review: in season, (A) a pool barely wider than its window had its repeat spacing
// capped at the whole pool, so every unit was too soon, the Thanksgiving window aired one episode
// and playout looped it all day; (B) a deck that fits its window had its seam repaired before the
// holiday episodes changed where its loop stopped, so a unit closed one window and opened the
// next. Walked Oct 1 → Jan 10 as playout airs it (looped): every window airs most of its budget
// and no unit airs twice in a row, in a window or across a seam.
func TestHolidayEpisodes_InSeasonWindowsFillAndNeverRepeatBackToBack(t *testing.T) {
	walkHolidaySeasons(t, false)
}

// The same walk with the viewing history fed back, as production always has it after the first
// day: each window's guard trades out what aired too recently, and that must not bring a unit back
// to back where the walk without history does not (#1939). On a pool wider than its window the walk
// opens a day after the channel aired otherwise (its lineup was edited): what that day aired is on
// record, and the guard must keep it out of the first window.
func TestHolidayEpisodes_InSeasonWindowsNeverRepeatBackToBackWithHistory(t *testing.T) {
	walkHolidaySeasons(t, true)
}

// walkHolidaySeasons airs Oct 1 → Jan 10 window by window, looped as playout airs it, and counts the
// episodes that air twice in a row.
func walkHolidaySeasons(t *testing.T, withHistory bool) {
	specials := func(n int) []special {
		return append(append(halloweenSpecials(n), thanksgivingSpecials(n)...), christmasSpecials(n)...)
	}
	for _, tc := range []struct {
		name     string
		episodes int // ordinary and holiday, 22 minutes each
		window   time.Duration
		// The back-to-backs allowed per order (sequential, shuffle, syndication), without and with
		// the history. Only five 22-minute episodes in a six-hour window loop more than three times
		// a window, so in sequence its seam repeats every window (257, as on main).
		maxRepeats, maxRepeatsWithHistory [3]int
		// With the history, how many of the first window's ordinary episodes the day before aired.
		recent int
	}{
		{"pool 1.0 windows", 66, 24 * time.Hour, [3]int{}, [3]int{}, 0},
		{"pool 1.05 windows", 69, 24 * time.Hour, [3]int{}, [3]int{}, 0},
		{"pool 1.2 windows", 79, 24 * time.Hour, [3]int{}, [3]int{}, 8},
		{"pool 1.5 windows", 99, 24 * time.Hour, [3]int{}, [3]int{}, 20},
		{"fitting 5", 5, 6 * time.Hour, [3]int{257, 0, 0}, [3]int{257, 0, 0}, 0},
		{"fitting 10", 10, 12 * time.Hour, [3]int{}, [3]int{}, 0},
		{"fitting 15", 15, 24 * time.Hour, [3]int{}, [3]int{}, 0},
		{"fitting 10", 10, 6 * time.Hour, [3]int{}, [3]int{}, 0},
	} {
		perHoliday := max(1, tc.episodes/25)
		avail := seriesLike(tc.episodes-3*perHoliday, specials(perHoliday)...)
		for o, order := range []schedule.OrderingMode{schedule.OrderSequential, schedule.OrderShuffle, schedule.OrderSyndication} {
			policy := seasonalPolicy(schedule.SeasonalPolicy{Strength: schedule.HolidayStrengthFeatured})
			policy.Ordering = order
			t.Run(fmt.Sprintf("%s/%v/%s", tc.name, tc.window, order), func(t *testing.T) {
				ch := channelWithWindow(tc.window, nil)
				maxRepeats := tc.maxRepeats[o]
				recent := map[string]bool{} // aired the day before the walk opens, on record
				if withHistory {
					maxRepeats = tc.maxRepeatsWithHistory[o]
					// The guard trades a shuffled deal's slice; it does not reorder a sequence.
					if order != schedule.OrderSequential && tc.recent > 0 {
						ch.WindowOpened = day(time.October, 1, 0)
						// The guard trades only for a unit on record as aired longer ago: the rest of
						// the series aired the week before.
						ch.LastAired = map[string]time.Time{}
						for i := range tc.episodes - 3*perHoliday {
							ch.LastAired[fmt.Sprintf("ep-%03d", i)] = ch.WindowOpened.AddDate(0, 0, -7)
						}
						at := ch.WindowOpened.Add(-tc.window)
						for _, a := range arrange(t, ch, avail, policy, ch.WindowOpened) {
							if len(recent) < tc.recent && !isHoliday(a.id) {
								recent[a.id], ch.LastAired[a.id] = true, at
								at = at.Add(episodeMs * time.Millisecond)
							}
						}
					}
				}
				var prev, first string
				repeats := 0
				for open := day(time.October, 1, 0); open.Before(time.Date(2027, time.January, 10, 0, 0, 0, 0, time.UTC)); open = open.Add(tc.window) {
					ch.WindowOpened = open
					if len(recent) > 0 && open.Equal(day(time.October, 1, 0)) {
						for _, a := range airedLooped(t, ch, avail, policy, open) {
							if recent[a.id] {
								t.Fatalf("%s aired the day before and airs again at %s", a.id, a.start.Format(time.Stamp))
							}
						}
					}
					if got := arrange(t, ch, avail, policy, open); tc.episodes*episodeMs < int(tc.window.Milliseconds()) {
						// A deck that fits its window airs all of it but the pool every window.
						if len(got) < 2 {
							t.Fatalf("the window opening %s airs %d programmes", open.Format(time.Stamp), len(got))
						}
					} else if aired := len(got) * episodeMs; aired < int(tc.window.Milliseconds())*3/4 {
						t.Fatalf("the window opening %s airs %v of its %v", open.Format(time.Stamp), time.Duration(aired)*time.Millisecond, tc.window)
					}
					var aired map[string]time.Time // what has aired, by unit, once this window has
					if withHistory {
						aired = maps.Clone(ch.LastAired)
						if aired == nil {
							aired = map[string]time.Time{}
						}
					}
					for _, a := range airedLooped(t, ch, avail, policy, open) {
						if a.id == prev {
							if repeats++; first == "" {
								first = fmt.Sprintf("%s at %s", a.id, a.start.Format(time.Stamp))
							}
						}
						prev = a.id
						if aired != nil {
							aired[a.id] = a.start
						}
					}
					if withHistory {
						ch.LastAired = aired
					}
				}
				if repeats > maxRepeats {
					t.Fatalf("%d episodes air twice in a row (first %s), want at most %d", repeats, first, maxRepeats)
				}
			})
		}
	}
}
