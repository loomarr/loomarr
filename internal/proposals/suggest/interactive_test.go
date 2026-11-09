package suggest

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/library/titlesearch"
	"github.com/loomarr/loomarr/internal/platform/llm"
	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/testkit/catalogfixture"
)

// gateObservingLLM records whether the interactive gate was busy while each model turn ran.
type gateObservingLLM struct {
	*testkit.LLM
	gate *llm.InteractiveGate
	busy []bool
}

func (m *gateObservingLLM) Chat(ctx context.Context, messages []llm.Message, opts llm.ChatOptions) (llm.Response, error) {
	m.busy = append(m.busy, m.gate.Busy())
	return m.LLM.Chat(ctx, messages, opts)
}

// A suggestion holds the interactive gate for its whole run, so background vision yields the AI
// server's slots to it (#1508), and releases it when the run returns.
func TestSuggestHoldsTheInteractiveGateForItsRun(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []titlesearch.Candidate{{
		MediaType: provision.Series, TMDBID: 90021502, TVDBID: 90269680, Name: "Murmur Bay", Year: 2013, InLibrary: true,
	}}}
	gate := &llm.InteractiveGate{}
	model := &gateObservingLLM{gate: gate, LLM: testkit.NewLLM(testkit.FinalResponse(
		`{"picks":[{"mediaType":"series","name":"Murmur Bay","year":2013}],"dateMeaning":{"kind":"none","anchors":[],"axes":[]}}`))}
	s := New(model, titlesearch.New(nil, corpus), nil, 3).WithInteractiveGate(gate)
	if _, err := s.Suggest(context.Background(), proposals.Intent{Description: "mysteries"}); err != nil {
		t.Fatal(err)
	}
	if len(model.busy) == 0 {
		t.Fatal("the model was never asked")
	}
	for i, busy := range model.busy {
		if !busy {
			t.Errorf("model turn %d ran with the interactive gate idle", i)
		}
	}
	if gate.Busy() {
		t.Error("the gate stayed busy after the suggestion returned")
	}
}
