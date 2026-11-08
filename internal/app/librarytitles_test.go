package app

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/provision"
)

type libraryTitlesFake []store.LibraryTitle

func (f libraryTitlesFake) LibraryTitles(_ context.Context, since time.Time) ([]store.LibraryTitle, error) {
	return f, nil
}

func (f libraryTitlesFake) LibraryTitleByProviderID(_ context.Context, kind, namespace, value string) (store.LibraryTitle, bool, error) {
	for _, t := range f {
		id := t.TMDBID
		if namespace == "tvdb" {
			id = t.TVDBID
		}
		if t.Kind == kind && id == value {
			return t, true, nil
		}
	}
	return store.LibraryTitle{}, false, nil
}

var testLibraryTitles = libraryTitlesFake{
	{ID: "lib-film", Kind: "movie", Name: "Film", TMDBID: "90000001", Rating: "PG", Genres: []string{"Drama"}, SizeBytes: 4_000},
	{ID: "lib-show", Kind: "series", Name: "Show", TMDBID: "90000003", TVDBID: "90000002", Rating: "TV-Y"},
}

// The availability scan reads the Library's titles, so a title confirms with its library item id
// (#2151 step 16).
func TestLibraryTitleScannerListsTheLibrary(t *testing.T) {
	scanner := libraryTitleScanner{titles: testLibraryTitles}
	got, err := scanner.AllItems(context.Background())
	if err != nil || len(got) != 2 {
		t.Fatalf("all items = %+v, %v; want the Library's two titles", got, err)
	}
	if got[1].LibraryItemID != "lib-show" || got[1].MediaType != library.Series || got[1].TVDBID != 90000002 || got[1].TMDBID != 90000003 {
		t.Errorf("show = %+v", got[1])
	}
	if got, err := scanner.RecentlyAdded(context.Background(), time.Now().Add(-time.Hour)); err != nil || len(got) != 2 ||
		got[0].LibraryItemID != "lib-film" {
		t.Errorf("recently added = %+v, %v; want the Library's titles", got, err)
	}
}

// The provisioning backstop's last look confirms a title the Library holds by its library item id
// (#2151 step 16); a title the Library lacks is absent.
func TestLibraryLookupReadsTheLibrary(t *testing.T) {
	ctx := context.Background()
	lookup := libraryLookup{titles: testLibraryTitles}
	if id, ok, err := lookup.Lookup(ctx, library.TMDB, "90000001", library.Movie); err != nil || !ok || id != "lib-film" {
		t.Errorf("film = %q, %v, %v; want its library item id", id, ok, err)
	}
	if id, ok, err := lookup.Lookup(ctx, library.TVDB, "90000002", library.Series); err != nil || !ok || id != "lib-show" {
		t.Errorf("show by tvdb = %q, %v, %v", id, ok, err)
	}
	if _, ok, err := lookup.Lookup(ctx, library.TMDB, "90000009", library.Movie); err != nil || ok {
		t.Errorf("a title the Library lacks: ok = %v, %v; want absent", ok, err)
	}
}

// Presence and ratings answer an owned title from the Library, by its library item id.
func TestPresenceAndRatingsReadTheLibrary(t *testing.T) {
	presence := libraryPresence{titles: testLibraryTitles}
	got, ok, err := presence.Present(context.Background(), provision.Movie, 90000001, 0)
	if err != nil || !ok || got.LibraryItemID != "lib-film" || got.OfficialRating != "PG" || got.SizeBytes != 4_000 ||
		len(got.Genres) != 1 {
		t.Errorf("film presence = %+v, %v, %v", got, ok, err)
	}
	if got, ok, err := presence.Present(context.Background(), provision.Series, 0, 90000002); err != nil || !ok || got.LibraryItemID != "lib-show" {
		t.Errorf("show by tvdb = %+v, %v, %v", got, ok, err)
	}
	if _, ok, err := presence.Present(context.Background(), provision.Movie, 90000009, 0); err != nil || ok {
		t.Errorf("a title the Library lacks: ok = %v, %v; want absent", ok, err)
	}

	ratings := libraryRatings{titles: testLibraryTitles}
	if r, ok, err := ratings.Rating(context.Background(), "series:tvdb:90000002"); err != nil || !ok || r != "TV-Y" {
		t.Errorf("show rating = %q, %v, %v", r, ok, err)
	}
	if _, ok, err := ratings.Rating(context.Background(), "movie:tmdb:90000009"); err != nil || ok {
		t.Errorf("an unowned title's rating: ok = %v, %v; want unresolved", ok, err)
	}
}
