package schedule_test

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/holidayvocab"
	"github.com/loomarr/loomarr/internal/schedule"
)

// SeasonAt is filler's view of the calendar (#2030 decision 5): which holidays have a season, and
// which are in season on t's date on the calendar's wall clock.
func TestSeasonAtNamesDatedAndInSeasonHolidays(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	cal := schedule.Calendar{Zone: ny, Country: "US"}

	// 02:00 UTC on Nov 1 is still Oct 31 in New York.
	s := cal.SeasonAt(time.Date(2026, time.November, 1, 2, 0, 0, 0, time.UTC))
	if !s.InSeason["halloween"] {
		t.Error("Halloween should be in season on the evening of Oct 31 in New York")
	}
	if s.InSeason["christmas"] || !s.Dated["christmas"] {
		t.Errorf("Christmas: dated=%v inSeason=%v, want dated and out of season", s.Dated["christmas"], s.InSeason["christmas"])
	}

	dec20 := cal.SeasonAt(time.Date(2026, time.December, 20, 12, 0, 0, 0, time.UTC))
	if !dec20.InSeason["christmas"] || !dec20.InSeason[schedule.GenericHolidayTag] {
		t.Errorf("Dec 20 in season = %v, want christmas and the generic holiday group", dec20.InSeason)
	}
	// Bonfire Night has dates, but only in GB: a US household never has it in season.
	if !dec20.Dated["bonfire-night"] {
		t.Error("Bonfire Night has dates somewhere, so its clips are seasonal")
	}
}

// An undated holiday (an operator's seasonal tag with no dates yet) is not seasonal: its clips air
// year-round. A zero instant is no date, so nothing is in season.
func TestSeasonAtUndatedAndZeroInstant(t *testing.T) {
	vocab, err := holidayvocab.New([]holidayvocab.Holiday{
		{ID: "advent", Label: "Advent"},
		{ID: "epiphany", Label: "Epiphany", Kept: []holidayvocab.Keeping{{Countries: []string{"IE"}, Rule: holidayvocab.Rule{
			Day:    holidayvocab.DayRule{Kind: holidayvocab.DayFixed, Month: time.January, Day: 6},
			Window: holidayvocab.WindowRule{Kind: holidayvocab.WindowAround, Before: 3}}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	cal := schedule.Calendar{Country: "IE", Holidays: &vocab}

	s := cal.SeasonAt(time.Date(2027, time.January, 5, 12, 0, 0, 0, time.UTC))
	if s.Dated["advent"] || s.InSeason["advent"] {
		t.Errorf("an undated holiday is neither dated nor in season: %+v", s)
	}
	if !s.InSeason["epiphany"] {
		t.Errorf("Epiphany should be in season on Jan 5: %+v", s)
	}

	zero := cal.SeasonAt(time.Time{})
	if !zero.Dated["epiphany"] || len(zero.InSeason) != 0 {
		t.Errorf("zero instant = %+v, want Epiphany dated and nothing in season", zero)
	}
}
