package moviecollections_test

import (
	"context"
	"errors"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/moviecollections"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/testkit/catalogfixture"
	"github.com/loomarr/loomarr/internal/testkit/moviecollectionsfixture"
)

func TestResolve_DeduplicatesCollectionsOrdersMembersAndMarksLibraryPresence(t *testing.T) {
	source := &moviecollectionsfixture.Source{
		Refs: map[int]moviecollections.CollectionRef{
			90000671: {TMDBID: 90001241, Name: "Wren Halloway Collection"},
			90000673: {TMDBID: 90001241, Name: "Wren Halloway Collection"},
		},
		Collections: map[int]moviecollections.SourceCollection{
			90001241: {
				TMDBID: 90001241,
				Name:   "Wren Halloway Collection",
				Members: []catalog.Candidate{
					{MediaType: provision.Movie, TMDBID: 90000673, Name: "Wren Halloway and the Glass Orchard", Year: 2004, Source: catalog.ScopeTMDB},
					{MediaType: provision.Movie, TMDBID: 90000671, Name: "Wren Halloway and the Lantern Key", Year: 2001, InLibrary: true, LibraryItemID: "untrusted-source-id", Source: catalog.ScopeTMDB},
					{MediaType: provision.Movie, TMDBID: 90000672, Name: "Wren Halloway and the Sunken Library", Year: 2002, Source: catalog.ScopeTMDB},
				},
			},
		},
	}
	presence := &catalogfixture.Presence{Hits: map[int]catalog.Presence{
		90000672: {LibraryItemID: "lib-672", OfficialRating: "PG", Genres: []string{"Fantasy"}},
	}}
	resolver := moviecollections.New(source).WithPresenceSource(func() catalog.LibraryPresence { return presence })

	got, err := resolver.Resolve(context.Background(), []provision.Key{
		"movie:tmdb:90000671", "movie:tmdb:90000673", "movie:tmdb:90000671",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Complete || len(got.Collections) != 1 {
		t.Fatalf("resolution = %+v, want one complete deduplicated collection", got)
	}
	collection := got.Collections[0]
	if collection.TMDBID != 90001241 || collection.Name != "Wren Halloway Collection" {
		t.Fatalf("collection = %+v", collection)
	}
	if len(collection.Members) != 3 || collection.Members[0].TMDBID != 90000671 || collection.Members[1].TMDBID != 90000672 || collection.Members[2].TMDBID != 90000673 {
		t.Fatalf("members = %+v, want canonical release order 90000671, 90000672, 90000673", collection.Members)
	}
	if !collection.Members[1].InLibrary || collection.Members[1].LibraryItemID != "lib-672" || collection.Members[1].OfficialRating != "PG" {
		t.Fatalf("owned member = %+v, want Library presence and enforcement metadata", collection.Members[1])
	}
	if collection.Members[0].InLibrary || collection.Members[0].LibraryItemID != "" {
		t.Fatalf("missing member = %+v, want only current Library presence treated as ownership", collection.Members[0])
	}
	if calls := source.CollectionCalls(); len(calls) != 1 || calls[0] != 90001241 {
		t.Fatalf("collection calls = %v, want one lookup for shared collection 90001241", calls)
	}
	if calls := source.ReferenceCalls(); len(calls) != 2 {
		t.Fatalf("movie reference calls = %v, want one lookup per distinct visible movie", calls)
	}
}

func TestResolve_OmitsAuthoritativeStandaloneMovies(t *testing.T) {
	source := &moviecollectionsfixture.Source{
		Refs: map[int]moviecollections.CollectionRef{
			90000550: {},
			90000671: {TMDBID: 90001241, Name: "Wren Halloway Collection"},
		},
		Collections: map[int]moviecollections.SourceCollection{
			90001241: {
				TMDBID: 90001241,
				Name:   "Wren Halloway Collection",
				Members: []catalog.Candidate{
					{MediaType: provision.Movie, TMDBID: 90000671, Name: "Wren Halloway and the Lantern Key", Year: 2001},
					{MediaType: provision.Movie, TMDBID: 90000672, Name: "Wren Halloway and the Sunken Library", Year: 2002},
				},
			},
		},
	}

	got, err := moviecollections.New(source).Resolve(context.Background(), []provision.Key{
		"movie:tmdb:90000550", "movie:tmdb:90000671",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Complete || len(got.Collections) != 1 || got.Collections[0].TMDBID != 90001241 {
		t.Fatalf("resolution = %+v, want standalone omitted without degrading the collection result", got)
	}
}

func TestResolve_ReturnsPartialGroundedCollectionsWithoutInventingFailedOnes(t *testing.T) {
	source := &moviecollectionsfixture.Source{
		Refs: map[int]moviecollections.CollectionRef{
			90000671: {TMDBID: 90001241, Name: "Wren Halloway Collection"},
			90000011: {TMDBID: 90000010, Name: "Hollow Moon Collection"},
		},
		Collections: map[int]moviecollections.SourceCollection{
			90001241: {
				TMDBID: 90001241,
				Name:   "Wren Halloway Collection",
				Members: []catalog.Candidate{
					{MediaType: provision.Movie, TMDBID: 90000671, Name: "Wren Halloway and the Lantern Key", Year: 2001},
					{MediaType: provision.Movie, TMDBID: 90000672, Name: "Wren Halloway and the Sunken Library", Year: 2002},
				},
			},
		},
		CollectionErrors: map[int]error{90000010: errors.New("tmdb unavailable for collection 90000010")},
	}

	got, err := moviecollections.New(source).Resolve(context.Background(), []provision.Key{
		"movie:tmdb:90000671", "movie:tmdb:90000011",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Complete || len(got.Collections) != 1 || got.Collections[0].TMDBID != 90001241 {
		t.Fatalf("resolution = %+v, want the one grounded collection and complete=false", got)
	}
}

func TestResolve_ReturnsErrorWhenEveryReferencedCollectionFails(t *testing.T) {
	source := &moviecollectionsfixture.Source{
		Refs: map[int]moviecollections.CollectionRef{
			90000671: {TMDBID: 90001241, Name: "Wren Halloway Collection"},
		},
		CollectionErrors: map[int]error{90001241: errors.New("tmdb unavailable")},
	}

	got, err := moviecollections.New(source).Resolve(context.Background(), []provision.Key{"movie:tmdb:90000671"})
	if err == nil {
		t.Fatalf("resolution = %+v, want a total upstream failure to return an error", got)
	}
}

func TestResolve_DropsMalformedDuplicateAndNonMovieRosterMembers(t *testing.T) {
	source := &moviecollectionsfixture.Source{
		Refs: map[int]moviecollections.CollectionRef{
			90000671: {TMDBID: 90001241, Name: "Wren Halloway Collection"},
		},
		Collections: map[int]moviecollections.SourceCollection{
			90001241: {
				TMDBID: 90001241,
				Name:   "Wren Halloway Collection",
				Members: []catalog.Candidate{
					{MediaType: provision.Movie, TMDBID: 90000671, Name: "Wren Halloway and the Lantern Key", Year: 2001},
					{MediaType: provision.Movie, TMDBID: 90000671, Name: "duplicate", Year: 2001},
					{MediaType: provision.Movie, TMDBID: 0, Name: "missing id"},
					{MediaType: provision.Movie, TMDBID: 90000672, Name: ""},
					{MediaType: provision.Series, TMDBID: 90000673, Name: "wrong media type"},
					{MediaType: provision.Movie, TMDBID: 90000674, Name: "Wren Halloway and the Ember Clock", Year: 2005},
				},
			},
		},
	}

	got, err := moviecollections.New(source).Resolve(context.Background(), []provision.Key{"movie:tmdb:90000671"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Collections) != 1 || len(got.Collections[0].Members) != 2 {
		t.Fatalf("resolution = %+v, want only the two grounded valid movie members", got)
	}
	if got.Complete {
		t.Fatalf("resolution = %+v, want malformed authoritative rows reported as incomplete", got)
	}
	if got.Collections[0].Members[0].Name == "duplicate" {
		t.Fatalf("members = %+v, want the first authoritative occurrence retained", got.Collections[0].Members)
	}
}

func TestResolve_RejectsACollectionResponseWithTheWrongIdentity(t *testing.T) {
	source := &moviecollectionsfixture.Source{
		Refs: map[int]moviecollections.CollectionRef{90000671: {TMDBID: 90001241, Name: "Wren Halloway Collection"}},
		Collections: map[int]moviecollections.SourceCollection{
			90001241: {
				TMDBID: 90009999,
				Name:   "Unrelated Collection",
				Members: []catalog.Candidate{
					{MediaType: provision.Movie, TMDBID: 90000671, Name: "First", Year: 2001},
					{MediaType: provision.Movie, TMDBID: 90000672, Name: "Second", Year: 2002},
				},
			},
		},
	}

	got, err := moviecollections.New(source).Resolve(context.Background(), []provision.Key{"movie:tmdb:90000671"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Complete || len(got.Collections) != 0 {
		t.Fatalf("resolution = %+v, want mismatched collection identity rejected as incomplete", got)
	}
}

func TestResolve_DoesNotInventMissingCollectionIdentity(t *testing.T) {
	source := &moviecollectionsfixture.Source{
		Refs: map[int]moviecollections.CollectionRef{90000671: {TMDBID: 90001241}},
		Collections: map[int]moviecollections.SourceCollection{
			90001241: {
				TMDBID: 90001241,
				Members: []catalog.Candidate{
					{MediaType: provision.Movie, TMDBID: 90000671, Name: "First", Year: 2001},
					{MediaType: provision.Movie, TMDBID: 90000672, Name: "Second", Year: 2002},
				},
			},
		},
	}

	got, err := moviecollections.New(source).Resolve(context.Background(), []provision.Key{"movie:tmdb:90000671"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Complete || len(got.Collections) != 0 {
		t.Fatalf("resolution = %+v, want an incomplete empty result without an invented collection name", got)
	}
}
