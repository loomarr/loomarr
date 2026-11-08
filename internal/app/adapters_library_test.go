package app

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/provision"
)

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

// The household's collections stamp a title under each key it can be known by (#2151 step 16).
func TestLibraryBoxSetsIndexesTheHouseholdsCollections(t *testing.T) {
	own := ownCollectionsFake{members: []store.LibraryCollectionMember{
		{CollectionID: "own-1", Kind: "series", TMDBID: "90000003", TVDBID: "90000002"},
		{CollectionID: "own-1", Kind: "movie", TMDBID: "90000001"},
		{CollectionID: "own-2", Kind: "movie", TMDBID: "90000001"},
		{CollectionID: "own-2", Kind: "movie"},
	}}
	sets := &libraryBoxSets{own: own, ttl: time.Hour}
	for key, want := range map[provision.Key][]string{
		"series:tvdb:90000002": {"own-1"},
		"series:tmdb:90000003": {"own-1"},
		"movie:tmdb:90000001":  {"own-1", "own-2"},
		"movie:tmdb:90000009":  nil,
	} {
		got, ok, err := sets.BoxSets(context.Background(), key)
		if err != nil || !ok || !slices.Equal(got, want) {
			t.Errorf("%s = %v, %v, %v; want %v", key, got, ok, err, want)
		}
	}
}

// The scope.collections picker lists the household's collections, the imported ones included,
// under their own ids.
func TestLibraryCollectionsListTheHouseholds(t *testing.T) {
	own := ownCollectionsFake{colls: []store.LibraryCollection{
		{ID: "own-1", Name: "Harbor Films", ImportedFrom: "emby", ImportedID: "emby-coll-1", Members: []string{"m1", "m2"}},
		{ID: "own-3", Name: "Rainy Days", Members: []string{"m3"}},
	}}
	got, err := libraryCollections{own: own}.Collections(context.Background())
	want := []api.LibraryCollection{
		{ID: "own-1", Name: "Harbor Films", ChildCount: 2},
		{ID: "own-3", Name: "Rainy Days", ChildCount: 1},
	}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("picker = %+v, %v; want %+v", got, err, want)
	}
	if got, err := (libraryCollections{own: ownCollectionsFake{}}).Collections(context.Background()); err != nil || len(got) != 0 {
		t.Errorf("no collections = %+v, %v; want none and no error", got, err)
	}
}

type libraryEpisodesFake map[string][]store.LibraryEpisode

func (f libraryEpisodesFake) LibraryEpisodes(_ context.Context, seriesID string) ([]store.LibraryEpisode, bool, error) {
	eps, ok := f[seriesID]
	return eps, ok, nil
}

// A library series expands from the Library's tables: an episode that cannot play is dropped, and
// a series with nothing to play, or an id that is no library series, stays empty.
func TestEpisodeResolverReadsLibrarySeriesFromTheTables(t *testing.T) {
	fake := libraryEpisodesFake{
		"lib-show": {
			{ItemID: "lib-e1", Name: "Pilot", Season: 1, Episode: 1, Year: 1992, PremiereDate: "1992-09-14",
				Rating: "TV-PG", Score: 7.5, Overview: "The first.", Tags: []string{"pilot"}, DurationMs: 1_320_000},
			{ItemID: "lib-e2", Name: "Not yet read", Season: 1, Episode: 2},
		},
		"lib-empty": nil,
	}
	resolve := episodeResolver(fake)
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
	for _, id := range []string{"lib-empty", "emby-show"} {
		if got, err := resolve(context.Background(), id); err != nil || len(got) != 0 {
			t.Errorf("%s resolved %+v, %v; want none", id, got, err)
		}
	}
}
