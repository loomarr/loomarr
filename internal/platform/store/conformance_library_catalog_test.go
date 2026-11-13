package store

import (
	"context"
	"slices"
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
	if _, err := st.SaveLibraryCatalog(ctx, old, `{"Kind":"movie"}`, 0, "en-US", at.Add(-100*24*time.Hour)); err != nil {
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
	// Due as new counts only the never-fetched movie: the failed show waits, and the old movie is
	// stale, not new.
	if n, err := st.CountDueLibraryCatalog(ctx, LibraryCatalogDue{Now: at, New: 1}); err != nil || n != 1 {
		t.Fatalf("due as new = %d, %v; want 1", n, err)
	}
	// Once the show's hold has passed, it counts too.
	if n, err := st.CountDueLibraryCatalog(ctx, LibraryCatalogDue{Now: at.Add(2 * time.Hour)}); err != nil || n != 2 {
		t.Fatalf("due as new after the hold = %d, %v; want 2", n, err)
	}

	if changed, err := st.SaveLibraryCatalog(ctx, movie, `{"Kind":"movie","IDs":[]}`, 0, "en-US", at); err != nil || !changed {
		t.Fatalf("first save changed = %v, %v", changed, err)
	}
	if changed, err := st.SaveLibraryCatalog(ctx, movie, `{"Kind":"movie","IDs":[]}`, 0, "en-US", at.Add(time.Minute)); err != nil || changed {
		t.Fatalf("same record changed = %v, %v", changed, err)
	}

	// A movie read in a newer shape: the fresh movie record saved in the old one is due with the
	// new titles, ahead of the stale one, and listed once; saved in the new shape, it is not due.
	moviesV1 := LibraryCatalogDue{Now: at, StaleBefore: at.Add(-90 * 24 * time.Hour), New: 10, Stale: 10,
		Versions: map[string]int{"movie": 1}}
	due, err = st.DueLibraryCatalog(ctx, moviesV1)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 2 || due[0] != movie || due[1] != old {
		t.Fatalf("due = %+v, want the outdated movie, then the old one once", due)
	}
	if _, err := st.SaveLibraryCatalog(ctx, movie, `{"Kind":"movie","IDs":[]}`, 1, "en-US", at.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if due, err = st.DueLibraryCatalog(ctx, moviesV1); err != nil || len(due) != 1 || due[0] != old {
		t.Fatalf("due = %+v, %v; want only the old movie once the fresh one is read in the new shape", due, err)
	}
	// The household's language changes: the fresh movie record, read in English, is due with the
	// new titles again; read in French, it is not.
	moviesV1.Language = "fr-US"
	if due, err = st.DueLibraryCatalog(ctx, moviesV1); err != nil || len(due) != 2 || due[0] != movie || due[1] != old {
		t.Fatalf("due = %+v, %v; want the movie read in another language, then the old one once", due, err)
	}
	if _, err := st.SaveLibraryCatalog(ctx, movie, `{"Kind":"movie","IDs":[]}`, 1, "fr-US", at.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if due, err = st.DueLibraryCatalog(ctx, moviesV1); err != nil || len(due) != 1 || due[0] != old {
		t.Fatalf("due = %+v, %v; want only the old movie once the fresh one is read in French", due, err)
	}

	if err := st.SaveLibraryItems(ctx, []LibraryItem{
		{ID: "m1", Kind: "movie", Name: "Film", Year: 2001, Catalog: movie},
		{ID: "s1", Kind: "series", Name: "Show", Catalog: show},
		{ID: "m2", Kind: "movie", Name: "Other"},
		{ID: "e1", Kind: "episode", Name: "Pilot"},
	}, at); err != nil {
		t.Fatal(err)
	}
	// A title being looked up is named as the Library holds it; a key no title has, by nothing.
	if name, year, err := st.LibraryCatalogTitle(ctx, movie); err != nil || name != "Film" || year != 2001 {
		t.Fatalf("movie title = %q (%d), %v; want Film (2001)", name, year, err)
	}
	if name, _, err := st.LibraryCatalogTitle(ctx, old); err != nil || name != "" {
		t.Fatalf("old title = %q, %v; want none", name, err)
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

	// Refresh everything: every present file is written again with its header kept, and every
	// fetched record, whatever its kind, is due again while it is still read.
	if _, err := st.SaveLibraryCatalog(ctx, old, `{"Kind":"movie"}`, 1, "fr-US", at); err != nil {
		t.Fatal(err)
	}
	if err := st.FailLibraryCatalog(ctx, show, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if due, err = st.DueLibraryCatalog(ctx, moviesV1); err != nil || len(due) != 1 || due[0] != show {
		t.Fatalf("due before refresh = %+v, %v; want only the never-fetched show", due, err)
	}
	if err := st.RefreshLibrary(ctx); err != nil {
		t.Fatal(err)
	}
	due, err = st.DueLibraryCatalog(ctx, moviesV1)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 3 || !slices.Contains(due, movie) || !slices.Contains(due, old) || !slices.Contains(due, show) {
		t.Fatalf("due after refresh = %+v, want every record", due)
	}
	if n, err := st.CountDueLibraryCatalog(ctx, moviesV1); err != nil || n != 3 {
		t.Fatalf("due as new after refresh = %d, %v; want every record", n, err)
	}
	if rec, fetched, err := st.LibraryCatalogRecord(ctx, movie); err != nil || !fetched || rec == "" {
		t.Fatalf("a refreshed record is still read: %q, %v, %v", rec, fetched, err)
	}
	if files, err = st.LibraryFiles(ctx, "/media"); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		present := f.MissingAt.IsZero()
		if present != (f.Identity == "") || f.Header != LibraryHeaderRead {
			t.Errorf("after refresh %s identity = %q header = %q, present %v", f.Path, f.Identity, f.Header, present)
		}
	}
	if _, err := st.SaveLibraryCatalog(ctx, movie, `{"Kind":"movie","IDs":[]}`, 1, "fr-US", at.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if due, err = st.DueLibraryCatalog(ctx, moviesV1); err != nil || slices.Contains(due, movie) {
		t.Fatalf("due = %+v, %v; a record fetched again is not due", due, err)
	}
	if n, err := st.CountDueLibraryCatalog(ctx, moviesV1); err != nil || n != 2 {
		t.Fatalf("due as new = %d, %v; want the two records still refreshing", n, err)
	}
}

// The change feed marks only fetched records it names; they are due as changed, not as new, until
// a save clears them. The feed's cursor survives a reopen of the store's rows (#3225).
func testLibraryCatalogChanges(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_790_000_000_000).UTC()
	fetched := LibraryCatalogKey{Provider: "tmdb", Kind: "movie", ExternalID: "90000111"}
	pending := LibraryCatalogKey{Provider: "tmdb", Kind: "movie", ExternalID: "90000112"}
	show := LibraryCatalogKey{Provider: "tmdb", Kind: "series", ExternalID: "90000111"}
	if err := st.WantLibraryCatalog(ctx, []LibraryCatalogKey{fetched, pending, show}); err != nil {
		t.Fatal(err)
	}
	for _, k := range []LibraryCatalogKey{fetched, show} {
		if _, err := st.SaveLibraryCatalog(ctx, k, `{}`, 0, "en-US", at); err != nil {
			t.Fatal(err)
		}
	}
	// A movie and a series share an id on TMDB; only the movie changed. The unfetched one and one
	// the cache does not hold are not marked.
	named := []LibraryCatalogKey{fetched, pending, {Provider: "tmdb", Kind: "movie", ExternalID: "90009999"}}
	if n, err := st.MarkLibraryCatalogChanged(ctx, named); err != nil || n != 1 {
		t.Fatalf("marked %d, %v; want the one fetched record named", n, err)
	}
	due := LibraryCatalogDue{Now: at, StaleBefore: at.Add(-time.Hour), Changed: 10, Language: "en-US"}
	if got, err := st.DueLibraryCatalog(ctx, due); err != nil || !slices.Equal(got, []LibraryCatalogKey{fetched}) {
		t.Fatalf("due as changed = %v, %v; want the changed movie", got, err)
	}
	if n, err := st.CountDueLibraryCatalog(ctx, due); err != nil || n != 1 {
		t.Fatalf("due as new = %d, %v; want only the unfetched one: a changed record is no new title", n, err)
	}
	if _, err := st.SaveLibraryCatalog(ctx, fetched, `{}`, 0, "en-US", at); err != nil {
		t.Fatal(err)
	}
	if got, err := st.DueLibraryCatalog(ctx, due); err != nil || len(got) != 0 {
		t.Fatalf("due as changed after the save = %v, %v; want none", got, err)
	}

	if _, ok, err := st.LibraryCatalogFeedRead(ctx, "tmdb"); err != nil || ok {
		t.Fatalf("feed read before any = %v, %v; want none", ok, err)
	}
	for _, read := range []time.Time{at, at.Add(24 * time.Hour)} {
		if err := st.SaveLibraryCatalogFeedRead(ctx, "tmdb", read); err != nil {
			t.Fatal(err)
		}
	}
	if got, ok, err := st.LibraryCatalogFeedRead(ctx, "tmdb"); err != nil || !ok || !got.Equal(at.Add(24*time.Hour)) {
		t.Fatalf("feed read = %v, %v, %v; want the latest", got, ok, err)
	}
}
