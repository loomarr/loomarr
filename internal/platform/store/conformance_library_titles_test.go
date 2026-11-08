package store

import (
	"context"
	"slices"
	"testing"
	"time"
)

// The Library's titles with a file to play, by their provider ids: a movie by its own file, a
// series by any episode's, changed since a time; one with no present file is not listed.
func testLibraryTitles(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	old, recent := time.UnixMilli(1_800_000_000_000), time.UnixMilli(1_800_000_600_000)
	if err := st.SaveLibraryItems(ctx, []LibraryItem{
		{ID: "film", Kind: "movie", Name: "Film", Rating: "PG", Genres: []string{"Drama", "Comedy"},
			IDs:     []LibraryID{{"tmdb", "90000001"}, {"imdb", "tt90000001"}},
			Sources: []LibrarySource{{Path: "/m/film.mkv", Plays: true, SizeBytes: 4_000}, {Path: "/m/film.copy.mkv", SizeBytes: 9_000}}},
		{ID: "gone", Kind: "movie", Name: "Gone", IDs: []LibraryID{{"tmdb", "90000004"}},
			Sources: []LibrarySource{{Path: "/m/gone.mkv", Plays: true}}},
	}, old); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveLibraryItems(ctx, []LibraryItem{
		{ID: "show", Kind: "series", Name: "Show", Rating: "TV-Y", IDs: []LibraryID{{"tvdb", "90000002"}, {"tmdb", "90000003"}}},
	}, recent); err != nil {
		t.Fatal(err)
	}
	file := func(path, item, series string) LibraryFile {
		return LibraryFile{Path: path, Root: "/m", Size: 1, ModifiedNs: 1, Kind: "movie", Header: LibraryHeaderRead,
			SeenAt: recent, ItemID: item, SeriesID: series}
	}
	missing := file("/m/gone.mkv", "gone", "")
	missing.MissingAt = recent
	if err := st.SaveLibraryFiles(ctx, []LibraryFile{
		file("/m/film.mkv", "film", ""), file("/m/show/e1.mkv", "show-e1", "show"), missing,
	}); err != nil {
		t.Fatal(err)
	}

	all, err := st.LibraryTitles(ctx, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	want := []LibraryTitle{
		{ID: "film", Kind: "movie", Name: "Film", TMDBID: "90000001", Rating: "PG", SizeBytes: 4_000},
		{ID: "show", Kind: "series", Name: "Show", TMDBID: "90000003", TVDBID: "90000002", Rating: "TV-Y"},
	}
	if !slices.EqualFunc(all, want, sameTitle) {
		t.Errorf("all titles = %+v, want %+v", all, want)
	}
	since, err := st.LibraryTitles(ctx, recent)
	if err != nil || len(since) != 1 || since[0].ID != "show" {
		t.Errorf("titles changed since = %+v, %v; want the show", since, err)
	}

	got, ok, err := st.LibraryTitleByProviderID(ctx, "movie", "tmdb", "90000001")
	if err != nil || !ok || !sameTitle(got, want[0]) || !slices.Equal(got.Genres, []string{"Drama", "Comedy"}) {
		t.Errorf("film by tmdb = %+v, %v, %v", got, ok, err)
	}
	if got, ok, err := st.LibraryTitleByProviderID(ctx, "series", "tvdb", "90000002"); err != nil || !ok || got.ID != "show" {
		t.Errorf("show by tvdb = %+v, %v, %v", got, ok, err)
	}
	for _, probe := range [][3]string{
		{"movie", "tmdb", "90000003"},  // the show's id, asked as a movie
		{"movie", "tmdb", "90000004"},  // a movie whose file went missing
		{"series", "tvdb", "90000009"}, // nothing
	} {
		if got, ok, err := st.LibraryTitleByProviderID(ctx, probe[0], probe[1], probe[2]); err != nil || ok {
			t.Errorf("%v = %+v, %v, %v; want none", probe, got, ok, err)
		}
	}
}

func sameTitle(a, b LibraryTitle) bool {
	return a.ID == b.ID && a.Kind == b.Kind && a.Name == b.Name && a.TMDBID == b.TMDBID && a.TVDBID == b.TVDBID &&
		a.Rating == b.Rating && a.SizeBytes == b.SizeBytes
}
