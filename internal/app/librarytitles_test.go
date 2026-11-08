package app

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/testkit"
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

// The availability scan reads the Library's titles first, so a title confirms with its library
// item id; the media server's items follow. With no media server configured, the Library's alone.
func TestLibraryTitleScannerListsTheLibraryFirst(t *testing.T) {
	none := libraryTitleScanner{titles: testLibraryTitles,
		server: library.NewDynamic(func() library.Connection { return library.Connection{} }, "device-1")}
	got, err := none.AllItems(context.Background())
	if err != nil || len(got) != 2 {
		t.Fatalf("no media server: %+v, %v; want the Library's two titles", got, err)
	}
	if got[1].LibraryItemID != "lib-show" || got[1].MediaType != library.Series || got[1].TVDBID != 90000002 || got[1].TMDBID != 90000003 {
		t.Errorf("show = %+v", got[1])
	}

	server := testkit.NewMediaServer(t)
	t.Cleanup(server.Close)
	server.SearchItems = []testkit.SearchStub{{LibraryItemID: "emby-9", Name: "Other", Type: "Movie", TMDBID: 90000009}}
	both := libraryTitleScanner{titles: testLibraryTitles, server: newTestLibraryClient(server)}
	got, err = both.RecentlyAdded(context.Background(), time.Now().Add(-time.Hour))
	if err != nil || len(got) != 3 || got[0].LibraryItemID != "lib-film" || got[2].LibraryItemID != "emby-9" {
		t.Errorf("with a media server: %+v, %v; want the Library's titles, then the server's", got, err)
	}

	down := libraryTitleScanner{titles: testLibraryTitles,
		server: library.New(library.Emby, "http://127.0.0.1:1", "token", "device-1")}
	if got, err := down.AllItems(context.Background()); err == nil || len(got) != 2 {
		t.Errorf("a media server that is down: %+v, %v; want the Library's titles with the error", got, err)
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
