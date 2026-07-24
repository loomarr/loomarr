package api

import (
	"testing"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
)

// A season window the suggester put on a series pick ("classic Simpsons" → 1–10)
// must carry through lineupEntries onto the LineupEntry the scheduler enforces —
// the last hop of the §8 season-scope path. A movie carries none.
func TestLineupEntries_CarriesSeasonWindow(t *testing.T) {
	p := suggest.Proposal{
		Lineup: []suggest.ProposalItem{
			{MediaType: provision.Series, TVDBID: 71663, Name: "The Simpsons", SeasonMin: 1, SeasonMax: 10},
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
	if matrix.SeasonMin != 0 || matrix.SeasonMax != 0 {
		t.Errorf("movie must carry no season window, got [%d,%d]", matrix.SeasonMin, matrix.SeasonMax)
	}
}

// The read DTO surfaces a stored season window so the UI can show/edit "seasons
// 1–10" (it used to drop it — the entry looked unscoped even though the scheduler
// enforced the window).
func TestChannelToDTO_SurfacesSeasonWindow(t *testing.T) {
	ch := store.Channel{Lineup: []schedule.LineupEntry{
		{Key: provision.Key("series:tvdb:71663"), Title: "The Simpsons", SeasonMin: 1, SeasonMax: 10},
	}}
	dto := channelToDTO(ch, nil)
	if len(dto.Lineup) != 1 {
		t.Fatalf("want 1 lineup entry, got %d", len(dto.Lineup))
	}
	if dto.Lineup[0].SeasonMin != 1 || dto.Lineup[0].SeasonMax != 10 {
		t.Errorf("DTO window = [%d,%d], want [1,10]", dto.Lineup[0].SeasonMin, dto.Lineup[0].SeasonMax)
	}
}

// mergeLineupEdit must PRESERVE an existing window when a client's edit omits it
// (an old UI that doesn't send season fields must not wipe a scope on reorder), and
// SET it when the edit provides one.
func TestMergeLineupEdit_SeasonWindowPreserveAndSet(t *testing.T) {
	current := []schedule.LineupEntry{
		{Key: provision.Key("series:tvdb:71663"), Title: "The Simpsons", SeasonMin: 1, SeasonMax: 10},
		{Key: provision.Key("series:tmdb:615"), Title: "Futurama"},
	}
	// Edit omits the window on Simpsons (preserve) and sets one on Futurama.
	edit := []LineupEntryDTO{
		{Key: "series:tvdb:71663", Name: "The Simpsons"},
		{Key: "series:tmdb:615", Name: "Futurama", SeasonMin: 1, SeasonMax: 5},
	}
	out, err := mergeLineupEdit(current, edit)
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[provision.Key]schedule.LineupEntry{}
	for _, e := range out {
		byKey[e.Key] = e
	}
	if s := byKey["series:tvdb:71663"]; s.SeasonMin != 1 || s.SeasonMax != 10 {
		t.Errorf("omitted edit WIPED the Simpsons window: got [%d,%d], want [1,10] preserved", s.SeasonMin, s.SeasonMax)
	}
	if f := byKey["series:tmdb:615"]; f.SeasonMin != 1 || f.SeasonMax != 5 {
		t.Errorf("edit did not SET the Futurama window: got [%d,%d], want [1,5]", f.SeasonMin, f.SeasonMax)
	}
}
