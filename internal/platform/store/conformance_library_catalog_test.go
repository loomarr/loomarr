package store

import (
	"context"
	"testing"
	"time"
)

// The catalog cache holds what the walk asked for, hands the enricher new titles before stale
// ones, skips a key whose failed fetch is held back, reports whether a fetch changed anything, and
// finds every present file of the titles a record describes so they are written again.
func testLibraryCatalog(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_790_000_000_000).UTC()
	movie := LibraryCatalogKey{Provider: "tmdb", Kind: "movie", ExternalID: "90000101"}
	show := LibraryCatalogKey{Provider: "tmdb", Kind: "series", ExternalID: "90000102"}
	old := LibraryCatalogKey{Provider: "tmdb", Kind: "movie", ExternalID: "90000103"}

	if got, ok := ParseLibraryCatalogKey(movie.String()); !ok || got != movie {
		t.Fatalf("key round trip = %+v, %v", got, ok)
	}
	if _, ok := ParseLibraryCatalogKey(""); ok {
		t.Fatal("the empty key parsed")
	}

	if err := st.WantLibraryCatalog(ctx, []LibraryCatalogKey{movie, show, old}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveLibraryCatalog(ctx, old, `{"Kind":"movie"}`, at.Add(-100*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	// Wanting a cached key again keeps its record.
	if err := st.WantLibraryCatalog(ctx, []LibraryCatalogKey{old}); err != nil {
		t.Fatal(err)
	}
	if rec, fetched, err := st.LibraryCatalogRecord(ctx, old); err != nil || !fetched || rec != `{"Kind":"movie"}` {
		t.Fatalf("old = %q, %v, %v", rec, fetched, err)
	}
	if _, fetched, err := st.LibraryCatalogRecord(ctx, movie); err != nil || fetched {
		t.Fatalf("movie fetched before any fetch: %v, %v", fetched, err)
	}

	if err := st.FailLibraryCatalog(ctx, show, at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	due, err := st.DueLibraryCatalog(ctx, LibraryCatalogDue{Now: at, StaleBefore: at.Add(-90 * 24 * time.Hour), New: 10, Stale: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 2 || due[0] != movie || due[1] != old {
		t.Fatalf("due = %+v, want the new movie then the stale one (the failed show waits)", due)
	}
	if n, err := st.CountLibraryCatalog(ctx); err != nil || n != 3 {
		t.Fatalf("count = %d, %v", n, err)
	}

	if changed, err := st.SaveLibraryCatalog(ctx, movie, `{"Kind":"movie","IDs":[]}`, at); err != nil || !changed {
		t.Fatalf("first save changed = %v, %v", changed, err)
	}
	if changed, err := st.SaveLibraryCatalog(ctx, movie, `{"Kind":"movie","IDs":[]}`, at.Add(time.Minute)); err != nil || changed {
		t.Fatalf("same record changed = %v, %v", changed, err)
	}

	if err := st.SaveLibraryItems(ctx, []LibraryItem{
		{ID: "m1", Kind: "movie", Name: "Film", Catalog: movie},
		{ID: "s1", Kind: "series", Name: "Show", Catalog: show},
		{ID: "m2", Kind: "movie", Name: "Other"},
	}, at); err != nil {
		t.Fatal(err)
	}
	file := func(path, item, series string) LibraryFile {
		return LibraryFile{Path: path, Root: "/media", Size: 1, ModifiedNs: 1, Identity: "digest", Kind: "movie",
			Header: LibraryHeaderRead, SeenAt: at, ItemID: item, SeriesID: series}
	}
	gone := file("/media/film/old.mkv", "m1", "")
	gone.MissingAt = at
	if err := st.SaveLibraryFiles(ctx, []LibraryFile{
		file("/media/film/film.mkv", "m1", ""),
		file("/media/show/e1.mkv", "e1", "s1"),
		file("/media/other/other.mkv", "m2", ""),
		gone,
	}); err != nil {
		t.Fatal(err)
	}
	paths, err := st.RewriteLibraryCatalogFiles(ctx, show)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != (LibraryFilePath{Root: "/media", Path: "/media/show/e1.mkv"}) {
		t.Fatalf("series rewrite = %+v, want its episode", paths)
	}
	paths, err = st.RewriteLibraryCatalogFiles(ctx, movie)
	if err != nil || len(paths) != 1 || paths[0].Path != "/media/film/film.mkv" {
		t.Fatalf("movie rewrite = %+v, %v, want its one present file", paths, err)
	}
	// An episode's own key (its TVDB id) finds that episode's file only.
	episode := LibraryCatalogKey{Provider: "tvdb", Kind: "episode", ExternalID: "90000901"}
	if err := st.SaveLibraryItems(ctx, []LibraryItem{{ID: "e2", Kind: "episode", Name: "Pilot", Catalog: episode}}, at); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveLibraryFiles(ctx, []LibraryFile{file("/media/show/e2.mkv", "e2", "s1")}); err != nil {
		t.Fatal(err)
	}
	paths, err = st.RewriteLibraryCatalogFiles(ctx, episode)
	if err != nil || len(paths) != 1 || paths[0].Path != "/media/show/e2.mkv" {
		t.Fatalf("episode rewrite = %+v, %v, want the episode with that key", paths, err)
	}

	files, err := st.LibraryFiles(ctx, "/media")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		rewritten := f.Path == "/media/film/film.mkv" || f.Path == "/media/show/e1.mkv" || f.Path == "/media/show/e2.mkv"
		if rewritten != (f.Identity == "") {
			t.Errorf("%s identity = %q, rewritten %v", f.Path, f.Identity, rewritten)
		}
	}
}
