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
	return testkit.FinalResponse(`{"channelName":"Friday Night","rationale":"A verified named block.","dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[{"mediaType":"movie","key":"movie:tmdb:90000603","name":"Marigold Protocol","rationale":"A verified constituent.","confidence":0.95}],"policy":{}}`), nil
}

func TestSuggestDiscoversNamedBlockSourceOnce(t *testing.T) {
	for _, request := range []string{"QFUN", "QFUN block", "Make a channel like QFUN", "a channel based on QFUN", "Recreate ZBN's QFUN Friday-night block."} {
		t.Run(request, func(t *testing.T) {
			meaning := dateMeaningNone()
			corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{marigoldCandidate()}}
			references := &testkit.ReferenceResolver{DiscoveryEvidence: reference.Evidence{
				URL: "https://lineups.example/frozen-block", Title: "Synthetic block roster", Excerpt: "Marigold Protocol", TitleAnchors: []string{"Marigold Protocol"},
			}}
			model := testkit.NewLLM(testkit.FinalResponse(finalWithDateMeaning(t, meaning)), testkit.FinalResponse(finalWithDateMeaning(t, meaning)))
			proposal, err := dateExecutionSuggester(model, corpus).WithReferences(references).Suggest(context.Background(), suggest.Intent{Description: request})
			if err != nil || len(proposal.Lineup) != 1 || proposal.Lineup[0].TMDBID != marigoldCandidate().TMDBID {
				t.Fatalf("proposal=%+v err=%v", proposal, err)
			}
			if got := references.Discoveries(); len(got) != 1 || got[0] != "QFUN" || len(references.Calls()) != 0 {
				t.Fatalf("discovery=%v pasted=%v", got, references.Calls())
			}
			if model.Calls != 2 || !strings.Contains(model.LastMessages[1].Content, "UNTRUSTED REFERENCE DATA") {
				t.Fatalf("calls=%d; discovered source was not supplied to finalization", model.Calls)
			}
		})
	}
}

func TestSuggestDiscoversAcronymSourceWithoutPhaseQualifier(t *testing.T) {
	for _, request := range []string{"LCU Phase One.", "Recreate the LCU Phase One lineup.", "The LCU Phase One lineup of films, please."} {
		t.Run(request, func(t *testing.T) {
			meaning := dateMeaningNone()
			corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{marigoldCandidate()}}
			references := &testkit.ReferenceResolver{ByLabel: map[string]reference.Evidence{
				"LCU": {
					URL: "https://lineups.example/lcu", Title: "Synthetic LCU roster",
					Excerpt: "Marigold Protocol", TitleAnchors: []string{"Marigold Protocol"},
				},
			}}
			model := testkit.NewLLM(
				testkit.FinalResponse(finalWithDateMeaning(t, meaning)),
				testkit.FinalResponse(finalWithDateMeaning(t, meaning)),
			)

			proposal, err := dateExecutionSuggester(model, corpus).WithReferences(references).
				Suggest(context.Background(), suggest.Intent{Description: request})
			if got := references.Discoveries(); len(got) != 1 || got[0] != "LCU" {
				t.Fatalf("discovery=%v, want the stable LCU source label", got)
			}
			if err != nil || len(proposal.Lineup) != 1 || proposal.Lineup[0].TMDBID != marigoldCandidate().TMDBID {
				t.Fatalf("proposal=%+v err=%v", proposal, err)
			}
		})
	}
}

func TestSuggestNamedBlockStopsRetrievalAfterReferenceGrounding(t *testing.T) {
	corpus := &catalogfixture.Corpus{
		SearchFunc: func(_ context.Context, query string, _ int) ([]catalog.Candidate, error) {
			if query == "Marigold Protocol" {
				return []catalog.Candidate{marigoldCandidate()}, nil
			}
			return nil, nil
		},
	}
	references := &testkit.ReferenceResolver{DiscoveryEvidence: reference.Evidence{
		URL: "https://lineups.example/frozen-block", Title: "Synthetic block roster", Excerpt: "Marigold Protocol", TitleAnchors: []string{"Marigold Protocol"},
	}}
	model := &namedSetToolAvailabilityLLM{}

	proposal, err := suggest.New(model, catalog.New(nil, corpus), nil, 10).WithReferences(references).Suggest(context.Background(), suggest.Intent{Description: "QFUN"})
	if err != nil {
		t.Fatalf("suggestion failed after %d model calls and %d catalog searches: %v", model.calls, len(corpus.Searches()), err)
	}
	if len(proposal.Lineup) != 1 || proposal.Lineup[0].TMDBID != marigoldCandidate().TMDBID {
		t.Fatalf("proposal=%+v, want the source-grounded constituent", proposal)
	}
	if model.calls != 2 {
		t.Fatalf("model calls=%d, want collection hypothesis followed by tool-free finalization", model.calls)
	}
	if got := references.Discoveries(); len(got) != 1 || got[0] != "QFUN" {
		t.Fatalf("discovery=%v, want one QFUN source resolution", got)
	}
}

func TestSuggestNamedBlockContinuesPastUnresolvedSourceAnchors(t *testing.T) {
	grandOak := catalog.Candidate{MediaType: "series", TMDBID: 90001001, Name: "Grand Oak Hotel", Year: 1987, InLibrary: true}
	crumbStreet := catalog.Candidate{MediaType: "series", TMDBID: 90001002, Name: "Crumb Street Bakery", Year: 1989, InLibrary: true}
	corpus := &catalogfixture.Corpus{
		SearchFunc: func(_ context.Context, query string, _ int) ([]catalog.Candidate, error) {
			switch query {
			case grandOak.Name:
				return []catalog.Candidate{grandOak}, nil
			case crumbStreet.Name:
				return []catalog.Candidate{crumbStreet}, nil
			default:
				return nil, nil
			}
		},
	}
	references := &testkit.ReferenceResolver{DiscoveryEvidence: reference.Evidence{
		URL: "https://lineups.example/qfun", Title: "QFUN", Excerpt: "A verified programming block.",
		TitleAnchors: []string{
			"Unavailable One", "Unavailable Two", "Unavailable Three", "Unavailable Four",
			"Unavailable Five", "Unavailable Six", "Unavailable Seven", "Unavailable Eight",
			grandOak.Name, crumbStreet.Name,
		},
	}}
	model := testkit.NewLLM(
		testkit.FinalResponse(`{"channelName":"Friday Night","rationale":"Interpreting the named block.","dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[],"policy":{}}`),
		testkit.FinalResponse(`{"channelName":"Friday Night","rationale":"A verified named block.","dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[{"mediaType":"series","key":"series:tmdb:90001001","name":"Grand Oak Hotel","rationale":"A defining constituent.","confidence":0.98},{"mediaType":"series","key":"series:tmdb:90001002","name":"Crumb Street Bakery","rationale":"A defining constituent.","confidence":0.98}],"policy":{}}`),
	)

	proposal, err := dateExecutionSuggester(model, corpus).WithReferences(references).Suggest(context.Background(), suggest.Intent{Description: "QFUN"})
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
		{MediaType: provision.Series, TMDBID: 90001001, Name: "Grand Oak Hotel", InLibrary: true},
		{MediaType: provision.Series, TMDBID: 90001007, Name: "Penny the Pocket Witch", InLibrary: true},
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
		URL: "https://lineups.example/qfun", Title: "QFUN", Excerpt: "A verified programming block.",
		TitleAnchors: []string{"Grand Oak Hotel", "Penny the Pocket Witch"},
	}}
	model := testkit.NewLLM(
		testkit.FinalResponse(`{"channelName":"QFUN","dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[],"policy":{}}`),
		testkit.FinalResponse(`{"channelName":"QFUN","dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[
			{"mediaType":"series","key":"series:tmdb:90001001","name":"Grand Oak Hotel"}
		],"policy":{}}`),
	)

	proposal, err := suggest.New(model, catalog.New(nil, corpus), referenceExistsValidator{}, 10).
		WithReferences(references).
		Suggest(context.Background(), suggest.Intent{Description: "QFUN"})
	if err != nil {
		t.Fatal(err)
	}
	items := append(append([]suggest.ProposalItem(nil), proposal.Lineup...), proposal.Acquisitions...)
	for _, item := range items {
		if item.Name == "Penny the Pocket Witch" {
			return
		}
	}
	t.Fatalf("selectable QFUN items = %+v, want grounded Sabrina beyond the model shortlist", items)
}

func TestSuggestNamedBlockPreservesDistinctRequestedTitlesAndRejectsNamesakes(t *testing.T) {
	grandOak := catalog.Candidate{MediaType: provision.Series, TMDBID: 90001001, Name: "Grand Oak Hotel", Year: 1987, InLibrary: true}
	crumbStreet := catalog.Candidate{MediaType: provision.Series, TMDBID: 90001002, Name: "Crumb Street Bakery", Year: 1989, InLibrary: true}
	wrongNamesake := catalog.Candidate{MediaType: provision.Series, TMDBID: 90002002, Name: "Crumb Street Bakery", Year: 2024}
	bowlingAlley := catalog.Candidate{MediaType: provision.Series, TMDBID: 90001003, Name: "Bowling Alley Blues", Year: 1991, InLibrary: true}
	corpus := &catalogfixture.Corpus{
		SearchFunc: func(_ context.Context, query string, _ int) ([]catalog.Candidate, error) {
			switch query {
			case grandOak.Name:
				return []catalog.Candidate{grandOak}, nil
			case crumbStreet.Name:
				return []catalog.Candidate{crumbStreet, wrongNamesake}, nil
			case bowlingAlley.Name:
				return []catalog.Candidate{bowlingAlley}, nil
			default:
				return nil, nil
			}
		},
	}
	references := &testkit.ReferenceResolver{DiscoveryEvidence: reference.Evidence{
		URL: "https://lineups.example/qfun", Title: "QFUN", Excerpt: "A verified programming block.",
		TitleAnchors: []string{grandOak.Name, crumbStreet.Name, bowlingAlley.Name},
	}}
	model := testkit.NewLLM(
		testkit.FinalResponse(`{"channelName":"QFUN","dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[],"policy":{}}`),
		testkit.FinalResponse(`{"channelName":"QFUN","dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[
			{"mediaType":"series","key":"series:tmdb:90001001","name":"Grand Oak Hotel"},
			{"mediaType":"series","key":"series:tmdb:90001001","name":"Grand Oak Hotel"},
			{"mediaType":"series","key":"series:tmdb:90001001","name":"Grand Oak Hotel"},
			{"mediaType":"series","key":"series:tmdb:90002002","name":"Crumb Street Bakery"}
		],"policy":{}}`),
	)

	proposal, err := suggest.New(model, catalog.New(nil, corpus), referenceExistsValidator{}, 10).
		WithReferences(references).
		Suggest(context.Background(), suggest.Intent{Description: "QFUN — include Grand Oak Hotel, Crumb Street Bakery, and Bowling Alley Blues."})
	if err != nil {
		t.Fatal(err)
	}
	items := append(append([]suggest.ProposalItem(nil), proposal.Lineup...), proposal.Acquisitions...)
	if len(items) != 3 {
		t.Fatalf("items=%+v searches=%+v trace=%+v, want the three distinct requested titles", items, corpus.Searches(), proposal.Trace)
	}
	want := map[int]bool{90001001: true, 90001002: true, 90001003: true}
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
	copperKnight := catalog.Candidate{MediaType: provision.Movie, TMDBID: 90001726, Name: "Copper Knight", Year: 2008, InLibrary: true}
	copperKnight2 := catalog.Candidate{MediaType: provision.Movie, TMDBID: 90010138, Name: "Copper Knight 2", Year: 2010}
	corpus := &catalogfixture.Corpus{SearchFunc: func(_ context.Context, query string, _ int) ([]catalog.Candidate, error) {
		switch query {
		case copperKnight.Name:
			return []catalog.Candidate{copperKnight}, nil
		case copperKnight2.Name:
			return []catalog.Candidate{copperKnight2}, nil
		default:
			return nil, nil
		}
	}}
	references := &testkit.ReferenceResolver{ByLabel: map[string]reference.Evidence{
		"LCU": {URL: "https://lineups.example/lcu", Title: "LCU", Excerpt: "Two reviewed members", TitleAnchors: []string{copperKnight.Name, copperKnight2.Name}},
	}}
	meaning := dateMeaningNone()
	model := testkit.NewLLM(
		testkit.FinalResponse(finalWithDateMeaning(t, meaning)),
		testkit.FinalResponse(`{"picks":[{"mediaType":"movie","key":"movie:tmdb:90001726"}],"dateMeaning":{"kind":"none","anchors":[],"axes":[]}}`),
	)

	proposal, err := dateExecutionSuggester(model, corpus).WithReferences(references).Suggest(context.Background(), suggest.Intent{
		Description: "LCU Phase One lineup, but leave out Copper Knight 2.",
		MustExclude: []string{"Copper Knight 2"},
	})
	if err != nil || len(proposal.Lineup) != 1 || proposal.Lineup[0].TMDBID != copperKnight.TMDBID || len(proposal.Acquisitions) != 0 {
		t.Fatalf("related-title exclusion removed the wrong member: proposal=%+v err=%v", proposal, err)
	}
}

func TestSuggestNamedSourceCompletionRespectsKnownMovieReleaseRange(t *testing.T) {
	oldMovie := catalog.Candidate{MediaType: provision.Movie, TMDBID: 90001726, Name: "Copper Knight", Year: 2008, InLibrary: true}
	inRange := catalog.Candidate{MediaType: provision.Movie, TMDBID: 90010138, Name: "Copper Knight 2", Year: 2010, InLibrary: true}
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
		"LCU": {URL: "https://lineups.example/lcu", Title: "LCU", Excerpt: "Two reviewed members", TitleAnchors: []string{oldMovie.Name, inRange.Name}},
	}}
	description := "LCU Phase One lineup, only movies released from 2010 through 2012."
	marker := "2010 through 2012"
	start := strings.Index(description, marker)
	meaning := fixtureDateMeaning("movie_release", "description", start, start+len(marker), 2010, 2012)
	final, err := json.Marshal(map[string]any{
		"picks":       []any{map[string]any{"mediaType": "movie", "key": "movie:tmdb:90010138"}},
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
		{MediaType: provision.Series, TMDBID: 90001001, Name: "Grand Oak Hotel", InLibrary: true},
		{MediaType: provision.Series, TMDBID: 90001002, Name: "Crumb Street Bakery", InLibrary: true},
		{MediaType: provision.Series, TMDBID: 90001003, Name: "Bowling Alley Blues", InLibrary: true},
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
		URL: "https://lineups.example/qfun", Title: "QFUN", Excerpt: "A verified programming block.",
		TitleAnchors: []string{"Grand Oak Hotel", "Crumb Street Bakery", "Bowling Alley Blues"},
	}}
	unsolicited := testkit.ToolCallResponse("catalog_search", map[string]any{
		"query": "Grand Oak Hotel", "dateMeaning": dateMeaningNone(),
	})
	model := testkit.NewLLM(
		testkit.FinalResponse(`{"channelName":"QFUN","dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[],"policy":{}}`),
		unsolicited, unsolicited, unsolicited,
	)
	proposal, err := suggest.New(model, catalog.New(nil, corpus), referenceExistsValidator{}, 10).
		WithReferences(references).
		Suggest(context.Background(), suggest.Intent{Description: "QFUN — include Grand Oak Hotel, Crumb Street Bakery, and Bowling Alley Blues."})
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
	movie := catalog.Candidate{MediaType: "movie", TMDBID: 90001001, Name: "Pinwheel Summer", Year: 1995, InLibrary: true}
	series := catalog.Candidate{MediaType: "series", TMDBID: 90001002, Name: "Pinwheel Summer", Year: 1996}
	corpus := &catalogfixture.Corpus{
		SearchFunc: func(_ context.Context, query string, _ int) ([]catalog.Candidate, error) {
			if query == "Pinwheel Summer" || query == "Pinwheel Summer (TV series)" {
				return []catalog.Candidate{movie, series}, nil
			}
			return nil, nil
		},
	}
	references := &testkit.ReferenceResolver{DiscoveryEvidence: reference.Evidence{
		URL: "https://lineups.example/qfun", Title: "QFUN", Excerpt: "A verified programming block.",
		TitleAnchors: []string{"Pinwheel Summer (TV series)"},
	}}
	model := testkit.NewLLM(
		testkit.FinalResponse(`{"channelName":"QFUN","rationale":"Interpreting the named block.","dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[],"policy":{}}`),
		testkit.FinalResponse(`{"channelName":"QFUN","rationale":"A verified named block.","dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[{"mediaType":"series","key":"series:tmdb:90001002","name":"Pinwheel Summer","rationale":"The source identifies the television series.","confidence":0.98}],"policy":{}}`),
	)

	proposal, err := suggest.New(model, catalog.New(nil, corpus), referenceExistsValidator{}, 10).
		WithReferences(references).
		Suggest(context.Background(), suggest.Intent{Description: "QFUN"})
	if err != nil {
		t.Fatalf("suggestion failed: %v", err)
	}
	if len(proposal.Lineup) != 0 || len(proposal.Acquisitions) != 1 || proposal.Acquisitions[0].MediaType != "series" {
		t.Fatalf("proposal=%+v, want the source-typed series rather than the owned namesake movie", proposal)
	}
}

func TestSuggestNamedBlockUsesSourceYearToResolveSeriesNamesake(t *testing.T) {
	animated := catalog.Candidate{MediaType: provision.Series, TMDBID: 90001001, Name: "Penny the Pocket Witch", Year: 1970}
	liveAction := catalog.Candidate{MediaType: provision.Series, TMDBID: 90001002, Name: "Penny the Pocket Witch", Year: 1996}
	corpus := &catalogfixture.Corpus{SearchFunc: func(_ context.Context, query string, _ int) ([]catalog.Candidate, error) {
		if query == "Penny the Pocket Witch" {
			return []catalog.Candidate{animated, liveAction}, nil
		}
		return nil, nil
	}}
	references := &testkit.ReferenceResolver{DiscoveryEvidence: reference.Evidence{
		URL: "https://lineups.example/qfun", Title: "QFUN", Excerpt: "A verified programming block.",
		TitleAnchors: []string{"Penny the Pocket Witch (1996 TV series)"},
	}}
	model := testkit.NewLLM(
		testkit.FinalResponse(`{"channelName":"QFUN","dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[],"policy":{}}`),
		testkit.FinalResponse(`{"channelName":"QFUN","dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[
			{"mediaType":"series","key":"series:tmdb:90001002","name":"Penny the Pocket Witch"}
		],"policy":{}}`),
	)

	proposal, err := suggest.New(model, catalog.New(nil, corpus), referenceExistsValidator{}, 10).
		WithReferences(references).
		Suggest(context.Background(), suggest.Intent{Description: "QFUN"})
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Acquisitions) != 1 || proposal.Acquisitions[0].TMDBID != 90001002 || proposal.Acquisitions[0].Year != 1996 {
		t.Fatalf("proposal = %+v, want the source-disambiguated 1996 series", proposal)
	}
}

func TestSuggestMissingNamedSourceDoesNotPromoteModelMemory(t *testing.T) {
	meaning := dateMeaningNone()
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{marigoldCandidate()}}
	references := &testkit.ReferenceResolver{}
	model := testkit.NewLLM(testkit.FinalResponse(finalWithDateMeaning(t, meaning)), testkit.FinalResponse(finalWithDateMeaning(t, meaning)))
	proposal, err := dateExecutionSuggester(model, corpus).WithReferences(references).Suggest(context.Background(), suggest.Intent{Description: "QFUN block"})
	if err == nil || len(proposal.Lineup) != 0 || len(references.Discoveries()) != 1 {
		t.Fatalf("proposal=%+v err=%v discovery=%v", proposal, err, references.Discoveries())
	}
}
