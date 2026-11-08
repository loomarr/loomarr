package store

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"
)

// inventorySeries records a series under origin, as the scanner does before projecting it, and
// returns the inventory id its library item takes: an episode links to its series through it.
func inventorySeries(t *testing.T, st Store, origin LibraryOrigin, at time.Time) string {
	t.Helper()
	snapshot := inventorySnapshot(origin.Authority, origin.Key, origin.Key+"#source", "r1", "Show", at)
	snapshot.Kind = "series"
	id, err := st.ApplyInventorySnapshot(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return string(id)
}

// A library item's facts: its playing file's duration over the catalog runtime, an episode's
// series' genres and rating when it has none, and only library ids answered, across chunks.
func testLibraryItemFacts(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_800_000_000_000)
	show := LibraryOrigin{Authority: "scan:test", Key: "series:dir:show"}
	showID := inventorySeries(t, st, show, at)
	items := []LibraryItem{
		{ID: "film", Kind: "movie", Name: "Film", Year: 1999, Overview: "A film.", Rating: "PG", RuntimeMs: 5_400_000,
			Genres:  []string{"Drama", "Comedy"},
			Sources: []LibrarySource{{Path: "/m/film.mkv", Plays: true, DurationMs: 5_432_100}}},
		{ID: "bare", Kind: "movie", Name: "Bare", RuntimeMs: 3_600_000},
		{ID: showID, Kind: "series", Name: "Show", Rating: "TV-PG", Genres: []string{"Animation"}, Origin: show},
		{ID: "ep", Kind: "episode", Name: "Pilot", Overview: "The first.", Series: show, Season: 1, Episode: 1,
			Sources: []LibrarySource{{Path: "/t/show/s01e01.mkv", Plays: true, DurationMs: 1_320_000}}},
	}
	if err := st.SaveLibraryItems(ctx, items, at); err != nil {
		t.Fatal(err)
	}
	ids := []string{"film", "bare", "ep", "90000001"}
	for i := range 600 {
		ids = append(ids, fmt.Sprintf("absent-%d", i))
	}
	ids = append(ids, showID)
	got, err := st.LibraryItemFacts(ctx, ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Errorf("%d answered, want the 4 library items: %+v", len(got), got)
	}
	check := func(id string, want LibraryItemFacts) {
		t.Helper()
		g := got[id]
		if g.Overview != want.Overview || g.Year != want.Year || g.Rating != want.Rating || g.RuntimeMs != want.RuntimeMs ||
			!slices.Equal(g.Genres, want.Genres) {
			t.Errorf("%s = %+v, want %+v", id, g, want)
		}
	}
	check("film", LibraryItemFacts{Overview: "A film.", Year: 1999, Rating: "PG", RuntimeMs: 5_432_100, Genres: []string{"Drama", "Comedy"}})
	check("bare", LibraryItemFacts{RuntimeMs: 3_600_000})
	check("ep", LibraryItemFacts{Overview: "The first.", Rating: "TV-PG", RuntimeMs: 1_320_000, Genres: []string{"Animation"}})
	check(showID, LibraryItemFacts{Rating: "TV-PG", Genres: []string{"Animation"}})
}
