package store

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// Every series' playable episodes and their runtime together, counted through the present files
// that name the series: a missing file and a series with none are not counted (#4333).
func testSeriesEpisodeTotals(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_800_000_000_000)
	if err := st.SaveLibraryItems(ctx, []librarystore.LibraryItem{
		{ID: "show", Kind: "series", Name: "Show"},
		{ID: "e1", Kind: "episode", Name: "One", RuntimeMs: 1_320_000},
		{ID: "e2", Kind: "episode", Name: "Two", RuntimeMs: 1_380_000},
		{ID: "e3", Kind: "episode", Name: "Three", RuntimeMs: 1_400_000},
		{ID: "empty", Kind: "series", Name: "Empty"},
		{ID: "x1", Kind: "episode", Name: "Gone", RuntimeMs: 1_000_000},
	}, at); err != nil {
		t.Fatal(err)
	}
	file := func(path, item, series string) librarystore.LibraryFile {
		return librarystore.LibraryFile{Path: path, Root: "/t", Size: 1, ModifiedNs: 1, Kind: "episode", Header: librarystore.LibraryHeaderRead,
			SeenAt: at, ItemID: item, SeriesID: series}
	}
	missing := file("/t/empty/x1.mkv", "x1", "empty")
	missing.MissingAt = at
	second := file("/t/show/e1.copy.mkv", "e1", "show") // a second copy is still one episode
	if err := st.SaveLibraryFiles(ctx, []librarystore.LibraryFile{
		file("/t/show/e1.mkv", "e1", "show"), second, file("/t/show/e2.mkv", "e2", "show"),
		file("/t/show/e3.mkv", "e3", "show"), missing,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := st.SeriesEpisodeTotals(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["show"] != (librarystore.SeriesEpisodes{Episodes: 3, RuntimeMs: 4_100_000}) {
		t.Fatalf("totals %+v, want the show's 3 episodes, 4,100,000 ms", got)
	}
}
