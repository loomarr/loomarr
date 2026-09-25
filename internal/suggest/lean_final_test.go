package suggest_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/testkit/catalogfixture"
)

// #1487: the final turn is the slowest step of "add a channel" (generation is ~24 tok/s), so it
// carries only what the server cannot re-derive. After a tool call the server already holds the
// accepted dateMeaning and every surfaced candidate's name and media type.
func TestSuggest_LeanFinalAfterToolCallOmitsDateMeaningNameAndMediaType(t *testing.T) {
	model := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"genres": []any{"action"}, "dateMeaning": dateMeaning1990()}),
		testkit.FinalResponse(`{"channelName":"Neo Nights","rationale":"r","picks":[{"key":"movie:tmdb:603","rationale":"fits","confidence":0.9}]}`),
	)
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
	proposal, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "1990s action films"})
	if err != nil {
		t.Fatal(err)
	}
	if model.Calls != 2 {
		t.Fatalf("calls=%d, want 2 (a lean final must not cost a repair turn)", model.Calls)
	}
	if len(proposal.Lineup) != 1 || proposal.Lineup[0].Name != "The Matrix" || proposal.Lineup[0].MediaType != "movie" {
		t.Fatalf("lineup=%+v, want The Matrix re-derived from the surfaced candidate", proposal.Lineup)
	}
}

// max_tokens is sized to the turn: a search call is a few hundred tokens and a final lineup about
// 300-600, so neither needs the old 2048. On a slow model the cap is what bounds a runaway turn.
func TestSuggest_MaxTokensAreSizedPerTurn(t *testing.T) {
	model := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"genres": []any{"action"}, "dateMeaning": dateMeaningNone()}),
		testkit.FinalResponse(finalWithDateMeaning(t, dateMeaningNone())),
	)
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
	if _, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "action films"}); err != nil {
		t.Fatal(err)
	}
	search, final := model.AllOpts[0].MaxTokens, model.AllOpts[1].MaxTokens
	if search <= 0 || search > 768 || final <= search || final > 1280 {
		t.Fatalf("max_tokens search=%d final=%d, want 0 < search <= 768 and search < final <= 1280", search, final)
	}
}

// The cached prefix is the tools plus the system prompt: nothing that varies per request may sit
// before the user turn, or a second channel request re-prefills ~5k tokens (#1403, #1487).
func TestSuggest_PrefixIsByteIdenticalAcrossIntents(t *testing.T) {
	prefix := func(description string) (string, string) {
		model := testkit.NewLLM(testkit.FinalResponse(""), testkit.FinalResponse(""), testkit.FinalResponse(""))
		corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
		_, _ = dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: description, Era: "1990s"})
		tools, err := json.Marshal(model.AllOpts[0].Tools)
		if err != nil {
			t.Fatal(err)
		}
		return model.LastMessages[0].Content, string(tools)
	}
	sysA, toolsA := prefix("cozy mysteries")
	sysB, toolsB := prefix("Saturday morning cartoons for kids")
	if sysA != sysB || toolsA != toolsB {
		t.Fatal("system prompt or tools vary with the request; the cached prefix is invalidated")
	}
}

// The omission is only a convenience for an ACCEPTED meaning: a final that states a different one
// is still rejected, and a final with no tool call still has to state it.
func TestSuggest_FinalStillMustNotContradictAcceptedDateMeaning(t *testing.T) {
	model := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"genres": []any{"action"}, "dateMeaning": dateMeaning1990()}),
		testkit.FinalResponse(finalWithDateMeaning(t, dateMeaningNone())),
	)
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
	if _, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "1990s action films"}); err == nil {
		t.Fatal("a final contradicting the accepted dateMeaning was accepted")
	}
}

func TestSuggest_DirectFinalWithoutToolCallStillRequiresDateMeaning(t *testing.T) {
	model := testkit.NewLLM(
		testkit.FinalResponse(`{"picks":[{"mediaType":"movie","key":"movie:tmdb:603","name":"The Matrix"}]}`),
	)
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
	if _, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "action films"}); err == nil {
		t.Fatal("a final with no dateMeaning and no accepted tool meaning was accepted")
	}
}
