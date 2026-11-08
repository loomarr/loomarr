package store

import (
	"context"
	"slices"
	"testing"
	"time"
)

// A library series' episodes: only those with a present file to play, in season and episode order,
// with the playing file's duration and the series' rating when an episode has none.
func testLibraryEpisodes(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_800_000_000_000)
	show := LibraryOrigin{Authority: "scan:test", Key: "series:dir:show"}
	ep := func(id string, season, episode int, path string) LibraryItem {
		return LibraryItem{ID: id, Kind: "episode", Name: id, Series: show, Season: season, Episode: episode,
			RuntimeMs: 1_200_000, Sources: []LibrarySource{{Path: path, Plays: true}}}
	}
	second := ep("s01e02", 1, 2, "/t/show/s01e02.mkv")
	second.Sources[0].DurationMs = 1_320_000
	second.Rating, second.Score, second.Overview = "TV-14", 8.5, "The second."
	second.PremiereDate, second.Year, second.EpisodeEnd = "1990-10-25", 1990, 3
	second.Tags = []string{"two-parter", "finale"}
	showID := inventorySeries(t, st, show, at)
	items := []LibraryItem{
		{ID: showID, Kind: "series", Name: "Show", Rating: "TV-PG", Origin: show},
		ep("s02e01", 2, 1, "/t/show/s02e01.mkv"),
		second,
		ep("s01e01", 1, 1, "/t/show/s01e01.mkv"),
		ep("gone", 1, 4, "/t/show/s01e04.mkv"),
		ep("unscanned", 1, 5, "/t/show/s01e05.mkv"),
		{ID: "film", Kind: "movie", Name: "Film"},
	}
	if err := st.SaveLibraryItems(ctx, items, at); err != nil {
		t.Fatal(err)
	}
	file := func(path string) LibraryFile {
		return LibraryFile{Path: path, Root: "/t", Size: 9, ModifiedNs: 100, Identity: "digest", Kind: "episode",
			Header: LibraryHeaderRead, SeenAt: at}
	}
	missing := file("/t/show/s01e04.mkv")
	missing.MissingAt = at
	if err := st.SaveLibraryFiles(ctx, []LibraryFile{
		file("/t/show/s01e01.mkv"), file("/t/show/s01e02.mkv"), file("/t/show/s02e01.mkv"), missing,
	}); err != nil {
		t.Fatal(err)
	}

	eps, found, err := st.LibraryEpisodes(ctx, showID)
	if err != nil || !found {
		t.Fatalf("found = %v, %v; want the series", found, err)
	}
	var order []string
	for _, e := range eps {
		order = append(order, e.ItemID)
	}
	if want := []string{"s01e01", "s01e02", "s02e01"}; !slices.Equal(order, want) {
		t.Fatalf("episodes = %v, want %v: the ones with a present file, in order", order, want)
	}
	if e := eps[0]; e.Rating != "TV-PG" || e.DurationMs != 1_200_000 || e.Season != 1 || e.Episode != 1 {
		t.Errorf("s01e01 = %+v, want the series' rating and the catalog runtime", e)
	}
	e := eps[1]
	if e.Name != "s01e02" || e.Rating != "TV-14" || e.Score != 8.5 || e.Overview != "The second." ||
		e.PremiereDate != "1990-10-25" || e.Year != 1990 || e.EpisodeEnd != 3 || e.DurationMs != 1_320_000 ||
		!slices.Equal(e.Tags, []string{"two-parter", "finale"}) {
		t.Errorf("s01e02 = %+v, want its own facts and the file's duration", e)
	}

	for _, id := range []string{"film", "s01e01", "90000001"} {
		if eps, found, err := st.LibraryEpisodes(ctx, id); err != nil || found || len(eps) != 0 {
			t.Errorf("%s: %v, %v, %v; want no series", id, eps, found, err)
		}
	}
}
