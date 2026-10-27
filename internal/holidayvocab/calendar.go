package holidayvocab

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// DayKind names how a holiday's own date is found in a year. The set is closed: a stored rule of
// any other kind is rejected on write rather than guessed at (#2030).
type DayKind string

const (
	DayFixed       DayKind = "fixed"        // Month, Day
	DayNthWeekday  DayKind = "nth_weekday"  // the Nth Weekday of Month, N from 1
	DayLastWeekday DayKind = "last_weekday" // the last Weekday of Month
	DayEaster      DayKind = "easter"       // Western Easter Sunday plus Offset days
)

// WindowKind names how a holiday's season is laid around its date.
type WindowKind string

const (
	WindowRange  WindowKind = "range"  // From through To; a To before From is next year's
	WindowAround WindowKind = "around" // Before days up to the date, through After days past it
)

// MonthDay is a date without a year.
type MonthDay struct {
	Month time.Month `json:"month"`
	Day   int        `json:"day"`
}

// DayRule finds a holiday's own date in a year.
type DayRule struct {
	Kind    DayKind      `json:"kind" enum:"fixed,nth_weekday,last_weekday,easter"`
	Month   time.Month   `json:"month,omitempty"`
	Day     int          `json:"day,omitempty"`
	Weekday time.Weekday `json:"weekday,omitempty"`
	N       int          `json:"n,omitempty"`
	Offset  int          `json:"offset,omitempty"`
}

// WindowRule lays a holiday's season, as whole dates.
type WindowRule struct {
	Kind   WindowKind `json:"kind" enum:"range,around"`
	From   MonthDay   `json:"from,omitzero"`
	To     MonthDay   `json:"to,omitzero"`
	Before int        `json:"before,omitempty"`
	After  int        `json:"after,omitempty"`
}

// Rule is a holiday's date and the season around it.
type Rule struct {
	Day    DayRule    `json:"day"`
	Window WindowRule `json:"window"`
}

// Keeping is when the listed countries keep a holiday.
type Keeping struct {
	Countries []string `json:"countries"`
	Rule
}

// Observance is one holiday as one country keeps it: its season window for a year, inclusive of
// End, and the holiday's own date (the night of a holiday-episode marathon). Only month and day of
// Day are read. Windows may straddle a year boundary (New Year); evaluate the prior year's too.
type Observance struct {
	ID     string
	Window func(year int) (start, end time.Time)
	Day    func(year int) time.Time
}

// defaultCountry is the calendar of an installation with no country set: today's behaviour.
const defaultCountry = "US"

// Date is the holiday's own date in year, at midnight UTC.
func (r DayRule) Date(year int) time.Time {
	switch r.Kind {
	case DayFixed:
		return time.Date(year, r.Month, r.Day, 0, 0, 0, 0, time.UTC)
	case DayNthWeekday:
		first := time.Date(year, r.Month, 1, 0, 0, 0, 0, time.UTC)
		return first.AddDate(0, 0, (int(r.Weekday)-int(first.Weekday())+7)%7+7*(r.N-1))
	case DayLastWeekday:
		last := time.Date(year, r.Month+1, 0, 0, 0, 0, 0, time.UTC)
		return last.AddDate(0, 0, -((int(last.Weekday()) - int(r.Weekday) + 7) % 7))
	case DayEaster:
		return easterSunday(year).AddDate(0, 0, r.Offset)
	}
	return time.Time{}
}

// Span is the season in year: its first date's midnight through the last instant of its last date.
func (r Rule) Span(year int) (start, end time.Time) {
	if r.Window.Kind == WindowRange {
		start = time.Date(year, r.Window.From.Month, r.Window.From.Day, 0, 0, 0, 0, time.UTC)
		last := time.Date(year, r.Window.To.Month, r.Window.To.Day, 0, 0, 0, 0, time.UTC)
		if last.Before(start) {
			last = last.AddDate(1, 0, 0)
		}
		return start, endOfDay(last)
	}
	d := r.Day.Date(year)
	return d.AddDate(0, 0, -r.Window.Before), endOfDay(d.AddDate(0, 0, r.Window.After))
}

func endOfDay(d time.Time) time.Time { return d.AddDate(0, 0, 1).Add(-time.Nanosecond) }

// Validate rejects a rule this package cannot evaluate exactly.
func (r Rule) Validate() error {
	switch r.Day.Kind {
	case DayFixed:
		if err := validMonthDay(MonthDay{r.Day.Month, r.Day.Day}); err != nil {
			return fmt.Errorf("day: %w", err)
		}
	case DayNthWeekday:
		if err := validMonthWeekday(r.Day); err != nil {
			return err
		}
		if r.Day.N < 1 || r.Day.N > 4 {
			return fmt.Errorf("day: n %d is not 1 to 4", r.Day.N)
		}
	case DayLastWeekday:
		if err := validMonthWeekday(r.Day); err != nil {
			return err
		}
	case DayEaster:
		if r.Day.Offset < -70 || r.Day.Offset > 70 {
			return fmt.Errorf("day: an Easter offset of %d days is out of range", r.Day.Offset)
		}
	default:
		return fmt.Errorf("day: unknown kind %q", r.Day.Kind)
	}
	switch r.Window.Kind {
	case WindowRange:
		if err := validMonthDay(r.Window.From); err != nil {
			return fmt.Errorf("window from: %w", err)
		}
		if err := validMonthDay(r.Window.To); err != nil {
			return fmt.Errorf("window to: %w", err)
		}
	case WindowAround:
		if r.Window.Before < 0 || r.Window.After < 0 || r.Window.Before+r.Window.After > 92 {
			return fmt.Errorf("window: %d days before and %d after is not a season", r.Window.Before, r.Window.After)
		}
	default:
		return fmt.Errorf("window: unknown kind %q", r.Window.Kind)
	}
	return nil
}

func validMonthDay(md MonthDay) error {
	if md.Month < time.January || md.Month > time.December {
		return fmt.Errorf("month %d is not 1 to 12", md.Month)
	}
	// 2024 is a leap year, so February 29 is a real date.
	if md.Day < 1 || time.Date(2024, md.Month, md.Day, 0, 0, 0, 0, time.UTC).Month() != md.Month {
		return fmt.Errorf("%s has no day %d", md.Month, md.Day)
	}
	return nil
}

func validMonthWeekday(d DayRule) error {
	if d.Month < time.January || d.Month > time.December {
		return fmt.Errorf("day: month %d is not 1 to 12", d.Month)
	}
	if d.Weekday < time.Sunday || d.Weekday > time.Saturday {
		return fmt.Errorf("day: weekday %d is not 0 to 6", d.Weekday)
	}
	return nil
}

func validCountry(c string) error {
	if len(c) != 2 || strings.ToUpper(c) != c || strings.Trim(c, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") != "" {
		return fmt.Errorf("country %q is not an ISO 3166-1 alpha-2 code", c)
	}
	return nil
}

// Calendar is the holidays country keeps, in the vocabulary's order. An empty country is the US;
// a country no holiday names keeps none. Undated and group holidays are not on it: a group has no
// window of its own (InSeason answers for it).
func (v Vocabulary) Calendar(country string) []Observance {
	country = v.CalendarCountry(country)
	var out []Observance
	for _, h := range v.holidays {
		if k, ok := h.keptIn(country); ok {
			out = append(out, Observance{ID: h.ID, Window: k.Span, Day: k.Day.Date})
		}
	}
	return out
}

// Countries lists every country some holiday names, sorted.
func (v Vocabulary) Countries() []string {
	var out []string
	for _, h := range v.holidays {
		for _, k := range h.Kept {
			for _, c := range k.Countries {
				if !slices.Contains(out, c) {
					out = append(out, c)
				}
			}
		}
	}
	slices.Sort(out)
	return out
}

// CalendarCountry is the country whose calendar country reads: itself when some holiday is kept
// there, else the US, so a household in a country no definition names keeps the calendar every
// household had before countries rather than none.
func (v Vocabulary) CalendarCountry(country string) string {
	country = strings.ToUpper(strings.TrimSpace(country))
	if slices.Contains(v.Countries(), country) {
		return country
	}
	return defaultCountry
}

func (h Holiday) keptIn(country string) (Keeping, bool) {
	for _, k := range h.Kept {
		if slices.Contains(k.Countries, country) {
			return k, true
		}
	}
	return Keeping{}, false
}

// easterSunday is Western Easter: the anonymous Gregorian computus.
func easterSunday(year int) time.Time {
	a, b, c := year%19, year/100, year%100
	d, e := b/4, b%4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i, k := c/4, c%4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	dayOfMonth := (h+l-7*m+114)%31 + 1
	return time.Date(year, time.Month(month), dayOfMonth, 0, 0, 0, 0, time.UTC)
}
