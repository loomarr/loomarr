package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Needs you lists the present titles nothing grounds and nobody answered; candidates replace one
// another whole and note what was searched; an answer outlives the title (it is kept by scan
// origin), a link takes the title off the list and an ignore takes it off the Library too; and a
// title's present files are found so the scanner writes them again.
func testLibraryNeeds(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_790_000_000_000).UTC()
	origin := func(key string) LibraryOrigin { return LibraryOrigin{Authority: "library:/media", Key: key} }

	if err := st.SaveLibraryItems(ctx, []LibraryItem{
		{ID: "m1", Kind: "movie", Name: "Grounded Film", Origin: origin("/media/a/a.mkv"),
			Match: LibraryMatch{Basis: "manager", App: "radarr", Page: "http://radarr.invalid/movie/grounded-film"}},
		{ID: "m2", Kind: "movie", Name: "Mystery Film", Origin: origin("/media/b/b.mkv")},
		{ID: "s1", Kind: "series", Name: "Mystery Show", Origin: origin("/media/show")},
		{ID: "m3", Kind: "movie", Name: "Gone Film", Origin: origin("/media/c/c.mkv")},
	}, at); err != nil {
		t.Fatal(err)
	}
	file := func(path, item, series string) LibraryFile {
		return LibraryFile{Path: path, Root: "/media", Size: 1, ModifiedNs: 1, Identity: "digest", Kind: "movie",
			Header: LibraryHeaderRead, SeenAt: at, ItemID: item, SeriesID: series}
	}
	gone := file("/media/c/c.mkv", "m3", "")
	gone.MissingAt = at
	if err := st.SaveLibraryFiles(ctx, []LibraryFile{
		file("/media/a/a.mkv", "m1", ""),
		file("/media/b/b.mkv", "m2", ""),
		file("/media/show/e1.mkv", "e1", "s1"),
		file("/media/show/e2.mkv", "e2", "s1"),
		gone,
	}); err != nil {
		t.Fatal(err)
	}

	needs, err := st.LibraryNeeds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(needs) != 2 || needs[0].ID != "m2" || needs[1].ID != "s1" {
		t.Fatalf("needs = %+v, want the ungrounded present movie and series", needs)
	}
	if needs[1].Files != 2 || needs[1].Origin != origin("/media/show") || needs[1].Searched != "" {
		t.Fatalf("series need = %+v", needs[1])
	}
	// What said which title it is reads back with the title, for "Wrong title?".
	grounded, err := st.LibraryItem(ctx, "m1")
	if err != nil {
		t.Fatal(err)
	}
	if want := (LibraryMatch{Basis: "manager", App: "radarr", Page: "http://radarr.invalid/movie/grounded-film"}); grounded.Match != want {
		t.Fatalf("match = %+v, want %+v", grounded.Match, want)
	}

	first := []LibraryCandidate{{Kind: "movie", TMDBID: "90000201", Name: "Mystery", Year: 2001, NameMatch: true}}
	if err := st.SaveLibraryCandidates(ctx, "m2", "mystery film", first); err != nil {
		t.Fatal(err)
	}
	second := []LibraryCandidate{
		{Kind: "movie", TMDBID: "90000202", Name: "Mystery Film", Year: 2002, Poster: "https://img.invalid/p.jpg",
			NameMatch: true, YearMatch: true, LengthMatch: true},
		{Kind: "movie", TMDBID: "90000203", Name: "Mystery Film II", Year: 2004},
	}
	if err := st.SaveLibraryCandidates(ctx, "m2", "mystery film 2002", second); err != nil {
		t.Fatal(err)
	}
	cands, err := st.LibraryCandidates(ctx, []string{"m2", "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cands["m2"]) != 2 || cands["m2"][0] != second[0] || cands["m2"][1] != second[1] || len(cands["s1"]) != 0 {
		t.Fatalf("candidates = %+v, want the second search whole, in rank order", cands)
	}
	if needs, _ := st.LibraryNeeds(ctx); needs[0].Searched != "mystery film 2002" {
		t.Fatalf("searched = %q", needs[0].Searched)
	}

	kind, o, err := st.LibraryItemOrigin(ctx, "s1")
	if err != nil || kind != "series" || o != origin("/media/show") {
		t.Fatalf("origin = %q, %+v, %v", kind, o, err)
	}
	if _, _, err := st.LibraryItemOrigin(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown origin err = %v", err)
	}

	if err := st.SaveLibraryAnswer(ctx, LibraryAnswer{Origin: origin("/media/b/b.mkv"), Kind: "movie", AnsweredAt: at}); err == nil {
		t.Fatal("an answer that is neither a link nor ignored saved")
	}
	link := LibraryAnswer{Origin: origin("/media/b/b.mkv"), Kind: "movie", TMDBID: "90000202", AnsweredAt: at}
	if err := st.SaveLibraryAnswer(ctx, link); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := st.LibraryAnswerFor(ctx, link.Origin); err != nil || !ok || got != link {
		t.Fatalf("answer = %+v, %v, %v", got, ok, err)
	}
	ignore := LibraryAnswer{Origin: origin("/media/show"), Kind: "series", Ignored: true, AnsweredAt: at}
	if err := st.SaveLibraryAnswer(ctx, ignore); err != nil {
		t.Fatal(err)
	}
	if needs, _ := st.LibraryNeeds(ctx); len(needs) != 0 {
		t.Fatalf("needs after answers = %+v", needs)
	}
	page, err := st.ListLibraryItems(ctx, LibraryItemQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || page.Items[0].ID != "m1" || page.Items[1].ID != "m2" {
		t.Fatalf("library = %+v, want the ignored series left out and the linked movie kept", page)
	}

	// Answering again replaces; taking it back puts the title back on the list.
	if err := st.SaveLibraryAnswer(ctx, LibraryAnswer{Origin: origin("/media/show"), Kind: "series", TMDBID: "90000204",
		AnsweredAt: at.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := st.LibraryAnswerFor(ctx, origin("/media/show")); got.Ignored || got.TMDBID != "90000204" {
		t.Fatalf("replaced answer = %+v", got)
	}
	if err := st.DeleteLibraryAnswer(ctx, origin("/media/show")); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := st.LibraryAnswerFor(ctx, origin("/media/show")); err != nil || ok {
		t.Fatalf("deleted answer still there: %v, %v", ok, err)
	}
	if needs, _ := st.LibraryNeeds(ctx); len(needs) != 1 || needs[0].ID != "s1" {
		t.Fatalf("needs after take-back = %+v", needs)
	}

	paths, err := st.RewriteLibraryItemFiles(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0].Path != "/media/show/e1.mkv" || paths[1].Path != "/media/show/e2.mkv" {
		t.Fatalf("series rewrite = %+v, want both episodes", paths)
	}
	files, err := st.LibraryFiles(ctx, "/media")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		rewritten := f.Path == "/media/show/e1.mkv" || f.Path == "/media/show/e2.mkv"
		if rewritten != (f.Identity == "") {
			t.Errorf("%s identity = %q, rewritten %v", f.Path, f.Identity, rewritten)
		}
	}
}
