package binder

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/suggest"
)

// A season window the suggester put on a series pick ("classic Mudpuddles" → 1–10)
// must carry through lineupEntries onto the LineupEntry the scheduler enforces —
// the last hop of the §8 season-scope path. A movie carries none.
func TestLineupEntriesCarriesSeasonWindow(t *testing.T) {
	t.Parallel()
	p := suggest.Proposal{
		Lineup: []suggest.ProposalItem{
			{MediaType: provision.Series, TVDBID: 90071663, Name: "The Mudpuddles", SeasonMin: 1, SeasonMax: 10,
				EpisodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeHighlights}},
			{MediaType: provision.Movie, TMDBID: 90000603, Name: "Marigold Protocol"},
		},
	}
	entries, err := lineupEntries(p, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(entries))
	}
	var series, movie schedule.LineupEntry
	for _, e := range entries {
		switch e.Title {
		case "The Mudpuddles":
			series = e
		case "Marigold Protocol":
			movie = e
		}
	}
	if series.SeasonMin != 1 || series.SeasonMax != 10 {
		t.Errorf("series window = [%d,%d], want [1,10]", series.SeasonMin, series.SeasonMax)
	}
	if series.EpisodeSelection.Mode != schedule.EpisodeHighlights {
		t.Errorf("series episode selection = %+v, want highlights", series.EpisodeSelection)
	}
	if movie.SeasonMin != 0 || movie.SeasonMax != 0 {
		t.Errorf("movie must carry no season window, got [%d,%d]", movie.SeasonMin, movie.SeasonMax)
	}
}

// Every entry the binder adds records the confidence of the pick that produced it (the fit
// retirement ranks on) and when it was added. A pick with no confidence stays unscored.
func TestLineupEntriesStampsFitAndAddedAt(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	p := suggest.Proposal{
		Lineup: []suggest.ProposalItem{
			{MediaType: provision.Movie, TMDBID: 90000601, Name: "Scored", Confidence: 0.85},
			{MediaType: provision.Movie, TMDBID: 90000602, Name: "Unscored"},
		},
		Acquisitions: []suggest.ProposalItem{
			{MediaType: provision.Movie, TMDBID: 90000603, Name: "Tiny", Confidence: 0.001},
		},
	}
	entries, err := lineupEntries(p, now)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{"Scored": 0.85, "Unscored": 0, "Tiny": schedule.MinFit}
	for _, e := range entries {
		if e.Fit != want[e.Title] {
			t.Errorf("%s fit = %v, want %v", e.Title, e.Fit, want[e.Title])
		}
		if !e.AddedAt.Equal(now) {
			t.Errorf("%s AddedAt = %v, want %v", e.Title, e.AddedAt, now)
		}
	}
}
