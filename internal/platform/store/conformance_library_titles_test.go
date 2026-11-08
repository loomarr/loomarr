package store

import (
	"context"
	"maps"
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

	// A title's art by the provider id an airing's key names (#2567): the film's logo and
	// backdrop, the show's backdrop by either id; a missing film's art, a poster, an id asked as
	// the wrong kind and an unknown id are not answered.
	logo, fanart, showFanart, poster, goneLogo := imageAt("film-logo", recent), imageAt("film-fanart", recent),
		imageAt("show-fanart", recent), imageAt("show-poster", recent), imageAt("gone-logo", recent)
	for _, img := range []Image{logo, fanart, showFanart, poster, goneLogo} {
		if err := st.PutImage(ctx, img); err != nil {
			t.Fatal(err)
		}
	}
	for _, ref := range []ImageRef{
		{ImageHash: logo.Hash, OwnerKind: LibraryItemImageOwner, OwnerID: "film", Role: "logo"},
		{ImageHash: fanart.Hash, OwnerKind: LibraryItemImageOwner, OwnerID: "film", Role: "backdrop"},
		{ImageHash: showFanart.Hash, OwnerKind: LibraryItemImageOwner, OwnerID: "show", Role: "backdrop"},
		{ImageHash: poster.Hash, OwnerKind: LibraryItemImageOwner, OwnerID: "show", Role: "poster"},
		{ImageHash: goneLogo.Hash, OwnerKind: LibraryItemImageOwner, OwnerID: "gone", Role: "logo"},
	} {
		if err := st.PutImageRef(ctx, ref); err != nil {
			t.Fatal(err)
		}
	}
	filmID := LibraryProviderID{Kind: "movie", Namespace: "tmdb", Value: "90000001"}
	showTVDB := LibraryProviderID{Kind: "series", Namespace: "tvdb", Value: "90000002"}
	showTMDB := LibraryProviderID{Kind: "series", Namespace: "tmdb", Value: "90000003"}
	art, err := st.LibraryTitleArt(ctx, []LibraryProviderID{
		filmID, showTVDB, showTMDB,
		{Kind: "movie", Namespace: "tmdb", Value: "90000004"},  // the missing film
		{Kind: "movie", Namespace: "tmdb", Value: "90000003"},  // the show's id, asked as a movie
		{Kind: "series", Namespace: "tvdb", Value: "90000009"}, // nothing
	})
	if err != nil {
		t.Fatal(err)
	}
	wantArt := map[LibraryProviderID]LibraryTitleArt{
		filmID:   {Backdrop: fanart.Hash, Logo: logo.Hash},
		showTVDB: {Backdrop: showFanart.Hash},
		showTMDB: {Backdrop: showFanart.Hash},
	}
	if !maps.Equal(art, wantArt) {
		t.Errorf("title art = %+v, want %+v", art, wantArt)
	}
	if none, err := st.LibraryTitleArt(ctx, nil); err != nil || len(none) != 0 {
		t.Errorf("no ids = %+v, %v; want an empty map", none, err)
	}

	// Search by name (#2151 step 15e): an exact name before one that only holds the term, the
	// missing film never, LIKE's wildcards taken literally, and the limit kept.
	if err := st.SaveLibraryItems(ctx, []LibraryItem{
		{ID: "showtime", Kind: "movie", Name: "Showtime 100%", IDs: []LibraryID{{"tmdb", "90000005"}},
			Sources: []LibrarySource{{Path: "/m/showtime.mkv", Plays: true}}},
	}, recent); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveLibraryFiles(ctx, []LibraryFile{file("/m/showtime.mkv", "showtime", "")}); err != nil {
		t.Fatal(err)
	}
	found, err := st.SearchLibraryTitles(ctx, " SHOW ", 10)
	if err != nil || len(found) != 2 || found[0].ID != "show" || found[1].ID != "showtime" || found[1].TMDBID != "90000005" {
		t.Errorf("search show = %+v, %v; want Show, then Showtime 100%%", found, err)
	}
	if one, err := st.SearchLibraryTitles(ctx, "show", 1); err != nil || len(one) != 1 || one[0].ID != "show" {
		t.Errorf("search show, limit 1 = %+v, %v", one, err)
	}
	if lit, err := st.SearchLibraryTitles(ctx, "0%", 10); err != nil || len(lit) != 1 || lit[0].ID != "showtime" {
		t.Errorf("search 0%% = %+v, %v; want the percent taken literally", lit, err)
	}
	for _, term := range []string{"gone", "_", ""} {
		if got, err := st.SearchLibraryTitles(ctx, term, 10); err != nil || len(got) != 0 {
			t.Errorf("search %q = %+v, %v; want nothing", term, got, err)
		}
	}
}

func sameTitle(a, b LibraryTitle) bool {
	return a.ID == b.ID && a.Kind == b.Kind && a.Name == b.Name && a.TMDBID == b.TMDBID && a.TVDBID == b.TVDBID &&
		a.Rating == b.Rating && a.SizeBytes == b.SizeBytes
}
