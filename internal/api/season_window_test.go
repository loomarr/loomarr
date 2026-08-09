package api

import (
	"testing"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

// The read DTO surfaces a stored season window so the UI can show/edit "seasons
// 1–10" (it used to drop it — the entry looked unscoped even though the scheduler
// enforced the window).
func TestChannelToDTO_SurfacesSeasonWindow(t *testing.T) {
	ch := store.Channel{Lineup: []schedule.LineupEntry{
		{Key: provision.Key("series:tvdb:71663"), Title: "The Simpsons", SeasonMin: 1, SeasonMax: 10},
	}}
	dto := channelToDTO(ch, nil, nil)
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
