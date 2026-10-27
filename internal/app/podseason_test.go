package app

import (
	"log/slog"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/holidayvocab"
)

// The pod assembler dates holidays on the household calendar (#2030): the stored definitions and
// filler.home_country reach it, so a holiday kept only in GB is in season for a GB household and
// out of season, all year, for a US one.
func TestPodAdapter_DatesHolidaysOnTheHouseholdCalendar(t *testing.T) {
	allYear := holidayvocab.Rule{
		Day:    holidayvocab.DayRule{Kind: holidayvocab.DayFixed, Month: time.July, Day: 1},
		Window: holidayvocab.WindowRule{Kind: holidayvocab.WindowRange, From: holidayvocab.MonthDay{Month: time.January, Day: 1}, To: holidayvocab.MonthDay{Month: time.December, Day: 31}},
	}
	vocab, err := holidayvocab.New([]holidayvocab.Holiday{{ID: "pancake-day", Label: "Pancake Day",
		Kept: []holidayvocab.Keeping{{Countries: []string{"GB"}, Rule: allYear}}}})
	if err != nil {
		t.Fatal(err)
	}
	holidays := func() holidayvocab.Vocabulary { return vocab }

	for _, tc := range []struct {
		country string
		want    filler.FitReason
	}{{"GB", ""}, {"US", filler.FitSeason}} {
		// A national ad of the household's own country, so only the season can refuse it.
		ad := filler.Clip{Hash: "ad", Path: "ad.mp4", Kind: filler.Commercial, Audience: filler.General,
			DurationMs: 15_000, Tags: []string{"pancake-day"}, AssertedTags: []string{"pancake-day"},
			GeographicScope: filler.GeographicNational, Country: tc.country}
		set := visionSet(t, map[string]string{"filler.home_country": tc.country})
		log := slog.New(slog.DiscardHandler)
		pods := buildPodAdapter(nil, set, householdCalendar(set, holidays, log), log)
		if got := pods.FitForChannel("ch", filler.Selection{}, ad).Reason; got != tc.want {
			t.Errorf("%s household: fit reason = %q, want %q", tc.country, got, tc.want)
		}
	}
}

// filler.language reaches the pod assembler as every channel's default commercial language
// (#2030 decision 12): a channel naming none refuses a clip in another language.
func TestPodAdapter_TheHouseholdLanguageIsEveryChannelsDefault(t *testing.T) {
	log := slog.New(slog.DiscardHandler)
	holidays := func() holidayvocab.Vocabulary { return holidayvocab.Vocabulary{} }
	for _, tc := range []struct {
		household string
		want      filler.FitReason
	}{{"es", ""}, {"en", filler.FitLanguage}} {
		ad := filler.Clip{Hash: "ad", Path: "ad.mp4", Kind: filler.Commercial, Audience: filler.General,
			DurationMs: 15_000, Language: "es"}
		set := visionSet(t, map[string]string{"filler.language": tc.household})
		pods := buildPodAdapter(nil, set, householdCalendar(set, holidays, log), log)
		if got := pods.FitForChannel("ch", filler.Selection{}, ad).Reason; got != tc.want {
			t.Errorf("%s household: fit reason = %q, want %q", tc.household, got, tc.want)
		}
	}
}
