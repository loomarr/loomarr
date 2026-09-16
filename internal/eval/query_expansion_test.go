//go:build eval

package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestQueryExpansionReviewedTGIFUsesProductionSuggestion(t *testing.T) {
	cases, err := QueryExpansionCases()
	if err != nil {
		t.Fatal(err)
	}
	config, err := QueryExpansionRunnerConfig(RunnerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	provider := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{
			"media_type": "series", "mode": "collection",
			"titles":      []any{"Family Matters", "Step by Step", "Boy Meets World", "Sabrina the Teenage Witch"},
			"dateMeaning": map[string]any{"kind": "none", "anchors": []any{}, "axes": []any{}},
		}),
		testkit.FinalResponse(`{"picks":[{"mediaType":"series","key":"series:tmdb:2685"},{"mediaType":"series","key":"series:tmdb:2617"},{"mediaType":"series","key":"series:tmdb:1777"},{"mediaType":"series","key":"series:tmdb:605"}],"dateMeaning":{"kind":"none","anchors":[],"axes":[]}}`),
	)
	generator, observer, err := NewEmbeddedQueryExpansionGenerator(provider)
	if err != nil {
		t.Fatal(err)
	}
	card := NewRunner(generator, config).WithObserver(observer).Run(context.Background(), cases[:1])
	if !card.Results[0].Passed() || !card.Assessment.Passed || card.Certified || !card.DevelopmentCorpus {
		t.Fatalf("reviewed TGIF development answer: %+v", card.Results[0])
	}
}

func TestQueryExpansionIndependentCausalOutcomeControls(t *testing.T) {
	cases, err := QueryExpansionCases()
	if err != nil {
		t.Fatal(err)
	}
	config, err := QueryExpansionRunnerConfig(RunnerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	ownedAndMissing := func(keys ...string) suggest.Proposal {
		proposal := proposalWithKeys(t, keys...)
		proposal.Acquisitions, proposal.Lineup = proposal.Lineup[2:], proposal.Lineup[:2]
		return proposal
	}
	positive := ownedAndMissing("series:tmdb:2685", "series:tmdb:2617", "series:tmdb:1777", "series:tmdb:605")
	missing := ownedAndMissing("series:tmdb:2685", "series:tmdb:2617", "series:tmdb:1777")
	wrong := ownedAndMissing("series:tmdb:2685", "series:tmdb:2617", "series:tmdb:1777", "series:tmdb:31183")
	duplicate := ownedAndMissing("series:tmdb:2685", "series:tmdb:2617", "series:tmdb:1777", "series:tmdb:605", "series:tmdb:605")
	wrongOwnership := proposalWithKeys(t, "series:tmdb:2685", "series:tmdb:2617", "series:tmdb:1777", "series:tmdb:605")
	wrongEditorialDate := positive
	wrongEditorialDate.Policy.Scope.Dates = &schedule.DateScope{SeriesAiring: []schedule.Range{{From: 1990, To: 1999}}}
	airing := positive
	airing.Policy.Scope.Dates = &schedule.DateScope{SeriesAiring: []schedule.Range{{From: 1990, To: 1999}}}
	wrongAxis := positive
	wrongAxis.Policy.Scope.Dates = &schedule.DateScope{SeriesPremiere: []schedule.Range{{From: 1990, To: 1999}}}
	controls := []struct {
		name, caseID, failure string
		positive, negative    suggest.Proposal
	}{
		{"missing-anchor", "exp-tgif-minimum", "605", positive, missing},
		{"wrong-member", "exp-tgif-minimum", "31183", positive, wrong},
		{"duplicate-padding", "exp-tgif-minimum", "duplicate", positive, duplicate},
		{"sparse-results", "exp-tgif-minimum", "grounded titles", positive, proposalWithKeys(t, "series:tmdb:605")},
		{"wrong-ownership", "exp-tgif-minimum", "acquisitions", positive, wrongOwnership},
		{"invented-playback-limit", "exp-tgif-editorial-epoch", "date", positive, wrongEditorialDate},
		{"outside-library", "exp-tgif-owned", "acceptable", proposalWithKeys(t, "series:tmdb:2685", "series:tmdb:2617"), positive},
		{"wrong-date-axis", "exp-tgif-airing", "date", airing, wrongAxis},
	}
	for _, control := range controls {
		t.Run(control.name, func(t *testing.T) {
			var selected []Case
			for _, c := range cases {
				if c.Name == control.caseID {
					selected = append(selected, c)
				}
			}
			if len(selected) != 1 {
				t.Fatal("causal control requires exactly one authored request")
			}
			good := NewRunner(scriptedGenerator{proposal: control.positive}, config).Run(context.Background(), selected)
			if !good.Results[0].Passed() || !good.Assessment.Passed || good.Certified {
				t.Fatalf("independently valid positive failed: %+v", good.Results[0])
			}
			bad := NewRunner(scriptedGenerator{proposal: control.negative}, config).Run(context.Background(), selected)
			if control.failure == "date" {
				if bad.Results[0].PolicyAccurate || bad.Assessment.Passed || bad.Certified {
					t.Fatalf("wrong date policy escaped strict assessment: %+v", bad)
				}
				return
			}
			if bad.Results[0].Passed() || !strings.Contains(strings.Join(bad.Results[0].Failures, " "), control.failure) || bad.Certified {
				t.Fatalf("deliberately wrong answer escaped its target: %+v", bad.Results[0])
			}
		})
	}
}

func TestQueryExpansionEveryAuthoredRequestUsesProductionSuggestion(t *testing.T) {
	corpus, err := LoadEmbeddedQueryExpansionCorpus()
	if err != nil {
		t.Fatal(err)
	}
	if len(corpus.Cases) != 30 {
		t.Fatalf("executable batch has %d requests, want 30", len(corpus.Cases))
	}
	cases, err := QueryExpansionCases()
	if err != nil {
		t.Fatal(err)
	}
	config, err := QueryExpansionRunnerConfig(RunnerConfig{Profile: "query_expansion_scripted", Generator: ModelIdentity{Provider: "fixture", Model: "cooperative"}})
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			generator, observer, err := NewEmbeddedQueryExpansionGenerator(testkit.NewLLM(queryExpansionResponses(t, corpus.Cases[i])...))
			if err != nil {
				t.Fatal(err)
			}
			card := NewRunner(generator, config).WithObserver(observer).Run(context.Background(), []Case{c})
			if !card.Results[0].Passed() || !card.Assessment.Passed || card.Certified || !card.DevelopmentCorpus {
				t.Fatalf("authored request: stage=%s failures=%v assessment=%v selected=%d", card.Results[0].FailureStage, card.Results[0].Failures, card.Assessment.Failures, card.Results[0].SelectedCount)
			}
		})
	}
}

// Provider replies are authored independently of the manifest's allowed keys,
// required anchors, minimum counts and date oracles. Only request text is read
// to locate the exact wire-protocol date span.
func queryExpansionResponses(t *testing.T, c QueryPilotCase) []llm.Response {
	t.Helper()
	ids := []int{2685, 2617, 1777, 605}
	args := map[string]any{"media_type": "series", "mode": "collection", "titles": []any{"Family Matters", "Step by Step", "Boy Meets World", "Sabrina the Teenage Witch"}}
	switch c.ID {
	case "exp-tgif-without-sabrina":
		ids = []int{2685, 2617, 1777}
	case "exp-tgif-without-family", "exp-tgif-without-family-text":
		ids = []int{2617, 1777, 605}
	case "exp-tgif-owned", "exp-tgif-no-additions":
		ids = []int{2685, 2617}
	case "exp-sabrina-minimum", "exp-sabrina-conversation", "exp-sabrina-sitcom", "exp-sabrina-typo", "exp-sabrina-airing":
		ids = []int{605}
		args = map[string]any{"media_type": "series", "query": "Sabrina the Teenage Witch"}
	case "exp-sabrina-and-boy":
		ids = []int{605, 1777}
	case "exp-family-only":
		ids = []int{2685}
		args = map[string]any{"media_type": "series", "query": "Family Matters"}
	case "exp-family-and-sabrina":
		ids = []int{2685, 605}
	case "exp-tgif-without-step":
		ids = []int{2685, 1777, 605}
	case "exp-tgif-without-two":
		ids = []int{1777, 605}
	}
	meaning := map[string]any{"kind": "none", "anchors": []any{}, "axes": []any{}}
	marker, from, to := "", 0, 0
	switch c.ID {
	case "exp-tgif-airing", "exp-tgif-airing-conversation":
		marker, from, to = "1990s", 1990, 1999
	case "exp-tgif-airing-narrow":
		marker, from, to = "1996-1999", 1996, 1999
	case "exp-tgif-airing-inclusive":
		marker, from, to = "1989-2000", 1989, 2000
	case "exp-sabrina-airing":
		marker, from, to = "1996-2000", 1996, 2000
	}
	if marker != "" {
		start := strings.Index(c.Description, marker)
		if start < 0 {
			t.Fatal("independent provider script has no date span")
		}
		meaning = map[string]any{"kind": "constraints", "anchors": []any{map[string]any{"field": "description", "start": start, "end": start + len(marker)}}, "axes": []any{map[string]any{"kind": "series_airing", "combine": "any", "intervals": []any{map[string]any{"anchor": 0, "start": from, "end": to}}}}}
	}
	args["dateMeaning"] = meaning
	picks := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		picks = append(picks, map[string]any{"mediaType": "series", "key": fmt.Sprintf("series:tmdb:%d", id)})
	}
	final, err := json.Marshal(map[string]any{"picks": picks, "dateMeaning": meaning})
	if err != nil {
		t.Fatal(err)
	}
	// Match provider-decoded JSON numbers/arrays, not Go-only wire shapes.
	wire, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(wire, &args); err != nil {
		t.Fatal(err)
	}
	return []llm.Response{testkit.ToolCallResponse("catalog_search", args), testkit.FinalResponse(string(final))}
}
