package suggest_test

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/testkit/catalogfixture"
)

// exampleFillCorpus is the real library's shape for #1499: the two named
// titles, on-era family/comedy neighbours, and off-era titles that a title-word
// search for "back" drags in (a 1993 sequel and a 1942 comedy were observed
// filling next to the 1980s anchors).
func exampleFillCorpus() *catalogfixture.Corpus {
	return exampleFillCorpusOf([]titleRow{
		{"Back to the Lighthouse", 1985, []string{"Adventure", "Comedy", "Science Fiction"}},
		{"Gloomwings", 1984, []string{"Comedy", "Horror", "Fantasy"}},
		{"Fogcatchers", 1984, []string{"Comedy", "Fantasy"}},
		{"Darling, I Folded the Twins", 1989, []string{"Comedy", "Family", "Science Fiction"}},
		{"Mothwhistle", 1988, []string{"Comedy", "Fantasy"}},
		{"Gusto", 1988, []string{"Comedy", "Fantasy", "Family"}},
		{"Pennywhistle's Day Off", 1986, []string{"Comedy"}},
		{"Arriving in Varnholm", 1988, []string{"Comedy", "Romance"}},
		{"Tin Sparks", 1986, []string{"Comedy", "Science Fiction", "Family"}},
		{"Teen Owl", 1985, []string{"Comedy", "Fantasy"}},
		{"Weekend at Wexcombe's", 1989, []string{"Comedy"}},
		{"Choir Camp 2: Back in the Loft", 1993, []string{"Comedy", "Family", "Music"}},
		{"To Sing or Not to Sing", 1942, []string{"Comedy", "War"}},
		{"Sweetheart Come Back", 1961, []string{"Comedy", "Romance"}},
		{"Wren Halloway and the Salt Lantern", 2004, []string{"Adventure", "Fantasy", "Family"}},
		{"Our Song.", 2016, []string{"Animation", "Romance", "Fantasy"}},
	})
}

// exampleFillCorpusOf builds the #1499 library from rows, in order, with ids from
// 90003000. Search is a title-word search; discovery honours genre and year filters.
func exampleFillCorpusOf(rows []titleRow) *catalogfixture.Corpus {
	cands := make([]catalog.Candidate, 0, len(rows))
	for i, r := range rows {
		cands = append(cands, catalog.Candidate{
			MediaType: "movie", TMDBID: 90_003_000 + i, Name: r.name, Year: r.year, InLibrary: true, Genres: r.genres,
			VoteCount: 5000 - i*100, Overview: "A film in the library.",
		})
	}
	corpus := &catalogfixture.Corpus{Candidates: cands}
	corpus.SearchFunc = func(_ context.Context, query string, limit int) ([]catalog.Candidate, error) {
		norm := func(s string) string {
			return strings.ToLower(strings.NewReplacer(":", " ", "-", " ", "&", " ", ",", " ").Replace(s))
		}
		var out []catalog.Candidate
	next:
		for _, c := range cands {
			for _, w := range strings.Fields(norm(query)) {
				if !strings.Contains(norm(c.Name), w) {
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
	// Discovery honours the structured filters the way TMDB does: genres AND
	// together and the year range bounds the release.
	corpus.DiscoverFunc = func(_ context.Context, q catalog.DiscoveryQuery, limit int) ([]catalog.Candidate, error) {
		var out []catalog.Candidate
	next:
		for _, c := range cands {
			if q.YearFrom > 0 && c.Year < q.YearFrom || q.YearTo > 0 && c.Year > q.YearTo {
				continue
			}
			for _, g := range q.Genres {
				if !slices.Contains(c.Genres, g) {
					continue next
				}
			}
			out = append(out, c)
		}
		return out, nil
	}
	return corpus
}

func exampleFillNames(p suggest.Proposal) []string {
	var names []string
	for _, item := range append(append([]suggest.ProposalItem(nil), p.Lineup...), p.Acquisitions...) {
		names = append(names, item.Name)
	}
	return names
}

func pickJSON(corpus *catalogfixture.Corpus, names ...string) string {
	var picks []string
	for _, name := range names {
		for _, c := range corpus.Candidates {
			if c.Name == name {
				picks = append(picks, `{"mediaType":"movie","key":"movie:tmdb:`+strconv.Itoa(c.TMDBID)+`","name":`+strconv.Quote(name)+`}`)
			}
		}
	}
	return `{"picks":[` + strings.Join(picks, ",") + `]}`
}

// #1499: the request names two 1980s titles and no era. The model's title-word
// search only reaches noise (Choir Camp 2, 1993), so the anchors' era and genre
// must feed a discovery whose on-era neighbours reach the model as ordinary tool
// evidence. The model still chooses: nothing is dropped or padded server-side.
func TestSuggest_ExampleAnchorsSurfaceOnEraNeighboursToModel(t *testing.T) {
	corpus := exampleFillCorpus()
	none := dateMeaningNone()
	model := testkit.NewLLM(
		catalogSearchResponse(map[string]any{"query": "back", "media_type": "movie", "dateMeaning": none}),
		finalResponseWithDateMeaning(pickJSON(corpus, "Back to the Lighthouse", "Gloomwings", "Fogcatchers", "Darling, I Folded the Twins"), none),
	)
	s := suggest.New(model, catalog.New(nil, corpus), referenceExistsValidator{}, 10)
	proposal, err := s.Suggest(context.Background(), suggest.Intent{Description: "Movies like Back to the Lighthouse and Gloomwings for a family night"})
	if err != nil {
		t.Fatal(err)
	}
	prompt := model.Prompt()
	for _, neighbour := range []string{"Fogcatchers", "Darling, I Folded the Twins", "Mothwhistle", "Gusto"} {
		if !strings.Contains(prompt, neighbour) {
			t.Errorf("on-era neighbour %q never reached the model as tool evidence", neighbour)
		}
	}
	for _, offEra := range []string{"To Sing or Not to Sing", "Wren Halloway", "Our Song."} {
		if strings.Contains(prompt, offEra) {
			t.Errorf("off-era %q reached the model", offEra)
		}
	}
	if names := exampleFillNames(proposal); len(names) != 4 {
		t.Errorf("the model's 4 picks were changed server-side: %v", names)
	}
}

// Control: anchors from different eras carry no clear era, so nothing is
// derived from them and no extra discovery runs.
func TestSuggest_ExampleAnchorsWithoutClearEraAreNotFilled(t *testing.T) {
	corpus := exampleFillCorpus()
	corpus.Candidates = append(corpus.Candidates,
		catalog.Candidate{MediaType: "movie", TMDBID: 90003900, Name: "Tessaline", Year: 1942, InLibrary: true, Genres: []string{"Drama", "Romance"}, VoteCount: 900},
		catalog.Candidate{MediaType: "movie", TMDBID: 90003901, Name: "Aetherion", Year: 2009, InLibrary: true, Genres: []string{"Science Fiction", "Adventure"}, VoteCount: 900},
	)
	search := corpus.SearchFunc
	corpus.SearchFunc = func(ctx context.Context, q string, limit int) ([]catalog.Candidate, error) {
		var out []catalog.Candidate
		for _, c := range corpus.Candidates {
			if strings.EqualFold(c.Name, q) {
				out = append(out, c)
			}
		}
		if len(out) == 0 {
			return search(ctx, q, limit)
		}
		return out, nil
	}
	none := dateMeaningNone()
	model := testkit.NewLLM(
		catalogSearchResponse(map[string]any{"query": "Tessaline", "media_type": "movie", "dateMeaning": none}),
		finalResponseWithDateMeaning(pickJSON(corpus, "Tessaline", "Aetherion"), none),
	)
	s := suggest.New(model, catalog.New(nil, corpus), referenceExistsValidator{}, 10)
	proposal, err := s.Suggest(context.Background(), suggest.Intent{Description: "Movies like Tessaline and Aetherion"})
	if err != nil {
		t.Fatal(err)
	}
	if names := exampleFillNames(proposal); len(names) != 2 {
		t.Errorf("anchors across eras were padded: %v", names)
	}
	if n := len(corpus.Discoveries()); n != 0 {
		t.Errorf("ran %d discovery queries for anchors with no clear era", n)
	}
}

// #1499 root cause on the real library: after a one-title search the model's
// tool-free final turn tried to search the second named title as raw text. The
// parser lifted that call's argument object, the accepted dateMeaning was added
// to it, and it read as a valid final with no picks, so the lineup stopped at
// the anchors without ever reaching repair. Text with no picks field is
// malformed and must be repaired.
func TestSuggest_LeakedToolCallFinalIsRepairedNotAnEmptyProposal(t *testing.T) {
	corpus := exampleFillCorpus()
	none := dateMeaningNone()
	leaked := "<tool_call>\n<function=catalog_search>\n<parameter=input>\n" +
		`{"dateMeaning": {"anchors": [], "axes": [], "kind": "none"}, "media_type": "movie", "query": "Gloomwings"}` +
		"\n</parameter>\n</function>\n</tool_call>"
	model := testkit.NewLLM(
		catalogSearchResponse(map[string]any{"query": "Back to the Lighthouse", "media_type": "movie", "dateMeaning": none}),
		llm.Response{Content: leaked},
		finalResponseWithDateMeaning(pickJSON(corpus, "Back to the Lighthouse", "Gloomwings", "Fogcatchers", "Darling, I Folded the Twins"), none),
	)
	s := suggest.New(model, catalog.New(nil, corpus), referenceExistsValidator{}, 10)
	proposal, err := s.Suggest(context.Background(), suggest.Intent{Description: "Movies like Back to the Lighthouse and Gloomwings for a family night"})
	if err != nil {
		t.Fatal(err)
	}
	if model.Calls != 3 {
		t.Errorf("model calls = %d, want 3: the leaked tool call must be sent to repair", model.Calls)
	}
	if names := exampleFillNames(proposal); !slices.Contains(names, "Fogcatchers") {
		t.Errorf("repaired lineup lost the model's on-era picks: %v", names)
	}
}

// The counterpart: a final that HAS a picks field, even an empty array, is the
// model's answer (nothing matched) and is not repaired. Presence of the field is
// what separates it from a leaked tool call, which carries none.
func TestSuggest_EmptyPicksArrayIsNotRepaired(t *testing.T) {
	corpus := exampleFillCorpus()
	none := dateMeaningNone()
	model := testkit.NewLLM(
		catalogSearchResponse(map[string]any{"query": "Back to the Lighthouse", "media_type": "movie", "dateMeaning": none}),
		finalResponseWithDateMeaning(`{"picks":[]}`, none),
	)
	s := suggest.New(model, catalog.New(nil, corpus), referenceExistsValidator{}, 10)
	proposal, err := s.Suggest(context.Background(), suggest.Intent{Description: "Movies like Back to the Lighthouse and Gloomwings for a family night"})
	if err != nil {
		t.Fatal(err)
	}
	if model.Calls != 2 {
		t.Errorf("model calls = %d, want 2: an explicit empty picks array is a real answer", model.Calls)
	}
	if names := exampleFillNames(proposal); len(names) != 2 {
		t.Errorf("empty picks should leave exactly the anchors: %v", names)
	}
}

// #1499 cause 1: "in the spirit of" (and its siblings; not "think X", see reference.go) offer example titles
// exactly like "like" does, so the anchors are required. The model here drops
// Gloomwings; the required-title check must bring it back for every phrasing.
func TestSuggest_ExampleCuePhrasingsRequireTheAnchors(t *testing.T) {
	for _, phrase := range []string{
		"in the spirit of", "inspired by", "a la", "à la",
	} {
		t.Run(phrase, func(t *testing.T) {
			corpus := exampleFillCorpus()
			none := dateMeaningNone()
			model := testkit.NewLLM(
				catalogSearchResponse(map[string]any{"query": "Back to the Lighthouse", "media_type": "movie", "dateMeaning": none}),
				finalResponseWithDateMeaning(pickJSON(corpus, "Back to the Lighthouse", "Fogcatchers", "Gusto"), none),
				finalResponseWithDateMeaning(pickJSON(corpus, "Back to the Lighthouse", "Gloomwings", "Fogcatchers", "Gusto"), none),
			)
			s := suggest.New(model, catalog.New(nil, corpus), referenceExistsValidator{}, 10)
			proposal, err := s.Suggest(context.Background(), suggest.Intent{Description: "Movies " + phrase + " Back to the Lighthouse and Gloomwings for a family night"})
			if err != nil {
				t.Fatal(err)
			}
			if names := exampleFillNames(proposal); !slices.Contains(names, "Gloomwings") || !slices.Contains(names, "Back to the Lighthouse") {
				t.Errorf("%q did not require its anchors: %v", phrase, names)
			}
		})
	}
}

// #1499 cause 2: when the model spends its tool call on mode=collection for the
// named titles, the era neighbours must still reach it as evidence.
func TestSuggest_CollectionCallForExampleTitlesSurfacesNeighbours(t *testing.T) {
	corpus := exampleFillCorpus()
	none := dateMeaningNone()
	model := testkit.NewLLM(
		catalogSearchResponse(map[string]any{"mode": "collection", "media_type": "movie", "titles": []any{"Back to the Lighthouse", "Gloomwings"}, "dateMeaning": none}),
		finalResponseWithDateMeaning(pickJSON(corpus, "Back to the Lighthouse", "Gloomwings", "Fogcatchers", "Gusto"), none),
	)
	s := suggest.New(model, catalog.New(nil, corpus), referenceExistsValidator{}, 10)
	proposal, err := s.Suggest(context.Background(), suggest.Intent{Description: "Movies like Back to the Lighthouse and Gloomwings for a family night"})
	if err != nil {
		t.Fatal(err)
	}
	prompt := model.Prompt()
	for _, neighbour := range []string{"Fogcatchers", "Darling, I Folded the Twins", "Gusto"} {
		if !strings.Contains(prompt, neighbour) {
			t.Errorf("collection result never surfaced on-era neighbour %q", neighbour)
		}
	}
	if strings.Contains(prompt, "To Sing or Not to Sing") {
		t.Error("off-era title reached the model")
	}
	if names := exampleFillNames(proposal); len(names) != 4 {
		t.Errorf("model's picks were changed server-side: %v", names)
	}
}

// Control: a collection intent (a named franchise, not examples) gets no era
// neighbours; membership evidence stays exact.
func TestSuggest_CollectionCallForFranchiseGetsNoNeighbours(t *testing.T) {
	corpus := exampleFillCorpus()
	none := dateMeaningNone()
	model := testkit.NewLLM(
		catalogSearchResponse(map[string]any{"mode": "collection", "media_type": "movie", "titles": []any{"Back to the Lighthouse", "Gloomwings"}, "dateMeaning": none}),
		finalResponseWithDateMeaning(pickJSON(corpus, "Back to the Lighthouse", "Gloomwings"), none),
	)
	s := suggest.New(model, catalog.New(nil, corpus), referenceExistsValidator{}, 10)
	if _, err := s.Suggest(context.Background(), suggest.Intent{Description: "A channel of the Back to the Lighthouse and Gloomwings movies"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(model.Prompt(), "Fogcatchers") {
		t.Error("a non-example request got era neighbours")
	}
}

// #1499 cause 3: a catalog_search carrying titles but no mode used to be
// rejected on every round until the rounds ran out (hard failure). It is read as
// a collection lookup, so the model gets its titles back and can finish.
func TestSuggest_TitlesWithoutModeAreACollectionLookupNotADeadEnd(t *testing.T) {
	corpus := exampleFillCorpus()
	none := dateMeaningNone()
	call := catalogSearchResponse(map[string]any{"media_type": "movie", "titles": []any{"Back to the Lighthouse", "Gloomwings"}, "dateMeaning": none})
	model := testkit.NewLLM(
		call, call, call, call, call, call, call,
		finalResponseWithDateMeaning(pickJSON(corpus, "Back to the Lighthouse", "Gloomwings", "Fogcatchers"), none),
	)
	s := suggest.New(model, catalog.New(nil, corpus), referenceExistsValidator{}, 10)
	proposal, err := s.Suggest(context.Background(), suggest.Intent{Description: "Movies like Back to the Lighthouse and Gloomwings for a family night"})
	if err != nil {
		t.Fatalf("titles without mode exhausted the rounds: %v", err)
	}
	if !strings.Contains(model.Prompt(), "Fogcatchers") {
		t.Error("the titles call produced no evidence for the model")
	}
	if names := exampleFillNames(proposal); !slices.Contains(names, "Gloomwings") {
		t.Errorf("lineup = %v", names)
	}
}
