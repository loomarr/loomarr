package suggest_test

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/moviecollections"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/testkit/catalogfixture"
	"github.com/loomarr/loomarr/internal/testkit/moviecollectionsfixture"
)

// titleRow is one library title of a test corpus.
type titleRow struct {
	name   string
	year   int
	genres []string
}

// namedTitlesCorpus is a library that CONTAINS the titles a request names plus
// the "Sunday afternoon" family/kids distractors seen in the #1497 repro.
func namedTitlesCorpus() *catalogfixture.Corpus {
	adventure := []string{"Adventure", "Family"}
	return namedTitlesCorpusOf([]titleRow{
		{"Seekers of the Sunken Bell", 1981, []string{"Adventure", "Action"}},
		{"Atlas Finch and the Glass Idol", 1984, []string{"Adventure", "Action"}},
		{"Atlas Finch and the Last Lantern", 1989, []string{"Adventure", "Action"}},
		{"The Tinkerlings", 1985, adventure},
		{"Courting the Comet", 1984, []string{"Adventure", "Comedy", "Romance"}},
		{"The Lighthouse Bride", 1987, adventure},
		{"Hedgemaze", 1986, adventure},
		{"Thistle", 1988, []string{"Adventure", "Fantasy"}},
		{"The EverTurning Page", 1984, adventure},
		{"Wait for Us", 1986, []string{"Adventure", "Drama"}},
		{"Kite over the Harbour", 1986, []string{"Animation", "Family", "Adventure"}},
		{"Biscuit the Bold", 1987, []string{"Family", "Adventure"}},
		{"Sky-Lad and Star-Lass: The Secret of the Shield", 1985, []string{"Animation", "Family"}},
		{"StormBadgers Go!", 1988, []string{"Animation", "Family"}},
		{"Good Old Dog", 1988, []string{"Family", "Drama"}},
		{"The Valley Before Rain", 1988, []string{"Animation", "Family"}},
		{"Pip & Company", 1988, []string{"Animation", "Family"}},
		// The real library holds a modern franchise entry, a namesake of a named
		// title, and more than one title named in a request (#1498 review).
		{"Atlas Finch and the Clockwork Sun", 2023, []string{"Adventure", "Action"}},
		{"The Tinkerlings", 2019, []string{"Documentary"}},
		{"Back to the Lighthouse", 1985, []string{"Adventure", "Comedy", "Science Fiction"}},
		{"Gloomwings", 1984, []string{"Comedy", "Horror", "Fantasy"}},
		{"Pocket Robots 5", 2026, []string{"Animation", "Family"}},
	})
}

// namedTitlesCorpusOf builds the #1497 library from rows, in order, with ids from
// 90002000. Search behaves like a title search: every query word must appear in the name.
func namedTitlesCorpusOf(rows []titleRow) *catalogfixture.Corpus {
	cands := make([]catalog.Candidate, 0, len(rows))
	for i, r := range rows {
		votes := 1000
		if r.year == 2019 {
			votes = 3 // the documentary namesake
		}
		cands = append(cands, catalog.Candidate{
			MediaType: "movie", TMDBID: 90_002_000 + i, Name: r.name, Year: r.year, InLibrary: true, Genres: r.genres,
			VoteCount: votes, Overview: "An 1980s film in the library.",
		})
	}
	corpus := &catalogfixture.Corpus{Candidates: cands}
	corpus.SearchFunc = func(_ context.Context, query string, limit int) ([]catalog.Candidate, error) {
		words := strings.Fields(strings.ToLower(strings.NewReplacer(":", " ", "-", " ", "&", " ").Replace(query)))
		var out []catalog.Candidate
	next:
		for _, c := range cands {
			name := strings.ToLower(strings.NewReplacer(":", " ", "-", " ", "&", " ").Replace(c.Name))
			for _, w := range words {
				if !strings.Contains(name, w) {
					continue next
				}
			}
			out = append(out, c)
		}
		if limit > 0 && len(out) > limit {
			out = out[:limit]
		}
		return out, nil
	}
	return corpus
}

// #1497: the model searched only the named franchise (never the second named film)
// and then finalized on a "Sunday afternoon" family pick. Example titles the user
// named must be resolved against the library independent of what the model searches.
func TestSuggest_ExampleTitlesInRequestAreAnchored(t *testing.T) {
	description := "A Sunday-afternoon channel of 1980s adventure movies like Atlas Finch and The Tinkerlings"
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
	kite := byName("Kite over the Harbour")
	model := testkit.NewLLM(
		catalogSearchResponse(map[string]any{"query": "Atlas Finch", "media_type": "movie", "dateMeaning": meaning}),
		finalResponseWithDateMeaning(`{"picks":[{"mediaType":"movie","key":"movie:tmdb:`+strconv.Itoa(kite.TMDBID)+`","name":"Kite over the Harbour"}]}`, meaning),
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
	if !slices.Contains(names, "The Tinkerlings") {
		t.Fatalf("named example The Tinkerlings was dropped: %v", names)
	}
	franchise := 0
	for _, name := range names {
		if strings.HasPrefix(name, "Atlas Finch") {
			franchise++
		}
	}
	if franchise == 0 {
		t.Fatalf("named franchise Atlas Finch surfaced no members: %v", names)
	}
}

// "I'd like a channel of…" uses "like" as a verb: no example was named, so the
// clause after it must not be searched as a title.
func TestSuggest_LikeAsVerbNamesNoExample(t *testing.T) {
	description := "I'd like a channel of 1980s adventure movies"
	meaning := fixtureDateMeaning("movie_release", "description", 20, 25, 1980, 1989)
	corpus := namedTitlesCorpus()
	tinkerlings := corpus.Candidates[3]
	model := testkit.NewLLM(
		catalogSearchResponse(map[string]any{"query": "The Tinkerlings", "media_type": "movie", "dateMeaning": meaning}),
		finalResponseWithDateMeaning(`{"picks":[{"mediaType":"movie","key":"movie:tmdb:`+strconv.Itoa(tinkerlings.TMDBID)+`","name":"The Tinkerlings"}]}`, meaning),
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
			"A Sunday-afternoon channel of 1980s adventure movies like Atlas Finch and The Tinkerlings",
			fixtureDateMeaning("movie_release", "description", 30, 35, 1980, 1989),
			[]string{"The Tinkerlings", "Seekers of the Sunken Bell", "Atlas Finch and the Glass Idol", "Atlas Finch and the Last Lantern"},
			[]string{"Atlas Finch and the Clockwork Sun"},
		},
		{
			"Movies like Back to the Lighthouse and Gloomwings for a family night",
			dateMeaningNone(),
			[]string{"Back to the Lighthouse", "Gloomwings"},
			nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			corpus := namedTitlesCorpus()
			var robots catalog.Candidate
			for _, c := range corpus.Candidates {
				if c.Name == "Pocket Robots 5" {
					robots = c
				}
			}
			model := testkit.NewLLM(
				catalogSearchResponse(map[string]any{"query": "Pocket Robots", "media_type": "movie", "dateMeaning": tc.meaning}),
				finalResponseWithDateMeaning(`{"picks":[{"mediaType":"movie","key":"movie:tmdb:`+strconv.Itoa(robots.TMDBID)+`","name":"Pocket Robots 5"}]}`, tc.meaning),
			)
			collections, presence := atlasFinchCollection(corpus)
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

// atlasFinchCollection is TMDB's authoritative roster for the franchise. It
// includes Seekers of the Sunken Bell, whose title does not begin "Atlas Finch".
func atlasFinchCollection(corpus *catalogfixture.Corpus) (*moviecollectionsfixture.Source, *catalogfixture.Presence) {
	var members []catalog.Candidate
	presence := &catalogfixture.Presence{Hits: map[int]catalog.Presence{}}
	refs := map[int]moviecollections.CollectionRef{}
	for _, c := range corpus.Candidates {
		presence.Hits[c.TMDBID] = catalog.Presence{LibraryItemID: "item-" + strconv.Itoa(c.TMDBID)}
		if c.Name == "Seekers of the Sunken Bell" || strings.HasPrefix(c.Name, "Atlas Finch") {
			members = append(members, catalog.Candidate{MediaType: "movie", TMDBID: c.TMDBID, Name: c.Name, Year: c.Year})
			refs[c.TMDBID] = moviecollections.CollectionRef{TMDBID: 90000084, Name: "Atlas Finch Collection"}
		}
	}
	return &moviecollectionsfixture.Source{
		Refs:        refs,
		Collections: map[int]moviecollections.SourceCollection{90000084: {TMDBID: 90000084, Name: "Atlas Finch Collection", Members: members}},
	}, presence
}

// A trailing parenthetical must not fuse into the last named title (the second
// named film followed by "(v3-1" resolved to nothing on the real library, #1498).
func TestSuggest_ExampleTitleSurvivesTrailingParenthetical(t *testing.T) {
	description := "A Sunday-afternoon channel of 1980s adventure movies like Atlas Finch and The Tinkerlings (v3-1)"
	meaning := fixtureDateMeaning("movie_release", "description", 30, 35, 1980, 1989)
	corpus := namedTitlesCorpus()
	model := testkit.NewLLM(
		catalogSearchResponse(map[string]any{"query": "Pocket Robots", "media_type": "movie", "dateMeaning": meaning}),
		finalResponseWithDateMeaning(`{"picks":[{"mediaType":"movie","key":"movie:tmdb:90002016","name":"Pocket Robots 5"}]}`, meaning),
	)
	s := suggest.New(model, catalog.New(nil, corpus), referenceExistsValidator{}, 10)
	proposal, err := s.Suggest(context.Background(), suggest.Intent{Description: description})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, item := range proposal.Lineup {
		names = append(names, item.Name)
	}
	if !slices.Contains(names, "The Tinkerlings") {
		t.Fatalf("The Tinkerlings lost to the trailing parenthetical: %v", names)
	}
}

// If the model never produces usable final JSON, the named examples alone are
// returned instead of "couldn't find any titles" (the heist failure, #1498).
func TestSuggest_ExampleAnchorsSurviveUnusableFinal(t *testing.T) {
	description := "A channel of 1980s adventure movies such as The Tinkerlings"
	meaning := fixtureDateMeaning("movie_release", "description", 13, 18, 1980, 1989)
	corpus := namedTitlesCorpus()
	responses := []llm.Response{catalogSearchResponse(map[string]any{"query": "Pocket Robots", "media_type": "movie", "dateMeaning": meaning})}
	for range 6 {
		responses = append(responses, llm.Response{Content: "I could not decide."})
	}
	s := suggest.New(testkit.NewLLM(responses...), catalog.New(nil, corpus), referenceExistsValidator{}, 10)
	proposal, err := s.Suggest(context.Background(), suggest.Intent{Description: description})
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Lineup) != 1 || proposal.Lineup[0].Name != "The Tinkerlings" {
		t.Fatalf("lineup = %+v, want the named example", proposal.Lineup)
	}
}

// b7acb924 on the real library: 24 titles surfaced, the model's final had no
// picks, and the request failed with selection_empty. A resolved example
// anchor is still a pick, so the request must succeed with it.
func TestSuggest_ExampleAnchorSurvivesEmptyPicks(t *testing.T) {
	description := "A channel of 1980s adventure movies such as The Tinkerlings"
	meaning := fixtureDateMeaning("movie_release", "description", 13, 18, 1980, 1989)
	model := testkit.NewLLM(
		catalogSearchResponse(map[string]any{"query": "Pocket Robots", "media_type": "movie", "dateMeaning": meaning}),
		finalResponseWithDateMeaning(`{"picks":[]}`, meaning),
	)
	s := suggest.New(model, catalog.New(nil, namedTitlesCorpus()), referenceExistsValidator{}, 10)
	proposal, err := s.Suggest(context.Background(), suggest.Intent{Description: description})
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Lineup) != 1 || proposal.Lineup[0].Name != "The Tinkerlings" {
		t.Fatalf("lineup = %+v, want the named example", proposal.Lineup)
	}
}
