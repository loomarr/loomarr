package schedule_test

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/testkit/libraryfixture"
)

// The household report (#1670): an October auto channel of one long series aired its
// "Puddlehouse of Horror" episodes only when the deck happened to deal one. These pin the
// holiday-episode feature (§6 "Holiday episodes") over a long-running cartoon series.

const episodeMs = 22 * 60 * 1000

var mudpuddlesKey = provision.Key("series:tmdb:90000456")

// mudpuddlesLike is 200 ordinary episodes plus `holiday` Puddlehouse of Horror episodes, the
// holiday ones scattered through the run as an annual special is in a long series.
func mudpuddlesLike(holiday int) libraryfixture.Availability {
	var eps []schedule.ResolvedProgram
	h := 0
	for i := range 200 {
		eps = append(eps, schedule.ResolvedProgram{
			LibraryItemID: fmt.Sprintf("ep-%03d", i), Title: fmt.Sprintf("Pip's day %d", i),
			Season: i/20 + 1, Episode: i%20 + 1, DurationMs: episodeMs,
		})
		if h < holiday && (i+1)*holiday/200 > h { // one each time i crosses a 200/holiday stride
			// The overview names the holiday itself, the evidence any matcher must accept.
			eps = append(eps, schedule.ResolvedProgram{
				LibraryItemID: fmt.Sprintf("thoh-%02d", h), Title: fmt.Sprintf("Puddlehouse of Horror %d", h+1),
				Overview: "The family's annual Halloween special.",
				Season:   i/20 + 1, Episode: 30 + h, DurationMs: episodeMs,
			})
			h++
		}
	}
	return libraryfixture.Availability{Series: map[provision.Key]schedule.EpisodeResolution{mudpuddlesKey: {Programs: eps}}}
}

func mudpuddlesChannel() schedule.Channel {
	return schedule.Channel{ID: "mudpuddles", Name: "Classics", Number: 7, Strategy: schedule.Shuffle,
		Shuffle: schedule.ShuffleParams{Seed: 42}, DefaultWindow: 24 * time.Hour}
}

func seasonalPolicy(s schedule.SeasonalPolicy) schedule.ChannelPolicy {
	return schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Seasonal: s}}
}

type airing struct {
	id    string
	start time.Time
}

// arrange returns the programmes of the window containing `at`, with their start times.
func arrange(t *testing.T, ch schedule.Channel, avail libraryfixture.Availability, policy schedule.ChannelPolicy, at time.Time) []airing {
	t.Helper()
	d := schedule.ComputeDesiredAt(ch, []schedule.LineupEntry{{Key: mudpuddlesKey, Title: "The Mudpuddles"}}, avail, schedule.PodFill, policy, at)
	clock := schedule.WindowStart(at, schedule.ResolveWindow(ch, policy, at), ch.WindowZone)
	var out []airing
	for _, s := range d.Slots {
		if s.IsProgram() {
			out = append(out, airing{s.LibraryItemID, clock})
		}
		clock = clock.Add(time.Duration(s.DurationMs) * time.Millisecond)
	}
	return out
}

// airedLooped is the programmes of the window containing `at` as playout airs them: a lineup
// shorter than its window is looped until the window closes, and a slot starting at or after the
// close never airs.
func airedLooped(t *testing.T, ch schedule.Channel, avail libraryfixture.Availability, policy schedule.ChannelPolicy, at time.Time) []airing {
	t.Helper()
	d := schedule.ComputeDesiredAt(ch, []schedule.LineupEntry{{Key: mudpuddlesKey, Title: "The Mudpuddles"}}, avail, schedule.PodFill, policy, at)
	window := schedule.ResolveWindow(ch, policy, at)
	clock := schedule.WindowStart(at, window, ch.WindowZone)
	closing := clock.Add(window)
	var out []airing
	for clock.Before(closing) {
		before := clock
		for _, s := range d.Slots {
			if !clock.Before(closing) {
				break
			}
			if s.IsProgram() {
				out = append(out, airing{s.LibraryItemID, clock})
			}
			clock = clock.Add(time.Duration(s.DurationMs) * time.Millisecond)
		}
		if clock.Equal(before) {
			break
		}
	}
	return out
}

func holidayAirings(as []airing) []airing {
	var out []airing
	for _, a := range as {
		if strings.HasPrefix(a.id, "thoh-") {
			out = append(out, a)
		}
	}
	return out
}

func inEvening(a airing, zone *time.Location) bool {
	if zone == nil {
		zone = time.UTC
	}
	h := a.start.In(zone).Hour()
	return h >= 17 && h < 23
}

func day(month time.Month, d, hour int) time.Time {
	return time.Date(2026, month, d, hour, 0, 0, 0, time.UTC)
}

func TestHolidayEpisodes_FeaturedEveningsInOctober(t *testing.T) {
	got := holidayAirings(arrange(t, mudpuddlesChannel(), mudpuddlesLike(9), seasonalPolicy(schedule.SeasonalPolicy{}), day(time.October, 13, 9)))
	if len(got) != 3 {
		t.Fatalf("Featured (the default) aired %d holiday episodes on Oct 13, want 3: %v", len(got), got)
	}
	for _, a := range got {
		if !inEvening(a, nil) {
			t.Fatalf("holiday episode %s starts %s, outside the evening band", a.id, a.start.Format(time.Kitchen))
		}
	}
}

func TestHolidayEpisodes_EveningsFollowTheChannelClock(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no tzdata")
	}
	ch := mudpuddlesChannel()
	ch.WindowZone = ny
	got := holidayAirings(arrange(t, ch, mudpuddlesLike(9), seasonalPolicy(schedule.SeasonalPolicy{}), time.Date(2026, 10, 13, 9, 0, 0, 0, ny)))
	if len(got) != 3 {
		t.Fatalf("got %d holiday episodes, want 3", len(got))
	}
	for _, a := range got {
		if !inEvening(a, ny) {
			t.Fatalf("%s starts %s New York time, outside the evening band", a.id, a.start.In(ny).Format(time.Kitchen))
		}
	}
}

func TestHolidayEpisodes_StrengthOrdersLightFeaturedHeavy(t *testing.T) {
	for _, tc := range []struct {
		pool                   int
		light, featured, heavy int
	}{
		{pool: 30, light: 1, featured: 3, heavy: 5},
		{pool: 9, light: 1, featured: 3, heavy: 4}, // Heavy capped by the two-day spacing floor
	} {
		count := func(s schedule.HolidayStrength) int {
			return len(holidayAirings(arrange(t, mudpuddlesChannel(), mudpuddlesLike(tc.pool),
				seasonalPolicy(schedule.SeasonalPolicy{Strength: s}), day(time.October, 14, 9))))
		}
		light, featured, heavy := count(schedule.HolidayStrengthLight), count(schedule.HolidayStrengthFeatured), count(schedule.HolidayStrengthHeavy)
		if light != tc.light || featured != tc.featured || heavy != tc.heavy {
			t.Fatalf("pool %d: light/featured/heavy = %d/%d/%d, want %d/%d/%d",
				tc.pool, light, featured, heavy, tc.light, tc.featured, tc.heavy)
		}
	}
}

func TestHolidayEpisodes_NeverRepeatWithinTwoDays(t *testing.T) {
	last := map[string]time.Time{}
	for d := 1; d <= 30; d++ { // Oct 31 is the documented marathon exception
		seen := map[string]bool{}
		for _, a := range holidayAirings(arrange(t, mudpuddlesChannel(), mudpuddlesLike(9),
			seasonalPolicy(schedule.SeasonalPolicy{Strength: schedule.HolidayStrengthHeavy}), day(time.October, d, 9))) {
			if seen[a.id] {
				t.Fatalf("Oct %d: %s aired twice in one day", d, a.id)
			}
			seen[a.id] = true
			if prev, ok := last[a.id]; ok && a.start.Sub(prev) < 36*time.Hour {
				t.Fatalf("Oct %d: %s aired again %s after its last airing", d, a.id, a.start.Sub(prev))
			}
			last[a.id] = a.start
		}
	}
}

func TestHolidayEpisodes_HolidayNightMarathon(t *testing.T) {
	for _, tc := range []struct {
		strength schedule.HolidayStrength
		want     int
	}{
		{schedule.HolidayStrengthLight, 1},
		{schedule.HolidayStrengthFeatured, 5},
		{schedule.HolidayStrengthHeavy, 9},
	} {
		got := holidayAirings(arrange(t, mudpuddlesChannel(), mudpuddlesLike(9),
			seasonalPolicy(schedule.SeasonalPolicy{Strength: tc.strength}), day(time.October, 31, 9)))
		if len(got) != tc.want {
			t.Fatalf("%s on Oct 31: %d holiday episodes, want %d", tc.strength, len(got), tc.want)
		}
		for _, a := range got {
			if !inEvening(a, nil) {
				t.Fatalf("%s on Oct 31: %s starts %s, outside the evening", tc.strength, a.id, a.start.Format(time.Kitchen))
			}
		}
	}
}

func TestHolidayEpisodes_AnytimeSpreadsAcrossTheDay(t *testing.T) {
	got := holidayAirings(arrange(t, mudpuddlesChannel(), mudpuddlesLike(9),
		seasonalPolicy(schedule.SeasonalPolicy{Placement: schedule.HolidayPlacementAnytime}), day(time.October, 13, 9)))
	if len(got) != 3 || !slices.ContainsFunc(got, func(a airing) bool { return a.start.Hour() < 17 }) {
		t.Fatalf("anytime placement = %v, want 3 spread across the day", got)
	}
}

func TestHolidayEpisodes_OutOfSeasonKeptOutOfTheEvening(t *testing.T) {
	// Nov 5: Halloween's window is closed, nothing is boosted, and a holiday episode the deck deals
	// is never benched but airs outside the evening band. In order there is no dealer spacing for
	// the move to cost, so every one leaves the evening.
	policy := seasonalPolicy(schedule.SeasonalPolicy{})
	policy.Ordering = schedule.OrderSequential
	off := seasonalPolicy(schedule.SeasonalPolicy{Mode: schedule.SeasonalOff})
	off.Ordering = schedule.OrderSequential
	seenHoliday := false
	for d := 2; d <= 9; d++ {
		all := arrange(t, mudpuddlesChannel(), mudpuddlesLike(30), policy, day(time.November, d, 9))
		for _, a := range holidayAirings(all) {
			seenHoliday = true
			if inEvening(a, nil) {
				t.Fatalf("Nov %d: out-of-season %s airs at %s, in the evening", d, a.id, a.start.Format(time.Kitchen))
			}
		}
		ignored := arrange(t, mudpuddlesChannel(), mudpuddlesLike(30), off, day(time.November, d, 9))
		if got, dealt := len(holidayAirings(all)), len(holidayAirings(ignored)); got > dealt {
			t.Fatalf("Nov %d: %d holiday episodes where the deck deals %d, nothing should be boosted", d, got, dealt)
		}
	}
	if !seenHoliday {
		t.Fatal("no holiday episode aired in a week of November: out of season must not bench them")
	}
}

// #1960: a shuffled channel keeps an off-season holiday episode in the evening only where moving it
// would cost the repeat spacing (TestHolidayEpisodes_SeamSpacingHoldsForEveryDealOrigin holds that
// side). It is still never benched, and the evening still moves the ones that cost nothing.
func TestHolidayEpisodes_OutOfSeasonShuffledLeavesTheEveningWhereSpacingAllows(t *testing.T) {
	evenings := seasonalPolicy(schedule.SeasonalPolicy{})
	anytime := seasonalPolicy(schedule.SeasonalPolicy{Placement: schedule.HolidayPlacementAnytime})
	off := seasonalPolicy(schedule.SeasonalPolicy{Mode: schedule.SeasonalOff})
	var moved, dealtInEvening int
	for d := 2; d <= 15; d++ { // March: no season's simulation reaches it
		at := day(time.March, d, 9)
		all := arrange(t, mudpuddlesChannel(), mudpuddlesLike(30), evenings, at)
		if got, dealt := len(holidayAirings(all)), len(holidayAirings(arrange(t, mudpuddlesChannel(), mudpuddlesLike(30), off, at))); got != dealt {
			t.Fatalf("Mar %d: %d holiday episodes air where the deck deals %d", d, got, dealt)
		}
		inEveningCount := func(as []airing) (n int) {
			for _, a := range holidayAirings(as) {
				if inEvening(a, nil) {
					n++
				}
			}
			return n
		}
		dealt := inEveningCount(arrange(t, mudpuddlesChannel(), mudpuddlesLike(30), anytime, at))
		dealtInEvening += dealt
		moved += dealt - inEveningCount(all)
	}
	if dealtInEvening == 0 {
		t.Fatal("the deck dealt no holiday episode into an evening in two weeks; the fixture proves nothing")
	}
	if moved <= 0 {
		t.Fatalf("of %d holiday episodes dealt into an evening, none left it", dealtInEvening)
	}
}

// #1960: a shuffled channel moves an off-season holiday episode out of the evening by swapping it
// with one ordinary unit after the band, so every programme between keeps its place. Deferring it
// instead would bring each of them forward a slot.
func TestHolidayEpisodes_OutOfSeasonShuffledSwapsWithOneUnit(t *testing.T) {
	ch := channelWithWindow(12*time.Hour, nil)
	avail := seriesLike(80, halloweenSpecials(4)...)
	evenings := seasonalPolicy(schedule.SeasonalPolicy{})
	evenings.Ordering = schedule.OrderShuffle
	anytime := seasonalPolicy(schedule.SeasonalPolicy{Placement: schedule.HolidayPlacementAnytime})
	anytime.Ordering = schedule.OrderShuffle
	swaps := 0
	for at := day(time.March, 2, 0); at.Before(day(time.March, 16, 0)); at = at.Add(ch.DefaultWindow) {
		dealt, aired := arrange(t, ch, avail, anytime, at), arrange(t, ch, avail, evenings, at)
		if len(dealt) != len(aired) {
			t.Fatalf("the window opening %s airs %d programmes where the deck deals %d", at.Format(time.Stamp), len(aired), len(dealt))
		}
		var moved []int
		for i := range dealt {
			if dealt[i].id != aired[i].id {
				moved = append(moved, i)
			}
		}
		if len(moved) != 2 || moved[1]-moved[0] < 2 {
			continue // unmoved, or deferred a slot (the fallback)
		}
		e, u := moved[0], moved[1]
		if !isHoliday(dealt[e].id) || isHoliday(dealt[u].id) || aired[u].id != dealt[e].id || aired[e].id != dealt[u].id {
			continue
		}
		if !inEvening(dealt[e], nil) || inEvening(aired[u], nil) {
			t.Fatalf("the window opening %s swapped %s from %s to %s", at.Format(time.Stamp), dealt[e].id,
				dealt[e].start.Format(time.Kitchen), aired[u].start.Format(time.Kitchen))
		}
		swaps++
	}
	if swaps == 0 {
		t.Fatal("no holiday episode swapped out of an evening in two weeks")
	}
}

func TestHolidayEpisodes_DeterministicWithinAWindow(t *testing.T) {
	policy := seasonalPolicy(schedule.SeasonalPolicy{})
	morning := arrange(t, mudpuddlesChannel(), mudpuddlesLike(9), policy, day(time.October, 20, 1))
	evening := arrange(t, mudpuddlesChannel(), mudpuddlesLike(9), policy, day(time.October, 20, 22))
	again := arrange(t, mudpuddlesChannel(), mudpuddlesLike(9), policy, day(time.October, 20, 1))
	if !slices.Equal(morning, evening) || !slices.Equal(morning, again) {
		t.Fatal("the same window arranged differently across reconciles")
	}
}

func TestHolidayEpisodes_IgnoreAndOtherHolidaysLeaveItAlone(t *testing.T) {
	plain := func(p schedule.SeasonalPolicy) []airing {
		return arrange(t, mudpuddlesChannel(), mudpuddlesLike(9), seasonalPolicy(p), day(time.October, 13, 9))
	}
	off := plain(schedule.SeasonalPolicy{Mode: schedule.SeasonalOff})
	christmasOnly := plain(schedule.SeasonalPolicy{Holidays: []string{"christmas"}})
	if !slices.Equal(off, christmasOnly) {
		t.Fatal("a channel that excludes Halloween still rearranged its Halloween episodes")
	}
	if n := len(holidayAirings(off)); n == 3 && !slices.ContainsFunc(holidayAirings(off), func(a airing) bool { return !inEvening(a, nil) }) {
		t.Fatal("Ignore holidays looks featured; the fixture no longer distinguishes the two")
	}
}

func TestHolidayEpisodes_SummaryIsTheConsequenceLine(t *testing.T) {
	d := schedule.ComputeDesiredAt(mudpuddlesChannel(), []schedule.LineupEntry{{Key: mudpuddlesKey, Title: "The Mudpuddles"}},
		mudpuddlesLike(9), schedule.PodFill, seasonalPolicy(schedule.SeasonalPolicy{}), day(time.October, 13, 9))
	if len(d.HolidayEpisodes) == 0 {
		t.Fatal("no holiday summary in auto mode")
	}
	want := schedule.HolidayEpisodes{Holiday: "halloween", Episodes: 9, PerDay: 3, EveryDays: 3, Active: true}
	if !reflect.DeepEqual(d.HolidayEpisodes[0], want) {
		t.Fatalf("first summary = %+v, want %+v", d.HolidayEpisodes[0], want)
	}
	if d.HolidayEpisodes[1].Active || d.HolidayEpisodes[1].Episodes != 0 {
		t.Fatalf("next holiday = %+v, want a closed holiday with no episodes", d.HolidayEpisodes[1])
	}
}

// withHolidayEvidence rewrites the holiday episodes' evidence, so a test can ask what the §6
// judgment makes of evidence other than an overview naming the holiday.
func withHolidayEvidence(avail libraryfixture.Availability, title, overview string, airDate time.Time) libraryfixture.Availability {
	res := avail.Series[mudpuddlesKey]
	for i, ep := range res.Programs {
		if strings.HasPrefix(ep.LibraryItemID, "thoh-") {
			res.Programs[i].Title, res.Programs[i].Overview, res.Programs[i].AirDate = title, overview, airDate
		}
	}
	avail.Series[mudpuddlesKey] = res
	return avail
}

func TestHolidayEpisodes_MoodTitleAndOctoberAirDateIsFeatured(t *testing.T) {
	// The annual-special shape: a mood word in the title, a late-October air date, and an
	// overview that never says Halloween.
	avail := withHolidayEvidence(mudpuddlesLike(9), "Puddlehouse of Horror XIV",
		"Grandpa Puddle takes over as the Grim Reaper.", time.Date(2003, time.October, 27, 0, 0, 0, 0, time.UTC))
	d := schedule.ComputeDesiredAt(mudpuddlesChannel(), []schedule.LineupEntry{{Key: mudpuddlesKey, Title: "The Mudpuddles"}},
		avail, schedule.PodFill, seasonalPolicy(schedule.SeasonalPolicy{}), day(time.October, 13, 9))
	if len(d.HolidayEpisodes) == 0 || d.HolidayEpisodes[0].Episodes != 9 {
		t.Fatalf("summary = %+v, want all 9 Puddlehouse-style episodes in the Halloween pool", d.HolidayEpisodes)
	}
	got := holidayAirings(arrange(t, mudpuddlesChannel(), avail, seasonalPolicy(schedule.SeasonalPolicy{}), day(time.October, 13, 9)))
	if len(got) != 3 {
		t.Fatalf("Featured aired %d Puddlehouse-style episodes on Oct 13, want 3: %v", len(got), got)
	}
	for _, a := range got {
		if !inEvening(a, nil) {
			t.Fatalf("Puddlehouse-style episode %s starts %s, outside the evening band", a.id, a.start.Format(time.Kitchen))
		}
	}
}

func TestHolidayEpisodes_BareGhostInAnOrdinaryOverviewIsNotFeatured(t *testing.T) {
	avail := withHolidayEvidence(mudpuddlesLike(9), "Pip's Attic",
		"Pip is sure a ghost lives in the attic until Tansy finds the raccoon.", time.Time{})
	d := schedule.ComputeDesiredAt(mudpuddlesChannel(), []schedule.LineupEntry{{Key: mudpuddlesKey, Title: "The Mudpuddles"}},
		avail, schedule.PodFill, seasonalPolicy(schedule.SeasonalPolicy{}), day(time.October, 13, 9))
	for _, h := range d.HolidayEpisodes {
		if h.Episodes != 0 {
			t.Fatalf("summary = %+v, want no holiday episodes: a ghost in an ordinary overview is not Halloween", d.HolidayEpisodes)
		}
	}
}

func TestHolidayRotation(t *testing.T) {
	for _, tc := range []struct {
		pool        int
		strength    schedule.HolidayStrength
		perDay, gap int
	}{
		{0, schedule.HolidayStrengthHeavy, 0, 0},
		{1, schedule.HolidayStrengthFeatured, 1, 2},
		{3, schedule.HolidayStrengthFeatured, 1, 3},
		{9, schedule.HolidayStrengthLight, 1, 9},
		{9, schedule.HolidayStrengthFeatured, 3, 3},
		{9, schedule.HolidayStrengthHeavy, 4, 2},
		{40, schedule.HolidayStrengthHeavy, 5, 8},
	} {
		perDay, gap := schedule.HolidayRotation(tc.pool, tc.strength)
		if perDay != tc.perDay || gap != tc.gap {
			t.Errorf("HolidayRotation(%d, %s) = %d/%d, want %d/%d", tc.pool, tc.strength, perDay, gap, tc.perDay, tc.gap)
		}
	}
}

func TestHolidayPolicyValidation(t *testing.T) {
	for _, s := range []schedule.SeasonalPolicy{{Strength: "loud"}, {Placement: "midnight"}} {
		if err := seasonalPolicy(s).Validate(); err == nil {
			t.Fatalf("Validate accepted %+v", s)
		}
	}
	if err := seasonalPolicy(schedule.SeasonalPolicy{Strength: schedule.HolidayStrengthHeavy, Placement: schedule.HolidayPlacementAnytime}).Validate(); err != nil {
		t.Fatal(err)
	}
}
