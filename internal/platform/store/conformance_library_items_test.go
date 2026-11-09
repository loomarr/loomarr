package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

// Library → Items lists what has present files, counts a series' episodes toward it, searches by
// name with LIKE's wildcards taken literally, and pages in sort order.
func testLibraryItems(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_790_000_000_000).UTC()
	if err := st.SaveLibraryItems(ctx, []LibraryItem{
		{ID: "m1", Kind: "movie", Name: "The 100% Film", SortName: "100% film", Year: 1999},
		{ID: "m2", Kind: "movie", Name: "Another Film", Year: 2004},
		{ID: "s1", Kind: "series", Name: "A Show", SortName: "show", Year: 2001},
		{ID: "gone", Kind: "movie", Name: "Gone Film"},
		{ID: "e1", Kind: "episode", Name: "Pilot"},
		{ID: "e2", Kind: "episode", Name: "Second"},
	}, at); err != nil {
		t.Fatal(err)
	}
	// m1's poster is the image its ref names; a fanart ref is not a poster.
	poster, fanart := imageAt("m1-poster", at), imageAt("m2-fanart", at)
	for _, img := range []Image{poster, fanart} {
		if err := st.PutImage(ctx, img); err != nil {
			t.Fatal(err)
		}
	}
	for _, ref := range []ImageRef{
		{ImageHash: poster.Hash, OwnerKind: LibraryItemImageOwner, OwnerID: "m1", Role: "poster"},
		{ImageHash: fanart.Hash, OwnerKind: LibraryItemImageOwner, OwnerID: "m2", Role: "backdrop"},
	} {
		if err := st.PutImageRef(ctx, ref); err != nil {
			t.Fatal(err)
		}
	}
	file := func(path, item, series, header string) LibraryFile {
		return LibraryFile{Path: path, Root: "/media", Size: 1, ModifiedNs: 1, Kind: "movie", Header: header,
			SeenAt: at, ItemID: item, SeriesID: series}
	}
	missing := file("/media/gone.mkv", "gone", "", LibraryHeaderRead)
	missing.MissingAt = at
	problem := file("/media/bad.iso", "m2", "", LibraryHeaderFailed)
	problem.Problem = "disc_image"
	if err := st.SaveLibraryFiles(ctx, []LibraryFile{
		file("/media/100.mkv", "m1", "", LibraryHeaderRead),
		file("/media/another.mkv", "m2", "", LibraryHeaderPending),
		file("/media/show/e1.mkv", "e1", "s1", LibraryHeaderRead),
		file("/media/show/e2.mkv", "e2", "s1", LibraryHeaderPending),
		missing, problem,
	}); err != nil {
		t.Fatal(err)
	}

	page, err := st.ListLibraryItems(ctx, LibraryItemQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || len(page.Items) != 3 {
		t.Fatalf("page = %+v, want three present items", page)
	}
	want := []LibraryItemSummary{
		{LibraryItem: LibraryItem{ID: "m1", Kind: "movie", Name: "The 100% Film", SortName: "100% film", Year: 1999}, Poster: poster.Hash, Files: 1},
		// m2's fanart is its backdrop, never its poster.
		{LibraryItem: LibraryItem{ID: "m2", Kind: "movie", Name: "Another Film", SortName: "another film", Year: 2004}, Backdrop: fanart.Hash, Files: 1, Pending: 1},
		{LibraryItem: LibraryItem{ID: "s1", Kind: "series", Name: "A Show", SortName: "show", Year: 2001}, Files: 2, Pending: 1},
	}
	for i, it := range page.Items {
		if !reflect.DeepEqual(it, want[i]) {
			t.Errorf("item %d = %+v, want %+v", i, it, want[i])
		}
	}

	series, err := st.ListLibraryItems(ctx, LibraryItemQuery{Kind: "series"})
	if err != nil || series.Total != 1 || series.Items[0].ID != "s1" {
		t.Fatalf("series = %+v, %v", series, err)
	}
	literal, err := st.ListLibraryItems(ctx, LibraryItemQuery{Search: "100%"})
	if err != nil || literal.Total != 1 || literal.Items[0].ID != "m1" {
		t.Fatalf("search 100%% = %+v, %v", literal, err)
	}
	if wild, err := st.ListLibraryItems(ctx, LibraryItemQuery{Search: "_"}); err != nil || wild.Total != 0 {
		t.Fatalf("search _ = %+v, %v; want the underscore taken literally", wild, err)
	}
	second, err := st.ListLibraryItems(ctx, LibraryItemQuery{Limit: 1, Offset: 1})
	if err != nil || second.Total != 3 || len(second.Items) != 1 || second.Items[0].ID != "m2" {
		t.Fatalf("second page = %+v, %v", second, err)
	}

	one, err := st.LibraryItem(ctx, "s1")
	if err != nil || one.Files != 2 {
		t.Fatalf("item s1 = %+v, %v", one, err)
	}
	if _, err := st.LibraryItem(ctx, "gone"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an item with no present file = %v, want ErrNotFound", err)
	}
	files, err := st.LibraryItemFiles(ctx, "s1")
	if err != nil || len(files) != 2 || files[0].Path != "/media/show/e1.mkv" || files[1].SeriesID != "s1" {
		t.Fatalf("series files = %+v, %v", files, err)
	}

	// A file Loomarr moved away goes missing at once, unheld; a missing one keeps when it went.
	held := file("/media/show/e2.mkv", "e2", "s1", LibraryHeaderPending)
	held.Held = true
	if err := st.SaveLibraryFiles(ctx, []LibraryFile{held}); err != nil {
		t.Fatal(err)
	}
	later := at.Add(time.Hour)
	for _, path := range []string{"/media/show/e2.mkv", "/media/gone.mkv"} {
		if err := st.RemoveLibraryFile(ctx, path, later); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := st.LibraryFiles(ctx, "/media")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		switch r.Path {
		case "/media/show/e2.mkv":
			if !r.MissingAt.Equal(later) || r.Held {
				t.Errorf("removed file = %+v, want missing at %v and unheld", r, later)
			}
		case "/media/gone.mkv":
			if !r.MissingAt.Equal(at) {
				t.Errorf("an already missing file = %+v, want it to keep %v", r, at)
			}
		}
	}
	if one, err := st.LibraryItem(ctx, "s1"); err != nil || one.Files != 1 {
		t.Errorf("series after a removal = %+v, %v; want one file", one, err)
	}
}

// A file whose header couldn't be read counts as unplayable toward its item and its series, and
// the problems list keeps just the titles that need a look: one with an unplayable file, or a movie
// with a copy that does not play, sized. A skipped file (a disc image) is neither, and nor is a
// movie whose two editions each play.
func testLibraryItemProblems(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_790_000_000_000).UTC()
	if err := st.SaveLibraryItems(ctx, []LibraryItem{
		{ID: "broken", Kind: "movie", Name: "Broken Film"},
		{ID: "twice", Kind: "movie", Name: "Twice Film", Sources: []LibrarySource{
			{Path: "/media/twice/a.mkv", SizeBytes: 5, Plays: true}, {Path: "/media/twice/b.mkv", SizeBytes: 30}}},
		{ID: "cuts", Kind: "movie", Name: "Cuts Film", Sources: []LibrarySource{
			{Path: "/media/cuts/a.mkv", SizeBytes: 5, Plays: true},
			{Path: "/media/cuts/b.mkv", SizeBytes: 7, Edition: "Extended", Plays: true}}},
		{ID: "fine", Kind: "movie", Name: "Fine Film"},
		{ID: "show", Kind: "series", Name: "A Show"},
		{ID: "shows", Kind: "series", Name: "Fine Show"},
		{ID: "e1", Kind: "episode", Name: "Pilot"},
		{ID: "e2", Kind: "episode", Name: "Second"},
		{ID: "f1", Kind: "episode", Name: "Fine Pilot"},
		{ID: "f2", Kind: "episode", Name: "Fine Second"},
	}, at); err != nil {
		t.Fatal(err)
	}
	file := func(path, item, series, header string) LibraryFile {
		return LibraryFile{Path: path, Root: "/media", Size: 1, ModifiedNs: 1, Kind: "movie", Header: header,
			SeenAt: at, ItemID: item, SeriesID: series}
	}
	disc := file("/media/fine/disc.iso", "fine", "", LibraryHeaderFailed)
	disc.Problem = "disc_image"
	if err := st.SaveLibraryFiles(ctx, []LibraryFile{
		file("/media/broken.mkv", "broken", "", LibraryHeaderFailed),
		file("/media/twice/a.mkv", "twice", "", LibraryHeaderRead),
		file("/media/twice/b.mkv", "twice", "", LibraryHeaderRead),
		file("/media/cuts/a.mkv", "cuts", "", LibraryHeaderRead),
		file("/media/cuts/b.mkv", "cuts", "", LibraryHeaderRead),
		file("/media/fine/fine.mkv", "fine", "", LibraryHeaderRead),
		disc,
		file("/media/show/e1.mkv", "e1", "show", LibraryHeaderRead),
		file("/media/show/e2.mkv", "e2", "show", LibraryHeaderFailed),
		file("/media/shows/e1.mkv", "f1", "shows", LibraryHeaderRead),
		file("/media/shows/e2.mkv", "f2", "shows", LibraryHeaderRead),
	}); err != nil {
		t.Fatal(err)
	}

	if one, err := st.LibraryItem(ctx, "show"); err != nil || one.Unplayable != 1 || one.Files != 2 || one.Pending != 0 {
		t.Fatalf("show = %+v, %v; want one of two episodes unplayable", one, err)
	}
	page, err := st.ListLibraryItems(ctx, LibraryItemQuery{Problems: true})
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, it := range page.Items {
		got = append(got, it.ID)
	}
	// Sorted by name: A Show, Broken Film, Twice Film.
	if page.Total != 3 || len(got) != 3 || got[0] != "show" || got[1] != "broken" || got[2] != "twice" {
		t.Fatalf("problems = %v (total %d), want show, broken and twice", got, page.Total)
	}
	if page.Items[1].Unplayable != 1 || page.Items[1].ExtraBytes != 0 || page.Items[2].Unplayable != 0 ||
		page.Items[2].Files != 2 || page.Items[2].ExtraBytes != 30 {
		t.Errorf("problem counts = %+v, want Twice Film's 30-byte copy extra", page.Items)
	}
	if all, err := st.ListLibraryItems(ctx, LibraryItemQuery{}); err != nil || all.Total != 6 {
		t.Errorf("every title = %d, %v; want six", all.Total, err)
	}
	if paged, err := st.ListLibraryItems(ctx, LibraryItemQuery{Problems: true, Limit: 1}); err != nil ||
		paged.Total != 3 || len(paged.Items) != 1 {
		t.Errorf("a page of problems = %d of %d, %v; want one of three", len(paged.Items), paged.Total, err)
	}
}
