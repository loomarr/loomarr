package titlesearch_test

import (
	"context"
	"slices"
	"testing"

	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/library/titlesearch"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/testkit/catalogfixture"
)

// fakeEvidence answers from fixed rows and records the ids and terms it was asked for.
type fakeEvidence struct {
	rows  map[string]librarystore.LibraryEvidence
	asked [][]string
}

func (f *fakeEvidence) LibraryEvidence(_ context.Context, ids, keywords, people []string) (map[string]librarystore.LibraryEvidence, error) {
	f.asked = append(f.asked, ids, keywords, people)
	return f.rows, nil
}

// An owned discovery hit carries the asked keyword its Library tags hold and the asked person its
// Library credits name (#3006), even for a two-keyword query TMDB cannot attribute; what TMDB
// attached stays, and a title outside the Library or naming nothing gains nothing.
func TestCatalogDiscover_OwnedHitCarriesLibraryEvidence(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []titlesearch.Candidate{
		{MediaType: provision.Movie, TMDBID: 90000701, Name: "Owned Match", Source: titlesearch.ScopeTMDB},
		{MediaType: provision.Movie, TMDBID: 90000702, Name: "Owned Plain", Source: titlesearch.ScopeTMDB},
		{MediaType: provision.Movie, TMDBID: 90000703, Name: "Not Owned", Source: titlesearch.ScopeTMDB},
		{MediaType: provision.Movie, TMDBID: 90000704, Name: "Owned Attributed", Source: titlesearch.ScopeTMDB,
			Keywords: []string{"snow"}},
	}}
	presence := &catalogfixture.Presence{Hits: map[int]titlesearch.Presence{
		90000701: {LibraryItemID: "lib-701"},
		90000702: {LibraryItemID: "lib-702"},
		90000704: {LibraryItemID: "lib-704"},
	}}
	evidence := &fakeEvidence{rows: map[string]librarystore.LibraryEvidence{
		"lib-701": {Keywords: []string{"christmas"}, People: []string{"Ada Example", "Bo Director"}},
		"lib-703": {Keywords: []string{"christmas"}}, // never asked: not owned
		"lib-704": {Keywords: []string{"christmas"}},
	}}
	c := titlesearch.New(nil, corpus).WithPresence(presence).WithEvidence(evidence)

	got, err := c.Discover(context.Background(), titlesearch.DiscoveryQuery{MediaType: provision.Movie,
		Keywords: []string{"Christmas", "snow"}, Cast: []string{"ada example"}, Creators: []string{"Bo Director"}}, 10)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int]titlesearch.Candidate{}
	for _, cand := range got {
		byID[cand.TMDBID] = cand
	}
	match := byID[90000701]
	if !slices.Equal(match.Keywords, []string{"christmas"}) || !slices.Equal(match.Cast, []string{"Ada Example"}) ||
		!slices.Equal(match.Creators, []string{"Bo Director"}) {
		t.Errorf("owned match = keywords %q cast %q creators %q, want christmas / Ada Example / Bo Director",
			match.Keywords, match.Cast, match.Creators)
	}
	if plain := byID[90000702]; len(plain.Keywords)+len(plain.Cast)+len(plain.Creators) != 0 {
		t.Errorf("an owned title naming nothing gained evidence: %+v", plain)
	}
	if other := byID[90000703]; len(other.Keywords) != 0 {
		t.Errorf("a title outside the Library gained Library evidence: %q", other.Keywords)
	}
	if kept := byID[90000704]; !slices.Equal(kept.Keywords, []string{"snow"}) {
		t.Errorf("TMDB's own attribution = %q, want it kept as snow", kept.Keywords)
	}
	if len(evidence.asked) != 3 || !slices.Equal(evidence.asked[0], []string{"lib-701", "lib-702", "lib-704"}) {
		t.Errorf("asked %q, want one read of the three owned ids", evidence.asked)
	}
}

// A query with no keyword or person reads no Library evidence: titles that cannot match cost nothing.
func TestCatalogDiscover_NoTermsReadsNoEvidence(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []titlesearch.Candidate{
		{MediaType: provision.Movie, TMDBID: 90000701, Name: "Owned", Source: titlesearch.ScopeTMDB},
	}}
	presence := &catalogfixture.Presence{Hits: map[int]titlesearch.Presence{90000701: {LibraryItemID: "lib-701"}}}
	evidence := &fakeEvidence{}
	_, err := titlesearch.New(nil, corpus).WithPresence(presence).WithEvidence(evidence).Discover(
		context.Background(), titlesearch.DiscoveryQuery{MediaType: provision.Movie, Genres: []string{"Drama"}}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence.asked) != 0 {
		t.Errorf("a query with no keyword or person read evidence: %q", evidence.asked)
	}
}
