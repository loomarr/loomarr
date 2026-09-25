package suggest_test

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
)

// #1497: the model searched only "Indiana Jones" (never "The Goonies") and then
// finalized on a "Sunday afternoon" family pick. Example titles the user named
// must be resolved against the library independent of what the model searches.
func TestSuggest_ExampleTitlesInRequestAreAnchored(t *testing.T) {
	description := "A Sunday-afternoon channel of 1980s adventure movies like Indiana Jones and The Goonies"
	meaning := fixtureDateMeaning("movie_release", "description", 30, 35, 1980, 1989)
	corpus := namedTitlesCorpus()
	byName := func(name string) catalog.Candidate {
		for _, c := range corpus.Candidates {
			if c.Name == name {
				return c
			}
		}
		t.Fatalf("fixture has no %q", name)
		return catalog.Candidate{}
	}
	castle := byName("Castle in the Sky")
	model := testkit.NewLLM(
		catalogSearchResponse(map[string]any{"query": "Indiana Jones", "media_type": "movie", "dateMeaning": meaning}),
		finalResponseWithDateMeaning(`{"picks":[{"mediaType":"movie","key":"movie:tmdb:`+strconv.Itoa(castle.TMDBID)+`","name":"Castle in the Sky"}]}`, meaning),
	)
	s := suggest.New(model, catalog.New(nil, corpus), referenceExistsValidator{}, 10)

	proposal, err := s.Suggest(context.Background(), suggest.Intent{Description: description})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, item := range append(append([]suggest.ProposalItem(nil), proposal.Lineup...), proposal.Acquisitions...) {
		names = append(names, item.Name)
	}
	if !slices.Contains(names, "The Goonies") {
		t.Fatalf("named example The Goonies was dropped: %v", names)
	}
	franchise := 0
	for _, name := range names {
		if strings.HasPrefix(name, "Indiana Jones") {
			franchise++
		}
	}
	if franchise == 0 {
		t.Fatalf("named franchise Indiana Jones surfaced no members: %v", names)
	}
}

// "I'd like a channel of…" uses "like" as a verb: no example was named, so the
// clause after it must not be searched as a title.
func TestSuggest_LikeAsVerbNamesNoExample(t *testing.T) {
	description := "I'd like a channel of 1980s adventure movies"
	meaning := fixtureDateMeaning("movie_release", "description", 20, 25, 1980, 1989)
	corpus := namedTitlesCorpus()
	goonies := corpus.Candidates[3]
	model := testkit.NewLLM(
		catalogSearchResponse(map[string]any{"query": "adventure", "media_type": "movie", "dateMeaning": meaning}),
		finalResponseWithDateMeaning(`{"picks":[{"mediaType":"movie","key":"movie:tmdb:`+strconv.Itoa(goonies.TMDBID)+`","name":"The Goonies"}]}`, meaning),
	)
	s := suggest.New(model, catalog.New(nil, corpus), referenceExistsValidator{}, 10)
	if _, err := s.Suggest(context.Background(), suggest.Intent{Description: description}); err != nil {
		t.Fatal(err)
	}
	for _, search := range corpus.Searches() {
		if strings.Contains(search.Query, "channel") {
			t.Fatalf("prose after a verb 'like' was searched as a title: %q", search.Query)
		}
	}
}
