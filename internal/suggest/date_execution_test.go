package suggest_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/testkit/catalogfixture"
)

func dateMeaningNone() map[string]any {
	return map[string]any{"kind": "none", "anchors": []any{}, "axes": []any{}}
}

func dateMeaning1990() map[string]any {
	return map[string]any{"kind": "constraints", "anchors": []any{map[string]any{"field": "description", "start": 0, "end": 4}}, "axes": []any{map[string]any{"kind": "movie_release", "combine": "any", "intervals": []any{map[string]any{"anchor": 0, "start": 1990, "end": 1999}}}}}
}

func dateMeaningAiring1990() map[string]any {
	return map[string]any{"kind": "constraints", "anchors": []any{map[string]any{"field": "description", "start": 0, "end": 4}}, "axes": []any{map[string]any{"kind": "series_airing", "combine": "any", "intervals": []any{map[string]any{"anchor": 0, "start": 1990, "end": 1999}}}}}
}

func dateExecutionSuggester(model *testkit.LLM, corpus *catalogfixture.Corpus) *suggest.Suggester {
	return suggest.New(model, catalog.New(nil, corpus), nil, 10)
}

func matrixCandidate() catalog.Candidate {
	return catalog.Candidate{MediaType: "movie", TMDBID: 603, Name: "The Matrix", Year: 1999, InLibrary: true}
}

func TestSuggest_ExplicitEpisodeDecadeRejectsNoneThenRecovers(t *testing.T) {
	const description = "Play sitcom episodes from the 1990s in episode order."
	meaning := map[string]any{
		"kind":    "constraints",
		"anchors": []any{map[string]any{"field": "description", "start": 30, "end": 35}},
		"axes": []any{map[string]any{
			"kind": "series_airing", "combine": "any",
			"intervals": []any{map[string]any{"anchor": 0, "start": 1990, "end": 1999}},
		}},
	}
	candidate := catalog.Candidate{MediaType: "series", TMDBID: 3452, Name: "Frasier", Year: 1993, InLibrary: true, Genres: []string{"Comedy"}}
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{candidate}}
	model := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"genres": []any{"Comedy"}, "media_type": "series", "dateMeaning": dateMeaningNone()}),
		testkit.ToolCallResponse("catalog_search", map[string]any{"genres": []any{"Comedy"}, "media_type": "series", "dateMeaning": meaning}),
		testkit.FinalResponse(`{"picks":[{"mediaType":"series","key":"series:tmdb:3452","name":"Frasier"}],"dateMeaning":{"kind":"constraints","anchors":[{"field":"description","start":30,"end":35}],"axes":[{"kind":"series_airing","combine":"any","intervals":[{"anchor":0,"start":1990,"end":1999}]}]}}`),
	)

	proposal, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: description})
	if err != nil {
		t.Fatal(err)
	}
	if model.Calls != 3 {
		t.Fatalf("model calls = %d, want rejected none, corrected tool call, and final response", model.Calls)
	}
	if got := corpus.Discoveries(); len(got) != 1 {
		t.Fatalf("catalog dispatches = %#v, want only the corrected dated search", got)
	}
	if proposal.Policy.Scope.Dates == nil || !equalRanges(proposal.Policy.Scope.Dates.SeriesAiring, []schedule.Range{{From: 1990, To: 1999}}) {
		t.Fatalf("date scope = %#v, want series-airing 1990-1999", proposal.Policy.Scope.Dates)
	}
}

func finalWithDateMeaning(t *testing.T, meaning any) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{
		"picks":       []any{map[string]any{"mediaType": "movie", "key": "movie:tmdb:603", "name": "The Matrix"}},
		"dateMeaning": meaning,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestSuggest_DateWindowUnionRecordsAggregateDispatches(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
	meaning := dateMeaning1990()
	model := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"genres": []any{"action"}, "dateMeaning": meaning}),
		testkit.FinalResponse(finalWithDateMeaning(t, meaning)),
	)
	proposal, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "1990 films"})
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Trace.WindowsCompleted != 2 || proposal.Trace.SourceQueriesDispatched != 2 {
		t.Fatalf("aggregate trace = %#v", proposal.Trace)
	}
	dispatches := corpus.Discoveries()
	if len(dispatches) != 2 || dispatches[0].Query.YearFrom != 1990 || dispatches[1].Query.YearFrom != 0 {
		t.Fatalf("date dispatches = %#v", dispatches)
	}
}

func TestSuggest_DateUnionFirstEmptyLaterNonemptyFinalizesAfterCompleteWindowSet(t *testing.T) {
	meaning := map[string]any{"kind": "constraints", "anchors": []any{map[string]any{"field": "description", "start": 0, "end": 4}}, "axes": []any{map[string]any{"kind": "movie_release", "combine": "any", "intervals": []any{
		map[string]any{"anchor": 0, "start": 1990, "end": 1990},
		map[string]any{"anchor": 0, "start": 1999, "end": 1999},
	}}}}
	corpus := &catalogfixture.Corpus{DiscoverFunc: func(_ context.Context, q catalog.DiscoveryQuery, _ int) ([]catalog.Candidate, error) {
		if q.YearFrom == 1999 {
			return []catalog.Candidate{matrixCandidate()}, nil
		}
		return nil, nil
	}}
	model := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"media_type": "movie", "dateMeaning": meaning}),
		testkit.FinalResponse(finalWithDateMeaning(t, meaning)),
	)
	model.OnChat = func() {
		if model.Calls == 1 && len(corpus.Discoveries()) != 2 {
			t.Fatalf("finalization started before every union window completed: discoveries=%#v", corpus.Discoveries())
		}
	}
	proposal, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "1990 and 1999 movies"})
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Lineup) != 1 || proposal.Lineup[0].TMDBID != 603 {
		t.Fatalf("proposal = %#v", proposal)
	}
	if proposal.Trace.WindowsCompleted != 2 || proposal.Trace.SourceQueriesDispatched != 2 {
		t.Fatalf("complete union trace = %#v", proposal.Trace)
	}
	dispatches := corpus.Discoveries()
	if len(dispatches) != 2 || dispatches[0].Query.YearFrom != 1990 || dispatches[1].Query.YearFrom != 1999 {
		t.Fatalf("date dispatches = %#v, want 1990 then 1999", dispatches)
	}
}

func TestSuggest_DateOnlyConstraintsDispatchWithoutOtherDiscoveryQualifiers(t *testing.T) {
	for name, tc := range map[string]struct {
		meaning        map[string]any
		wantDispatches int
	}{
		"movie release": {meaning: dateMeaning1990(), wantDispatches: 2},
		"series airing": {meaning: dateMeaningAiring1990(), wantDispatches: 1},
	} {
		t.Run(name, func(t *testing.T) {
			corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
			model := testkit.NewLLM(
				testkit.ToolCallResponse("catalog_search", map[string]any{"dateMeaning": tc.meaning}),
				testkit.FinalResponse(finalWithDateMeaning(t, tc.meaning)),
			)
			proposal, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "1990 films"})
			if err != nil {
				t.Fatal(err)
			}
			if len(proposal.Lineup) != 1 {
				t.Fatalf("proposal = %#v", proposal)
			}
			if got := corpus.Discoveries(); len(got) != tc.wantDispatches {
				t.Fatalf("date-only dispatches = %#v, want %d", got, tc.wantDispatches)
			}
		})
	}
}

func TestSuggest_DateMeaningMismatchNeverDispatchesCatalog(t *testing.T) {
	corpus := &catalogfixture.Corpus{DiscoverFunc: func(_ context.Context, q catalog.DiscoveryQuery, _ int) ([]catalog.Candidate, error) {
		if len(q.Genres) == 1 && q.Genres[0] == "later" {
			return []catalog.Candidate{matrixCandidate()}, nil
		}
		return nil, nil
	}}
	model := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"genres": []any{"first"}, "dateMeaning": dateMeaningNone()}),
		testkit.ToolCallResponse("catalog_search", map[string]any{"genres": []any{"must-not-dispatch"}, "dateMeaning": dateMeaning1990()}),
		testkit.ToolCallResponse("catalog_search", map[string]any{"genres": []any{"later"}, "dateMeaning": dateMeaningNone()}),
		testkit.FinalResponse(finalWithDateMeaning(t, dateMeaning1990())),
		testkit.FinalResponse(`{"picks":[{"mediaType":"movie","key":"movie:tmdb:603","name":"The Matrix"}],"dateMeaning":{"kind":"none","anchors":[],"axes":[]}}`),
	)
	proposal, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "1990 films"})
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Lineup) != 1 || proposal.Lineup[0].TMDBID != 603 {
		t.Fatalf("proposal = %#v", proposal)
	}
	if got := corpus.Discoveries(); len(got) != 2 || len(got[0].Query.Genres) != 1 || got[0].Query.Genres[0] != "first" || len(got[1].Query.Genres) != 1 || got[1].Query.Genres[0] != "later" {
		t.Fatalf("catalog dispatches = %#v, want first and later only", got)
	}
}

func TestSuggest_NoneDateMeaningWithoutQualifierDoesNotDispatchBeforeValidDateDiscovery(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
	meaning := dateMeaning1990()
	model := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"dateMeaning": dateMeaningNone()}),
		testkit.ToolCallResponse("catalog_search", map[string]any{"genres": []any{"action"}, "dateMeaning": meaning}),
		testkit.FinalResponse(finalWithDateMeaning(t, meaning)),
	)
	proposal, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "1990 action films"})
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Lineup) != 1 || proposal.Lineup[0].TMDBID != 603 {
		t.Fatalf("proposal = %#v", proposal)
	}
	dispatches := corpus.Discoveries()
	if len(dispatches) != 2 || len(dispatches[0].Query.Genres) != 1 || dispatches[0].Query.Genres[0] != "action" {
		t.Fatalf("catalog dispatches = %#v, want only the valid date discovery union", dispatches)
	}
}

func TestSuggest_DateUnionProviderErrorDoesNotSurfacePartialCandidates(t *testing.T) {
	meaning := map[string]any{"kind": "constraints", "anchors": []any{map[string]any{"field": "description", "start": 0, "end": 4}}, "axes": []any{map[string]any{"kind": "movie_release", "combine": "any", "intervals": []any{
		map[string]any{"anchor": 0, "start": 1990, "end": 1990},
		map[string]any{"anchor": 0, "start": 2000, "end": 2000},
	}}}}
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}, DiscoverFunc: func(_ context.Context, q catalog.DiscoveryQuery, _ int) ([]catalog.Candidate, error) {
		if q.YearFrom == 2000 {
			return nil, catalogfixture.ErrDiscover
		}
		return []catalog.Candidate{matrixCandidate()}, nil
	}}
	model := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"dateMeaning": meaning, "media_type": "movie"}),
		testkit.ToolCallResponse("catalog_search", map[string]any{"query": "The Matrix", "dateMeaning": meaning}),
		testkit.FinalResponse(finalWithDateMeaning(t, meaning)),
	)
	var failedUnionToolMessages []string
	model.OnChat = func() {
		if model.Calls != 2 {
			return
		}
		if got := corpus.Searches(); len(got) != 1 || got[0].Query != "The Matrix" {
			t.Fatalf("recovery title search = %#v, want exactly The Matrix", got)
		}
		for _, message := range model.LastMessages {
			if message.Role == llm.Tool {
				failedUnionToolMessages = append(failedUnionToolMessages, message.Content)
			}
		}
	}
	proposal, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "1990 films"})
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Lineup) != 1 || proposal.Lineup[0].TMDBID != 603 {
		t.Fatalf("proposal = %#v", proposal)
	}
	if proposal.Trace.WindowsCompleted != 1 || proposal.Trace.SourceQueriesDispatched != 2 {
		t.Fatalf("partial aggregate trace = %#v", proposal.Trace)
	}
	if got := corpus.Discoveries(); len(got) != 2 {
		t.Fatalf("date dispatches = %#v, want both windows before the error", got)
	}
	if len(corpus.Searches()) != 1 {
		t.Fatalf("recovery title searches = %#v, want one", corpus.Searches())
	}
	if len(failedUnionToolMessages) == 0 || !strings.Contains(failedUnionToolMessages[0], "error") || strings.Contains(failedUnionToolMessages[0], "The Matrix") {
		t.Fatalf("failed union tool message = %#v, want error without partial candidate", failedUnionToolMessages)
	}
}

func TestSuggest_DateUnionCreditExhaustionSpansRepairsAndGroundingRetry(t *testing.T) {
	meaning := map[string]any{"kind": "constraints", "anchors": []any{map[string]any{"field": "description", "start": 0, "end": 4}}, "axes": []any{map[string]any{"kind": "movie_release", "combine": "any", "intervals": []any{
		map[string]any{"anchor": 0, "start": 1990, "end": 1990},
		map[string]any{"anchor": 0, "start": 2000, "end": 2000},
		map[string]any{"anchor": 0, "start": 2010, "end": 2010},
	}}}}
	union := testkit.ToolCallResponse("catalog_search", map[string]any{"media_type": "movie", "dateMeaning": meaning})
	unsupported := testkit.ToolCallResponse("unsupported_tool", nil)
	emptyFinal, err := json.Marshal(map[string]any{"picks": []any{}, "dateMeaning": meaning})
	if err != nil {
		t.Fatal(err)
	}
	model := testkit.NewLLM(
		union, unsupported, unsupported, unsupported, testkit.FinalResponse("not json first repair"),
		union, unsupported, unsupported, unsupported, testkit.FinalResponse(string(emptyFinal)),
		union, unsupported, unsupported, unsupported, testkit.FinalResponse("not json second repair"),
		union, unsupported, unsupported, unsupported, union,
	)
	corpus := &catalogfixture.Corpus{DiscoverFunc: func(_ context.Context, _ catalog.DiscoveryQuery, _ int) ([]catalog.Candidate, error) {
		return nil, catalogfixture.ErrDiscover
	}}
	_, err = dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "1990, 2000, and 2010 movies"})
	var failure *suggest.Failure
	if !errors.As(err, &failure) || failure.Code != suggest.FailureBudgetExhausted {
		t.Fatalf("failure = %#v, want budget exhaustion", err)
	}
	if model.Calls != 20 {
		t.Fatalf("model calls = %d, want 20", model.Calls)
	}
	if got := corpus.Discoveries(); len(got) != 4 {
		t.Fatalf("discoveries = %#v, want one first-window dispatch per generation", got)
	}
	if got := corpus.Searches(); len(got) != 0 {
		t.Fatalf("searches = %#v, want none", got)
	}
	if failure.Trace.WindowsCompleted != 0 || failure.Trace.SourceQueriesDispatched != 4 {
		t.Fatalf("failure trace = %#v, want no completed windows and four source dispatches", failure.Trace)
	}
	bounds := suggest.ProductionBounds()
	if bounds.MaxModelCalls != 24 || bounds.MaxToolCalls != 24 {
		t.Fatalf("production bounds = %#v, want unchanged 24 model and tool calls", bounds)
	}
}

func TestSuggest_TwoEmptyDateUnionsStopAfterLogicalRetrievals(t *testing.T) {
	meaning := dateMeaning1990()
	corpus := &catalogfixture.Corpus{}
	model := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"genres": []any{"first"}, "dateMeaning": meaning}),
		testkit.ToolCallResponse("catalog_search", map[string]any{"genres": []any{"second"}, "dateMeaning": meaning}),
	)
	_, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "1990 films"})
	var failure *suggest.Failure
	if !errors.As(err, &failure) || failure.Trace.Terminal != suggest.ReasonRetrievalEmpty {
		t.Fatalf("failure = %#v, want two empty logical retrievals", err)
	}
	if got := corpus.Discoveries(); len(got) != 4 {
		t.Fatalf("date dispatches = %#v, want two complete two-window unions", got)
	}
}

func TestSuggest_DateUnionCapacityExhaustionDispatchesNoWindows(t *testing.T) {
	meaning := map[string]any{"kind": "constraints", "anchors": []any{map[string]any{"field": "description", "start": 0, "end": 4}}, "axes": []any{map[string]any{"kind": "movie_release", "combine": "any", "intervals": []any{
		map[string]any{"anchor": 0, "start": 1990, "end": 1990},
		map[string]any{"anchor": 0, "start": 2000, "end": 2000},
		map[string]any{"anchor": 0, "start": 2010, "end": 2010},
	}}}}
	invalid := testkit.ToolCallResponse("catalog_search", map[string]any{"dateMeaning": dateMeaningNone()})
	model := testkit.NewLLM(
		invalid, invalid, invalid, invalid,
		testkit.ToolCallResponse("catalog_search", map[string]any{"media_type": "movie", "dateMeaning": meaning}),
	)
	corpus := &catalogfixture.Corpus{}
	_, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "1990 films"})
	var failure *suggest.Failure
	if !errors.As(err, &failure) || failure.Code != suggest.FailureBudgetExhausted {
		t.Fatalf("failure = %#v, want budget exhaustion", err)
	}
	if got := corpus.Discoveries(); len(got) != 0 {
		t.Fatalf("date dispatches = %#v, want none after atomic reservation failure", got)
	}
}

func TestSuggest_InvalidToolDoesNotCommitDateMeaning(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
	model := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"mode": "collection", "media_type": "movie", "titles": []any{}, "dateMeaning": dateMeaningNone()}),
		testkit.ToolCallResponse("catalog_search", map[string]any{"genres": []any{"action"}, "dateMeaning": dateMeaning1990()}),
		testkit.FinalResponse(`{"picks":[{"mediaType":"movie","key":"movie:tmdb:603","name":"The Matrix"}],"dateMeaning":{"kind":"constraints","anchors":[{"field":"description","start":0,"end":4}],"axes":[{"kind":"movie_release","combine":"any","intervals":[{"anchor":0,"start":1990,"end":1999}]}]}}`),
	)
	proposal, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "1990 films"})
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Lineup) != 1 || proposal.Policy.Scope.Dates == nil {
		t.Fatalf("proposal = %#v", proposal)
	}
}

func TestSuggest_FinalDateMeaningMismatchRepairsBeforeClarifying(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
	model := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"genres": []any{"action"}, "dateMeaning": dateMeaning1990()}),
		testkit.FinalResponse(`{"picks":[{"mediaType":"movie","key":"movie:tmdb:603","name":"The Matrix"}],"dateMeaning":{"kind":"ambiguous","anchors":[{"field":"description","start":5,"end":10}],"axes":[]}}`),
		testkit.FinalResponse(`{"picks":[{"mediaType":"movie","key":"movie:tmdb:603","name":"The Matrix"}],"dateMeaning":{"kind":"constraints","anchors":[{"field":"description","start":0,"end":4}],"axes":[{"kind":"movie_release","combine":"any","intervals":[{"anchor":0,"start":1990,"end":1999}]}]}}`),
	)
	proposal, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "1990 then 2000 films"})
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Lineup) != 1 {
		t.Fatalf("proposal = %#v", proposal)
	}
}

func TestSuggest_InitialAmbiguousDateMeaningDoesNotDispatchCatalog(t *testing.T) {
	corpus := &catalogfixture.Corpus{}
	model := testkit.NewLLM(testkit.ToolCallResponse("catalog_search", map[string]any{
		"genres": []any{"action"},
		"dateMeaning": map[string]any{
			"kind": "ambiguous", "anchors": []any{map[string]any{"field": "description", "start": 0, "end": 5}}, "axes": []any{},
		},
	}))
	_, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "dated action"})
	var failure *suggest.Failure
	if !errors.As(err, &failure) || failure.Trace.Terminal != suggest.TerminalDateSemanticsUnclear {
		t.Fatalf("failure = %#v, want initial date clarification", err)
	}
	if got := corpus.Discoveries(); len(got) != 0 {
		t.Fatalf("catalog dispatches = %#v, want none", got)
	}
}

func TestSuggest_InitialConflictingDateMeaningDoesNotDispatchCatalog(t *testing.T) {
	corpus := &catalogfixture.Corpus{}
	model := testkit.NewLLM(testkit.ToolCallResponse("catalog_search", map[string]any{
		"genres": []any{"action"},
		"dateMeaning": map[string]any{
			"kind": "constraints", "anchors": []any{map[string]any{"field": "description", "start": 0, "end": 4}},
			"axes": []any{map[string]any{"kind": "movie_release", "combine": "all", "intervals": []any{
				map[string]any{"anchor": 0, "start": 1990, "end": 1999}, map[string]any{"anchor": 0, "start": 2000, "end": 2009},
			}}},
		},
	}))
	_, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "1990 action"})
	var failure *suggest.Failure
	if !errors.As(err, &failure) || failure.Trace.Terminal != suggest.TerminalConstraintsConflict {
		t.Fatalf("failure = %#v, want initial date conflict", err)
	}
	if got := corpus.Discoveries(); len(got) != 0 {
		t.Fatalf("catalog dispatches = %#v, want none", got)
	}
}

func TestSuggest_NoneDateMeaningDropsRawPolicyEra(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
	model := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"genres": []any{"action"}, "dateMeaning": dateMeaningNone()}),
		testkit.FinalResponse(`{"picks":[{"mediaType":"movie","key":"movie:tmdb:603","name":"The Matrix"}],"policy":{"era":{"from":1990,"to":1999}},"dateMeaning":{"kind":"none","anchors":[],"axes":[]}}`),
	)
	proposal, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "action"})
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Policy.Scope.Era != nil || proposal.Policy.Scope.Dates != nil {
		t.Fatalf("scope = %#v, want no date scope", proposal.Policy.Scope)
	}
}

func TestSuggest_ExhaustedFinalDateMeaningMismatchIsProviderFailure(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
	mismatch := `{"picks":[{"mediaType":"movie","key":"movie:tmdb:603","name":"The Matrix"}],"dateMeaning":{"kind":"ambiguous","anchors":[{"field":"description","start":5,"end":10}],"axes":[]}}`
	model := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"genres": []any{"action"}, "dateMeaning": dateMeaning1990()}),
		testkit.FinalResponse(mismatch), testkit.FinalResponse(mismatch), testkit.FinalResponse(mismatch),
	)
	_, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "1990 then 2000 films"})
	var failure *suggest.Failure
	if !errors.As(err, &failure) || failure.Code != "provider_failure" || failure.Trace.Terminal != "malformed_exhausted" {
		t.Fatalf("failure = %#v, want exhausted provider repair", err)
	}
}

func TestSuggest_StrictToolDateMeaningJSONRejectsThenRecovers(t *testing.T) {
	valid := dateMeaning1990()
	cases := []struct {
		name      string
		malformed map[string]any
	}{
		{"missing anchor start", map[string]any{"kind": "constraints", "anchors": []any{map[string]any{"field": "description", "end": 4}}, "axes": valid["axes"]}},
		{"unknown field", map[string]any{"kind": "none", "anchors": []any{}, "axes": []any{}, "unexpected": true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
			model := testkit.NewLLM(
				testkit.ToolCallResponse("catalog_search", map[string]any{"genres": []any{"must-not-dispatch"}, "dateMeaning": tc.malformed}),
				testkit.ToolCallResponse("catalog_search", map[string]any{"genres": []any{"valid"}, "dateMeaning": valid}),
				testkit.FinalResponse(finalWithDateMeaning(t, valid)),
			)
			if _, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "1990 films"}); err != nil {
				t.Fatal(err)
			}
			if got := corpus.Discoveries(); len(got) != 2 || len(got[0].Query.Genres) != 1 || got[0].Query.Genres[0] != "valid" || len(got[1].Query.Genres) != 1 || got[1].Query.Genres[0] != "valid" {
				t.Fatalf("catalog dispatches = %#v, want the valid date-discovery union only", got)
			}
		})
	}
}

func TestSuggest_StrictFinalDateMeaningJSONRejectsBeforeGrounding(t *testing.T) {
	cases := []struct {
		name      string
		malformed any
	}{
		{"null interval anchor", map[string]any{"kind": "constraints", "anchors": []any{map[string]any{"field": "description", "start": 0, "end": 4}}, "axes": []any{map[string]any{"kind": "movie_release", "combine": "any", "intervals": []any{map[string]any{"anchor": nil, "start": 1990, "end": 1999}}}}}},
		{"wrong interval type", map[string]any{"kind": "constraints", "anchors": []any{map[string]any{"field": "description", "start": 0, "end": 4}}, "axes": []any{map[string]any{"kind": "movie_release", "combine": "any", "intervals": []any{map[string]any{"anchor": 0, "start": "1990", "end": 1999}}}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
			valid := dateMeaning1990()
			model := testkit.NewLLM(
				testkit.ToolCallResponse("catalog_search", map[string]any{"genres": []any{"valid"}, "dateMeaning": valid}),
				testkit.FinalResponse(finalWithDateMeaning(t, tc.malformed)),
				testkit.FinalResponse(finalWithDateMeaning(t, valid)),
			)
			if _, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "1990 films"}); err != nil {
				t.Fatal(err)
			}
			if got := corpus.Discoveries(); len(got) != 2 || len(got[0].Query.Genres) != 1 || got[0].Query.Genres[0] != "valid" || len(got[1].Query.Genres) != 1 || got[1].Query.Genres[0] != "valid" {
				t.Fatalf("catalog dispatches = %#v, want the valid date-discovery union only", got)
			}
			if got := corpus.Searches(); len(got) != 0 {
				t.Fatalf("name grounding searches = %#v, want none", got)
			}
		})
	}
}

func TestSuggest_ExhaustedMalformedFinalDateMeaningDoesNotGroundNames(t *testing.T) {
	malformed := map[string]any{"kind": "constraints", "anchors": []any{map[string]any{"field": "description", "start": 0, "end": 4}}, "axes": []any{map[string]any{"kind": "movie_release", "combine": "any", "intervals": []any{map[string]any{"anchor": nil, "start": 1990, "end": 1999}}}}}
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
	model := testkit.NewLLM(testkit.FinalResponse(finalWithDateMeaning(t, malformed)), testkit.FinalResponse(finalWithDateMeaning(t, malformed)), testkit.FinalResponse(finalWithDateMeaning(t, malformed)))
	_, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "1990 films"})
	var failure *suggest.Failure
	if !errors.As(err, &failure) || failure.Code != "provider_failure" || failure.Trace.Terminal != "malformed_exhausted" {
		t.Fatalf("failure = %#v, want exhausted provider repair", err)
	}
	if got := corpus.Searches(); len(got) != 0 {
		t.Fatalf("name grounding searches = %#v, want none", got)
	}
	if got := corpus.Discoveries(); len(got) != 0 {
		t.Fatalf("catalog discoveries = %#v, want none", got)
	}
}

func TestSuggest_EquivalentCanonicalToolAndFinalDateMeaningsKeepDisjointScope(t *testing.T) {
	toolMeaning := map[string]any{
		"kind":    "constraints",
		"anchors": []any{map[string]any{"field": "description", "start": 0, "end": 4}},
		"axes": []any{map[string]any{"kind": "movie_release", "combine": "any", "intervals": []any{
			map[string]any{"anchor": 0, "start": 2000, "end": 2000},
			map[string]any{"anchor": 0, "start": 1990, "end": 1991},
			map[string]any{"anchor": 0, "start": 2001, "end": 2001},
			map[string]any{"anchor": 0, "start": 2000, "end": 2000},
		}}},
	}
	finalMeaning := map[string]any{
		"kind":    "constraints",
		"anchors": []any{map[string]any{"field": "description", "start": 0, "end": 4}},
		"axes": []any{map[string]any{"kind": "movie_release", "combine": "any", "intervals": []any{
			map[string]any{"anchor": 0, "start": 2000, "end": 2001},
			map[string]any{"anchor": 0, "start": 1990, "end": 1991},
		}}},
	}
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
	model := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"genres": []any{"action"}, "dateMeaning": toolMeaning}),
		testkit.FinalResponse(finalWithDateMeaning(t, finalMeaning)),
	)
	proposal, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "1990 2000 films"})
	if err != nil {
		t.Fatal(err)
	}
	want := []schedule.Range{{From: 1990, To: 1991}, {From: 2000, To: 2001}}
	if proposal.Policy.Scope.Dates == nil || !equalRanges(proposal.Policy.Scope.Dates.MovieRelease, want) {
		t.Fatalf("movie release scope = %#v, want %#v", proposal.Policy.Scope.Dates, want)
	}
}

func equalRanges(got, want []schedule.Range) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
