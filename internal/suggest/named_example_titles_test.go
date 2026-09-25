package suggest_test

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/moviecollections"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/testkit/catalogfixture"
	"github.com/loomarr/loomarr/internal/testkit/moviecollectionsfixture"
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
		catalogSearchResponse(map[string]any{"query": "The Goonies", "media_type": "movie", "dateMeaning": meaning}),
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

// #1498 review, on the real library's shape: a named title with an owned
// namesake still anchors, every named title AND the franchise anchor together,
// the franchise is filtered to the request's decade, and a trailing "for a
// family night" is not part of the last title.
func TestSuggest_ExampleTitlesAnchorTogetherWithinEra(t *testing.T) {
	cases := []struct {
		description string
		meaning     map[string]any
		want        []string
		absent      []string
	}{
		{
			"A Sunday-afternoon channel of 1980s adventure movies like Indiana Jones and The Goonies",
			fixtureDateMeaning("movie_release", "description", 30, 35, 1980, 1989),
			[]string{"The Goonies", "Raiders of the Lost Ark", "Indiana Jones and the Temple of Doom", "Indiana Jones and the Last Crusade"},
			[]string{"Indiana Jones and the Dial of Destiny"},
		},
		{
			"Movies like Back to the Future and Gremlins for a family night",
			dateMeaningNone(),
			[]string{"Back to the Future", "Gremlins"},
			nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			corpus := namedTitlesCorpus()
			var toyStory catalog.Candidate
			for _, c := range corpus.Candidates {
				if c.Name == "Toy Story 5" {
					toyStory = c
				}
			}
			model := testkit.NewLLM(
				catalogSearchResponse(map[string]any{"query": "Toy Story", "media_type": "movie", "dateMeaning": tc.meaning}),
				finalResponseWithDateMeaning(`{"picks":[{"mediaType":"movie","key":"movie:tmdb:`+strconv.Itoa(toyStory.TMDBID)+`","name":"Toy Story 5"}]}`, tc.meaning),
			)
			collections, presence := indianaJonesCollection(corpus)
			s := suggest.New(model, catalog.New(nil, corpus), referenceExistsValidator{}, 10).
				WithMovieCollections(moviecollections.New(collections).WithPresenceSource(func() catalog.LibraryPresence { return presence }))
			proposal, err := s.Suggest(context.Background(), suggest.Intent{Description: tc.description})
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, item := range append(append([]suggest.ProposalItem(nil), proposal.Lineup...), proposal.Acquisitions...) {
				names = append(names, item.Name)
			}
			for _, want := range tc.want {
				if !slices.Contains(names, want) {
					t.Errorf("named %q was dropped: %v", want, names)
				}
			}
			for _, bad := range tc.absent {
				if slices.Contains(names, bad) {
					t.Errorf("%q is outside the requested era: %v", bad, names)
				}
			}
		})
	}
}

// indianaJonesCollection is TMDB's authoritative roster for the franchise. It
// includes Raiders of the Lost Ark, whose title does not begin "Indiana Jones".
func indianaJonesCollection(corpus *catalogfixture.Corpus) (*moviecollectionsfixture.Source, *catalogfixture.Presence) {
	var members []catalog.Candidate
	presence := &catalogfixture.Presence{Hits: map[int]catalog.Presence{}}
	refs := map[int]moviecollections.CollectionRef{}
	for _, c := range corpus.Candidates {
		presence.Hits[c.TMDBID] = catalog.Presence{LibraryItemID: "item-" + strconv.Itoa(c.TMDBID)}
		if c.Name == "Raiders of the Lost Ark" || strings.HasPrefix(c.Name, "Indiana Jones") {
			members = append(members, catalog.Candidate{MediaType: "movie", TMDBID: c.TMDBID, Name: c.Name, Year: c.Year})
			refs[c.TMDBID] = moviecollections.CollectionRef{TMDBID: 84, Name: "Indiana Jones Collection"}
		}
	}
	return &moviecollectionsfixture.Source{
		Refs:        refs,
		Collections: map[int]moviecollections.SourceCollection{84: {TMDBID: 84, Name: "Indiana Jones Collection", Members: members}},
	}, presence
}
