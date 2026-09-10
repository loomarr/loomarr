package suggest_test

import (
	"context"
	"encoding/json"
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

func TestSuggest_ReferenceInterpretationPhaseSurvivesRepairThenEnds(t *testing.T) {
	for _, dated := range []bool{false, true} {
		name, meaning := "none", dateMeaningNone()
		intent := suggest.Intent{Description: "Use https://lineups.example/friday"}
		if dated {
			name, meaning = "dated", dateMeaning1990()
			intent.Description = "1990s films from https://lineups.example/friday"
		}
		t.Run(name, func(t *testing.T) {
			bootstrap, err := json.Marshal(map[string]any{"dateMeaning": meaning, "picks": []any{}})
			if err != nil {
				t.Fatal(err)
			}
			model := testkit.NewLLM(testkit.FinalResponse(""), testkit.FinalResponse(string(bootstrap)), testkit.FinalResponse(finalWithDateMeaning(t, meaning)))
			references := &testkit.ReferenceResolver{Evidence: reference.Evidence{TitleAnchors: []string{"The Matrix"}}}
			corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
			const prefix = "This is the reference intent-interpretation phase."
			model.OnChat = func() {
				if model.Calls == 1 || model.Calls == 2 {
					notes := 0
					for _, m := range model.LastMessages {
						if strings.HasPrefix(m.Content, prefix) {
							notes++
						}
					}
					last := model.LastMessages[len(model.LastMessages)-1]
					if notes != 1 || !strings.HasPrefix(last.Content, prefix) || last.Role != llm.User {
						t.Errorf("interpretation request %d has %d phase notes; last=%s", model.Calls, notes, last.Role)
					}
					if len(model.LastOpts.Tools) != 0 || !model.LastOpts.JSONMode {
						t.Error("interpretation enabled tools or omitted JSON mode")
					}
				}
				if model.Calls < 2 && (len(references.Calls()) != 0 || len(corpus.Searches()) != 0) {
					t.Error("invalid interpretation dispatched source work")
				}
			}
			proposal, err := dateExecutionSuggester(model, corpus).WithReferences(references).Suggest(context.Background(), intent)
			if err != nil {
				t.Fatal(err)
			}
			if model.Calls != 3 || len(proposal.Lineup) != 1 || len(references.Calls()) != 1 {
				t.Fatalf("calls=%d lineup=%v sources=%d", model.Calls, proposal.Lineup, len(references.Calls()))
			}
			for _, m := range model.LastMessages {
				if strings.HasPrefix(m.Content, prefix) {
					t.Fatal("interpretation note leaked into evidence-backed finalization")
				}
			}
		})
	}
}

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
