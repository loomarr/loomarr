package suggest_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/reference"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/testkit/catalogfixture"
)

func TestSuggest_DateSourceOrderInitialDenialMatrix(t *testing.T) {
	ambiguity := `{"picks":[],"dateMeaning":{"kind":"ambiguous","anchors":[{"field":"description","start":0,"end":4}],"axes":[]}}`
	conflict := `{"picks":[],"dateMeaning":{"kind":"constraints","anchors":[{"field":"description","start":0,"end":4},{"field":"description","start":9,"end":13}],"axes":[{"kind":"movie_release","combine":"all","intervals":[{"anchor":0,"start":1990,"end":1999},{"anchor":1,"start":2000,"end":2009}]}]}}`
	for _, source := range []struct {
		name   string
		intent suggest.Intent
	}{
		{name: "reference", intent: suggest.Intent{Description: "1990 and 2000 https://lineups.example/friday"}},
		{name: "curated", intent: suggest.Intent{Description: "1990 and 2000 Classic The Matrix"}},
		{name: "named", intent: suggest.Intent{Description: "1990 and 2000 A named programming block", MustInclude: []string{"The Matrix"}}},
	} {
		for _, failureCase := range []struct {
			name, response, code, terminal string
			calls                          int
		}{
			{name: "ambiguity", response: ambiguity, code: suggest.FailureCodeNoGroundedTitles, terminal: suggest.TerminalDateSemanticsUnclear, calls: 1},
			{name: "conflict", response: conflict, code: suggest.FailureCodeNoGroundedTitles, terminal: suggest.TerminalConstraintsConflict, calls: 1},
			{name: "malformed", response: `{"picks":[]}`, code: suggest.FailureProvider, terminal: suggest.TerminalMalformedExhausted, calls: 3},
		} {
			t.Run(source.name+"/"+failureCase.name, func(t *testing.T) {
				corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
				references := &testkit.ReferenceResolver{Evidence: reference.Evidence{TitleAnchors: []string{"The Matrix"}}}
				responses := make([]llm.Response, failureCase.calls)
				for i := range responses {
					responses[i] = testkit.FinalResponse(failureCase.response)
				}
				model := testkit.NewLLM(responses...)
				_, err := dateExecutionSuggester(model, corpus).WithReferences(references).Suggest(context.Background(), source.intent)
				var failure *suggest.Failure
				if !errors.As(err, &failure) || failure.Code != failureCase.code || failure.Trace.Terminal != failureCase.terminal {
					t.Fatalf("err = %#v, want code %q terminal %q", err, failureCase.code, failureCase.terminal)
				}
				if model.Calls != failureCase.calls {
					t.Fatalf("model calls = %d, want %d", model.Calls, failureCase.calls)
				}
				if len(corpus.Searches()) != 0 || len(corpus.Discoveries()) != 0 || len(references.Calls()) != 0 {
					t.Fatalf("source calls = searches %d, discoveries %d, references %d; want all zero", len(corpus.Searches()), len(corpus.Discoveries()), len(references.Calls()))
				}
			})
		}
	}
}

func TestSuggest_DateSourceOrderReferenceEvidenceTitleAnchorsTheMatrix(t *testing.T) {
	meaning := dateMeaningNone()
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
	references := &testkit.ReferenceResolver{Evidence: reference.Evidence{
		Title:        "Friday night science fiction",
		URL:          "https://lineups.example/friday",
		Excerpt:      "The Matrix follows an ordinary programmer into a hidden war.",
		TitleAnchors: []string{"The Matrix"},
	}}
	bootstrap, err := json.Marshal(map[string]any{
		"picks":       []any{map[string]any{"mediaType": "movie", "tmdbId": 999999, "name": "Fabricated Bootstrap Pick"}},
		"dateMeaning": meaning,
	})
	if err != nil {
		t.Fatal(err)
	}
	model := testkit.NewLLM(testkit.FinalResponse(string(bootstrap)), testkit.FinalResponse(finalWithDateMeaning(t, meaning)))
	proposal, err := dateExecutionSuggester(model, corpus).WithReferences(references).Suggest(context.Background(), suggest.Intent{Description: "1990 and 2000 https://lineups.example/friday"})
	if err != nil || len(proposal.Lineup) != 1 || proposal.Lineup[0].TMDBID != matrixCandidate().TMDBID || model.Calls != 2 || len(references.Calls()) != 1 || len(corpus.Searches()) != 1 || len(corpus.Discoveries()) != 0 {
		t.Fatalf("proposal=%+v err=%v model calls=%d reference calls=%d searches=%d discoveries=%d", proposal, err, model.Calls, len(references.Calls()), len(corpus.Searches()), len(corpus.Discoveries()))
	}
	if len(model.LastOpts.Tools) != 0 {
		t.Fatalf("final tools = %#v, want nil", model.LastOpts.Tools)
	}
	if proposal.Trace.SurfacedTotal != 1 || proposal.Trace.RecordedTotal != 1 {
		t.Fatalf("reference trace = %#v, want one physical reference result", proposal.Trace)
	}
	var prompt string
	for _, message := range model.LastMessages {
		if message.Role == llm.User {
			prompt = message.Content
			break
		}
	}
	if prompt == "" {
		t.Fatal("final chat has no user prompt")
	}
	for _, want := range []string{
		"UNTRUSTED REFERENCE DATA", references.Evidence.Title, references.Evidence.URL, references.Evidence.Excerpt,
		"--- BEGIN UNTRUSTED REFERENCE DATA ---", "--- END UNTRUSTED REFERENCE DATA ---",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("final user prompt missing %q:\n%s", want, prompt)
		}
	}
	toolCalls := 0
	for _, message := range model.LastMessages {
		for _, call := range message.ToolCalls {
			toolCalls++
			got, ok := call.Arguments["dateMeaning"]
			if !ok {
				t.Fatalf("tool call %q missing dateMeaning", call.Name)
			}
			var wantDecoded, actualDecoded any
			wantJSON, marshalErr := json.Marshal(meaning)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if unmarshalErr := json.Unmarshal(wantJSON, &wantDecoded); unmarshalErr != nil {
				t.Fatal(unmarshalErr)
			}
			actualJSON, marshalErr := json.Marshal(got)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if unmarshalErr := json.Unmarshal(actualJSON, &actualDecoded); unmarshalErr != nil {
				t.Fatal(unmarshalErr)
			}
			if !reflect.DeepEqual(actualDecoded, wantDecoded) {
				t.Fatalf("dateMeaning=%v want %v", actualDecoded, wantDecoded)
			}
		}
	}
	if toolCalls != 1 {
		t.Fatalf("synthetic catalog tool calls = %d, want 1", toolCalls)
	}
}

func TestSuggest_DateSourceOrderReferenceContinuationUsesTwoRepairs(t *testing.T) {
	meaning := dateMeaningNone()
	for name, responses := range map[string][]llm.Response{
		"malformed-before-bootstrap": {testkit.FinalResponse("not json"), testkit.FinalResponse("still not json"), testkit.FinalResponse(finalWithDateMeaning(t, meaning)), testkit.FinalResponse(finalWithDateMeaning(t, meaning))},
		"malformed-after-bootstrap":  {testkit.FinalResponse(finalWithDateMeaning(t, meaning)), testkit.FinalResponse("not json"), testkit.FinalResponse(finalWithDateMeaning(t, meaning))},
	} {
		t.Run(name, func(t *testing.T) {
			corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
			references := &testkit.ReferenceResolver{Evidence: reference.Evidence{TitleAnchors: []string{"The Matrix"}}}
			model := testkit.NewLLM(responses...)
			proposal, err := dateExecutionSuggester(model, corpus).WithReferences(references).Suggest(context.Background(), suggest.Intent{Description: "1990 and 2000 https://lineups.example/friday"})
			if err != nil || len(proposal.Lineup) != 1 || proposal.Lineup[0].TMDBID != matrixCandidate().TMDBID || len(references.Calls()) != 1 || len(corpus.Searches()) != 1 {
				t.Fatalf("proposal=%+v err=%v calls=%d refs=%d searches=%d", proposal, err, model.Calls, len(references.Calls()), len(corpus.Searches()))
			}
			wantCalls := 3
			if name == "malformed-before-bootstrap" {
				wantCalls = 4
			}
			if model.Calls != wantCalls {
				t.Fatalf("model calls = %d, want %d", model.Calls, wantCalls)
			}
		})
	}
}

func TestSuggest_DateSourceOrderSourceCapacityReservedBeforeInitialization(t *testing.T) {
	meaning := map[string]any{"kind": "constraints", "anchors": []any{map[string]any{"field": "description", "start": 0, "end": 4}, map[string]any{"field": "description", "start": 9, "end": 13}, map[string]any{"field": "description", "start": 18, "end": 22}}, "axes": []any{map[string]any{"kind": "movie_release", "combine": "any", "intervals": []any{map[string]any{"anchor": 0, "start": 1990, "end": 1990}, map[string]any{"anchor": 1, "start": 2000, "end": 2000}, map[string]any{"anchor": 2, "start": 2010, "end": 2010}}}}}
	invalid := testkit.ToolCallResponse("catalog_search", map[string]any{"dateMeaning": dateMeaningNone()})
	model := testkit.NewLLM(invalid, invalid, invalid, invalid, testkit.ToolCallResponse("catalog_search", map[string]any{"media_type": "movie", "dateMeaning": meaning}))
	corpus := &catalogfixture.Corpus{}
	references := &testkit.ReferenceResolver{}
	_, err := dateExecutionSuggester(model, corpus).WithReferences(references).Suggest(context.Background(), suggest.Intent{Description: "1990 and 2000 and 2010 A named programming block", MustInclude: []string{"The Matrix"}})
	var failure *suggest.Failure
	if !errors.As(err, &failure) || failure.Code != suggest.FailureBudgetExhausted || model.Calls != 5 || len(corpus.Searches()) != 0 || len(corpus.Discoveries()) != 0 || len(references.Calls()) != 0 {
		t.Fatalf("err=%v calls=%d searches=%d discoveries=%d references=%d", err, model.Calls, len(corpus.Searches()), len(corpus.Discoveries()), len(references.Calls()))
	}
}

func TestSuggest_DateSourceOrderInitialAmbiguityDoesNotResolveReference(t *testing.T) {
	references := &testkit.ReferenceResolver{Evidence: reference.Evidence{TitleAnchors: []string{"The Matrix"}}}
	model := testkit.NewLLM(testkit.FinalResponse(`{"picks":[],"dateMeaning":{"kind":"ambiguous","anchors":[{"field":"description","start":0,"end":5}],"axes":[]}}`))
	_, err := buildSuggester(t, model).WithReferences(references).Suggest(context.Background(), suggest.Intent{Description: "dated https://lineups.example/friday"})
	var failure *suggest.Failure
	if !errors.As(err, &failure) || failure.Trace.Terminal != suggest.TerminalDateSemanticsUnclear {
		t.Fatalf("failure = %#v", err)
	}
	if got := len(references.Calls()); got != 0 {
		t.Fatalf("reference calls = %d, want zero before valid date meaning", got)
	}
}

func TestSuggest_DateSourceOrderValidTitleAndCollectionCallsReserveNoNegativeWork(t *testing.T) {
	meaning := dateMeaningNone()
	for name, arguments := range map[string]map[string]any{
		"title":      {"query": "The Matrix", "dateMeaning": meaning},
		"collection": {"mode": "collection", "media_type": "movie", "titles": []any{"The Matrix"}, "dateMeaning": meaning},
	} {
		t.Run(name, func(t *testing.T) {
			corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
			model := testkit.NewLLM(testkit.ToolCallResponse("catalog_search", arguments), testkit.FinalResponse(finalWithDateMeaning(t, meaning)))
			proposal, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "matrix"})
			if err != nil || len(proposal.Lineup) != 1 {
				t.Fatalf("proposal=%#v err=%v", proposal, err)
			}
		})
	}
}

func TestSuggest_DateSourceOrderInvalidCollectionExtraFieldDoesNotInitializeSources(t *testing.T) {
	meaning := dateMeaningNone()
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
	invalid := map[string]any{"mode": "collection", "media_type": "movie", "titles": []any{"The Matrix"}, "genres": []any{"Action"}, "dateMeaning": meaning}
	valid := map[string]any{"mode": "collection", "media_type": "movie", "titles": []any{"The Matrix"}, "dateMeaning": meaning}
	model := testkit.NewLLM(testkit.ToolCallResponse("catalog_search", invalid), testkit.ToolCallResponse("catalog_search", valid), testkit.FinalResponse(finalWithDateMeaning(t, meaning)))
	model.OnChat = func() {
		if model.Calls == 1 && len(corpus.Searches()) != 0 {
			t.Fatalf("sources initialized before corrected tool call: searches=%#v", corpus.Searches())
		}
	}
	proposal, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "A named programming block", MustInclude: []string{"The Matrix"}})
	if err != nil || len(proposal.Lineup) != 1 {
		t.Fatalf("proposal=%#v err=%v", proposal, err)
	}
	// The corrected call performs one explicit-membership lookup and one exact
	// collection lookup. Any initialization before it would add another lookup.
	if got := len(corpus.Searches()); got != 2 {
		t.Fatalf("catalog searches = %d, want 2 only after corrected collection call", got)
	}
}

func TestSuggest_DateSourceOrderCuratedIdentitySurvivesSourceInitialization(t *testing.T) {
	meaning := dateMeaningNone()
	curated := matrixCandidate()
	unrelated := catalog.Candidate{MediaType: "movie", TMDBID: 604, Name: "Unrelated Movie", Year: 2000, InLibrary: true}
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{curated, unrelated}}
	final, err := json.Marshal(map[string]any{
		"picks": []any{
			map[string]any{"mediaType": "movie", "tmdbId": curated.TMDBID, "name": curated.Name},
			map[string]any{"mediaType": "movie", "tmdbId": unrelated.TMDBID, "name": unrelated.Name},
		},
		"dateMeaning": meaning,
	})
	if err != nil {
		t.Fatal(err)
	}
	model := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"query": unrelated.Name, "dateMeaning": meaning}),
		testkit.FinalResponse(string(final)),
	)
	proposal, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{
		Description: "Classic The Matrix",
		MustInclude: []string{"Unrelated Movie"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Lineup) != 1 || proposal.Lineup[0].TMDBID != curated.TMDBID {
		t.Fatalf("lineup = %#v, want only exact curated identity", proposal.Lineup)
	}
}

func TestSuggest_DateSourceOrderReferenceInitialMalformedFinalDoesNotResolveUntilRepair(t *testing.T) {
	references := &testkit.ReferenceResolver{Evidence: reference.Evidence{TitleAnchors: []string{"The Matrix"}}}
	meaning := dateMeaningNone()
	bootstrap := finalWithDateMeaning(t, meaning)
	continuation, err := json.Marshal(map[string]any{"picks": []any{}, "dateMeaning": meaning})
	if err != nil {
		t.Fatal(err)
	}
	model := testkit.NewLLM(testkit.FinalResponse("not json"), testkit.FinalResponse(bootstrap), testkit.FinalResponse(string(continuation)))
	model.OnChat = func() {
		if model.Calls == 1 && len(references.Calls()) != 0 {
			t.Fatalf("reference resolved before malformed bootstrap repaired: calls=%#v", references.Calls())
		}
	}
	_, err = buildSuggester(t, model).WithReferences(references).Suggest(context.Background(), suggest.Intent{Description: "https://lineups.example/friday"})
	if err == nil || len(references.Calls()) != 1 {
		t.Fatalf("err=%v reference calls=%d, want source initialization only after repaired valid bootstrap", err, len(references.Calls()))
	}
}
