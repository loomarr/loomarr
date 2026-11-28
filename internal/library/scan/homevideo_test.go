package scan

import (
	"context"
	"slices"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// A kept home video takes its name and year from its file or folder name, sits in the Home videos
// genre, and a later scan neither returns it to Needs you nor drops either.
func TestKeptHomeVideoStaysAcrossScans(t *testing.T) {
	for what, tc := range map[string]struct {
		files []string
		name  string
	}{
		"one file": {[]string{"Beach day (2009).mkv"}, "Beach day"},
		"a multi-part folder is one title": {[]string{"Lake Weekend (2009)/part1.mkv",
			"Lake Weekend (2009)/part2.mkv"}, "Lake Weekend"},
	} {
		t.Run(what, func(t *testing.T) { keptAcrossScans(t, tc.files, tc.name) })
	}
}

func keptAcrossScans(t *testing.T, files []string, name string) {
	ctx := context.Background()
	h := newHarness(t)
	h.im.Answers = h.st
	for _, f := range files {
		h.file(f, 10)
	}
	h.run()
	lib := &Library{Roots: &Roots{Store: h.st}, NeedStore: h.st, ItemStore: h.st, Inventory: h.inv, Keys: fakeCatalogKeys}
	needs, err := lib.Needs(ctx)
	if err != nil || len(needs) != 1 {
		t.Fatalf("needs = %+v, %v", needs, err)
	}
	if err := lib.Keep(ctx, needs[0].ID); err != nil {
		t.Fatal(err)
	}
	check := func(step string) {
		t.Helper()
		h.run()
		page, err := lib.Items(ctx, librarystore.LibraryItemQuery{Genre: HomeVideoGenre})
		if err != nil || page.Total != 1 {
			t.Fatalf("%s: home videos = %+v, %v; want one", step, page, err)
		}
		if got := page.Items[0]; got.Name != name || got.Year != 2009 || got.Files != len(files) {
			t.Fatalf("%s: kept as %q (%d) with %d files", step, got.Name, got.Year, got.Files)
		}
		if needs, err := lib.Needs(ctx); err != nil || len(needs) != 0 {
			t.Fatalf("%s: needs = %+v, %v", step, needs, err)
		}
		genres, err := lib.Genres(ctx, "", true)
		if err != nil || !slices.ContainsFunc(genres, func(g librarystore.LibraryGenre) bool { return g.Name == HomeVideoGenre }) {
			t.Fatalf("%s: a member's genres = %+v, %v", step, genres, err)
		}
	}
	check("after the keep")
	h.file(files[0], 20)
	check("after a file changed")
}
