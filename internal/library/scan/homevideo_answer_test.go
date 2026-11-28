package scan

import (
	"context"
	"os"
	"slices"
	"testing"

	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// keptTitle is a harness with one title kept as a home video, and the title's id.
func keptTitle(t *testing.T, files ...string) (*harness, *Library, string) {
	t.Helper()
	ctx := context.Background()
	h := newHarness(t)
	h.im.Answers, h.im.Catalog = h.st, h.st
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
	h.run()
	return h, lib, needs[0].ID
}

// The answer is the title's whichever file's origin its row has by now: undoing, ignoring or
// linking a kept title leaves no part of it a home video.
func TestAnswersAfterAKeptTitlesFilesChange(t *testing.T) {
	const part1, part2 = "Lake Weekend (2009)/part1.mkv", "Lake Weekend (2009)/part2.mkv"
	for what, tc := range map[string]struct {
		answer                  func(context.Context, *Library, string) error
		listed, waiting, hidden int
		basis                   mp.Basis
	}{
		"undo":   {func(ctx context.Context, l *Library, id string) error { return l.Unanswer(ctx, id) }, 1, 1, 0, ""},
		"ignore": {func(ctx context.Context, l *Library, id string) error { return l.Ignore(ctx, id) }, 0, 0, 1, ""},
		"link": {func(ctx context.Context, l *Library, id string) error {
			return l.Link(ctx, id, tmdbLink("90000300"))
		}, 1, 0, 0, mp.BasisLink},
	} {
		t.Run(what, func(t *testing.T) {
			ctx := context.Background()
			h, lib, id := keptTitle(t, part1, part2)
			h.file(part2, 20)
			h.run()
			if err := tc.answer(ctx, lib, id); err != nil {
				t.Fatal(err)
			}
			h.run()
			h.file(part1, 30)
			h.run()
			page, err := lib.Items(ctx, librarystore.LibraryItemQuery{})
			if err != nil || page.Total != tc.listed {
				t.Fatalf("titles = %+v, %v; want %d", page, err, tc.listed)
			}
			for _, got := range page.Items {
				if got.HomeVideo || got.Match.Basis != string(tc.basis) {
					t.Errorf("title %q: home video = %v, matched by %q, want %q", got.Name, got.HomeVideo, got.Match.Basis, tc.basis)
				}
			}
			if needs, err := lib.Needs(ctx); err != nil || len(needs) != tc.waiting {
				t.Errorf("needs = %d, %v; want %d", len(needs), err, tc.waiting)
			}
			if _, hidden, err := lib.Ignored(ctx, 10, 0); err != nil || hidden != tc.hidden {
				t.Errorf("ignored = %d, %v; want %d", hidden, err, tc.hidden)
			}
			if genres, err := lib.Genres(ctx, "", false); err != nil || len(genres) != 0 {
				t.Errorf("genres = %+v, %v; want none", genres, err)
			}
		})
	}
}

// An NFO that arrives after the keep asks the catalog for nothing.
func TestKeptHomeVideoIgnoresALaterNFO(t *testing.T) {
	ctx := context.Background()
	const film = "Beach day (2009)/Beach day (2009).mkv"
	h, lib, _ := keptTitle(t, film)
	p := h.file("Beach day (2009)/Beach day (2009).nfo", 0)
	nfo := `<movie><title>Charlie Feature</title><genre>Drama</genre><uniqueid type="tmdb" default="true">90000611</uniqueid></movie>`
	if err := os.WriteFile(p, []byte(nfo), 0o644); err != nil {
		t.Fatal(err)
	}
	h.file(film, 20)
	h.run()
	catalog := &fakeCatalog{records: map[string]mp.Record{"90000611": {
		Source: mp.Source{Provider: mp.IDTMDB, Basis: mp.BasisCatalog, Catalog: mp.NamespaceTMDB},
		Kind:   mp.KindMovie, IDs: []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: "90000611"}},
		Descriptive: &mp.Descriptive{Title: "Charlie Feature", Genres: []string{"Drama"}},
	}}}
	enricher := &Enricher{Store: h.st, Provider: catalog, Rewrite: func(root string, dirs ...string) {
		if err := h.im.RunFolders(ctx, root, dirs); err != nil {
			t.Fatal(err)
		}
	}}
	if err := enricher.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	h.run()
	if len(catalog.fetched) != 0 {
		t.Errorf("fetched %v for a home video", catalog.fetched)
	}
	genres, err := lib.Genres(ctx, "", false)
	if err != nil || !slices.Equal(genres, []librarystore.LibraryGenre{{Name: HomeVideoGenre, Titles: 1}}) {
		t.Errorf("genres = %+v, %v; want Home videos alone", genres, err)
	}
}
