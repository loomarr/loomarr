package suggest_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/testkit/catalogfixture"
)

// A single-slot llama.cpp server renders the tools before the system prompt, so a turn that
// omits them re-prefills the whole conversation. Every turn — search, finalization and the
// repair — must carry the same tools bytes; finalization is enforced by tool_choice none.
func TestSuggest_EveryTurnSendsIdenticalToolsAndFinalizesWithToolChoiceNone(t *testing.T) {
	meaning := dateMeaningNone()
	model := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"genres": []any{"action"}, "dateMeaning": meaning}),
		testkit.FinalResponse(""),
		testkit.FinalResponse(finalWithDateMeaning(t, meaning)),
	)
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
	if _, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "action films"}); err != nil {
		t.Fatal(err)
	}
	if len(model.AllOpts) != 3 {
		t.Fatalf("calls=%d, want 3", len(model.AllOpts))
	}
	first, err := json.Marshal(model.AllOpts[0].Tools)
	if err != nil {
		t.Fatal(err)
	}
	if len(model.AllOpts[0].Tools) == 0 {
		t.Fatal("first turn sent no tools")
	}
	wantChoice := []string{"", llm.ToolChoiceNone, llm.ToolChoiceNone}
	wantJSON := []bool{false, true, true}
	for i, opts := range model.AllOpts {
		got, err := json.Marshal(opts.Tools)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(first) {
			t.Errorf("turn %d tools differ from turn 0; the cached prefix is invalidated", i)
		}
		if opts.ToolChoice != wantChoice[i] || opts.JSONMode != wantJSON[i] {
			t.Errorf("turn %d: tool_choice=%q json=%v, want %q %v", i, opts.ToolChoice, opts.JSONMode, wantChoice[i], wantJSON[i])
		}
	}
}

// canCallTools reports whether a request lets the model call a tool. Tools may be on the wire
// (to keep the cached prefix) while tool_choice none forbids using them.
func canCallTools(opts llm.ChatOptions) bool {
	return len(opts.Tools) > 0 && opts.ToolChoice != llm.ToolChoiceNone
}
