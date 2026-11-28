package scan

import (
	"context"
	"os"
	"slices"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// A kept home video is the title, not one file of it: it stays kept, under its file's or folder's
// name and out of Needs you, whatever happens to its files and sidecars.
func TestKeptHomeVideoBelongsToTheTitle(t *testing.T) {
	const part1, part2 = "Lake Weekend (2009)/part1.mkv", "Lake Weekend (2009)/part2.mkv"
	const film = "Beach day (2009)/Beach day (2009).mkv"
	nfo := `<movie><title>Charlie Feature</title><genre>Drama</genre><uniqueid type="tmdb" default="true">90000611</uniqueid></movie>`
	for what, tc := range map[string]struct {
		files  []string
		change func(*harness)
		name   string
		count  int
	}{
		"part 2 changes": {[]string{part1, part2}, func(h *harness) { h.file(part2, 20) }, "Lake Weekend", 2},
		"a part 3 arrives": {[]string{part1, part2},
			func(h *harness) { h.file("Lake Weekend (2009)/part3.mkv", 10) }, "Lake Weekend", 3},
		"a second copy arrives": {[]string{film},
			func(h *harness) { h.file("Beach day (2009)/Beach day (2009) - 1080p.mkv", 20) }, "Beach day", 2},
		"an NFO naming a catalog title arrives": {[]string{film}, func(h *harness) {
			p := h.file("Beach day (2009)/Beach day (2009).nfo", 0)
			if err := os.WriteFile(p, []byte(nfo), 0o644); err != nil {
				h.t.Fatal(err)
			}
			h.file(film, 20)
		}, "Beach day", 1},
		// Loose episodes at the root are a show with no folder: it is named as its files name it.
		"loose episodes at the root": {[]string{"Our Trips S01E01.mkv"},
			func(h *harness) { h.file("Our Trips S01E02.mkv", 10) }, "Our Trips", 2},
	} {
		t.Run(what, func(t *testing.T) {
			ctx := context.Background()
			h := newHarness(t)
			h.im.Answers = h.st
			for _, f := range tc.files {
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
			h.run()
			tc.change(h)
			h.run()
			h.run()
			page, err := lib.Items(ctx, librarystore.LibraryItemQuery{})
			if err != nil || page.Total != 1 {
				t.Fatalf("titles = %+v, %v; want one", page, err)
			}
			got := page.Items[0]
			if got.ID != needs[0].ID || !got.HomeVideo || got.Unidentified || got.Name != tc.name || got.Files != tc.count {
				t.Errorf("title = %s %q kept=%v waiting=%v with %d files; want %s %q kept with %d",
					got.ID, got.Name, got.HomeVideo, got.Unidentified, got.Files, needs[0].ID, tc.name, tc.count)
			}
			if got.Match.Basis != "" || got.Catalog.String() != "" {
				t.Errorf("matched to %q by %q; a home video is matched to nothing", got.Catalog.String(), got.Match.Basis)
			}
			genres, err := lib.Genres(ctx, "", false)
			if err != nil || !slices.Equal(genres, []librarystore.LibraryGenre{{Name: HomeVideoGenre, Titles: 1}}) {
				t.Errorf("genres = %+v, %v; want Home videos alone", genres, err)
			}
		})
	}
}
