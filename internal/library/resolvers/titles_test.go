package resolvers

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/library/tmdb"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

func TestFranchises_UnconfiguredIsUnavailableNotReconcileFailure(t *testing.T) {
	resolver := NewFranchises(tmdb.NewDynamic(func() string { return "" }))

	collectionID, resolved, err := resolver.Collection(context.Background(), provision.Key("movie:tmdb:90000603"))
	if err != nil {
		t.Fatalf("Collection() error = %v, want nil for an unconfigured optional enrichment", err)
	}
	if resolved || collectionID != 0 {
		t.Fatalf("Collection() = (%d, %v), want unresolved so a configured reconcile can heal it", collectionID, resolved)
	}
}

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
func TestBoxSetsIndexesTheHouseholdsCollections(t *testing.T) {
	own := ownCollectionsFake{members: []librarystore.LibraryCollectionMember{
		{CollectionID: "own-1", Kind: "series", IDs: []librarystore.LibraryID{
			{Namespace: "tmdb", Value: "90000003"}, {Namespace: "tvdb", Value: "90000002"}}},
		{CollectionID: "own-1", Kind: "movie", IDs: []librarystore.LibraryID{{Namespace: "tmdb", Value: "90000001"}}},
		// An id only another catalog knows makes no provisioning key.
		{CollectionID: "own-2", Kind: "movie", IDs: []librarystore.LibraryID{
			{Namespace: "imdb", Value: "tt90000001"}, {Namespace: "tmdb", Value: "90000001"}}},
		{CollectionID: "own-2", Kind: "movie"},
	}}
	sets := NewBoxSets(own, time.Hour)
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

type libraryTitlesFake []librarystore.LibraryTitle

func (f libraryTitlesFake) LibraryTitles(_ context.Context, since time.Time) ([]librarystore.LibraryTitle, error) {
	return f, nil
}

func (f libraryTitlesFake) LibraryTitleByProviderID(_ context.Context, kind, namespace, value string) (librarystore.LibraryTitle, bool, error) {
	for _, t := range f {
		if t.Kind == kind && librarystore.IDIn(t.IDs, namespace) == value {
			return t, true, nil
		}
	}
	return librarystore.LibraryTitle{}, false, nil
}

var testLibraryTitles = libraryTitlesFake{
	{ID: "lib-film", Kind: "movie", Name: "Film", IDs: []librarystore.LibraryID{{Namespace: "tmdb", Value: "90000001"}},
		Rating: "PG", Genres: []string{"Drama"}, SizeBytes: 4_000},
	{ID: "lib-show", Kind: "series", Name: "Show", IDs: []librarystore.LibraryID{
		{Namespace: "tmdb", Value: "90000003"}, {Namespace: "tvdb", Value: "90000002"}}, Rating: "TV-Y"},
}

// The availability scan reads the Library's titles, so a title confirms with its library item id
// (#2151 step 16).
func TestLibraryTitleScannerListsTheLibrary(t *testing.T) {
	scanner := Scanner{titles: testLibraryTitles}
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
	lookup := OwnedLookup{titles: testLibraryTitles}
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

// Ratings answer an owned title from the Library, by its library item id.
func TestRatingsReadTheLibrary(t *testing.T) {
	ratings := Ratings{titles: testLibraryTitles}
	if r, ok, err := ratings.Rating(context.Background(), "series:tvdb:90000002"); err != nil || !ok || r != "TV-Y" {
		t.Errorf("show rating = %q, %v, %v", r, ok, err)
	}
	if _, ok, err := ratings.Rating(context.Background(), "movie:tmdb:90000009"); err != nil || ok {
		t.Errorf("an unowned title's rating: ok = %v, %v; want unresolved", ok, err)
	}
}
