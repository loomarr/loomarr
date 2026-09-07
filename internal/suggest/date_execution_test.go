package suggest_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
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

func dateExecutionSuggester(model *testkit.LLM, corpus *catalogfixture.Corpus) *suggest.Suggester {
	return suggest.New(model, catalog.New(nil, corpus), nil, 10)
}

func matrixCandidate() catalog.Candidate {
	return catalog.Candidate{MediaType: "movie", TMDBID: 603, Name: "The Matrix", Year: 1999, InLibrary: true}
}

func finalWithDateMeaning(t *testing.T, meaning any) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{
		"picks":       []any{map[string]any{"mediaType": "movie", "tmdbId": 603, "name": "The Matrix"}},
		"dateMeaning": meaning,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
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
		testkit.FinalResponse(`{"picks":[{"mediaType":"movie","tmdbId":603,"name":"The Matrix"}],"dateMeaning":{"kind":"none","anchors":[],"axes":[]}}`),
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

func TestSuggest_InvalidToolDoesNotCommitDateMeaning(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
	model := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"mode": "collection", "media_type": "movie", "titles": []any{}, "dateMeaning": dateMeaningNone()}),
		testkit.ToolCallResponse("catalog_search", map[string]any{"genres": []any{"action"}, "dateMeaning": dateMeaning1990()}),
		testkit.FinalResponse(`{"picks":[{"mediaType":"movie","tmdbId":603,"name":"The Matrix"}],"dateMeaning":{"kind":"constraints","anchors":[{"field":"description","start":0,"end":4}],"axes":[{"kind":"movie_release","combine":"any","intervals":[{"anchor":0,"start":1990,"end":1999}]}]}}`),
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
		testkit.FinalResponse(`{"picks":[{"mediaType":"movie","tmdbId":603,"name":"The Matrix"}],"dateMeaning":{"kind":"ambiguous","anchors":[{"field":"description","start":5,"end":10}],"axes":[]}}`),
		testkit.FinalResponse(`{"picks":[{"mediaType":"movie","tmdbId":603,"name":"The Matrix"}],"dateMeaning":{"kind":"constraints","anchors":[{"field":"description","start":0,"end":4}],"axes":[{"kind":"movie_release","combine":"any","intervals":[{"anchor":0,"start":1990,"end":1999}]}]}}`),
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
		testkit.FinalResponse(`{"picks":[{"mediaType":"movie","tmdbId":603,"name":"The Matrix"}],"policy":{"era":{"from":1990,"to":1999}},"dateMeaning":{"kind":"none","anchors":[],"axes":[]}}`),
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
	mismatch := `{"picks":[{"mediaType":"movie","tmdbId":603,"name":"The Matrix"}],"dateMeaning":{"kind":"ambiguous","anchors":[{"field":"description","start":5,"end":10}],"axes":[]}}`
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
			if got := corpus.Discoveries(); len(got) != 1 || len(got[0].Query.Genres) != 1 || got[0].Query.Genres[0] != "valid" {
				t.Fatalf("catalog dispatches = %#v, want valid only", got)
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
			if got := corpus.Discoveries(); len(got) != 1 || len(got[0].Query.Genres) != 1 || got[0].Query.Genres[0] != "valid" {
				t.Fatalf("catalog dispatches = %#v, want valid tool only", got)
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
