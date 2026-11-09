package catalog_test

import (
	"context"
	"errors"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/testkit/catalogfixture"
	"github.com/loomarr/loomarr/internal/tmdb"
)

func twoTitleCorpus() *catalogfixture.Corpus {
	return &catalogfixture.Corpus{Candidates: []catalog.Candidate{
		{MediaType: provision.Movie, TMDBID: 90000603, Name: "Marigold Protocol", Source: catalog.ScopeTMDB},
		{MediaType: provision.Movie, TMDBID: 90000604, Name: "Marigold Protocol II", Source: catalog.ScopeTMDB},
	}}
}

// A presence lookup that fails must fail discovery: skipping it would return an owned title as an
// acquisition candidate, which the lineup then requests from the arr.
func TestCatalogDiscover_PresenceLookupFailureFailsDiscovery(t *testing.T) {
	boom := errors.New("library store unreachable")
	presence := &catalogfixture.Presence{Err: boom}

	got, err := catalog.New(nil, twoTitleCorpus()).WithPresence(presence).Discover(
		context.Background(), catalog.DiscoveryQuery{MediaType: provision.Movie}, 10)
	if !errors.Is(err, boom) {
		t.Fatalf("Discover err = %v (candidates %+v), want the presence error", err, got)
	}
	if got != nil {
		t.Errorf("Discover returned candidates %+v beside an error", got)
	}
}

func TestCatalogDiscoverUnion_PresenceLookupFailureFailsDiscovery(t *testing.T) {
	boom := errors.New("library store unreachable")
	presence := &catalogfixture.Presence{Err: boom}

	_, err := catalog.New(nil, twoTitleCorpus()).WithPresence(presence).DiscoverUnion(
		context.Background(), []catalog.DiscoveryQuery{{MediaType: provision.Movie}})
	if !errors.Is(err, boom) {
		t.Fatalf("DiscoverUnion err = %v, want the presence error", err)
	}
}

// A cancelled context stops the presence walk at once instead of looking up every remaining title.
func TestCatalogDiscover_CancelledContextStopsPresenceLookups(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	presence := &catalogfixture.Presence{OnCall: cancel}

	_, err := catalog.New(nil, twoTitleCorpus()).WithPresence(presence).Discover(
		ctx, catalog.DiscoveryQuery{MediaType: provision.Movie}, 10)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Discover err = %v, want context.Canceled", err)
	}
	if calls := presence.Calls(); len(calls) != 1 {
		t.Errorf("presence lookups after cancel = %v, want the walk to stop after the first", calls)
	}
}

func TestAdjacent_PresenceLookupFailureFailsTheWalk(t *testing.T) {
	mt := testkit.NewTMDB(t).WithRecommendations(map[int][]int{
		90000100: {90000603}, 90000101: {90000603},
	})
	boom := errors.New("library store unreachable")
	c := catalog.New(testLibrary(t), tmdb.NewWithBase(mt.URL, "test-key")).WithPresence(&catalogfixture.Presence{Err: boom})

	_, err := c.Adjacent(context.Background(),
		[]catalog.Candidate{seed(90000100, "Quiet Engines"), seed(90000101, "Lemon Tree Heist")},
		map[provision.Key]bool{}, 10)
	if !errors.Is(err, boom) {
		t.Fatalf("Adjacent err = %v, want the presence error", err)
	}
}
