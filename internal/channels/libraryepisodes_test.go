package channels

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store"
)

type libraryEpisodesFake map[string][]store.LibraryEpisode

func (f libraryEpisodesFake) LibraryEpisodes(_ context.Context, seriesID string) ([]store.LibraryEpisode, bool, error) {
	eps, ok := f[seriesID]
	return eps, ok, nil
}

// A library series expands from the Library's tables: an episode that cannot play is dropped, and
// a series with nothing to play, or an id that is no library series, stays empty.
func TestLibraryEpisodeResolverReadsTheTables(t *testing.T) {
	fake := libraryEpisodesFake{
		"lib-show": {
			{ItemID: "lib-e1", Name: "Pilot", Season: 1, Episode: 1, Year: 1992, PremiereDate: "1992-09-14",
				Rating: "TV-PG", Score: 7.5, Overview: "The first.", Tags: []string{"pilot"}, DurationMs: 1_320_000},
			{ItemID: "lib-e2", Name: "Not yet read", Season: 1, Episode: 2},
		},
		"lib-empty": nil,
	}
	resolve := LibraryEpisodeResolver(fake)
	got, err := resolve(context.Background(), "lib-show")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("resolved %+v, want only the playable episode", got)
	}
	e := got[0]
	if e.LibraryItemID != "lib-e1" || e.Title != "Pilot" || e.DurationMs != 1_320_000 || e.Season != 1 ||
		e.Episode != 1 || e.Year != 1992 || !e.AirDate.Equal(time.Date(1992, 9, 14, 0, 0, 0, 0, time.UTC)) ||
		e.OfficialRating == "" || e.CommunityRating != 7.5 || e.Overview != "The first." || len(e.Tags) != 1 {
		t.Errorf("episode = %+v", e)
	}
	for _, id := range []string{"lib-empty", "emby-show"} {
		if got, err := resolve(context.Background(), id); err != nil || len(got) != 0 {
			t.Errorf("%s resolved %+v, %v; want none", id, got, err)
		}
	}
}
