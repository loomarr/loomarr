package suggest_test

import (
	"context"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/reference"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/testkit/catalogfixture"
)

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
