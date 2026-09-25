package suggest_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/reference"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/testkit/catalogfixture"
)

type namedSetToolAvailabilityLLM struct {
	calls int
}

type referenceExistsValidator struct{}

func (referenceExistsValidator) Exists(context.Context, provision.MediaType, int) (bool, error) {
	return true, nil
}

func (m *namedSetToolAvailabilityLLM) Name() string { return "named-set-tool-availability" }

func (m *namedSetToolAvailabilityLLM) Chat(_ context.Context, _ []llm.Message, opts llm.ChatOptions) (llm.Response, error) {
	m.calls++
	if canCallTools(opts) {
		return testkit.ToolCallResponse("catalog_search", map[string]any{
			"mode": "collection", "media_type": "movie", "titles": []any{"Model Guess"},
			"dateMeaning": dateMeaningNone(),
		}), nil
	}
	return testkit.FinalResponse(`{"channelName":"Friday Night","rationale":"A verified named block.","dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[{"mediaType":"movie","key":"movie:tmdb:603","name":"The Matrix","rationale":"A verified constituent.","confidence":0.95}],"policy":{}}`), nil
}

func TestSuggestDiscoversNamedBlockSourceOnce(t *testing.T) {
	for _, request := range []string{"TGIF", "TGIF block", "Make a channel like TGIF", "a channel based on TGIF", "Recreate ABC's TGIF Friday-night block."} {
		t.Run(request, func(t *testing.T) {
			meaning := dateMeaningNone()
			corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
			references := &testkit.ReferenceResolver{DiscoveryEvidence: reference.Evidence{
				URL: "https://lineups.example/frozen-block", Title: "Synthetic block roster", Excerpt: "The Matrix", TitleAnchors: []string{"The Matrix"},
			}}
			model := testkit.NewLLM(testkit.FinalResponse(finalWithDateMeaning(t, meaning)), testkit.FinalResponse(finalWithDateMeaning(t, meaning)))
			proposal, err := dateExecutionSuggester(model, corpus).WithReferences(references).Suggest(context.Background(), suggest.Intent{Description: request})
			if err != nil || len(proposal.Lineup) != 1 || proposal.Lineup[0].TMDBID != matrixCandidate().TMDBID {
				t.Fatalf("proposal=%+v err=%v", proposal, err)
			}
			if got := references.Discoveries(); len(got) != 1 || got[0] != "TGIF" || len(references.Calls()) != 0 {
				t.Fatalf("discovery=%v pasted=%v", got, references.Calls())
			}
			if model.Calls != 2 || !strings.Contains(model.LastMessages[1].Content, "UNTRUSTED REFERENCE DATA") {
				t.Fatalf("calls=%d; discovered source was not supplied to finalization", model.Calls)
			}
		})
	}
}

func TestSuggestDiscoversAcronymSourceWithoutPhaseQualifier(t *testing.T) {
	for _, request := range []string{"MCU Phase One.", "Recreate the MCU Phase One lineup.", "The MCU Phase One lineup of films, please."} {
		t.Run(request, func(t *testing.T) {
			meaning := dateMeaningNone()
			corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
			references := &testkit.ReferenceResolver{ByLabel: map[string]reference.Evidence{
				"MCU": {
					URL: "https://lineups.example/mcu", Title: "Synthetic MCU roster",
					Excerpt: "The Matrix", TitleAnchors: []string{"The Matrix"},
				},
			}}
			model := testkit.NewLLM(
				testkit.FinalResponse(finalWithDateMeaning(t, meaning)),
				testkit.FinalResponse(finalWithDateMeaning(t, meaning)),
			)

			proposal, err := dateExecutionSuggester(model, corpus).WithReferences(references).
				Suggest(context.Background(), suggest.Intent{Description: request})
			if got := references.Discoveries(); len(got) != 1 || got[0] != "MCU" {
				t.Fatalf("discovery=%v, want the stable MCU source label", got)
			}
			if err != nil || len(proposal.Lineup) != 1 || proposal.Lineup[0].TMDBID != matrixCandidate().TMDBID {
				t.Fatalf("proposal=%+v err=%v", proposal, err)
			}
		})
	}
}

func TestSuggestNamedBlockStopsRetrievalAfterReferenceGrounding(t *testing.T) {
	corpus := &catalogfixture.Corpus{
		SearchFunc: func(_ context.Context, query string, _ int) ([]catalog.Candidate, error) {
			if query == "The Matrix" {
				return []catalog.Candidate{matrixCandidate()}, nil
			}
			return nil, nil
		},
	}
	references := &testkit.ReferenceResolver{DiscoveryEvidence: reference.Evidence{
		URL: "https://lineups.example/frozen-block", Title: "Synthetic block roster", Excerpt: "The Matrix", TitleAnchors: []string{"The Matrix"},
	}}
	model := &namedSetToolAvailabilityLLM{}

	proposal, err := suggest.New(model, catalog.New(nil, corpus), nil, 10).WithReferences(references).Suggest(context.Background(), suggest.Intent{Description: "TGIF"})
	if err != nil {
		t.Fatalf("suggestion failed after %d model calls and %d catalog searches: %v", model.calls, len(corpus.Searches()), err)
	}
	if len(proposal.Lineup) != 1 || proposal.Lineup[0].TMDBID != matrixCandidate().TMDBID {
		t.Fatalf("proposal=%+v, want the source-grounded constituent", proposal)
	}
	if model.calls != 2 {
		t.Fatalf("model calls=%d, want collection hypothesis followed by tool-free finalization", model.calls)
	}
	if got := references.Discoveries(); len(got) != 1 || got[0] != "TGIF" {
		t.Fatalf("discovery=%v, want one TGIF source resolution", got)
	}
}

func TestSuggestNamedBlockContinuesPastUnresolvedSourceAnchors(t *testing.T) {
	fullHouse := catalog.Candidate{MediaType: "series", TMDBID: 1001, Name: "Full House", Year: 1987, InLibrary: true}
	familyMatters := catalog.Candidate{MediaType: "series", TMDBID: 1002, Name: "Family Matters", Year: 1989, InLibrary: true}
	corpus := &catalogfixture.Corpus{
		SearchFunc: func(_ context.Context, query string, _ int) ([]catalog.Candidate, error) {
			switch query {
			case fullHouse.Name:
				return []catalog.Candidate{fullHouse}, nil
			case familyMatters.Name:
				return []catalog.Candidate{familyMatters}, nil
			default:
				return nil, nil
			}
		},
	}
	references := &testkit.ReferenceResolver{DiscoveryEvidence: reference.Evidence{
		URL: "https://lineups.example/tgif", Title: "TGIF", Excerpt: "A verified programming block.",
		TitleAnchors: []string{
			"Unavailable One", "Unavailable Two", "Unavailable Three", "Unavailable Four",
			"Unavailable Five", "Unavailable Six", "Unavailable Seven", "Unavailable Eight",
			fullHouse.Name, familyMatters.Name,
		},
	}}
	model := testkit.NewLLM(
		testkit.FinalResponse(`{"channelName":"Friday Night","rationale":"Interpreting the named block.","dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[],"policy":{}}`),
		testkit.FinalResponse(`{"channelName":"Friday Night","rationale":"A verified named block.","dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[{"mediaType":"series","key":"series:tmdb:1001","name":"Full House","rationale":"A defining constituent.","confidence":0.98},{"mediaType":"series","key":"series:tmdb:1002","name":"Family Matters","rationale":"A defining constituent.","confidence":0.98}],"policy":{}}`),
	)

	proposal, err := dateExecutionSuggester(model, corpus).WithReferences(references).Suggest(context.Background(), suggest.Intent{Description: "TGIF"})
	if err != nil {
		t.Fatalf("suggestion failed after %d source lookups: %v", len(corpus.Searches()), err)
	}
	if len(proposal.Lineup) != 2 {
		t.Fatalf("lineup=%+v, want later usable source members after early misses", proposal.Lineup)
	}
	if got := len(corpus.Searches()); got != 10 {
		t.Fatalf("source lookups=%d, want the bounded roster scanned through both usable members", got)
	}
}

func TestSuggestNamedBlockSurfacesGroundedMembersBeyondModelShortlist(t *testing.T) {
	titles := []catalog.Candidate{
		{MediaType: provision.Series, TMDBID: 1001, Name: "Full House", InLibrary: true},
		{MediaType: provision.Series, TMDBID: 1007, Name: "Sabrina the Teenage Witch", InLibrary: true},
	}
	corpus := &catalogfixture.Corpus{SearchFunc: func(_ context.Context, query string, _ int) ([]catalog.Candidate, error) {
		for _, candidate := range titles {
			if query == candidate.Name {
				return []catalog.Candidate{candidate}, nil
			}
		}
		return nil, nil
	}}
	references := &testkit.ReferenceResolver{DiscoveryEvidence: reference.Evidence{
		URL: "https://lineups.example/tgif", Title: "TGIF", Excerpt: "A verified programming block.",
		TitleAnchors: []string{"Full House", "Sabrina the Teenage Witch"},
	}}
	model := testkit.NewLLM(
		testkit.FinalResponse(`{"channelName":"TGIF","dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[],"policy":{}}`),
		testkit.FinalResponse(`{"channelName":"TGIF","dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[
			{"mediaType":"series","key":"series:tmdb:1001","name":"Full House"}
		],"policy":{}}`),
	)

	proposal, err := suggest.New(model, catalog.New(nil, corpus), referenceExistsValidator{}, 10).
		WithReferences(references).
		Suggest(context.Background(), suggest.Intent{Description: "TGIF"})
	if err != nil {
		t.Fatal(err)
	}
	items := append(append([]suggest.ProposalItem(nil), proposal.Lineup...), proposal.Acquisitions...)
	for _, item := range items {
		if item.Name == "Sabrina the Teenage Witch" {
			return
		}
	}
	t.Fatalf("selectable TGIF items = %+v, want grounded Sabrina beyond the model shortlist", items)
}

func TestSuggestNamedBlockPreservesDistinctRequestedTitlesAndRejectsNamesakes(t *testing.T) {
	fullHouse := catalog.Candidate{MediaType: provision.Series, TMDBID: 1001, Name: "Full House", Year: 1987, InLibrary: true}
	familyMatters := catalog.Candidate{MediaType: provision.Series, TMDBID: 1002, Name: "Family Matters", Year: 1989, InLibrary: true}
	wrongNamesake := catalog.Candidate{MediaType: provision.Series, TMDBID: 2002, Name: "Family Matters", Year: 2024}
	stepByStep := catalog.Candidate{MediaType: provision.Series, TMDBID: 1003, Name: "Step by Step", Year: 1991, InLibrary: true}
	corpus := &catalogfixture.Corpus{
		SearchFunc: func(_ context.Context, query string, _ int) ([]catalog.Candidate, error) {
			switch query {
			case fullHouse.Name:
				return []catalog.Candidate{fullHouse}, nil
			case familyMatters.Name:
				return []catalog.Candidate{familyMatters, wrongNamesake}, nil
			case stepByStep.Name:
				return []catalog.Candidate{stepByStep}, nil
			default:
				return nil, nil
			}
		},
	}
	references := &testkit.ReferenceResolver{DiscoveryEvidence: reference.Evidence{
		URL: "https://lineups.example/tgif", Title: "TGIF", Excerpt: "A verified programming block.",
		TitleAnchors: []string{fullHouse.Name, familyMatters.Name, stepByStep.Name},
	}}
	model := testkit.NewLLM(
		testkit.FinalResponse(`{"channelName":"TGIF","dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[],"policy":{}}`),
		testkit.FinalResponse(`{"channelName":"TGIF","dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[
			{"mediaType":"series","key":"series:tmdb:1001","name":"Full House"},
			{"mediaType":"series","key":"series:tmdb:1001","name":"Full House"},
			{"mediaType":"series","key":"series:tmdb:1001","name":"Full House"},
			{"mediaType":"series","key":"series:tmdb:2002","name":"Family Matters"}
		],"policy":{}}`),
	)

	proposal, err := suggest.New(model, catalog.New(nil, corpus), referenceExistsValidator{}, 10).
		WithReferences(references).
		Suggest(context.Background(), suggest.Intent{Description: "TGIF — include Full House, Family Matters, and Step by Step."})
	if err != nil {
		t.Fatal(err)
	}
	items := append(append([]suggest.ProposalItem(nil), proposal.Lineup...), proposal.Acquisitions...)
	if len(items) != 3 {
		t.Fatalf("items=%+v searches=%+v trace=%+v, want the three distinct requested titles", items, corpus.Searches(), proposal.Trace)
	}
	want := map[int]bool{1001: true, 1002: true, 1003: true}
	for _, item := range items {
		if !want[item.TMDBID] {
			t.Fatalf("unexpected or wrong namesake survived: %+v", items)
		}
		delete(want, item.TMDBID)
	}
	if len(want) != 0 {
		t.Fatalf("missing requested title ids %v from %+v", want, items)
	}
	if err := suggest.ValidateDecisionTrace(proposal.Trace); err != nil {
		t.Fatalf("preserved required-title trace is not persistable: %v; trace=%+v", err, proposal.Trace)
	}
}

func TestSuggestNamedSetExclusionDoesNotRemoveShorterRelatedTitle(t *testing.T) {
	ironMan := catalog.Candidate{MediaType: provision.Movie, TMDBID: 1726, Name: "Iron Man", Year: 2008, InLibrary: true}
	ironMan2 := catalog.Candidate{MediaType: provision.Movie, TMDBID: 10138, Name: "Iron Man 2", Year: 2010}
	corpus := &catalogfixture.Corpus{SearchFunc: func(_ context.Context, query string, _ int) ([]catalog.Candidate, error) {
		switch query {
		case ironMan.Name:
			return []catalog.Candidate{ironMan}, nil
		case ironMan2.Name:
			return []catalog.Candidate{ironMan2}, nil
		default:
			return nil, nil
		}
	}}
	references := &testkit.ReferenceResolver{ByLabel: map[string]reference.Evidence{
		"MCU": {URL: "https://lineups.example/mcu", Title: "MCU", Excerpt: "Two reviewed members", TitleAnchors: []string{ironMan.Name, ironMan2.Name}},
	}}
	meaning := dateMeaningNone()
	model := testkit.NewLLM(
		testkit.FinalResponse(finalWithDateMeaning(t, meaning)),
		testkit.FinalResponse(`{"picks":[{"mediaType":"movie","key":"movie:tmdb:1726"}],"dateMeaning":{"kind":"none","anchors":[],"axes":[]}}`),
	)

	proposal, err := dateExecutionSuggester(model, corpus).WithReferences(references).Suggest(context.Background(), suggest.Intent{
		Description: "MCU Phase One lineup, but leave out Iron Man 2.",
		MustExclude: []string{"Iron Man 2"},
	})
	if err != nil || len(proposal.Lineup) != 1 || proposal.Lineup[0].TMDBID != ironMan.TMDBID || len(proposal.Acquisitions) != 0 {
		t.Fatalf("related-title exclusion removed the wrong member: proposal=%+v err=%v", proposal, err)
	}
}

func TestSuggestNamedSourceCompletionRespectsKnownMovieReleaseRange(t *testing.T) {
	oldMovie := catalog.Candidate{MediaType: provision.Movie, TMDBID: 1726, Name: "Iron Man", Year: 2008, InLibrary: true}
	inRange := catalog.Candidate{MediaType: provision.Movie, TMDBID: 10138, Name: "Iron Man 2", Year: 2010, InLibrary: true}
	corpus := &catalogfixture.Corpus{SearchFunc: func(_ context.Context, query string, _ int) ([]catalog.Candidate, error) {
		switch query {
		case oldMovie.Name:
			return []catalog.Candidate{oldMovie}, nil
		case inRange.Name:
			return []catalog.Candidate{inRange}, nil
		default:
			return nil, nil
		}
	}}
	references := &testkit.ReferenceResolver{ByLabel: map[string]reference.Evidence{
		"MCU": {URL: "https://lineups.example/mcu", Title: "MCU", Excerpt: "Two reviewed members", TitleAnchors: []string{oldMovie.Name, inRange.Name}},
	}}
	description := "MCU Phase One lineup, only movies released from 2010 through 2012."
	marker := "2010 through 2012"
	start := strings.Index(description, marker)
	meaning := fixtureDateMeaning("movie_release", "description", start, start+len(marker), 2010, 2012)
	final, err := json.Marshal(map[string]any{
		"picks":       []any{map[string]any{"mediaType": "movie", "key": "movie:tmdb:10138"}},
		"dateMeaning": meaning,
	})
	if err != nil {
		t.Fatal(err)
	}
	model := testkit.NewLLM(
		testkit.FinalResponse(finalWithDateMeaning(t, meaning)),
		testkit.FinalResponse(string(final)),
	)

	proposal, err := dateExecutionSuggester(model, corpus).WithReferences(references).
		Suggest(context.Background(), suggest.Intent{Description: description})
	if err != nil || len(proposal.Lineup) != 1 || proposal.Lineup[0].TMDBID != inRange.TMDBID || len(proposal.Acquisitions) != 0 {
		t.Fatalf("source completion restored an out-of-range film: proposal=%+v err=%v", proposal, err)
	}
}

func TestSuggestNamedBlockFallsBackToGroundedRequiredTitles(t *testing.T) {
	titles := []catalog.Candidate{
		{MediaType: provision.Series, TMDBID: 1001, Name: "Full House", InLibrary: true},
		{MediaType: provision.Series, TMDBID: 1002, Name: "Family Matters", InLibrary: true},
		{MediaType: provision.Series, TMDBID: 1003, Name: "Step by Step", InLibrary: true},
	}
	corpus := &catalogfixture.Corpus{SearchFunc: func(_ context.Context, query string, _ int) ([]catalog.Candidate, error) {
		for _, candidate := range titles {
			if query == candidate.Name {
				return []catalog.Candidate{candidate}, nil
			}
		}
		return nil, nil
	}}
	references := &testkit.ReferenceResolver{DiscoveryEvidence: reference.Evidence{
		URL: "https://lineups.example/tgif", Title: "TGIF", Excerpt: "A verified programming block.",
		TitleAnchors: []string{"Full House", "Family Matters", "Step by Step"},
	}}
	unsolicited := testkit.ToolCallResponse("catalog_search", map[string]any{
		"query": "Full House", "dateMeaning": dateMeaningNone(),
	})
	model := testkit.NewLLM(
		testkit.FinalResponse(`{"channelName":"TGIF","dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[],"policy":{}}`),
		unsolicited, unsolicited, unsolicited,
	)
	proposal, err := suggest.New(model, catalog.New(nil, corpus), referenceExistsValidator{}, 10).
		WithReferences(references).
		Suggest(context.Background(), suggest.Intent{Description: "TGIF — include Full House, Family Matters, and Step by Step."})
	if err != nil {
		t.Fatal(err)
	}
	items := append(append([]suggest.ProposalItem(nil), proposal.Lineup...), proposal.Acquisitions...)
	if len(items) != 3 {
		t.Fatalf("fallback items = %+v, want every independently grounded required title", items)
	}
	if err := suggest.ValidateDecisionTrace(proposal.Trace); err != nil {
		t.Fatalf("fallback trace is not persistable: %v; trace=%+v", err, proposal.Trace)
	}
}

func TestSuggestNamedBlockPreservesSourceMediaType(t *testing.T) {
	movie := catalog.Candidate{MediaType: "movie", TMDBID: 1001, Name: "Clueless", Year: 1995, InLibrary: true}
	series := catalog.Candidate{MediaType: "series", TMDBID: 1002, Name: "Clueless", Year: 1996}
	corpus := &catalogfixture.Corpus{
		SearchFunc: func(_ context.Context, query string, _ int) ([]catalog.Candidate, error) {
			if query == "Clueless" || query == "Clueless (TV series)" {
				return []catalog.Candidate{movie, series}, nil
			}
			return nil, nil
		},
	}
	references := &testkit.ReferenceResolver{DiscoveryEvidence: reference.Evidence{
		URL: "https://lineups.example/tgif", Title: "TGIF", Excerpt: "A verified programming block.",
		TitleAnchors: []string{"Clueless (TV series)"},
	}}
	model := testkit.NewLLM(
		testkit.FinalResponse(`{"channelName":"TGIF","rationale":"Interpreting the named block.","dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[],"policy":{}}`),
		testkit.FinalResponse(`{"channelName":"TGIF","rationale":"A verified named block.","dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[{"mediaType":"series","key":"series:tmdb:1002","name":"Clueless","rationale":"The source identifies the television series.","confidence":0.98}],"policy":{}}`),
	)

	proposal, err := suggest.New(model, catalog.New(nil, corpus), referenceExistsValidator{}, 10).
		WithReferences(references).
		Suggest(context.Background(), suggest.Intent{Description: "TGIF"})
	if err != nil {
		t.Fatalf("suggestion failed: %v", err)
	}
	if len(proposal.Lineup) != 0 || len(proposal.Acquisitions) != 1 || proposal.Acquisitions[0].MediaType != "series" {
		t.Fatalf("proposal=%+v, want the source-typed series rather than the owned namesake movie", proposal)
	}
}

func TestSuggestNamedBlockUsesSourceYearToResolveSeriesNamesake(t *testing.T) {
	animated := catalog.Candidate{MediaType: provision.Series, TMDBID: 1001, Name: "Sabrina the Teenage Witch", Year: 1970}
	liveAction := catalog.Candidate{MediaType: provision.Series, TMDBID: 1002, Name: "Sabrina the Teenage Witch", Year: 1996}
	corpus := &catalogfixture.Corpus{SearchFunc: func(_ context.Context, query string, _ int) ([]catalog.Candidate, error) {
		if query == "Sabrina the Teenage Witch" {
			return []catalog.Candidate{animated, liveAction}, nil
		}
		return nil, nil
	}}
	references := &testkit.ReferenceResolver{DiscoveryEvidence: reference.Evidence{
		URL: "https://lineups.example/tgif", Title: "TGIF", Excerpt: "A verified programming block.",
		TitleAnchors: []string{"Sabrina the Teenage Witch (1996 TV series)"},
	}}
	model := testkit.NewLLM(
		testkit.FinalResponse(`{"channelName":"TGIF","dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[],"policy":{}}`),
		testkit.FinalResponse(`{"channelName":"TGIF","dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[
			{"mediaType":"series","key":"series:tmdb:1002","name":"Sabrina the Teenage Witch"}
		],"policy":{}}`),
	)

	proposal, err := suggest.New(model, catalog.New(nil, corpus), referenceExistsValidator{}, 10).
		WithReferences(references).
		Suggest(context.Background(), suggest.Intent{Description: "TGIF"})
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Acquisitions) != 1 || proposal.Acquisitions[0].TMDBID != 1002 || proposal.Acquisitions[0].Year != 1996 {
		t.Fatalf("proposal = %+v, want the source-disambiguated 1996 series", proposal)
	}
}

func TestSuggestMissingNamedSourceDoesNotPromoteModelMemory(t *testing.T) {
	meaning := dateMeaningNone()
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
	references := &testkit.ReferenceResolver{}
	model := testkit.NewLLM(testkit.FinalResponse(finalWithDateMeaning(t, meaning)), testkit.FinalResponse(finalWithDateMeaning(t, meaning)))
	proposal, err := dateExecutionSuggester(model, corpus).WithReferences(references).Suggest(context.Background(), suggest.Intent{Description: "TGIF block"})
	if err == nil || len(proposal.Lineup) != 0 || len(references.Discoveries()) != 1 {
		t.Fatalf("proposal=%+v err=%v discovery=%v", proposal, err, references.Discoveries())
	}
}
