package binder

import (
	"testing"

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
	entries, err := lineupEntries(p)
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
