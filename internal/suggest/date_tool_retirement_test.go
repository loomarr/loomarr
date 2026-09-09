package suggest_test

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/testkit/catalogfixture"
)

func TestSuggest_RetiredEraCannotBeProjectedAway(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{matrixCandidate()}}
	model := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{
			"media_type": "series", "network": "ABC", "era": "1990s", "dateMeaning": dateMeaningNone(),
		}),
		testkit.ToolCallResponse("catalog_search", map[string]any{
			"query": "The Matrix", "dateMeaning": dateMeaningNone(),
		}),
		testkit.FinalResponse(finalWithDateMeaning(t, dateMeaningNone())),
	)
	model.OnChat = func() {
		if model.Calls == 1 && (len(corpus.Searches()) != 0 || len(corpus.Discoveries()) != 0) {
			t.Fatalf("invalid raw era call reached sources: searches=%#v discoveries=%#v", corpus.Searches(), corpus.Discoveries())
		}
	}

	proposal, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{
		Description: "A named programming block",
		MustInclude: []string{"The Matrix"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Lineup) != 1 || proposal.Lineup[0].Name != "The Matrix" {
		t.Fatalf("lineup = %#v, want The Matrix only", proposal.Lineup)
	}
	if got := len(corpus.Searches()); got != 2 {
		t.Fatalf("searches = %d, want 2", got)
	}
	if got := len(corpus.Discoveries()); got != 0 {
		t.Fatalf("discoveries = %d, want 0", got)
	}
}
