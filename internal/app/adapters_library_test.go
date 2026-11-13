package app

import (
	"context"
	"slices"
	"testing"

	libraryapi "github.com/loomarr/loomarr/internal/api/library"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
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
