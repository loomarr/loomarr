package app

import (
	"context"
	"slices"
	"testing"
	"time"

	libraryapi "github.com/loomarr/loomarr/internal/api/library"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/provision"
)

type ownCollectionsFake struct {
	members []librarystore.LibraryCollectionMember
	colls   []librarystore.LibraryCollection
}

func (f ownCollectionsFake) LibraryCollectionMemberIDs(context.Context) ([]librarystore.LibraryCollectionMember, error) {
	return f.members, nil
}

func (f ownCollectionsFake) ListLibraryCollections(context.Context) ([]librarystore.LibraryCollection, error) {
	return f.colls, nil
}

// The household's collections stamp a title under each key it can be known by (#2151 step 16).
func TestLibraryBoxSetsIndexesTheHouseholdsCollections(t *testing.T) {
	own := ownCollectionsFake{members: []librarystore.LibraryCollectionMember{
		{CollectionID: "own-1", Kind: "series", IDs: []librarystore.LibraryID{
			{Namespace: "tmdb", Value: "90000003"}, {Namespace: "tvdb", Value: "90000002"}}},
		{CollectionID: "own-1", Kind: "movie", IDs: []librarystore.LibraryID{{Namespace: "tmdb", Value: "90000001"}}},
		// An id only another catalog knows makes no provisioning key.
		{CollectionID: "own-2", Kind: "movie", IDs: []librarystore.LibraryID{
			{Namespace: "imdb", Value: "tt90000001"}, {Namespace: "tmdb", Value: "90000001"}}},
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
	own := ownCollectionsFake{colls: []librarystore.LibraryCollection{
		{ID: "own-1", Name: "Harbor Films", ImportedFrom: "emby", ImportedID: "emby-coll-1", Members: []string{"m1", "m2"}},
		{ID: "own-3", Name: "Rainy Days", Members: []string{"m3"}},
	}}
	got, err := libraryCollections{own: own}.Collections(context.Background())
	want := []libraryapi.LibraryCollection{
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
