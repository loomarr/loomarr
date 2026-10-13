package schedule_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/testkit/libraryfixture"
)

// The household report (#1670): an October auto channel of one long series aired its
// "Treehouse of Horror" episodes only when the deck happened to deal one. These pin the
// holiday-episode feature (§6 "Holiday episodes") over a Simpsons-like series.

const episodeMs = 22 * 60 * 1000

var simpsonsKey = provision.Key("series:tmdb:456")

// simpsonsLike is 200 ordinary episodes plus `holiday` Treehouse of Horror episodes, the
// holiday ones scattered through the run as they are in the real series.
func simpsonsLike(holiday int) libraryfixture.Availability {
	var eps []schedule.ResolvedProgram
	h := 0
	for i := range 200 {
		eps = append(eps, schedule.ResolvedProgram{
			LibraryItemID: fmt.Sprintf("ep-%03d", i), Title: fmt.Sprintf("Bart's day %d", i),
			Season: i/20 + 1, Episode: i%20 + 1, DurationMs: episodeMs,
		})
		if h < holiday && i%(200/max(holiday, 1)) == 3 {
			// The overview names the holiday itself, the evidence any matcher must accept.
			eps = append(eps, schedule.ResolvedProgram{
				LibraryItemID: fmt.Sprintf("thoh-%02d", h), Title: fmt.Sprintf("Treehouse of Horror %d", h+1),
				Overview: "The family's annual Halloween special.",
				Season:   i/20 + 1, Episode: 30 + h, DurationMs: episodeMs,
			})
			h++
		}
	}
	return libraryfixture.Availability{Series: map[provision.Key]schedule.EpisodeResolution{simpsonsKey: {Programs: eps}}}
}

func simpsonsChannel() schedule.Channel {
	return schedule.Channel{ID: "simpsons", Name: "Classics", Number: 7, Strategy: schedule.Shuffle,
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
	d := schedule.ComputeDesiredAt(ch, []schedule.LineupEntry{{Key: simpsonsKey, Title: "The Simpsons"}}, avail, schedule.PodFill, policy, at)
	clock := schedule.WindowStart(at, 24*time.Hour, ch.WindowZone)
	var out []airing
	for _, s := range d.Slots {
		if s.IsProgram() {
			out = append(out, airing{s.LibraryItemID, clock})
		}
		clock = clock.Add(time.Duration(s.DurationMs) * time.Millisecond)
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
	got := holidayAirings(arrange(t, simpsonsChannel(), simpsonsLike(9), seasonalPolicy(schedule.SeasonalPolicy{}), day(time.October, 13, 9)))
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
	ch := simpsonsChannel()
	ch.WindowZone = ny
	got := holidayAirings(arrange(t, ch, simpsonsLike(9), seasonalPolicy(schedule.SeasonalPolicy{}), time.Date(2026, 10, 13, 9, 0, 0, 0, ny)))
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
			return len(holidayAirings(arrange(t, simpsonsChannel(), simpsonsLike(tc.pool),
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
		for _, a := range holidayAirings(arrange(t, simpsonsChannel(), simpsonsLike(9),
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
		got := holidayAirings(arrange(t, simpsonsChannel(), simpsonsLike(9),
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
	got := holidayAirings(arrange(t, simpsonsChannel(), simpsonsLike(9),
		seasonalPolicy(schedule.SeasonalPolicy{Placement: schedule.HolidayPlacementAnytime}), day(time.October, 13, 9)))
	if len(got) != 3 || !slices.ContainsFunc(got, func(a airing) bool { return a.start.Hour() < 17 }) {
		t.Fatalf("anytime placement = %v, want 3 spread across the day", got)
	}
}

func TestHolidayEpisodes_OutOfSeasonKeptOutOfTheEvening(t *testing.T) {
	// Nov 5: Halloween's window is closed, nothing is boosted, and a holiday episode the deck deals
	// is never benched but airs outside the evening band.
	seenHoliday := false
	for d := 2; d <= 9; d++ {
		all := arrange(t, simpsonsChannel(), simpsonsLike(30), seasonalPolicy(schedule.SeasonalPolicy{}), day(time.November, d, 9))
		for _, a := range holidayAirings(all) {
			seenHoliday = true
			if inEvening(a, nil) {
				t.Fatalf("Nov %d: out-of-season %s airs at %s, in the evening", d, a.id, a.start.Format(time.Kitchen))
			}
		}
		ignored := arrange(t, simpsonsChannel(), simpsonsLike(30), seasonalPolicy(schedule.SeasonalPolicy{Mode: schedule.SeasonalOff}), day(time.November, d, 9))
		if got, dealt := len(holidayAirings(all)), len(holidayAirings(ignored)); got > dealt {
			t.Fatalf("Nov %d: %d holiday episodes where the deck deals %d, nothing should be boosted", d, got, dealt)
		}
	}
	if !seenHoliday {
		t.Fatal("no holiday episode aired in a week of November: out of season must not bench them")
	}
}

func TestHolidayEpisodes_DeterministicWithinAWindow(t *testing.T) {
	policy := seasonalPolicy(schedule.SeasonalPolicy{})
	morning := arrange(t, simpsonsChannel(), simpsonsLike(9), policy, day(time.October, 20, 1))
	evening := arrange(t, simpsonsChannel(), simpsonsLike(9), policy, day(time.October, 20, 22))
	again := arrange(t, simpsonsChannel(), simpsonsLike(9), policy, day(time.October, 20, 1))
	if !slices.Equal(morning, evening) || !slices.Equal(morning, again) {
		t.Fatal("the same window arranged differently across reconciles")
	}
}

func TestHolidayEpisodes_IgnoreAndOtherHolidaysLeaveItAlone(t *testing.T) {
	plain := func(p schedule.SeasonalPolicy) []airing {
		return arrange(t, simpsonsChannel(), simpsonsLike(9), seasonalPolicy(p), day(time.October, 13, 9))
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
	d := schedule.ComputeDesiredAt(simpsonsChannel(), []schedule.LineupEntry{{Key: simpsonsKey, Title: "The Simpsons"}},
		simpsonsLike(9), schedule.PodFill, seasonalPolicy(schedule.SeasonalPolicy{}), day(time.October, 13, 9))
	if len(d.HolidayEpisodes) == 0 {
		t.Fatal("no holiday summary in auto mode")
	}
	want := schedule.HolidayEpisodes{Holiday: "halloween", Episodes: 9, PerDay: 3, EveryDays: 3, Active: true}
	if d.HolidayEpisodes[0] != want {
		t.Fatalf("first summary = %+v, want %+v", d.HolidayEpisodes[0], want)
	}
	if d.HolidayEpisodes[1].Active || d.HolidayEpisodes[1].Episodes != 0 {
		t.Fatalf("next holiday = %+v, want a closed holiday with no episodes", d.HolidayEpisodes[1])
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
