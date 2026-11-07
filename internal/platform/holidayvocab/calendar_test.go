package holidayvocab

import (
	"slices"
	"testing"
	"time"
)

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 12, 0, 0, 0, time.UTC) }

// observed lists the holiday ids in season, for country, at t.
func observed(country string, t time.Time) []string {
	v := Default()
	var out []string
	for _, d := range v.Definitions() {
		if v.InSeason(d.ID, country, t) {
			out = append(out, d.ID)
		}
	}
	slices.Sort(out)
	return out
}

// #2030: the calendar follows the household's country. A holiday a country does not keep never
// opens there, and one kept on a different date opens on that country's date.
func TestCalendarFollowsTheCountry(t *testing.T) {
	cases := []struct {
		name, country string
		at            time.Time
		want          []string
	}{
		{"Canadian Thanksgiving is in October", "CA", day(2026, time.October, 10), []string{"halloween", "thanksgiving"}},
		{"US Thanksgiving is not", "US", day(2026, time.October, 10), []string{"halloween"}},
		{"Canada has no November Thanksgiving", "CA", day(2026, time.November, 20), nil},
		{"US Thanksgiving week", "US", day(2026, time.November, 20), []string{"thanksgiving"}},
		{"Bonfire Night in Britain", "GB", day(2026, time.November, 3), []string{"bonfire-night"}},
		{"no Bonfire Night in the US", "US", day(2026, time.November, 3), nil},
		{"an unset country is the US", "", day(2026, time.July, 2), []string{"independence-day"}},
		// A country no definition names keeps the US calendar, as every household did before
		// countries: Germany's channels still bench Christmas in July.
		{"an unlisted country is the US", "DE", day(2026, time.July, 2), []string{"independence-day"}},
		{"so is a slip for Britain", "UK", day(2026, time.December, 20), []string{"christmas"}},
		{"a country is read loosely", " gb ", day(2026, time.November, 3), []string{"bonfire-night"}},
		{"Canada Day", "CA", day(2026, time.June, 28), []string{"canada-day"}},
		{"Easter 2027 is March 28", "GB", day(2027, time.March, 20), []string{"easter"}},
		{"through Easter Monday", "IE", day(2027, time.March, 29), []string{"easter"}},
		{"not after it", "AU", day(2027, time.March, 31), nil},
		{"Mothering Sunday is three weeks before Easter", "GB", day(2027, time.March, 7), []string{"mothers-day"}},
		{"American Mother's Day is in May", "US", day(2027, time.May, 9), []string{"mothers-day"}},
		{"Australian Father's Day is in September", "AU", day(2026, time.September, 6), []string{"fathers-day"}},
		{"school starts in summer down under", "NZ", day(2027, time.January, 20), []string{"back-to-school"}},
		{"not in the north", "US", day(2027, time.January, 20), nil},
		{"Memorial Day 2026 is May 25", "US", day(2026, time.May, 25), []string{"memorial-day"}},
		{"St Patrick's in Ireland", "IE", day(2026, time.March, 17), []string{"st-patricks"}},
		{"New Year straddles the year", "NZ", day(2027, time.January, 1), []string{"newyear"}},
	}
	for _, tc := range cases {
		if got := observed(tc.country, tc.at); !slices.Equal(got, tc.want) {
			t.Errorf("%s: %s on %s = %v, want %v", tc.name, tc.country, tc.at.Format("2006-01-02"), got, tc.want)
		}
	}
}

// Every built-in identity is kept somewhere, and every supported country keeps the holidays every
// one of them keeps.
func TestEveryHolidayIsObservedAndEveryCountryHasTheCommonOnes(t *testing.T) {
	v := Default()
	if got, want := v.Countries(), []string{"AU", "CA", "GB", "IE", "NZ", "US"}; !slices.Equal(got, want) {
		t.Errorf("countries = %v, want %v", got, want)
	}
	kept := map[string]bool{}
	for _, c := range v.Countries() {
		ids := map[string]bool{}
		for _, o := range v.Calendar(c) {
			ids[o.ID], kept[o.ID] = true, true
		}
		for _, common := range []string{"halloween", "christmas", "newyear", "valentines", "easter", "mothers-day", "fathers-day", "back-to-school"} {
			if !ids[common] {
				t.Errorf("%s does not keep %s", c, common)
			}
		}
	}
	for _, d := range v.Definitions() {
		if !kept[d.ID] {
			t.Errorf("no country keeps %s", d.ID)
		}
	}
	ids := func(os []Observance) (out []string) {
		for _, o := range os {
			out = append(out, o.ID)
		}
		return out
	}
	if got, want := ids(v.Calendar("ZZ")), ids(v.Calendar("US")); !slices.Equal(got, want) {
		t.Errorf("an unsupported country keeps %v, want the US calendar %v", got, want)
	}
}

// #2030 decision 26: a holiday added for countries other than the US is boosted on its name, but
// benched out of season only with evidence beyond the name. A film titled after one is not
// shelved for the rest of the year; an episode first shown for it is. The original five holidays
// are unchanged.
func TestANewHolidayBenchesOnlyWithEvidenceBeyondTheName(t *testing.T) {
	v := Default()
	film := v.Judge("independence-day", Evidence{Title: "Independence Day"})
	if film.Verdict != VerdictAbout || film.Benches() {
		t.Errorf("a film titled after the holiday = %+v, benches %v; want about, not benched", film, film.Benches())
	}
	episode := v.Judge("independence-day", Evidence{Title: "Independence Day", Episode: true, AirDate: day(2001, time.July, 2)})
	if !episode.Benches() {
		t.Errorf("an episode first shown for the holiday does not bench: %+v", episode)
	}
	if original := v.Judge("halloween", Evidence{Title: "Halloween"}); !original.Benches() {
		t.Errorf("an original holiday's name alone no longer benches: %+v", original)
	}
}

// A channel request names a holiday only outside its look-alikes: documentaries about Easter Island
// are not an Easter channel.
func TestMatchIntentSkipsLookAlikes(t *testing.T) {
	v := Default()
	if got := v.MatchIntent("documentaries on Easter Island"); len(got) != 0 {
		t.Errorf("Easter Island intent = %v, want no holiday", got)
	}
	if got := v.MatchIntent("an Easter channel for the kids"); !slices.Equal(got, []string{"easter"}) {
		t.Errorf("Easter intent = %v, want easter", got)
	}
}

// Western Easter against published dates, including the computus's edge years.
func TestEasterSundayMatchesKnownDates(t *testing.T) {
	for year, want := range map[int]time.Time{
		1818: time.Date(1818, time.March, 22, 0, 0, 0, 0, time.UTC), // the earliest possible
		1943: time.Date(1943, time.April, 25, 0, 0, 0, 0, time.UTC), // the latest possible
		2024: time.Date(2024, time.March, 31, 0, 0, 0, 0, time.UTC),
		2025: time.Date(2025, time.April, 20, 0, 0, 0, 0, time.UTC),
		2026: time.Date(2026, time.April, 5, 0, 0, 0, 0, time.UTC),
		2027: time.Date(2027, time.March, 28, 0, 0, 0, 0, time.UTC),
		2038: time.Date(2038, time.April, 25, 0, 0, 0, 0, time.UTC),
		2049: time.Date(2049, time.April, 18, 0, 0, 0, 0, time.UTC),
	} {
		if got := easterSunday(year); !got.Equal(want) {
			t.Errorf("Easter %d = %s, want %s", year, got.Format(time.DateOnly), want.Format(time.DateOnly))
		}
	}
}

// A season that opens in the previous December (a holiday early in January) is in season on its
// December days too.
func TestASeasonOpeningLastYearIsInSeasonInDecember(t *testing.T) {
	v, err := New([]Holiday{{ID: "epiphany", Label: "Epiphany",
		Kept: []Keeping{keep([]string{"IE"}, fixed(time.January, 6), around(10, 0))}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		at   time.Time
		want bool
	}{
		{day(2026, time.December, 27), true},
		{day(2027, time.January, 6), true},
		{day(2026, time.December, 26), false},
		{day(2027, time.January, 7), false},
	} {
		if got := v.InSeason("epiphany", "IE", tc.at); got != tc.want {
			t.Errorf("Epiphany on %s = %v, want %v", tc.at.Format(time.DateOnly), got, tc.want)
		}
	}
}

// The generic holiday is in season whenever one of its winter members is, and never otherwise.
func TestAGroupHolidayIsInSeasonWithAnyMember(t *testing.T) {
	v := Default()
	for _, tc := range []struct {
		country string
		at      time.Time
		want    bool
	}{
		{"US", day(2026, time.November, 20), true},  // US Thanksgiving
		{"CA", day(2026, time.November, 20), false}, // Canada's was in October
		{"CA", day(2026, time.December, 20), true},  // Christmas
		{"GB", day(2027, time.January, 2), true},    // New Year
		{"GB", day(2027, time.January, 3), false},
		{"US", day(2026, time.October, 31), false}, // Halloween is not a winter holiday
	} {
		if got := v.InSeason(GenericHoliday, tc.country, tc.at); got != tc.want {
			t.Errorf("generic holiday in %s on %s = %v, want %v", tc.country, tc.at.Format(time.DateOnly), got, tc.want)
		}
	}
	if v.InSeason("no-such-holiday", "US", day(2026, time.December, 20)) {
		t.Error("an unknown holiday is in season")
	}
}

// #2030 decision 38: a holiday is kept where it has a season. A group is kept where a member is; an
// unknown or undated holiday is kept nowhere; an unlisted country reads the US calendar.
func TestKeptInFollowsTheCountry(t *testing.T) {
	v := Default()
	for _, tc := range []struct {
		id, country string
		want        bool
	}{
		{"thanksgiving", "US", true},
		{"thanksgiving", "CA", true},
		{"thanksgiving", "GB", false},
		{"bonfire-night", "GB", true},
		{"bonfire-night", "US", false},
		{"independence-day", "", true},
		{"independence-day", "DE", true},
		{GenericHoliday, "GB", true},
		{"no-such-holiday", "US", false},
	} {
		if got := v.KeptIn(tc.id, tc.country); got != tc.want {
			t.Errorf("KeptIn(%q, %q) = %v, want %v", tc.id, tc.country, got, tc.want)
		}
	}
	undated, err := New([]Holiday{{ID: "harvest", Label: "Harvest"}})
	if err != nil {
		t.Fatal(err)
	}
	if undated.KeptIn("harvest", "US") {
		t.Error("an undated holiday is kept somewhere")
	}
}

// Stored holidays are validated: a rule of an unknown kind, an impossible date, a group with dates
// of its own, or a member that is no holiday is refused rather than guessed at.
func TestNewRejectsWhatItCannotEvaluate(t *testing.T) {
	ok := Holiday{ID: "harvest", Label: "Harvest", Kept: []Keeping{keep([]string{"GB"}, fixed(time.September, 20), around(6, 0))}}
	if _, err := New([]Holiday{ok}); err != nil {
		t.Fatalf("a valid holiday is refused: %v", err)
	}
	broken := func(mut func(*Holiday)) Holiday {
		h := ok
		h.Kept = []Keeping{keep([]string{"GB"}, fixed(time.September, 20), around(6, 0))}
		mut(&h)
		return h
	}
	for name, h := range map[string]Holiday{
		"unknown day kind":    broken(func(h *Holiday) { h.Kept[0].Day.Kind = "lunar" }),
		"unknown window kind": broken(func(h *Holiday) { h.Kept[0].Window.Kind = "season" }),
		"February 30":         broken(func(h *Holiday) { h.Kept[0].Day = fixed(time.February, 30) }),
		"fifth Sunday":        broken(func(h *Holiday) { h.Kept[0].Day = nth(5, time.Sunday, time.May) }),
		"a lowercase country": broken(func(h *Holiday) { h.Kept[0].Countries = []string{"gb"} }),
		"a season too long":   broken(func(h *Holiday) { h.Kept[0].Window = around(90, 30) }),
		"not a slug":          broken(func(h *Holiday) { h.ID = "Harvest Festival" }),
		"no label":            broken(func(h *Holiday) { h.Label = " " }),
		"a dated group":       broken(func(h *Holiday) { h.Members = []string{"harvest"} }),
	} {
		if _, err := New([]Holiday{h}); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	group := func(id string, members ...string) Holiday { return Holiday{ID: id, Label: id, Members: members} }
	for name, hs := range map[string][]Holiday{
		"a member that is no holiday": {ok, group("autumn", "missing")},
		"a member that is itself":     {ok, group("autumn", "autumn", "harvest")},
		// A group of groups could cycle, and evaluating a cycle never ends.
		"a member that is a group":  {ok, group("autumn", "harvest"), group("seasons", "autumn")},
		"two groups of each other":  {group("a", "b"), group("b", "a")},
		"a group with intent words": {ok, func() Holiday { g := group("autumn", "harvest"); g.Words.Intents = []string{"autumn"}; return g }()},
		"a group with mood words":   {ok, func() Holiday { g := group("autumn", "harvest"); g.Words.Moods = []string{"leaves"}; return g }()},
		"a group with look-alikes":  {ok, func() Holiday { g := group("autumn", "harvest"); g.Words.LookAlikes = []string{"x"}; return g }()},
	} {
		if _, err := New(hs); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
