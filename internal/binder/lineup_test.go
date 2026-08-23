package binder

import (
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/suggest"
)

// A season window the suggester put on a series pick ("classic Simpsons" → 1–10)
// must carry through lineupEntries onto the LineupEntry the scheduler enforces —
// the last hop of the §8 season-scope path. A movie carries none.
func TestLineupEntriesCarriesSeasonWindow(t *testing.T) {
	p := suggest.Proposal{
		Lineup: []suggest.ProposalItem{
			{MediaType: provision.Series, TVDBID: 71663, Name: "The Simpsons", SeasonMin: 1, SeasonMax: 10,
				EpisodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeHighlights}},
			{MediaType: provision.Movie, TMDBID: 603, Name: "The Matrix"},
		},
	}
	entries, err := lineupEntries(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(entries))
	}
	var simpsons, matrix schedule.LineupEntry
	for _, e := range entries {
		switch e.Title {
		case "The Simpsons":
			simpsons = e
		case "The Matrix":
			matrix = e
		}
	}
	if simpsons.SeasonMin != 1 || simpsons.SeasonMax != 10 {
		t.Errorf("Simpsons window = [%d,%d], want [1,10]", simpsons.SeasonMin, simpsons.SeasonMax)
	}
	if simpsons.EpisodeSelection.Mode != schedule.EpisodeHighlights {
		t.Errorf("Simpsons episode selection = %+v, want highlights", simpsons.EpisodeSelection)
	}
	if matrix.SeasonMin != 0 || matrix.SeasonMax != 0 {
		t.Errorf("movie must carry no season window, got [%d,%d]", matrix.SeasonMin, matrix.SeasonMax)
	}
}
