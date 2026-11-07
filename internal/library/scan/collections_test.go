package scan

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/store"
)

// A collection lists its titles in the household's order; a member that has left the library drops
// out of the titles and comes back in its place when it returns.
func TestCollectionsKeepTheHouseholdsOrder(t *testing.T) {
	h := newHarness(t)
	h.file("Alpha (1999) {tmdb-90000651}/Alpha.mkv", 10)
	h.file("Bravo (2003) {tmdb-90000652}/Bravo.mkv", 10)
	h.file("Charlie (2005) {tmdb-90000653}/Charlie.mkv", 10)
	h.run()

	lib := &Library{ItemStore: h.st, CollectionStore: h.st}
	ctx := context.Background()
	ids := map[string]string{}
	all, err := lib.Items(ctx, store.LibraryItemQuery{})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range all.Items {
		ids[it.Name] = it.ID
	}
	id, err := lib.CreateCollection(ctx, "Favourites", "", []string{ids["Charlie"], "gone", ids["Alpha"]})
	if err != nil {
		t.Fatal(err)
	}
	if err := lib.UpdateCollection(ctx, id, "Favourites", "Ours", []string{ids["Charlie"], "gone", ids["Alpha"], ids["Bravo"]}); err != nil {
		t.Fatal(err)
	}

	c, err := lib.Collection(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, it := range c.Titles {
		names = append(names, it.Name)
	}
	if len(names) != 3 || names[0] != "Charlie" || names[1] != "Alpha" || names[2] != "Bravo" {
		t.Fatalf("titles = %v, want Charlie, Alpha, Bravo", names)
	}
	if len(c.Members) != 4 || c.Description != "Ours" {
		t.Fatalf("collection = %+v, want the missing member kept", c.LibraryCollection)
	}
	list, err := lib.Collections(ctx)
	if err != nil || len(list) != 1 || len(list[0].Titles) != 3 {
		t.Fatalf("collections = %+v, %v", list, err)
	}
}
