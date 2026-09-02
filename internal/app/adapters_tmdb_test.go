package app

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/testkit/catalogfixture"
	"github.com/loomarr/loomarr/internal/tmdb"
)

func TestSearchAdapterCarriesGroundedEditorialEvidence(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{{
		MediaType: provision.Movie, TMDBID: 603, Name: "The Matrix", Year: 1999,
		Genres: []string{"Science Fiction"}, Overview: "A simulated world.",
		OriginalLanguage: "en", OriginCountries: []string{"US"}, RuntimeMinutes: 136,
		VoteAverage: 8.2, VoteCount: 27_000, Keywords: []string{"virtual reality"},
	}}}

	got, err := (searchAdapter{cat: catalog.New(nil, corpus)}).Search(context.Background(), "matrix", "tmdb", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Overview != "A simulated world." || got[0].OriginalLanguage != "en" ||
		len(got[0].OriginCountries) != 1 || got[0].OriginCountries[0] != "US" || got[0].RuntimeMinutes != 136 ||
		got[0].VoteAverage != 8.2 || got[0].VoteCount != 27_000 || len(got[0].Keywords) != 1 {
		t.Fatalf("search API candidate lost grounded evidence: %+v", got)
	}
}

func TestTMDBFranchises_UnconfiguredIsUnavailableNotReconcileFailure(t *testing.T) {
	resolver := tmdbFranchises{tmdb: tmdb.NewDynamic(func() string { return "" })}

	collectionID, resolved, err := resolver.Collection(context.Background(), provision.Key("movie:tmdb:603"))
	if err != nil {
		t.Fatalf("Collection() error = %v, want nil for an unconfigured optional enrichment", err)
	}
	if resolved || collectionID != 0 {
		t.Fatalf("Collection() = (%d, %v), want unresolved so a configured reconcile can heal it", collectionID, resolved)
	}
}
