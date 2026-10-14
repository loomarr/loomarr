package app

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/moviecollections"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/testkit/catalogfixture"
	"github.com/loomarr/loomarr/internal/tmdb"
)

func TestSearchAdapterCarriesGroundedEditorialEvidence(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{{
		MediaType: provision.Movie, TMDBID: 90000603, Name: "Marigold Protocol", Year: 1999,
		Genres: []string{"Science Fiction"}, Overview: "A florist's coded orders.",
		OriginalLanguage: "en", OriginCountries: []string{"US"}, RuntimeMinutes: 136,
		VoteAverage: 8.2, VoteCount: 27_000, Keywords: []string{"secret code"},
	}}}

	got, err := (searchAdapter{cat: catalog.New(nil, corpus)}).Search(context.Background(), api.SearchRequest{
		Query: "marigold", Scope: "tmdb", Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Overview != "A florist's coded orders." || got[0].OriginalLanguage != "en" ||
		len(got[0].OriginCountries) != 1 || got[0].OriginCountries[0] != "US" || got[0].RuntimeMinutes != 136 ||
		got[0].VoteAverage != 8.2 || got[0].VoteCount != 27_000 || len(got[0].Keywords) != 1 {
		t.Fatalf("search API candidate lost grounded evidence: %+v", got)
	}
}

func TestMovieCollectionAdapterUsesAuthoritativeTMDBRoster(t *testing.T) {
	tmdbServer := testkit.NewTMDB(t)
	tmdbServer.AddMovie(90000671, "Wren Halloway and the Lantern Key", 2001, []int{12, 14}, "A young archivist begins school.")
	tmdbServer.AddMovie(90000672, "Wren Halloway and the Sunken Library", 2002, []int{12, 14}, "The second school year.")
	tmdbServer.AddMovie(90000673, "Wren Halloway and the Glass Orchard", 2004, []int{12, 14}, "A runaway gardener shadows the school.")
	tmdbServer.SetMovieCollection(90001241, "Wren Halloway Collection", 90000671, 90000672, 90000673)
	presence := &catalogfixture.Presence{Hits: map[int]catalog.Presence{
		90000672: {LibraryItemID: "library-672", OfficialRating: "PG"},
	}}
	client := tmdb.NewWithBase(tmdbServer.URL, "key")
	adapter := movieCollectionAdapter{resolver: moviecollections.New(client).WithPresenceSource(
		func() catalog.LibraryPresence { return presence },
	)}

	got, err := adapter.ResolveMovieCollections(context.Background(), api.MovieCollectionRequest{
		Keys: []string{"movie:tmdb:90000671", "movie:tmdb:90000673"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Complete || len(got.Collections) != 1 || got.Collections[0].TMDBID != 90001241 {
		t.Fatalf("resolution = %+v, want one complete authoritative collection", got)
	}
	if members := got.Collections[0].Members; len(members) != 3 || members[1].TMDBID != 90000672 || !members[1].InLibrary || members[1].LibraryItemID != "library-672" {
		t.Fatalf("members = %+v, want release order and Library presence", members)
	}
}

func TestSearchAdapterUsesCatalogDiscoveryForStructuredRequests(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{{
		MediaType: provision.Series, TMDBID: 90_020_001, Name: "Signal House", Networks: []string{"Kestrel One"},
	}}}
	discovery := &api.SearchDiscovery{
		MediaType: "series", Genres: []string{"Comedy"}, Network: "Kestrel One", OriginCountry: "US",
	}

	got, err := (searchAdapter{cat: catalog.New(nil, corpus)}).Search(context.Background(), api.SearchRequest{
		Scope: "all", Limit: 12, Discovery: discovery,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].TMDBID != 90_020_001 || len(got[0].Networks) != 1 || got[0].Networks[0] != "Kestrel One" {
		t.Fatalf("structured search result = %+v", got)
	}
	if searches := corpus.Searches(); len(searches) != 0 {
		t.Fatalf("structured request used title search: %+v", searches)
	}
	discoveries := corpus.Discoveries()
	// Catalog deliberately asks its discovery source for a larger bounded pool before
	// applying the owned/outside-Library blend; the public limit is applied afterward.
	if len(discoveries) != 1 || discoveries[0].Limit != 24 || discoveries[0].Query.MediaType != provision.Series ||
		discoveries[0].Query.Network != "Kestrel One" || discoveries[0].Query.OriginCountry != "US" ||
		len(discoveries[0].Query.Genres) != 1 || discoveries[0].Query.Genres[0] != "Comedy" {
		t.Fatalf("catalog discovery request = %+v", discoveries)
	}
}

func TestSearchAdapterNarrowsTitleResultsByMediaType(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{
		{MediaType: provision.Movie, TMDBID: 90000001, Name: "Movie"},
		{MediaType: provision.Series, TMDBID: 90000002, Name: "Series"},
	}}

	got, err := (searchAdapter{cat: catalog.New(nil, corpus)}).Search(context.Background(), api.SearchRequest{
		Query: "shared title", MediaType: "series", Scope: "tmdb", Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].TMDBID != 90000002 {
		t.Fatalf("media-type narrowed title results = %+v", got)
	}
}

func TestTMDBFranchises_UnconfiguredIsUnavailableNotReconcileFailure(t *testing.T) {
	resolver := tmdbFranchises{tmdb: tmdb.NewDynamic(func() string { return "" })}

	collectionID, resolved, err := resolver.Collection(context.Background(), provision.Key("movie:tmdb:90000603"))
	if err != nil {
		t.Fatalf("Collection() error = %v, want nil for an unconfigured optional enrichment", err)
	}
	if resolved || collectionID != 0 {
		t.Fatalf("Collection() = (%d, %v), want unresolved so a configured reconcile can heal it", collectionID, resolved)
	}
}
