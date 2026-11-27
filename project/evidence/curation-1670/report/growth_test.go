package main

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/growth"
)

var testGrowth = GrowthParams{Margin: 3, Bar: 0.5, MaxShowShare: 0.15, DailyAirtime: 20 * time.Hour}

func show(key string, year int) growth.Title {
	return growth.Title{Key: keyOf(key), Kind: growth.Series, Year: year, Rating: "TV-PG", Genres: []string{"Comedy"}}
}

// A channel short of shows counts the library's clear fits toward its target, and never counts a
// title it already carries.
func TestGrowthCountsClearFitsTowardTheShowTarget(t *testing.T) {
	c := Channel{ID: "c", Number: 7, Lineup: []LineupTitle{
		{Key: "series:tvdb:90000601", Units: 100, RuntimeMs: 100 * 22 * 60_000},
		{Key: "series:tvdb:90000602", Units: 100, RuntimeMs: 100 * 22 * 60_000},
	}}
	seeds := []growth.Title{show("series:tvdb:90000601", 1990), show("series:tvdb:90000602", 1992)}
	library := []candidate{
		{title: show("series:tvdb:90000601", 1990), episodes: 100}, // already on the channel
		{title: show("series:tvdb:90000603", 1991), episodes: 40},
		{title: show("series:tvdb:90000604", 2015), episodes: 40}, // outside the years
	}
	r := growthRow(c, seeds, library, testGrowth)
	if r.Clear != 1 || r.ClearEpisodes != 40 || r.Out != 1 || r.Shows != 2 || r.Target.Shows != 7 {
		t.Fatalf("row %+v", r)
	}
	// A week of 20 h days is 382 half-hour-less-ads episodes; 240 are on hand with the clear fit.
	if r.ShortAfterClear != "142 ep, 4 shows" {
		t.Errorf("short after clear %q, want 142 ep, 4 shows", r.ShortAfterClear)
	}
}

// A single-show channel is that size by design (decision 6): it is never short.
func TestASingleShowChannelIsNeverShort(t *testing.T) {
	c := Channel{ID: "c", Lineup: []LineupTitle{{Key: "series:tvdb:90000601", Units: 300, RuntimeMs: 300 * 22 * 60_000}}}
	r := growthRow(c, []growth.Title{show("series:tvdb:90000601", 1990)}, []candidate{{title: show("series:tvdb:90000603", 1991), episodes: 40}}, testGrowth)
	if !r.Fixed || r.Clear != 0 || r.ShortAfterClear != "-" {
		t.Fatalf("row %+v", r)
	}
}
