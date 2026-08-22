package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/testkit"
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
	index := map[provision.Key][]string{"movie:tmdb:1": {"collection-1"}}
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
				"movie:tmdb:1": {"collection-1"},
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
		LibraryItemID: "episode-1", Name: "Homer at the Bat", RunTimeMs: 1_320_000,
		Season: 3, Episode: 17, ProductionYear: 1992, OfficialRating: "TV-PG",
	})
	lib := library.New(library.Emby, mediaServer.URL, mediaServer.AdminToken, "test-device")

	episodes, err := episodeResolver(lib)(context.Background(), "the-simpsons")
	if err != nil {
		t.Fatal(err)
	}
	if len(episodes) != 1 || episodes[0].Year != 1992 {
		t.Fatalf("resolved episodes = %+v, want one episode carrying year 1992", episodes)
	}
}
