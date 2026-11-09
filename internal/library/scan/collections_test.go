package scan

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store"
)

// A collection lists its titles in the household's order; a member that has left the library drops
// out of the titles and comes back in its place when it returns.
func TestCollectionsKeepTheHouseholdsOrder(t *testing.T) {
	h := newHarness(t)
	h.file("Alpha (1999) {tmdb-90000651}/Alpha.mkv", 10)
	bravo := h.file("Bravo (2003) {tmdb-90000652}/Bravo.mkv", 10)
	h.file("Charlie (2005) {tmdb-90000653}/Charlie.mkv", 10)
	h.run()

	lib := &Library{ItemStore: h.st, CollectionStore: h.st, Scanner: NewScanner(h.im, 0)}
	ctx := context.Background()
	ids := map[string]string{}
	all, err := lib.Items(ctx, store.LibraryItemQuery{})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range all.Items {
		ids[it.Name] = it.ID
	}
	id, err := lib.CreateCollection(ctx, "Favourites", "", []string{ids["Charlie"], ids["Alpha"]})
	if err != nil {
		t.Fatal(err)
	}
	if err := lib.UpdateCollection(ctx, id, "Favourites", "Ours", []string{ids["Charlie"], ids["Bravo"], ids["Alpha"]}); err != nil {
		t.Fatal(err)
	}
	titles := func() []string {
		t.Helper()
		c, err := lib.Collection(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if len(c.Members) != 3 || c.Description != "Ours" {
			t.Fatalf("collection = %+v, want all three members kept", c.LibraryCollection)
		}
		var names []string
		for _, it := range c.Titles {
			names = append(names, it.Name)
		}
		return names
	}
	if got := titles(); !slices.Equal(got, []string{"Charlie", "Bravo", "Alpha"}) {
		t.Fatalf("titles = %v, want Charlie, Bravo, Alpha", got)
	}

	// Bravo leaves the library (one file of three is held for a person to confirm).
	if err := os.Remove(bravo); err != nil {
		t.Fatal(err)
	}
	h.run()
	if err := lib.ConfirmRemovals(ctx, h.root); err != nil {
		t.Fatal(err)
	}
	h.run()
	if got := titles(); !slices.Equal(got, []string{"Charlie", "Alpha"}) {
		t.Fatalf("titles after Bravo left = %v, want Charlie, Alpha", got)
	}
	list, err := lib.Collections(ctx)
	if err != nil || len(list) != 1 || len(list[0].Titles) != 2 {
		t.Fatalf("collections = %+v, %v", list, err)
	}

	// It comes back in its place.
	h.file("Bravo (2003) {tmdb-90000652}/Bravo.mkv", 10)
	h.run()
	if got := titles(); !slices.Equal(got, []string{"Charlie", "Bravo", "Alpha"}) {
		t.Fatalf("titles after Bravo returned = %v, want Charlie, Bravo, Alpha", got)
	}
}

// A collection naming a title the library has no record of is refused, and an edit that does
// changes nothing.
func TestCollectionsRefuseAnUnknownTitle(t *testing.T) {
	h := newHarness(t)
	h.file("Alpha (1999) {tmdb-90000661}/Alpha.mkv", 10)
	h.run()
	lib := &Library{ItemStore: h.st, CollectionStore: h.st}
	ctx := context.Background()
	all, err := lib.Items(ctx, store.LibraryItemQuery{})
	if err != nil || len(all.Items) != 1 {
		t.Fatalf("items = %+v, %v", all, err)
	}
	alpha := all.Items[0].ID
	if _, err := lib.CreateCollection(ctx, "Strays", "", []string{alpha, "90000662"}); !errors.Is(err, store.ErrUnknownLibraryItem) {
		t.Fatalf("create = %v, want ErrUnknownLibraryItem", err)
	}
	if list, err := lib.Collections(ctx); err != nil || len(list) != 0 {
		t.Fatalf("collections after a refused create = %+v, %v", list, err)
	}
}
