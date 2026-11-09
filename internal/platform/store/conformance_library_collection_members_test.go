package store

import (
	"context"
	"reflect"
	"testing"
	"time"
)

// A collection's members, by the provider ids a lineup knows them by: one with no provider id
// lists empty ids.
func testLibraryCollectionMemberIDs(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_800_000_000_000)
	if err := st.SaveLibraryItems(ctx, []LibraryItem{
		{ID: "film", Kind: "movie", Name: "Film", IDs: []LibraryID{{"tmdb", "90000001"}, {"imdb", "tt90000001"}}},
		{ID: "show", Kind: "series", Name: "Show", IDs: []LibraryID{{"tvdb", "90000002"}, {"tmdb", "90000003"}}},
		{ID: "home", Kind: "movie", Name: "Home video"},
	}, at); err != nil {
		t.Fatal(err)
	}
	night, err := st.CreateLibraryCollection(ctx, LibraryCollection{
		Name: "Nights", Members: []string{"show", "film"},
	}, at)
	if err != nil {
		t.Fatal(err)
	}
	home, err := st.CreateLibraryCollection(ctx, LibraryCollection{Name: "Home", Members: []string{"home"}}, at)
	if err != nil {
		t.Fatal(err)
	}

	got, err := st.LibraryCollectionMemberIDs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]LibraryCollectionMember{
		night: {
			{CollectionID: night, Kind: "series", IDs: []LibraryID{{"tmdb", "90000003"}, {"tvdb", "90000002"}}},
			{CollectionID: night, Kind: "movie", IDs: []LibraryID{{"imdb", "tt90000001"}, {"tmdb", "90000001"}}},
		},
		home: {{CollectionID: home, Kind: "movie"}},
	}
	byCollection := map[string][]LibraryCollectionMember{}
	for _, m := range got {
		byCollection[m.CollectionID] = append(byCollection[m.CollectionID], m)
	}
	if !reflect.DeepEqual(byCollection, want) {
		t.Errorf("members = %+v, want %+v", byCollection, want)
	}
}
