package suggest_test

import (
	"context"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/reference"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/testkit/catalogfixture"
)

type namedSetToolAvailabilityLLM struct {
	calls int
}

func (m *namedSetToolAvailabilityLLM) Name() string { return "named-set-tool-availability" }

func (m *namedSetToolAvailabilityLLM) Chat(_ context.Context, _ []llm.Message, opts llm.ChatOptions) (llm.Response, error) {
	m.calls++
	if len(opts.Tools) > 0 {
		return testkit.ToolCallResponse("catalog_search", map[string]any{
			"mode": "collection", "media_type": "movie", "titles": []any{"Model Guess"},
			"dateMeaning": dateMeaningNone(),
		}), nil
	}
	return testkit.FinalResponse(`{"channelName":"Friday Night","rationale":"A verified named block.","dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[{"mediaType":"movie","key":"movie:tmdb:603","name":"The Matrix","rationale":"A verified constituent.","confidence":0.95}],"policy":{}}`), nil
}

func TestSuggestDiscoversNamedBlockSourceOnce(t *testing.T) {
	for _, request := range []string{"TGIF", "TGIF block", "Make a channel like TGIF", "a channel based on TGIF"} {
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
