package suggest_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/testkit/catalogfixture"
)

// A grounded tool result must reach the finalizer with the complete accepted
// interpretation. The live failure returned empty turns or lost interval anchor
// coordinates while reconstructing that state from the earlier tool request.
func TestSuggest_FinalizationCarriesAcceptedMeaningThroughRepair(t *testing.T) {
	for _, dated := range []bool{false, true} {
		name := "none"
		meaning := dateMeaningNone()
		intent := suggest.Intent{Description: "action films"}
		if dated {
			name = "dated"
			meaning = dateMeaning1990()
			intent.Description = "1990s action films"
		}
		t.Run(name, func(t *testing.T) {
			model := testkit.NewLLM(
				testkit.ToolCallResponse("catalog_search", map[string]any{"genres": []any{"action"}, "dateMeaning": meaning}),
				testkit.FinalResponse(""),
				testkit.FinalResponse(finalWithDateMeaning(t, meaning)),
			)
			model.OnChat = func() {
				if model.Calls == 2 {
					previousFinal := model.LastMessages[len(model.LastMessages)-1]
					if !strings.HasPrefix(previousFinal.Content, "Retrieval is complete and no further tools are available.") {
						t.Error("first finalization request omitted explicit completion state")
					}
				}
			}
			corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
			proposal, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), intent)
			if err != nil {
				t.Fatal(err)
			}
			if len(proposal.Lineup) != 1 || model.Calls != 3 {
				t.Fatalf("lineup=%v calls=%d", proposal.Lineup, model.Calls)
			}
			if len(model.LastOpts.Tools) != 0 || !model.LastOpts.JSONMode {
				t.Fatal("repair re-enabled retrieval")
			}
			notes := 0
			for _, message := range model.LastMessages {
				if strings.HasPrefix(message.Content, "Retrieval is complete and no further tools are available.") {
					notes++
				}
			}
			if notes != 1 {
				t.Fatalf("finalization notes=%d, want one request-only note", notes)
			}
			last := model.LastMessages[len(model.LastMessages)-1]
			if last.Role != llm.User {
				t.Fatalf("finalization role=%s", last.Role)
			}
			start := strings.IndexByte(last.Content, '{')
			if start < 0 {
				t.Fatal("finalization omitted accepted dateMeaning JSON")
			}
			var actual map[string]any
			if err := json.Unmarshal([]byte(last.Content[start:]), &actual); err != nil {
				t.Fatal(err)
			}
			blob, err := json.Marshal(meaning)
			if err != nil {
				t.Fatal(err)
			}
			var expected map[string]any
			if err := json.Unmarshal(blob, &expected); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(actual, expected) {
				t.Fatalf("finalization meaning=%v, want %v", actual, expected)
			}
		})
	}
}
