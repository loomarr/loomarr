//go:build eval

package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestRunnerDevelopmentEvidenceCannotCertify(t *testing.T) {
	proposal := suggest.Proposal{Lineup: []suggest.ProposalItem{{MediaType: provision.Series, TMDBID: 3921, Name: "Sabrina the Teenage Witch"}}}
	card := NewRunner(scriptedGenerator{proposal: proposal}, RunnerConfig{DevelopmentCorpus: true}).Run(context.Background(), []Case{{
		Name: "development-anchor", MinGrounded: 1, RequireKeys: []provision.Key{"series:tmdb:3921"},
	}})
	if !card.Results[0].Passed() || card.Certified || !card.DevelopmentCorpus {
		t.Fatalf("development evidence was lost or certified: %+v", card)
	}
	blob, err := json.Marshal(card)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Development bool `json:"developmentCorpus"`
		Certified   bool `json:"certified"`
	}
	if err := json.Unmarshal(blob, &wire); err != nil {
		t.Fatal(err)
	}
	if !wire.Development || wire.Certified {
		t.Fatalf("development wire identity = %s", blob)
	}
}

func TestQueryPilotAllAuthoredCasesUseProductionProtocol(t *testing.T) {
	corpus, err := LoadEmbeddedQueryPilotCorpus()
	if err != nil {
		t.Fatal(err)
	}
	cases, err := QueryPilotCases()
	if err != nil {
		t.Fatal(err)
	}
	config, err := QueryPilotRunnerConfig(RunnerConfig{Profile: "query_pilot_scripted", Generator: ModelIdentity{Provider: "fixture", Model: "scripted"}})
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			provider := testkit.NewLLM(queryPilotResponses(t, corpus.Cases[i])...)
			generator, observer, err := NewEmbeddedQueryPilotGenerator(provider)
			if err != nil {
				t.Fatal(err)
			}
			card := NewRunner(generator, config).WithObserver(observer).Run(context.Background(), []Case{c})
			if !card.Results[0].Passed() || !card.Assessment.Passed || card.Certified {
				result := card.Results[0]
				t.Fatalf("authored case: stage=%s failures=%v assessment=%v policyAccurate=%v selected=%d", result.FailureStage, result.Failures, card.Assessment.Failures, result.PolicyAccurate, result.SelectedCount)
			}
		})
	}
}

// Independent cooperative model scripts exercise application behavior only.
// Picks and tool inputs are fixed here, not derived from the grader expectations.
func queryPilotResponses(t *testing.T, c QueryPilotCase) []llm.Response {
	t.Helper()
	args := map[string]any{"media_type": "series"}
	var ids []int
	switch c.Group {
	case "tgif-membership":
		args["mode"], args["titles"] = "collection", []string{"Full House", "Family Matters", "Step by Step", "Perfect Strangers", "Boy Meets World", "Sabrina the Teenage Witch"}
		ids = []int{20101, 20102, 20103, 20104, 20105, 20106}
		if c.ID == "tgif-exclude" {
			ids = []int{20102, 20103, 20104, 20105, 20106}
		}
	case "history-editorial-epoch":
		args["network"] = "History"
		ids = []int{50001, 50002, 50003, 50004, 50005, 50006}
	case "broad-comedy":
		args["genres"] = []string{"Comedy"}
		ids = []int{20102, 20301, 20302, 20303}
		if c.ID == "comedy-6" {
			ids = []int{20102, 20302, 20303}
		}
	case "movie-release-era":
		args["media_type"], args["genres"] = "movie", []string{"Action"}
		ids = []int{20401, 20402, 20403}
	case "matrix-franchise":
		args["media_type"], args["mode"], args["titles"] = "movie", "collection", []string{"The Matrix", "The Matrix Reloaded", "The Matrix Revolutions", "The Animatrix"}
		ids = []int{21201, 21202, 21203, 21204}
		if c.ID == "matrix-6" {
			ids = []int{21201, 21202, 21203}
		}
	case "mcu-phase-one":
		args["media_type"], args["mode"], args["titles"] = "movie", "collection", []string{"Iron Man", "The Incredible Hulk", "Iron Man 2", "Thor", "Captain America: The First Avenger", "The Avengers"}
		ids = []int{21101, 21102, 21103, 21104, 21105, 21106}
		if c.ID == "mcu-6" {
			ids = []int{21101, 21103, 21104, 21105, 21106}
		}
	case "tom-hanks-era":
		args["media_type"], args["cast"] = "movie", []string{"Tom Hanks"}
		ids = []int{21301, 21302, 21303, 21304}
		if c.ID == "hanks-6" {
			ids = []int{21301, 21302, 21303}
		}
	case "cozy-mysteries":
		args["keywords"] = []string{"cozy", "mystery"}
		ids = []int{21501, 21502, 21503}
	case "apple-science-fiction":
		args["network"], args["genres"] = "Apple TV+", []string{"Science Fiction"}
		ids = []int{21701, 21702, 21703, 21704}
		if c.ID == "apple-6" {
			ids = []int{21701, 21702, 21703}
		}
	case "explicit-title-exclusion":
		args["keywords"] = []string{"cozy", "mystery"}
		ids = []int{21502, 21503}
		if c.ID == "exclude-6" {
			ids = []int{21503}
		}
	case "library-ownership":
		args["keywords"] = []string{"cozy", "mystery"}
		ids = []int{51001, 51002, 51003}
		if c.ID == "owned-6" {
			ids = []int{51001, 51002}
		}
	case "exact-named-shows":
		args["query"] = "Full House"
		ids = []int{20101}
		if c.ID == "named-6" {
			ids = []int{20101, 20106}
		}
	default:
		t.Fatalf("no independent script for group %q", c.Group)
	}
	meaning := map[string]any{"kind": "none", "anchors": []any{}, "axes": []any{}}
	axis, marker, from, to := "", "1990s", 1990, 1999
	if c.Group == "broad-comedy" {
		axis = "series_airing"
	}
	if c.Group == "movie-release-era" || c.Group == "tom-hanks-era" {
		axis = "movie_release"
	}
	if c.ID == "history-explicit-airing" {
		axis, marker, from, to = "series_airing", "2000s", 2000, 2009
	}
	if axis != "" {
		start := strings.Index(c.Description, marker)
		if start < 0 {
			t.Fatalf("independent script date marker absent in %q", c.Description)
		}
		meaning = map[string]any{"kind": "constraints", "anchors": []any{map[string]any{"field": "description", "start": start, "end": start + len(marker)}}, "axes": []any{map[string]any{"kind": axis, "combine": "any", "intervals": []any{map[string]any{"anchor": 0, "start": from, "end": to}}}}}
	}
	// Real provider tool JSON decodes arrays to []any. Round-trip the fixed
	// scripts into that same wire shape instead of passing Go-only []string.
	args["dateMeaning"] = meaning
	wireArgs, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(wireArgs, &args); err != nil {
		t.Fatal(err)
	}
	picks := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		picks = append(picks, map[string]any{"mediaType": args["media_type"], "key": fmt.Sprintf("%s:tmdb:%d", args["media_type"], id)})
	}
	final, err := json.Marshal(map[string]any{"picks": picks, "dateMeaning": meaning})
	if err != nil {
		t.Fatal(err)
	}
	return []llm.Response{testkit.ToolCallResponse("catalog_search", args), testkit.FinalResponse(string(final))}
}

func TestQueryPilotHistoryUsesProductionSuggester(t *testing.T) {
	cases, err := QueryPilotCases()
	if err != nil {
		t.Fatal(err)
	}
	var epoch Case
	for _, c := range cases {
		if c.Name == "history-original" {
			epoch = c
		}
	}
	meaning := map[string]any{"kind": "none", "anchors": []any{}, "axes": []any{}}
	provider := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"network": "History", "media_type": "series", "dateMeaning": meaning}),
		testkit.FinalResponse(`{"picks":[{"mediaType":"series","key":"series:tmdb:50001"},{"mediaType":"series","key":"series:tmdb:50002"},{"mediaType":"series","key":"series:tmdb:50003"},{"mediaType":"series","key":"series:tmdb:50004"},{"mediaType":"series","key":"series:tmdb:50005"},{"mediaType":"series","key":"series:tmdb:50006"}],"dateMeaning":{"kind":"none","anchors":[],"axes":[]}}`),
	)
	generator, observer, err := NewEmbeddedQueryPilotGenerator(provider)
	if err != nil {
		t.Fatal(err)
	}
	config, err := QueryPilotRunnerConfig(RunnerConfig{Profile: "query_pilot_scripted", Generator: ModelIdentity{Provider: "fixture", Model: "scripted"}})
	if err != nil {
		t.Fatal(err)
	}
	card := NewRunner(generator, config).WithObserver(observer).Run(context.Background(), []Case{epoch})
	if !card.Results[0].Passed() || !card.Assessment.Passed || card.Certified {
		t.Fatalf("production epoch pilot = %+v", card)
	}
	if card.Results[0].NetworkCalls != 1 || card.Results[0].CatalogOperations == 0 || card.Results[0].SelectedCount != 6 {
		t.Fatalf("missing production observation: %+v", card.Results[0])
	}
}

func TestQueryPilotCoverageIsAuthoredAndExplicitlyDevelopment(t *testing.T) {
	corpus, err := LoadEmbeddedQueryPilotCorpus()
	if err != nil {
		t.Fatal(err)
	}
	groups := map[string]bool{}
	types := map[string]int{}
	for _, c := range corpus.Cases {
		groups[c.Group] = true
		types[c.Type]++
	}
	if corpus.Split != "development" || len(corpus.Cases) != 72 || len(groups) != 12 || types["minimum"] != 12 || types["directional"] < 12 {
		t.Fatalf("pilot coverage: split=%s cases=%d groups=%d types=%v", corpus.Split, len(corpus.Cases), len(groups), types)
	}
}

func TestQueryPilotCausalOutcomeControls(t *testing.T) {
	cases, err := QueryPilotCases()
	if err != nil {
		t.Fatal(err)
	}
	config, err := QueryPilotRunnerConfig(RunnerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	owned := proposalWithKeys(t, "series:tmdb:51001", "series:tmdb:51002", "series:tmdb:51003")
	owned.Acquisitions, owned.Lineup = owned.Lineup[2:], owned.Lineup[:2]
	controls := []struct {
		caseID             string
		positive, negative suggest.Proposal
		failure            string
	}{
		{"history-original", proposalWithKeys(t, "series:tmdb:50001", "series:tmdb:50002", "series:tmdb:50003", "series:tmdb:50004", "series:tmdb:50005", "series:tmdb:50006"), proposalWithKeys(t, "series:tmdb:50001"), "grounded titles"},
		{"tgif-terse", proposalWithKeys(t, "series:tmdb:20101", "series:tmdb:20102", "series:tmdb:20103", "series:tmdb:20104", "series:tmdb:20105", "series:tmdb:20106"), proposalWithKeys(t, "series:tmdb:20101", "series:tmdb:20102", "series:tmdb:20103", "series:tmdb:20104", "series:tmdb:20105", "series:tmdb:20201"), "20201"},
		{"owned-1", owned, proposalWithKeys(t, "series:tmdb:51001", "series:tmdb:51002", "series:tmdb:51003"), "acquisitions"},
	}
	for _, control := range controls {
		t.Run(control.caseID, func(t *testing.T) {
			var selected []Case
			for _, c := range cases {
				if c.Name == control.caseID {
					selected = append(selected, c)
				}
			}
			if len(selected) != 1 {
				t.Fatal("missing causal control case")
			}
			positive := NewRunner(scriptedGenerator{proposal: control.positive}, config).Run(context.Background(), selected)
			if !positive.Results[0].Passed() || !positive.Assessment.Passed || positive.Certified {
				t.Fatalf("positive control failed: %+v", positive.Results[0])
			}
			negative := NewRunner(scriptedGenerator{proposal: control.negative}, config).Run(context.Background(), selected)
			if negative.Results[0].Passed() || !strings.Contains(strings.Join(negative.Results[0].Failures, " "), control.failure) {
				t.Fatalf("causal negative did not fail its target: %+v", negative.Results[0])
			}
		})
	}
}

func TestQueryPilotScorecardBindsAllAuthoredInputs(t *testing.T) {
	config, err := QueryPilotRunnerConfig(RunnerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if config.Contract.CorpusManifestSHA256 == "" || config.Contract.SourcesFixtureSHA256 == "" || config.Contract.SupplementalCatalogFixtureSHA256 == "" || config.Contract.SourceVersion != suggest.PlannerSourceVersion {
		t.Fatalf("pilot scorecard omits an authored input: %+v", config.Contract)
	}
}

func TestQueryPilotProductionRejectsInventedSelection(t *testing.T) {
	corpus, err := LoadEmbeddedQueryPilotCorpus()
	if err != nil {
		t.Fatal(err)
	}
	cases, err := QueryPilotCases()
	if err != nil {
		t.Fatal(err)
	}
	config, err := QueryPilotRunnerConfig(RunnerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	index := -1
	for i, c := range cases {
		if c.Name == "history-original" {
			index = i
		}
	}
	if index < 0 {
		t.Fatal("missing production grounding control")
	}
	responses := queryPilotResponses(t, corpus.Cases[index])
	responses[len(responses)-1] = testkit.FinalResponse(`{"picks":[{"mediaType":"series","key":"series:tmdb:99999999"}],"dateMeaning":{"kind":"none","anchors":[],"axes":[]}}`)
	generator, observer, err := NewEmbeddedQueryPilotGenerator(testkit.NewLLM(responses...))
	if err != nil {
		t.Fatal(err)
	}
	card := NewRunner(generator, config).WithObserver(observer).Run(context.Background(), cases[index:index+1])
	if card.Results[0].Passed() || card.Results[0].Lineup+card.Results[0].Acquisitions != 0 || card.Certified {
		t.Fatalf("invented selection was accepted: %+v", card.Results[0])
	}
}

func TestQueryPilotHistoryEpochRejectsUnwantedEpisodeDates(t *testing.T) {
	cases, err := QueryPilotCases()
	if err != nil {
		t.Fatal(err)
	}
	var epoch Case
	for _, c := range cases {
		if c.Name == "history-original" {
			epoch = c
		}
	}
	if epoch.Name == "" {
		t.Fatal("original History-era regression is absent")
	}
	proposal := proposalWithKeys(t, "series:tmdb:50001", "series:tmdb:50002", "series:tmdb:50003", "series:tmdb:50004", "series:tmdb:50005", "series:tmdb:50006")
	proposal.Policy.Scope.Dates = &schedule.DateScope{SeriesAiring: []schedule.Range{{From: 1990, To: 1999}}}
	config, err := QueryPilotRunnerConfig(RunnerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	card := NewRunner(scriptedGenerator{proposal: proposal}, config).Run(context.Background(), []Case{epoch})
	if card.Assessment.Passed || card.Results[0].PolicyAccurate {
		t.Fatalf("editorial epoch accepted episode cutoff: %+v", card)
	}
	proposal.Policy.Scope.Dates = nil
	positive := NewRunner(scriptedGenerator{proposal: proposal}, config).Run(context.Background(), []Case{epoch})
	if !positive.Assessment.Passed || !positive.Results[0].Passed() {
		t.Fatalf("independent no-dates control failed: %+v", positive)
	}
}

func TestRunnerPilotRejectsDuplicateChoices(t *testing.T) {
	item := suggest.ProposalItem{MediaType: provision.Series, TMDBID: 3921, Name: "Sabrina the Teenage Witch"}
	card := NewRunner(scriptedGenerator{proposal: suggest.Proposal{Lineup: []suggest.ProposalItem{item, item}}}, RunnerConfig{}).Run(context.Background(), []Case{{
		Name: "duplicate-choices", MinGrounded: 2, RequireUniqueKeys: true,
	}})
	if card.Results[0].Passed() || !strings.Contains(strings.Join(card.Results[0].Failures, " "), "duplicate grounded key") {
		t.Fatalf("duplicate choices passed: %+v", card.Results[0])
	}
}

func TestRunnerPilotCannotPadAnAcceptableLineupWithAnUnclassifiedTitle(t *testing.T) {
	proposal := proposalWithKeys(t, "series:tmdb:20101", "series:tmdb:20106", "series:tmdb:20201")
	card := NewRunner(scriptedGenerator{proposal: proposal}, RunnerConfig{}).Run(context.Background(), []Case{{
		Name: "unclassified-padding", AcceptableKeys: []provision.Key{"series:tmdb:20101", "series:tmdb:20106"}, MinAcceptableKeys: 2, OnlyAcceptableKeys: true,
	}})
	if card.Results[0].Passed() || !strings.Contains(strings.Join(card.Results[0].Failures, " "), "outside the acceptable set") {
		t.Fatalf("unclassified padding passed: %+v", card.Results[0])
	}
}

func TestQueryPilotSabrinaOmissionIsAnActualFailure(t *testing.T) {
	cases, err := QueryPilotCases()
	if err != nil {
		t.Fatal(err)
	}
	var anchor Case
	for _, c := range cases {
		if c.Name == "tgif-sabrina" {
			anchor = c
		}
	}
	if anchor.Name == "" {
		t.Fatal("authored Sabrina case is absent")
	}
	proposal := proposalWithKeys(t, "series:tmdb:20101", "series:tmdb:20102", "series:tmdb:20103", "series:tmdb:20104", "series:tmdb:20105")
	config, err := QueryPilotRunnerConfig(RunnerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	card := NewRunner(scriptedGenerator{proposal: proposal}, config).Run(context.Background(), []Case{anchor})
	if card.Results[0].Passed() || !strings.Contains(strings.Join(card.Results[0].Failures, " "), "20106") {
		t.Fatalf("Sabrina omission was accepted: %+v", card.Results[0])
	}
	proposal.Lineup = append(proposal.Lineup, proposalWithKeys(t, "series:tmdb:20106").Lineup...)
	positive := NewRunner(scriptedGenerator{proposal: proposal}, config).Run(context.Background(), []Case{anchor})
	if !positive.Results[0].Passed() || !positive.Assessment.Passed || positive.Certified {
		t.Fatalf("positive development control: %+v", positive)
	}
}
