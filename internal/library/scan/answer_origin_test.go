package scan

import (
	"context"
	"os"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// answerHarness is a harness that reads answers and the catalog rows they name, and its Library.
func answerHarness(t *testing.T) (*harness, *Library) {
	t.Helper()
	h := newHarness(t)
	h.im.Answers, h.im.Catalog = h.st, h.st
	return h, &Library{Roots: &Roots{Store: h.st}, NeedStore: h.st, ItemStore: h.st, Inventory: h.inv, Keys: fakeCatalogKeys}
}

// onlyNeed is the one title waiting in Needs you.
func onlyNeed(t *testing.T, lib *Library) string {
	t.Helper()
	needs, err := lib.Needs(context.Background())
	if err != nil || len(needs) != 1 {
		t.Fatalf("needs = %+v, %v; want one", needs, err)
	}
	return needs[0].ID
}

// An answer is its origin's: a file linked to a film the maintainer linked before joins it, and
// taking the file's link back splits it out and leaves the film's own link.
func TestTakingBackALinkLeavesTheFilmItJoinedLinked(t *testing.T) {
	ctx := context.Background()
	h, lib := answerHarness(t)
	h.file("Owned Film (2010)/Owned Film (2010).mkv", 10)
	h.run()
	owned := onlyNeed(t, lib)
	if err := lib.Link(ctx, owned, tmdbLink("90000300")); err != nil {
		t.Fatal(err)
	}
	h.run()
	h.file("Mystery Film (2002)/Mystery Film (2002).mkv", 10)
	h.run()
	unknown := onlyNeed(t, lib)
	if err := lib.Link(ctx, unknown, tmdbLink("90000300")); err != nil {
		t.Fatal(err)
	}
	h.run()
	if page, err := lib.Items(ctx, librarystore.LibraryItemQuery{}); err != nil || page.Total != 1 || page.Items[0].Files != 2 {
		t.Fatalf("after the link = %+v, %v; want one title with two copies", page, err)
	}

	if err := lib.Unanswer(ctx, unknown); err != nil {
		t.Fatal(err)
	}
	h.run()
	if got := onlyNeed(t, lib); got == owned {
		t.Fatalf("the film linked first waits in Needs you")
	}
	page, err := lib.Items(ctx, librarystore.LibraryItemQuery{})
	if err != nil || page.Total != 2 {
		t.Fatalf("after taking it back = %+v, %v; want the copy split out", page, err)
	}
	for _, it := range page.Items {
		if it.ID == owned && it.Match.Basis != "link" {
			t.Errorf("the film linked first is matched by %q, want its link", it.Match.Basis)
		}
	}
}

// Two films one sidecar id joins are one title of two origins, and an answer is the origin's the
// title reads: once the films are told apart, one of them carries the ignore or the link and the
// other is as it was.
func TestAnAnswerOnAJoinedTitleMovesOneFilm(t *testing.T) {
	nfo := `<movie><title>Shared Feature</title><uniqueid type="tmdb" default="true">90000400</uniqueid></movie>`
	for what, tc := range map[string]struct {
		answer         func(context.Context, *Library, string) error
		listed, hidden int
	}{
		"ignore": {func(ctx context.Context, l *Library, id string) error { return l.Ignore(ctx, id) }, 1, 1},
		"link": {func(ctx context.Context, l *Library, id string) error {
			return l.Link(ctx, id, tmdbLink("90000401"))
		}, 2, 0},
	} {
		t.Run(what, func(t *testing.T) {
			ctx := context.Background()
			h, lib := answerHarness(t)
			for _, dir := range []string{"Film A (2001)", "Film B (2003)"} {
				h.file(dir+"/"+dir+".mkv", 10)
				if err := os.WriteFile(h.file(dir+"/"+dir+".nfo", 0), []byte(nfo), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			h.run()
			page, err := lib.Items(ctx, librarystore.LibraryItemQuery{})
			if err != nil || page.Total != 1 || page.Items[0].Files != 2 {
				t.Fatalf("before = %+v, %v; want one title of both films", page, err)
			}
			if err := tc.answer(ctx, lib, page.Items[0].ID); err != nil {
				t.Fatal(err)
			}
			h.run()
			// The sidecar is mended: film B is its own film again, and only one origin was answered.
			own := `<movie><title>Film B</title><uniqueid type="tmdb" default="true">90000402</uniqueid></movie>`
			if err := os.WriteFile(h.file("Film B (2003)/Film B (2003).nfo", 0), []byte(own), 0o644); err != nil {
				t.Fatal(err)
			}
			h.file("Film B (2003)/Film B (2003).mkv", 20)
			h.run()
			h.run()
			if page, err = lib.Items(ctx, librarystore.LibraryItemQuery{}); err != nil || page.Total != tc.listed {
				t.Errorf("titles = %+v, %v; want %d", page, err, tc.listed)
			}
			for _, it := range page.Items {
				if it.Files != 1 {
					t.Errorf("title %q holds %d files; want each film on its own", it.Name, it.Files)
				}
			}
			if _, hidden, err := lib.Ignored(ctx, 10, 0); err != nil || hidden != tc.hidden {
				t.Errorf("ignored = %d, %v; want %d", hidden, err, tc.hidden)
			}
		})
	}
}
