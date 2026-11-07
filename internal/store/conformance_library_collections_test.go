package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

// testLibraryCollections: a collection keeps its titles in the household's order, an edit replaces
// them whole, and a deleted one is gone with its members.
func testLibraryCollections(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_790_000_000_000).UTC()

	if _, err := st.CreateLibraryCollection(ctx, LibraryCollection{Name: "  "}, at); err == nil {
		t.Fatal("a collection with no name was saved")
	}
	night, err := st.CreateLibraryCollection(ctx, LibraryCollection{
		Name: "Night Ferry Nights", Description: "Late shows", Members: []string{"c", "a", "c", "b"},
	}, at)
	if err != nil {
		t.Fatal(err)
	}
	empty, err := st.CreateLibraryCollection(ctx, LibraryCollection{Name: "autumn", ImportedFrom: "emby"}, at)
	if err != nil {
		t.Fatal(err)
	}
	if night == empty || night == "" {
		t.Fatalf("ids %q and %q", night, empty)
	}

	all, err := st.ListLibraryCollections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].ID != empty || all[1].ID != night {
		t.Fatalf("listed %+v, want autumn then Night Ferry Nights", all)
	}
	if all[0].ImportedFrom != "emby" || len(all[0].Members) != 0 || all[0].Members == nil {
		t.Fatalf("empty collection read %+v", all[0])
	}
	if got := all[1].Members; !reflect.DeepEqual(got, []string{"c", "a", "b"}) {
		t.Fatalf("members %v, want c, a, b (a repeat keeps its first place)", got)
	}

	later := at.Add(time.Hour)
	if err := st.UpdateLibraryCollection(ctx, LibraryCollection{
		ID: night, Name: "Nights", Members: []string{"b", "d"},
	}, later); err != nil {
		t.Fatal(err)
	}
	got, err := st.LibraryCollection(ctx, night)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Nights" || got.Description != "" || !reflect.DeepEqual(got.Members, []string{"b", "d"}) ||
		!got.UpdatedAt.Equal(later) {
		t.Fatalf("after the edit %+v", got)
	}
	if err := st.UpdateLibraryCollection(ctx, LibraryCollection{ID: "gone", Name: "x"}, later); !errors.Is(err, ErrNotFound) {
		t.Fatalf("editing a missing collection: %v, want ErrNotFound", err)
	}

	if err := st.DeleteLibraryCollection(ctx, night); err != nil {
		t.Fatal(err)
	}
	if _, err := st.LibraryCollection(ctx, night); !errors.Is(err, ErrNotFound) {
		t.Fatalf("read after delete: %v, want ErrNotFound", err)
	}
	if err := st.DeleteLibraryCollection(ctx, night); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: %v, want ErrNotFound", err)
	}
	// Its members went with it: a new collection under no member rows reads empty.
	again, err := st.CreateLibraryCollection(ctx, LibraryCollection{Name: "Again"}, later)
	if err != nil {
		t.Fatal(err)
	}
	if c, err := st.LibraryCollection(ctx, again); err != nil || len(c.Members) != 0 {
		t.Fatalf("new collection %+v, %v", c, err)
	}
}

// testLibraryItemIDs: the item list narrows to the ids asked for.
func testLibraryItemIDs(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_790_000_000_000).UTC()
	if err := st.SaveLibraryItems(ctx, []LibraryItem{
		{ID: "a", Kind: "movie", Name: "Alpha"},
		{ID: "b", Kind: "movie", Name: "Bravo"},
		{ID: "c", Kind: "movie", Name: "Charlie"},
	}, at); err != nil {
		t.Fatal(err)
	}
	var files []LibraryFile
	for _, id := range []string{"a", "b", "c"} {
		files = append(files, LibraryFile{Path: "/media/" + id + ".mkv", Root: "/media", Size: 1, ModifiedNs: 1,
			Kind: "movie", Header: LibraryHeaderRead, SeenAt: at, ItemID: id})
	}
	if err := st.SaveLibraryFiles(ctx, files); err != nil {
		t.Fatal(err)
	}
	page, err := st.ListLibraryItems(ctx, LibraryItemQuery{IDs: []string{"c", "a", "gone"}})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Items) != 2 || page.Items[0].ID != "a" || page.Items[1].ID != "c" {
		t.Fatalf("page %+v, want a and c", page)
	}
}
