package holidayvocab

import (
	"slices"
	"testing"
	"time"
)

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 12, 0, 0, 0, time.UTC) }

// observed lists the holiday ids whose window, for country, contains t.
func observed(country string, t time.Time) []string {
	var out []string
	for _, o := range Default().Calendar(country) {
		for _, y := range []int{t.Year() - 1, t.Year()} {
			if start, end := o.Window(y); !t.Before(start) && !t.After(end) {
				out = append(out, o.ID)
				break
			}
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
	if got := v.Calendar("ZZ"); got != nil {
		t.Errorf("an unsupported country has a calendar: %v", got)
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
	if _, err := New([]Holiday{ok, {ID: "autumn", Label: "Autumn", Members: []string{"missing"}}}); err == nil {
		t.Error("a group whose member is no holiday is accepted")
	}
}
