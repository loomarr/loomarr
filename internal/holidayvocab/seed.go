package holidayvocab

import "time"

// The shipped countries. A new country is a code in the keepings below and a case in
// TestCalendarFollowsTheCountry.
var (
	allCountries = []string{"US", "CA", "GB", "IE", "AU", "NZ"}
	northAmerica = []string{"US", "CA"}
	britainIrl   = []string{"GB", "IE"}
	australasia  = []string{"AU", "NZ"}
)

func fixed(m time.Month, d int) DayRule { return DayRule{Kind: DayFixed, Month: m, Day: d} }

func nth(n int, wd time.Weekday, m time.Month) DayRule {
	return DayRule{Kind: DayNthWeekday, Month: m, Weekday: wd, N: n}
}

func last(wd time.Weekday, m time.Month) DayRule {
	return DayRule{Kind: DayLastWeekday, Month: m, Weekday: wd}
}

func easter(offset int) DayRule { return DayRule{Kind: DayEaster, Offset: offset} }

func dates(fm time.Month, fd int, tm time.Month, td int) WindowRule {
	return WindowRule{Kind: WindowRange, From: MonthDay{fm, fd}, To: MonthDay{tm, td}}
}

func around(before, after int) WindowRule {
	return WindowRule{Kind: WindowAround, Before: before, After: after}
}

func keep(countries []string, day DayRule, window WindowRule) Keeping {
	return Keeping{Countries: countries, Rule: Rule{Day: day, Window: window}}
}

func air(day DayRule, window WindowRule) *Rule { return &Rule{Day: day, Window: window} }

var familyFantasy = func() [][]string {
	var out [][]string
	for _, family := range []string{"family", "children", "kids", "animation"} {
		out = append(out, []string{family, "fantasy"})
	}
	return out
}()

// GenericHoliday is the group holiday for clips that name no one holiday ("happy holidays").
const GenericHoliday = "holiday"

// Seed is the shipped holiday definitions (#2030): the store seeds them into an empty table, and a
// later addition ships as a seed revision. A holiday is here when television marked it with themed
// episodes or commercials; solemn and contested days are not.
//
// The first five predate the country calendar and keep its behaviour: a name alone benches. The
// later ones are also ordinary titles (a film called "Independence Day" is not shelved eleven
// months a year), so for them a name alone boosts in season but benches only with more evidence.
func Seed() []Holiday {
	return []Holiday{
		{
			ID: "halloween", Label: "Halloween",
			Words: Words{
				Intents:    []string{"halloween"},
				Names:      []string{"halloween"},
				Moods:      []string{"spooky", "haunted", "ghost", "ghosts", "witch", "witches", "horror", "terror"},
				MoodGenres: append([][]string{{"horror"}}, familyFantasy...),
			},
			Kept:      []Keeping{keep(allCountries, fixed(time.October, 31), dates(time.October, 1, time.October, 31))},
			AirWindow: air(fixed(time.October, 31), dates(time.October, 15, time.November, 7)),
		},
		{
			ID: "thanksgiving", Label: "Thanksgiving",
			Words: Words{Intents: []string{"thanksgiving", "turkey day"}, Names: []string{"thanksgiving", "turkey day"}},
			Kept: []Keeping{
				keep([]string{"US"}, nth(4, time.Thursday, time.November), dates(time.November, 15, time.November, 30)),
				keep([]string{"CA"}, nth(2, time.Monday, time.October), around(13, 0)),
			},
			AirWindow: air(nth(4, time.Thursday, time.November), around(7, 3)),
		},
		{
			ID: "christmas", Label: "Christmas",
			Words: Words{
				Intents: []string{"christmas", "xmas", "santa", "noel", "yuletide"},
				Names:   []string{"christmas", "xmas", "santa", "noel", "yuletide"},
				// A character's pet name and a city are not Christmas proof.
				LookAlikes: []string{"santa's little helper", "santa clarita", "santa monica", "santa barbara", "santa fe", "santa cruz",
					"santa ana", "santa rosa", "santa maria", "santa catalina"},
			},
			// Through Boxing Day.
			Kept:      []Keeping{keep(allCountries, fixed(time.December, 25), dates(time.December, 1, time.December, 26))},
			AirWindow: air(fixed(time.December, 25), dates(time.December, 1, time.January, 2)),
		},
		{
			ID: "newyear", Label: "New Year",
			Words: Words{
				Intents:    []string{"new year's", "new years", "new year", "nye", "hogmanay"},
				Names:      []string{"new year's", "new years", "new year", "nye", "hogmanay"},
				LookAlikes: []string{"bill nye"},
			},
			Kept:      []Keeping{keep(allCountries, fixed(time.December, 31), dates(time.December, 27, time.January, 2))},
			AirWindow: air(fixed(time.December, 31), dates(time.December, 26, time.January, 7)),
		},
		{
			ID: "valentines", Label: "Valentine's Day",
			Words:     Words{Intents: []string{"valentine's", "valentines", "valentine"}, Names: []string{"valentine's", "valentines", "valentine"}},
			Kept:      []Keeping{keep(allCountries, fixed(time.February, 14), dates(time.February, 1, time.February, 14))},
			AirWindow: air(fixed(time.February, 14), dates(time.February, 7, time.February, 21)),
		},
		{
			ID: "easter", Label: "Easter", NameNeedsCorroboration: true,
			Words: Words{Intents: []string{"easter"}, Names: []string{"easter"}, LookAlikes: []string{"easter island"}},
			// The fortnight up to Easter Monday.
			Kept:      []Keeping{keep(allCountries, easter(0), around(12, 1))},
			AirWindow: air(easter(0), around(12, 1)),
		},
		{
			ID: "mothers-day", Label: "Mother's Day", NameNeedsCorroboration: true,
			Words: Words{
				Intents: []string{"mother's day", "mothers day", "mothering sunday"},
				Names:   []string{"mother's day", "mothers day", "mothering sunday"},
			},
			Kept: []Keeping{
				keep([]string{"US", "CA", "AU", "NZ"}, nth(2, time.Sunday, time.May), around(13, 0)),
				keep(britainIrl, easter(-21), around(13, 0)), // Mothering Sunday
			},
			AirWindow: air(nth(2, time.Sunday, time.May), around(13, 0)),
		},
		{
			ID: "fathers-day", Label: "Father's Day", NameNeedsCorroboration: true,
			Words: Words{Intents: []string{"father's day", "fathers day"}, Names: []string{"father's day", "fathers day"}},
			Kept: []Keeping{
				keep([]string{"US", "CA", "GB", "IE"}, nth(3, time.Sunday, time.June), around(13, 0)),
				keep(australasia, nth(1, time.Sunday, time.September), around(13, 0)),
			},
			AirWindow: air(nth(3, time.Sunday, time.June), around(13, 0)),
		},
		{
			ID: "st-patricks", Label: "St Patrick's Day", NameNeedsCorroboration: true,
			Words: Words{
				Intents: []string{"st patrick's", "st. patrick's", "saint patrick's", "st patricks", "st paddy's"},
				Names:   []string{"st patrick's", "st. patrick's", "saint patrick's", "st patricks", "st paddy's"},
			},
			Kept:      []Keeping{keep([]string{"IE", "US"}, fixed(time.March, 17), dates(time.March, 10, time.March, 17))},
			AirWindow: air(fixed(time.March, 17), dates(time.March, 10, time.March, 18)),
		},
		{
			ID: "independence-day", Label: "Independence Day", NameNeedsCorroboration: true,
			Words: Words{
				Intents: []string{"independence day", "fourth of july", "4th of july", "july 4th"},
				Names:   []string{"independence day", "fourth of july", "4th of july", "july 4th"},
			},
			Kept:      []Keeping{keep([]string{"US"}, fixed(time.July, 4), dates(time.June, 27, time.July, 4))},
			AirWindow: air(fixed(time.July, 4), dates(time.June, 27, time.July, 6)),
		},
		{
			ID: "canada-day", Label: "Canada Day", NameNeedsCorroboration: true,
			Words:     Words{Intents: []string{"canada day", "dominion day"}, Names: []string{"canada day", "dominion day"}},
			Kept:      []Keeping{keep([]string{"CA"}, fixed(time.July, 1), dates(time.June, 24, time.July, 1))},
			AirWindow: air(fixed(time.July, 1), dates(time.June, 24, time.July, 3)),
		},
		{
			ID: "bonfire-night", Label: "Bonfire Night", NameNeedsCorroboration: true,
			Words:     Words{Intents: []string{"bonfire night", "guy fawkes"}, Names: []string{"bonfire night", "guy fawkes"}},
			Kept:      []Keeping{keep([]string{"GB"}, fixed(time.November, 5), dates(time.October, 29, time.November, 5))},
			AirWindow: air(fixed(time.November, 5), dates(time.October, 29, time.November, 7)),
		},
		{
			ID: "memorial-day", Label: "Memorial Day", NameNeedsCorroboration: true,
			Words:     Words{Intents: []string{"memorial day"}, Names: []string{"memorial day"}},
			Kept:      []Keeping{keep([]string{"US"}, last(time.Monday, time.May), around(6, 0))},
			AirWindow: air(last(time.Monday, time.May), around(6, 0)),
		},
		{
			ID: "back-to-school", Label: "Back to school", NameNeedsCorroboration: true,
			Words: Words{Intents: []string{"back to school", "back-to-school"}, Names: []string{"back to school", "back-to-school"}},
			// The school year starts in summer: August in the north, late January down under.
			Kept: []Keeping{
				keep(northAmerica, fixed(time.September, 1), dates(time.August, 1, time.September, 15)),
				keep(britainIrl, fixed(time.September, 1), dates(time.August, 15, time.September, 10)),
				keep(australasia, fixed(time.February, 1), dates(time.January, 10, time.February, 7)),
			},
			AirWindow: air(fixed(time.September, 1), dates(time.August, 1, time.September, 15)),
		},
		{
			ID: GenericHoliday, Label: "Holidays (general)",
			Members: []string{"thanksgiving", "christmas", "newyear"},
		},
	}
}
