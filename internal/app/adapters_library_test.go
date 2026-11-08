package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/testkit"
)

func boxSetGeneration(t *testing.T, connection library.Connection) library.ConnectionGeneration {
	t.Helper()
	generation, err := connection.Generation()
	if err != nil {
		t.Fatal(err)
	}
	return generation
}

func TestLibraryBoxSetsCacheUsesFullConnectionGeneration(t *testing.T) {
	now := time.Now()
	connection := library.Connection{Flavor: library.Emby, BaseURL: "https://emby.invalid/", Token: "token-a"}
	generation := boxSetGeneration(t, connection)
	index := map[provision.Key][]string{"movie:tmdb:90000001": {"collection-1"}}
	cache := &libraryBoxSets{
		lib: library.NewDynamic(func() library.Connection { return connection }, "device-1"),
		ttl: time.Hour, index: index, fetched: now, generation: generation,
	}

	equivalent := boxSetGeneration(t, library.Connection{
		Flavor: library.Emby, BaseURL: " https://emby.invalid ", Token: "token-a",
	})
	if got, ok := cache.cachedIndex(equivalent, now); !ok || got == nil {
		t.Fatal("equivalent normalized connection missed a fresh cache entry")
	}
	if got, err := cache.ensureIndex(context.Background()); err != nil || got == nil {
		t.Fatalf("ensureIndex on a complete matching generation = %v, %v; want cache hit", got, err)
	}

	for _, test := range []struct {
		name       string
		connection library.Connection
	}{
		{name: "flavor", connection: library.Connection{Flavor: library.Jellyfin, BaseURL: "https://emby.invalid", Token: "token-a"}},
		{name: "URL", connection: library.Connection{Flavor: library.Emby, BaseURL: "https://other.invalid", Token: "token-a"}},
		{name: "token", connection: library.Connection{Flavor: library.Emby, BaseURL: "https://emby.invalid", Token: "token-b"}},
	} {
		if _, ok := cache.cachedIndex(boxSetGeneration(t, test.connection), now); ok {
			t.Fatalf("changed %s reused a cached collection index", test.name)
		}
	}
}

type ownCollectionsFake struct {
	members []store.LibraryCollectionMember
	colls   []store.LibraryCollection
}

func (f ownCollectionsFake) LibraryCollectionMemberIDs(context.Context) ([]store.LibraryCollectionMember, error) {
	return f.members, nil
}

func (f ownCollectionsFake) ListLibraryCollections(context.Context) ([]store.LibraryCollection, error) {
	return f.colls, nil
}

// The household's own collections stamp a title under each key it can be known by, with no media
// server configured; a configured one that can't be read still fails the pass open.
func TestLibraryBoxSetsIndexesTheHouseholdsCollections(t *testing.T) {
	own := ownCollectionsFake{members: []store.LibraryCollectionMember{
		{CollectionID: "own-1", Kind: "series", TMDBID: "90000003", TVDBID: "90000002"},
		{CollectionID: "own-1", Kind: "movie", TMDBID: "90000001"},
		{CollectionID: "own-2", Kind: "movie", TMDBID: "90000001"},
		{CollectionID: "own-2", Kind: "movie"},
	}}
	none := &libraryBoxSets{
		lib: library.NewDynamic(func() library.Connection { return library.Connection{} }, "device-1"),
		own: own, ttl: time.Hour,
	}
	for key, want := range map[provision.Key][]string{
		"series:tvdb:90000002": {"own-1"},
		"series:tmdb:90000003": {"own-1"},
		"movie:tmdb:90000001":  {"own-1", "own-2"},
		"movie:tmdb:90000009":  nil,
	} {
		got, ok, err := none.BoxSets(context.Background(), key)
		if err != nil || !ok || !slices.Equal(got, want) {
			t.Errorf("%s = %v, %v, %v; want %v", key, got, ok, err, want)
		}
	}

	partial := &libraryBoxSets{
		lib: library.NewDynamic(func() library.Connection {
			return library.Connection{Flavor: library.Emby, BaseURL: "https://emby.invalid"}
		}, "device-1"),
		own: own, ttl: time.Hour,
	}
	if _, ok, err := partial.BoxSets(context.Background(), "movie:tmdb:90000001"); err == nil || ok {
		t.Errorf("a configured media server that can't be read: ok = %v, %v; want the pass unresolved", ok, err)
	}
}

// boxSetServer is a media server holding two collections: emby-coll-1, which the household's
// own-1 was imported from, and emby-coll-2, which was not imported.
func boxSetServer(t *testing.T) *library.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()
		switch {
		case q.Get("IncludeItemTypes") == "BoxSet":
			_, _ = w.Write([]byte(`{"Items":[{"Id":"emby-coll-1","Name":"Harbor Films","ChildCount":2},` +
				`{"Id":"emby-coll-2","Name":"Lighthouse Films","ChildCount":1}]}`))
		case q.Get("ParentId") == "emby-coll-1":
			_, _ = w.Write([]byte(`{"Items":[{"Id":"e1","Name":"A","Type":"Movie","ProviderIds":{"Tmdb":"90000011"}}]}`))
		case q.Get("ParentId") == "emby-coll-2":
			_, _ = w.Write([]byte(`{"Items":[{"Id":"e2","Name":"B","Type":"Movie","ProviderIds":{"Tmdb":"90000012"}}]}`))
		default:
			_, _ = w.Write([]byte(`{"Items":[]}`))
		}
	}))
	t.Cleanup(srv.Close)
	return library.New(library.Emby, srv.URL, "token", "device-1")
}

var importedOwn = ownCollectionsFake{
	members: []store.LibraryCollectionMember{{CollectionID: "own-1", Kind: "movie", TMDBID: "90000011"}},
	colls: []store.LibraryCollection{
		{ID: "own-1", Name: "Harbor Films", ImportedFrom: "emby", ImportedID: "emby-coll-1", Members: []string{"m1", "m2"}},
		{ID: "own-3", Name: "Rainy Days", Members: []string{"m3"}},
	},
}

// The collection scope reads the household's collections first (#2151 step 15e): a media-server
// collection the household's was imported from stands only under the household's id, and the
// media server adds just the ones it never imported.
func TestLibraryBoxSetsLeaveImportedCollectionsToTheHousehold(t *testing.T) {
	sets := &libraryBoxSets{lib: boxSetServer(t), own: importedOwn, ttl: time.Hour}
	for key, want := range map[provision.Key][]string{
		"movie:tmdb:90000011": {"own-1"},
		"movie:tmdb:90000012": {"emby-coll-2"},
	} {
		got, ok, err := sets.BoxSets(context.Background(), key)
		if err != nil || !ok || !slices.Equal(got, want) {
			t.Errorf("%s = %v, %v, %v; want %v", key, got, ok, err, want)
		}
	}
}

// The scope.collections picker lists the household's collections first, then the media server's
// that were never imported; with no media server, the household's alone, without an error.
func TestLibraryCollectionsListTheHouseholdsFirst(t *testing.T) {
	got, err := libraryCollections{own: importedOwn, lib: boxSetServer(t)}.Collections(context.Background())
	want := []api.LibraryCollection{
		{ID: "own-1", Name: "Harbor Films", ChildCount: 2},
		{ID: "own-3", Name: "Rainy Days", ChildCount: 1},
		{ID: "emby-coll-2", Name: "Lighthouse Films", ChildCount: 1},
	}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("picker = %+v, %v; want %+v", got, err, want)
	}

	none := library.NewDynamic(func() library.Connection { return library.Connection{} }, "device-1")
	got, err = libraryCollections{own: ownCollectionsFake{}, lib: none}.Collections(context.Background())
	if err != nil || len(got) != 0 {
		t.Errorf("no collections and no media server = %+v, %v; want none and no error", got, err)
	}
}

func TestLibraryBoxSetsValidatesConnectionBeforeCacheHit(t *testing.T) {
	valid := library.Connection{Flavor: library.Emby, BaseURL: "https://emby.invalid", Token: "token-a"}
	for _, incomplete := range []library.Connection{
		{},
		{Flavor: library.Emby, BaseURL: "https://emby.invalid"},
	} {
		cache := &libraryBoxSets{
			lib: library.NewDynamic(func() library.Connection { return incomplete }, "device-1"),
			ttl: time.Hour,
			index: map[provision.Key][]string{
				"movie:tmdb:90000001": {"collection-1"},
			},
			fetched:    time.Now(),
			generation: boxSetGeneration(t, valid),
		}
		if _, err := cache.ensureIndex(context.Background()); !errors.Is(err, library.ErrConnectionRequired) {
			t.Fatalf("ensureIndex error = %v, want ErrConnectionRequired", err)
		}
	}
}

func TestEpisodeResolverCarriesProductionYearToScheduler(t *testing.T) {
	mediaServer := testkit.NewMediaServer(t)
	t.Cleanup(mediaServer.Close)
	mediaServer.SetEpisodeItems(testkit.EpisodeStub{
		LibraryItemID: "episode-1", Name: "Puddle Town Plays Ball", RunTimeMs: 1_320_000,
		Season: 3, Episode: 17, ProductionYear: 1992, OfficialRating: "TV-PG",
	})
	lib := library.New(library.Emby, mediaServer.URL, mediaServer.AdminToken, "test-device")

	episodes, err := episodeResolver(libraryEpisodesFake{}, lib)(context.Background(), "the-mudpuddles")
	if err != nil {
		t.Fatal(err)
	}
	if len(episodes) != 1 || episodes[0].Year != 1992 {
		t.Fatalf("resolved episodes = %+v, want one episode carrying year 1992", episodes)
	}
}

type libraryEpisodesFake map[string][]store.LibraryEpisode

func (f libraryEpisodesFake) LibraryEpisodes(_ context.Context, seriesID string) ([]store.LibraryEpisode, bool, error) {
	eps, ok := f[seriesID]
	return eps, ok, nil
}

// A library series expands from the Library's tables under the media server's listing rules: an
// episode that cannot play is dropped, and a series with nothing to play stays empty rather than
// asking the media server.
func TestEpisodeResolverReadsLibrarySeriesFromTheTables(t *testing.T) {
	fake := libraryEpisodesFake{
		"lib-show": {
			{ItemID: "lib-e1", Name: "Pilot", Season: 1, Episode: 1, Year: 1992, PremiereDate: "1992-09-14",
				Rating: "TV-PG", Score: 7.5, Overview: "The first.", Tags: []string{"pilot"}, DurationMs: 1_320_000},
			{ItemID: "lib-e2", Name: "Not yet read", Season: 1, Episode: 2},
		},
		"lib-empty": nil,
	}
	resolve := episodeResolver(fake, nil)
	got, err := resolve(context.Background(), "lib-show")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("resolved %+v, want only the playable episode", got)
	}
	e := got[0]
	if e.LibraryItemID != "lib-e1" || e.Title != "Pilot" || e.DurationMs != 1_320_000 || e.Season != 1 ||
		e.Episode != 1 || e.Year != 1992 || !e.AirDate.Equal(time.Date(1992, 9, 14, 0, 0, 0, 0, time.UTC)) ||
		e.OfficialRating == "" || e.CommunityRating != 7.5 || e.Overview != "The first." || len(e.Tags) != 1 {
		t.Errorf("episode = %+v", e)
	}
	if got, err := resolve(context.Background(), "lib-empty"); err != nil || len(got) != 0 {
		t.Errorf("a library series with nothing to play = %+v, %v; want none", got, err)
	}
}
