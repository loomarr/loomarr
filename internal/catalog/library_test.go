package catalog_test

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/provision"
)

// tmdbIDs is a title's ids when TMDB's is its only one.
func tmdbIDs(v string) []store.LibraryID { return []store.LibraryID{{Namespace: "tmdb", Value: v}} }

type libraryTitlesFake []store.LibraryTitle

func (f libraryTitlesFake) SearchLibraryTitles(_ context.Context, _ string, limit int) ([]store.LibraryTitle, error) {
	if len(f) > limit {
		return f[:limit], nil
	}
	return f, nil
}

func (f libraryTitlesFake) LibraryTitles(context.Context, time.Time) ([]store.LibraryTitle, error) {
	return f, nil
}

func (f libraryTitlesFake) LibraryTitleByProviderID(_ context.Context, kind, namespace, value string) (store.LibraryTitle, bool, error) {
	for _, t := range f {
		if t.Kind == kind && store.IDIn(t.IDs, namespace) == value {
			return t, true, nil
		}
	}
	return store.LibraryTitle{}, false, nil
}

type itemFactsFake map[string]store.LibraryItemFacts

func (f itemFactsFake) LibraryItemFacts(_ context.Context, ids []string) (map[string]store.LibraryItemFacts, error) {
	out := map[string]store.LibraryItemFacts{}
	for _, id := range ids {
		if facts, ok := f[id]; ok {
			out[id] = facts
		}
	}
	return out, nil
}

var ownedTitles = libraryTitlesFake{
	{ID: "lib-film", Kind: "movie", Name: "Harbor Lights", IDs: tmdbIDs("90000001"), Rating: "PG",
		Genres: []string{"Drama"}, SizeBytes: 4_000},
	{ID: "lib-show", Kind: "series", Name: "Harbor Watch", IDs: append(tmdbIDs("90000003"),
		store.LibraryID{Namespace: "tvdb", Value: "90000002"}), Rating: "TV-Y"},
}

// Title search answers from the Library only (#2151 step 16): its titles come back under their
// library item ids with their facts, and a term the Library has no title for is no results, not
// an error.
func TestLibrarySearchAnswersFromTheLibrary(t *testing.T) {
	ctx := context.Background()
	facts := itemFactsFake{"lib-film": {Year: 1999, Overview: "A lighthouse.", Genres: []string{"Drama"}, RuntimeMs: 95 * 60_000}}

	got, err := catalog.LibrarySearch{Titles: ownedTitles, Facts: facts}.Search(ctx, "harbor", 10)
	if err != nil || len(got) != 2 {
		t.Fatalf("search = %+v, %v; want the Library's two titles", got, err)
	}
	if film := got[0]; film.LibraryItemID != "lib-film" || film.TMDBID != 90000001 || film.Year != 1999 ||
		film.RuntimeMinutes != 95 || film.Overview != "A lighthouse." || film.OfficialRating != "PG" || film.SizeBytes != 4_000 {
		t.Errorf("film = %+v; want it under its library item id with its facts", film)
	}
	if show := got[1]; show.LibraryItemID != "lib-show" || show.MediaType != library.Series || show.TVDBID != 90000002 {
		t.Errorf("show = %+v", show)
	}
	if got, err := (catalog.LibrarySearch{Titles: libraryTitlesFake{}, Facts: facts}).Search(ctx, "nothing", 10); err != nil || len(got) != 0 {
		t.Errorf("no match = %+v, %v; want no results and no error", got, err)
	}
}

// Presence answers an owned title from the Library, by its library item id, preferring TMDB and
// falling back to TVDB; a title the Library lacks is absent.
func TestLibraryOwnershipReadsTheLibrary(t *testing.T) {
	ctx := context.Background()
	owned := catalog.LibraryOwnership{Titles: ownedTitles}
	got, ok, err := owned.Present(ctx, provision.Movie, 90000001, 0)
	if err != nil || !ok || got.LibraryItemID != "lib-film" || got.OfficialRating != "PG" || got.SizeBytes != 4_000 ||
		len(got.Genres) != 1 {
		t.Errorf("film presence = %+v, %v, %v", got, ok, err)
	}
	if got, ok, err := owned.Present(ctx, provision.Series, 0, 90000002); err != nil || !ok || got.LibraryItemID != "lib-show" {
		t.Errorf("show by tvdb = %+v, %v, %v", got, ok, err)
	}
	if _, ok, err := owned.Present(ctx, provision.Movie, 90000009, 0); err != nil || ok {
		t.Errorf("a title the Library lacks: ok = %v, %v; want absent", ok, err)
	}
	if _, ok, err := (catalog.LibraryOwnership{}).Present(ctx, provision.Movie, 90000001, 0); err != nil || ok {
		t.Errorf("no Library: ok = %v, %v; want absent", ok, err)
	}
}
