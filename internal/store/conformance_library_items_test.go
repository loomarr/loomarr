package store

import (
	"context"
	"errors"
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
	}, at); err != nil {
		t.Fatal(err)
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
		{LibraryItem: LibraryItem{ID: "m1", Kind: "movie", Name: "The 100% Film", SortName: "100% film", Year: 1999}, Files: 1},
		{LibraryItem: LibraryItem{ID: "m2", Kind: "movie", Name: "Another Film", SortName: "another film", Year: 2004}, Files: 1, Pending: 1},
		{LibraryItem: LibraryItem{ID: "s1", Kind: "series", Name: "A Show", SortName: "show", Year: 2001}, Files: 2, Pending: 1},
	}
	for i, it := range page.Items {
		if it != want[i] {
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
}
