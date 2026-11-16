package store

import (
	"context"
	"reflect"
	"testing"
	"time"
)

// A held removal names its titles: a show once however many of its episodes are held, a movie by
// its own name, in name order, only the folder's own held files, and how many titles in all.
func testLibraryHeldTitles(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_800_000_000_000).UTC()
	if err := st.SaveLibraryItems(ctx, []LibraryItem{
		{ID: "s1", Kind: "series", Name: "The Ferrow Lane", SortName: "ferrow lane"},
		{ID: "e1", Kind: "episode", Name: "One"}, {ID: "e2", Kind: "episode", Name: "Two"},
		{ID: "m1", Kind: "movie", Name: "Calder Point"},
		{ID: "m2", Kind: "movie", Name: "Amber Hollow"},
		{ID: "m3", Kind: "movie", Name: "Brightwater"},
		{ID: "m4", Kind: "movie", Name: "Quiet One"},
		{ID: "m5", Kind: "movie", Name: "Dune Shore"},
		{ID: "m6", Kind: "movie", Name: "Long Gone"},
	}, at); err != nil {
		t.Fatal(err)
	}
	file := func(root, path, item, series string, held bool) LibraryFile {
		return LibraryFile{Path: root + "/" + path, Root: root, Size: 1, ModifiedNs: 1, Kind: "movie",
			Header: LibraryHeaderRead, Held: held, SeenAt: at, ItemID: item, SeriesID: series}
	}
	gone := file("/media/a", "gone.mkv", "m6", "", true)
	gone.MissingAt = at
	if err := st.SaveLibraryFiles(ctx, []LibraryFile{
		file("/media/a", "lane/1.mkv", "e1", "s1", true),
		file("/media/a", "lane/2.mkv", "e2", "s1", true),
		file("/media/a", "calder.mkv", "m1", "", true),
		file("/media/a", "amber.mkv", "m2", "", true),
		file("/media/a", "bright.mkv", "m3", "", true),
		// Not held, another folder's, gone already, or no title's: none is named.
		file("/media/a", "quiet.mkv", "m4", "", false),
		file("/media/b", "dune.mkv", "m5", "", true),
		gone,
		file("/media/a", "stray.nfo", "", "", true),
	}); err != nil {
		t.Fatal(err)
	}
	names, total, err := st.LibraryHeldTitles(ctx, "/media/a", 3)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Amber Hollow", "Brightwater", "Calder Point"}; !reflect.DeepEqual(names, want) || total != 4 {
		t.Fatalf("held titles = %q of %d, want %q of 4", names, total, want)
	}
	if names, total, err := st.LibraryHeldTitles(ctx, "/media/c", 3); err != nil || len(names) != 0 || total != 0 {
		t.Fatalf("a folder holding nothing = %q of %d, %v", names, total, err)
	}
}
