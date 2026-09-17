//go:build eval

package eval

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestQueryExpansionPriorArtifactsRemainImmutable(t *testing.T) {
	want := map[string]string{
		"testdata/query-expansion-v1.json":         "1b78d1c288c1bf913b5391705a2f2054895bb8e07770b58102f911ce6fa36a83",
		"testdata/query-expansion-catalog-v1.json": "42fe1981f6d38256e08219044e17ca688436325bf012ba69db50a15a1f55d611",
		"testdata/query-expansion-sources-v1.json": "9f7a590b5db976d064f33d8e5a01bf33be0951548726a11f1ce79be1883ae849",
		"testdata/query-expansion-v2.json":         "fc86fec4b51b980a4816263468f38de4a2878e5d9df814e649d4f955f82785d8",
		"testdata/query-expansion-catalog-v2.json": "ca54b9ae981ed4d8da9409bb6ee876819342435f9ccf825ebb95e06afb43c482",
		"testdata/query-expansion-sources-v2.json": "c6784c9defa523b8fe147dc280ac31055b047ae5167390172cb8d89e81f9923a",
		"testdata/query-expansion-v3.json":         "3610462320a2a899d9718a1d8cda9e6d123708060279d173fc89d877c0767123",
		"testdata/query-expansion-catalog-v3.json": "bb5c884ea3dabbf5c9cb41203cb451a87f647c685348b92e4464c244248d612a",
		"testdata/query-expansion-v4.json":         "89fd0375a099ec52f5ae60f684724aed6c3a2ed8fb4b1d683717b2f805e92e7c",
		"testdata/query-expansion-catalog-v4.json": "f99b4b6dfbdd2ca54a7a0db0a4aeb859fdfe4d5ce9ee7a52aeff2bffaa4d47ce",
		"testdata/query-expansion-v5.json":         "e231deba1436bfa07b50e2d2632adf8d1b1b0e0f46828bf3d4d5bb39e2f46166",
		"testdata/query-expansion-catalog-v5.json": "96bb8a7bf3c2011e6124e238f2b3d0e0bfa5788db9361dfc5ca9ee1cad285589",
	}
	for path, expected := range want {
		blob, err := queryPilotFiles.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(blob)); got != expected {
			t.Fatalf("%s digest = %s, want immutable %s", path, got, expected)
		}
	}
}

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

func TestQueryExpansionReviewedMCUUsesProductionSuggestion(t *testing.T) {
	corpus, err := LoadEmbeddedQueryExpansionCorpus()
	if err != nil {
		t.Fatal(err)
	}
	cases, err := QueryExpansionCases()
	if err != nil {
		t.Fatal(err)
	}
	var selected []Case
	for _, c := range cases {
		if c.Name == "exp-mcu-minimum" {
			selected = append(selected, c)
		}
	}
	if len(selected) != 1 {
		t.Fatalf("MCU tracer cases = %d, want one", len(selected))
	}
	config, err := QueryExpansionRunnerConfig(RunnerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	var authored QueryPilotCase
	for _, c := range corpus.Cases {
		if c.ID == "exp-mcu-minimum" {
			authored = c
		}
	}
	provider := testkit.NewLLM(queryExpansionResponses(t, authored)...)
	generator, observer, err := NewEmbeddedQueryExpansionGenerator(provider)
	if err != nil {
		t.Fatal(err)
	}
	card := NewRunner(generator, config).WithObserver(observer).Run(context.Background(), selected)
	if !card.Results[0].Passed() || !card.Assessment.Passed || card.Certified || !card.DevelopmentCorpus {
		t.Fatalf("reviewed MCU development answer: %+v", card.Results[0])
	}
}

func TestQueryExpansionReviewedHistoryUsesProductionSuggestion(t *testing.T) {
	corpus, err := LoadEmbeddedQueryExpansionCorpus()
	if err != nil {
		t.Fatal(err)
	}
	cases, err := QueryExpansionCases()
	if err != nil {
		t.Fatal(err)
	}
	var selected []Case
	for _, c := range cases {
		if c.Name == "exp-history-minimum" {
			selected = append(selected, c)
		}
	}
	if len(selected) != 1 {
		t.Fatalf("History tracer cases = %d, want one", len(selected))
	}
	config, err := QueryExpansionRunnerConfig(RunnerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	var authored QueryPilotCase
	for _, c := range corpus.Cases {
		if c.ID == "exp-history-minimum" {
			authored = c
		}
	}
	provider := testkit.NewLLM(queryExpansionResponses(t, authored)...)
	generator, observer, err := NewEmbeddedQueryExpansionGenerator(provider)
	if err != nil {
		t.Fatal(err)
	}
	card := NewRunner(generator, config).WithObserver(observer).Run(context.Background(), selected)
	if !card.Results[0].Passed() || !card.Assessment.Passed || card.Certified || !card.DevelopmentCorpus {
		t.Fatalf("reviewed History development answer: %+v", card.Results[0])
	}
}

func TestQueryExpansionReviewedMovieEpochUsesProductionSuggestion(t *testing.T) {
	corpus, err := LoadEmbeddedQueryExpansionCorpus()
	if err != nil {
		t.Fatal(err)
	}
	cases, err := QueryExpansionCases()
	if err != nil {
		t.Fatal(err)
	}
	var selected []Case
	var authored QueryPilotCase
	for i, c := range cases {
		if c.Name == "exp-movie-epoch-1940s" {
			selected = append(selected, c)
			authored = corpus.Cases[i]
		}
	}
	if len(selected) != 1 {
		t.Fatalf("movie epoch tracer cases = %d, want one", len(selected))
	}
	config, err := QueryExpansionRunnerConfig(RunnerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	provider := testkit.NewLLM(queryExpansionResponses(t, authored)...)
	generator, observer, err := NewEmbeddedQueryExpansionGenerator(provider)
	if err != nil {
		t.Fatal(err)
	}
	card := NewRunner(generator, config).WithObserver(observer).Run(context.Background(), selected)
	if !card.Results[0].Passed() || !card.Assessment.Passed || card.Certified || !card.DevelopmentCorpus {
		t.Fatalf("reviewed movie epoch development answer: %+v", card.Results[0])
	}
}

func TestQueryExpansionMovieSubjectiveRubricsAreBoundToModelAttestedAuthority(t *testing.T) {
	corpus, err := LoadEmbeddedQueryExpansionCorpus()
	if err != nil {
		t.Fatal(err)
	}
	if corpus.MoodReviewAuthority.Path != "testdata/query-mood-review-authority-v1.json" || corpus.MoodReviewAuthority.SHA256 != "7e763f965b84f59af7b41931e01cc7921c0f03f6c06cfb8e5b9bb401252d02d3" {
		t.Fatalf("mood authority binding = %+v", corpus.MoodReviewAuthority)
	}
	reviewed := 0
	for _, authored := range corpus.Cases {
		if authored.SubjectiveReview == nil {
			continue
		}
		reviewed++
		if authored.SubjectiveReview.Version != "movie-mood-ordinal-v1" || authored.SubjectiveReview.Status != MoodReviewStatusModelAttested || authored.SubjectiveReview.Rubric == "" || authored.SubjectiveReview.AuthoritySHA256 != corpus.MoodReviewAuthority.SHA256 || len(authored.SubjectiveReview.Rules) == 0 {
			t.Fatalf("case %q has incomplete subjective review: %+v", authored.ID, authored.SubjectiveReview)
		}
		for _, rule := range authored.SubjectiveReview.Rules {
			if rule.Axis == "attentionalDemand" {
				t.Fatalf("case %q depends on an unresolved mood axis", authored.ID)
			}
		}
	}
	if reviewed != 8 {
		t.Fatalf("movie requests with authored subjective rubrics = %d, want 8", reviewed)
	}
	cases, err := QueryExpansionCases()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		if strings.HasPrefix(c.Name, "exp-movie-mood-") && c.JudgeRubric != "" {
			t.Fatalf("development review for %q was misreported as a live judge pass", c.Name)
		}
	}
}

func TestQueryExpansionMoviePeopleUseRequestedCatalogRoles(t *testing.T) {
	corpus, err := LoadEmbeddedQueryExpansionCorpus()
	if err != nil {
		t.Fatal(err)
	}
	cases, err := QueryExpansionCases()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"exp-movie-director-spielberg":    true,
		"exp-movie-cast-tom-hanks":        true,
		"exp-movie-people-hanks-zemeckis": true,
	}
	config, err := QueryExpansionRunnerConfig(RunnerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	run := 0
	for i, c := range cases {
		if !want[c.Name] {
			continue
		}
		run++
		provider := testkit.NewLLM(queryExpansionResponses(t, corpus.Cases[i])...)
		generator, observer, err := NewEmbeddedQueryExpansionGenerator(provider)
		if err != nil {
			t.Fatal(err)
		}
		card := NewRunner(generator, config).WithObserver(observer).Run(context.Background(), []Case{c})
		if !card.Results[0].Passed() || !card.Results[0].CorrectToolOperation {
			t.Fatalf("movie role request %q: %+v", c.Name, card.Results[0])
		}
	}
	if run != len(want) {
		t.Fatalf("movie person-role requests = %d, want %d", run, len(want))
	}
}

func TestQueryExpansionMovieIntervalsAndRefinementsUseProductionSuggestion(t *testing.T) {
	corpus, err := LoadEmbeddedQueryExpansionCorpus()
	if err != nil {
		t.Fatal(err)
	}
	cases, err := QueryExpansionCases()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"exp-movie-epoch-disjoint": true,
		"exp-movie-refine-add":     true,
		"exp-movie-refine-remove":  true,
	}
	config, err := QueryExpansionRunnerConfig(RunnerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	run := 0
	for i, c := range cases {
		if !want[c.Name] {
			continue
		}
		run++
		provider := testkit.NewLLM(queryExpansionResponses(t, corpus.Cases[i])...)
		generator, observer, err := NewEmbeddedQueryExpansionGenerator(provider)
		if err != nil {
			t.Fatal(err)
		}
		card := NewRunner(generator, config).WithObserver(observer).Run(context.Background(), []Case{c})
		if !card.Results[0].Passed() || !card.Assessment.Passed {
			t.Fatalf("movie interval/refine request %q: %+v", c.Name, card.Results[0])
		}
	}
	if run != len(want) {
		t.Fatalf("movie interval/refine requests = %d, want %d", run, len(want))
	}
}

func TestQueryExpansionMovieExclusionsAndThinResultsUseProductionSuggestion(t *testing.T) {
	corpus, err := LoadEmbeddedQueryExpansionCorpus()
	if err != nil {
		t.Fatal(err)
	}
	cases, err := QueryExpansionCases()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"exp-movie-exclude-jurassic":        true,
		"exp-movie-exclude-spielberg-two":   true,
		"exp-movie-exclude-toy-story":       true,
		"exp-movie-exclude-modern-two":      true,
		"exp-movie-thin-library-2010s":      true,
		"exp-movie-thin-acquisition-korean": true,
	}
	config, err := QueryExpansionRunnerConfig(RunnerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	run := 0
	for i, c := range cases {
		if !want[c.Name] {
			continue
		}
		run++
		provider := testkit.NewLLM(queryExpansionResponses(t, corpus.Cases[i])...)
		generator, observer, err := NewEmbeddedQueryExpansionGenerator(provider)
		if err != nil {
			t.Fatal(err)
		}
		card := NewRunner(generator, config).WithObserver(observer).Run(context.Background(), []Case{c})
		if !card.Results[0].Passed() || !card.Assessment.Passed {
			t.Fatalf("movie exclusion/thin request %q: %+v", c.Name, card.Results[0])
		}
	}
	if run != len(want) {
		t.Fatalf("movie exclusion/thin requests = %d, want %d", run, len(want))
	}
}

func TestQueryExpansionMovieAudienceCeilingsUseOneRatingAuthority(t *testing.T) {
	corpus, err := LoadEmbeddedQueryExpansionCorpus()
	if err != nil {
		t.Fatal(err)
	}
	cases, err := QueryExpansionCases()
	if err != nil {
		t.Fatal(err)
	}
	config, err := QueryExpansionRunnerConfig(RunnerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	run := 0
	for i, authored := range corpus.Cases {
		if authored.AudienceAuthority != "US-MPA" || authored.ExpectAbstention {
			continue
		}
		run++
		provider := testkit.NewLLM(queryExpansionResponses(t, authored)...)
		generator, observer, err := NewEmbeddedQueryExpansionGenerator(provider)
		if err != nil {
			t.Fatal(err)
		}
		card := NewRunner(generator, config).WithObserver(observer).Run(context.Background(), []Case{cases[i]})
		if !card.Results[0].Passed() || !card.Assessment.Passed || card.Results[0].Ceiling == "" {
			t.Fatalf("movie audience request %q: %+v", authored.ID, card.Results[0])
		}
	}
	if run != 3 {
		t.Fatalf("US-MPA audience requests = %d, want 3", run)
	}
}

func TestQueryExpansionMovieAudienceEmptyResultsAreExplicitAbstentions(t *testing.T) {
	corpus, err := LoadEmbeddedQueryExpansionCorpus()
	if err != nil {
		t.Fatal(err)
	}
	cases, err := QueryExpansionCases()
	if err != nil {
		t.Fatal(err)
	}
	config, err := QueryExpansionRunnerConfig(RunnerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	run := 0
	for i, authored := range corpus.Cases {
		if !authored.ExpectAbstention {
			continue
		}
		run++
		provider := testkit.NewLLM(queryExpansionResponses(t, authored)...)
		generator, observer, err := NewEmbeddedQueryExpansionGenerator(provider)
		if err != nil {
			t.Fatal(err)
		}
		card := NewRunner(generator, config).WithObserver(observer).Run(context.Background(), []Case{cases[i]})
		result := card.Results[0]
		if !result.Passed() || !card.Assessment.Passed || result.GroundedCompletion || !result.ProposalQuality {
			t.Fatalf("movie empty-result request %q: %+v", authored.ID, result)
		}
	}
	if run != 2 {
		t.Fatalf("movie empty-result requests = %d, want 2", run)
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

func TestQueryExpansionHistoryIndependentCausalOutcomeControls(t *testing.T) {
	cases, err := QueryExpansionCases()
	if err != nil {
		t.Fatal(err)
	}
	config, err := QueryExpansionRunnerConfig(RunnerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	owned := proposalWithKeys(t, "series:tmdb:6145")
	missing := proposalWithKeys(t, "series:tmdb:6145")
	missing.Acquisitions, missing.Lineup = missing.Lineup, nil
	laterAncient := proposalWithKeys(t, "series:tmdb:6145", "series:tmdb:32608")
	laterOak := proposalWithKeys(t, "series:tmdb:6145", "series:tmdb:60603")
	duplicate := proposalWithKeys(t, "series:tmdb:6145", "series:tmdb:6145")
	inventedPlayback := owned
	inventedPlayback.Policy.Scope.Dates = &schedule.DateScope{SeriesAiring: []schedule.Range{{From: 1990, To: 1999}}}
	airing := owned
	airing.Policy.Scope.Dates = &schedule.DateScope{SeriesAiring: []schedule.Range{{From: 1996, To: 1999}}}
	wrongAxis := owned
	wrongAxis.Policy.Scope.Dates = &schedule.DateScope{SeriesPremiere: []schedule.Range{{From: 1996, To: 1999}}}
	controls := []struct {
		name, caseID, failure string
		positive, negative    suggest.Proposal
	}{
		{"missing-anchor", "exp-history-minimum", "grounded titles", owned, proposalWithKeys(t)},
		{"later-era-ancient-aliens", "exp-history-minimum", "outside the acceptable set", owned, laterAncient},
		{"later-era-oak-island", "exp-history-minimum", "outside the acceptable set", owned, laterOak},
		{"duplicate-padding", "exp-history-minimum", "duplicate", owned, duplicate},
		{"invented-playback-limit", "exp-history-minimum", "date", owned, inventedPlayback},
		{"owned-title-as-acquisition", "exp-history-owned", "lineup", owned, missing},
		{"missing-title-as-owned", "exp-history-acquisition", "acquisitions", missing, owned},
		{"wrong-date-axis", "exp-history-airing-1996-1999", "date", airing, wrongAxis},
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
				t.Fatal("causal control requires exactly one authored History request")
			}
			good := NewRunner(scriptedGenerator{proposal: control.positive}, config).Run(context.Background(), selected)
			if !good.Results[0].Passed() || !good.Assessment.Passed || good.Certified {
				t.Fatalf("independently valid positive failed: %+v", good.Results[0])
			}
			bad := NewRunner(scriptedGenerator{proposal: control.negative}, config).Run(context.Background(), selected)
			if control.failure == "date" {
				if bad.Results[0].PolicyAccurate || bad.Assessment.Passed || bad.Certified {
					t.Fatalf("wrong History date policy escaped strict assessment: %+v", bad)
				}
				return
			}
			if bad.Results[0].Passed() || !strings.Contains(strings.Join(bad.Results[0].Failures, " "), control.failure) || bad.Certified {
				t.Fatalf("deliberately wrong History answer escaped its target: %+v", bad.Results[0])
			}
		})
	}
}

func TestQueryExpansionMCUIndependentCausalOutcomeControls(t *testing.T) {
	cases, err := QueryExpansionCases()
	if err != nil {
		t.Fatal(err)
	}
	config, err := QueryExpansionRunnerConfig(RunnerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	ownedAndMissing := func(owned int, keys ...string) suggest.Proposal {
		proposal := proposalWithKeys(t, keys...)
		proposal.Acquisitions, proposal.Lineup = proposal.Lineup[owned:], proposal.Lineup[:owned]
		return proposal
	}
	positive := ownedAndMissing(3, "movie:tmdb:1726", "movie:tmdb:10138", "movie:tmdb:10195", "movie:tmdb:1724", "movie:tmdb:1771", "movie:tmdb:24428")
	missingAlias := ownedAndMissing(3, "movie:tmdb:1726", "movie:tmdb:10138", "movie:tmdb:10195", "movie:tmdb:1724", "movie:tmdb:1771")
	wrongPhase := ownedAndMissing(3, "movie:tmdb:1726", "movie:tmdb:10138", "movie:tmdb:10195", "movie:tmdb:1724", "movie:tmdb:1771", "movie:tmdb:68721")
	wrongCollision := ownedAndMissing(3, "movie:tmdb:1726", "movie:tmdb:10138", "movie:tmdb:10195", "movie:tmdb:1724", "movie:tmdb:1771", "movie:tmdb:63736")
	duplicate := ownedAndMissing(3, "movie:tmdb:1726", "movie:tmdb:10138", "movie:tmdb:10195", "movie:tmdb:1724", "movie:tmdb:1771", "movie:tmdb:24428", "movie:tmdb:24428")
	wrongOwnership := proposalWithKeys(t, "movie:tmdb:1726", "movie:tmdb:10138", "movie:tmdb:10195", "movie:tmdb:1724", "movie:tmdb:1771", "movie:tmdb:24428")
	ownedOnly := proposalWithKeys(t, "movie:tmdb:1726", "movie:tmdb:10138", "movie:tmdb:10195")
	inRange := ownedAndMissing(2, "movie:tmdb:10138", "movie:tmdb:10195", "movie:tmdb:1771", "movie:tmdb:24428")
	inRange.Policy.Scope.Dates = &schedule.DateScope{MovieRelease: []schedule.Range{{From: 2010, To: 2012}}}
	wrongAxis := inRange
	wrongAxis.Policy.Scope.Dates = &schedule.DateScope{SeriesPremiere: []schedule.Range{{From: 2010, To: 2012}}}
	controls := []struct {
		name, caseID, failure string
		positive, negative    suggest.Proposal
	}{
		{"missing-regional-alias", "exp-mcu-minimum", "24428", positive, missingAlias},
		{"phase-two-padding", "exp-mcu-minimum", "68721", positive, wrongPhase},
		{"title-collision", "exp-mcu-minimum", "63736", positive, wrongCollision},
		{"duplicate-padding", "exp-mcu-minimum", "duplicate", positive, duplicate},
		{"sparse-results", "exp-mcu-minimum", "grounded titles", positive, proposalWithKeys(t, "movie:tmdb:24428")},
		{"wrong-ownership", "exp-mcu-minimum", "acquisitions", positive, wrongOwnership},
		{"outside-library", "exp-mcu-owned", "acceptable", ownedOnly, positive},
		{"wrong-date-axis", "exp-mcu-release-2010s", "date", inRange, wrongAxis},
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
				t.Fatal("causal control requires exactly one authored MCU request")
			}
			good := NewRunner(scriptedGenerator{proposal: control.positive}, config).Run(context.Background(), selected)
			if !good.Results[0].Passed() || !good.Assessment.Passed || good.Certified {
				t.Fatalf("independently valid positive failed: %+v", good.Results[0])
			}
			bad := NewRunner(scriptedGenerator{proposal: control.negative}, config).Run(context.Background(), selected)
			if control.failure == "date" {
				if bad.Results[0].PolicyAccurate || bad.Assessment.Passed || bad.Certified {
					t.Fatalf("wrong MCU date policy escaped strict assessment: %+v", bad)
				}
				return
			}
			if bad.Results[0].Passed() || !strings.Contains(strings.Join(bad.Results[0].Failures, " "), control.failure) || bad.Certified {
				t.Fatalf("deliberately wrong MCU answer escaped its target: %+v", bad.Results[0])
			}
		})
	}
}

func TestQueryExpansionMovieIndependentCausalOutcomeControls(t *testing.T) {
	cases, err := QueryExpansionCases()
	if err != nil {
		t.Fatal(err)
	}
	config, err := QueryExpansionRunnerConfig(RunnerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	ownedAndMissing := func(owned int, keys ...string) suggest.Proposal {
		proposal := proposalWithKeys(t, keys...)
		proposal.Acquisitions, proposal.Lineup = proposal.Lineup[owned:], proposal.Lineup[:owned]
		return proposal
	}
	withMovieDates := func(proposal suggest.Proposal, ranges ...schedule.Range) suggest.Proposal {
		proposal.Policy.Scope.Dates = &schedule.DateScope{MovieRelease: ranges}
		return proposal
	}
	withSeriesDates := func(proposal suggest.Proposal, ranges ...schedule.Range) suggest.Proposal {
		proposal.Policy.Scope.Dates = &schedule.DateScope{SeriesPremiere: ranges}
		return proposal
	}
	spielberg := ownedAndMissing(2, "movie:tmdb:578", "movie:tmdb:601", "movie:tmdb:329")
	spielbergWithProducerCredit := ownedAndMissing(3, "movie:tmdb:578", "movie:tmdb:601", "movie:tmdb:329", "movie:tmdb:105")
	hanks := withMovieDates(ownedAndMissing(1, "movie:tmdb:13", "movie:tmdb:862"), schedule.Range{From: 1990, To: 1999})
	hanksWithWrongCast := withMovieDates(ownedAndMissing(2, "movie:tmdb:13", "movie:tmdb:862", "movie:tmdb:105"), schedule.Range{From: 1990, To: 1999})
	comforting := proposalWithKeys(t, "movie:tmdb:346648")
	comfortingWithJaws := proposalWithKeys(t, "movie:tmdb:346648", "movie:tmdb:578")
	tense := ownedAndMissing(1, "movie:tmdb:578", "movie:tmdb:496243")
	tenseWithPaddington := ownedAndMissing(2, "movie:tmdb:578", "movie:tmdb:496243", "movie:tmdb:346648")
	pg := proposalWithKeys(t, "movie:tmdb:578", "movie:tmdb:601")
	pg.Policy.Audience.Ceiling = schedule.Rating("PG")
	for i := range pg.Lineup {
		pg.Lineup[i].OfficialRating = "PG"
	}
	overPG := proposalWithKeys(t, "movie:tmdb:578", "movie:tmdb:601")
	overPG.Policy.Audience.Ceiling = schedule.Rating("PG")
	overPG.Lineup[0].OfficialRating = "PG"
	overPG.Lineup[1].OfficialRating = "PG-13"
	mpaPG := ownedAndMissing(1, "movie:tmdb:578", "movie:tmdb:862")
	mpaPG.Policy.Audience.Ceiling = schedule.Rating("PG")
	mpaPG.Lineup[0].OfficialRating = "PG"
	mpaPG.Acquisitions[0].OfficialRating = "G"
	mpaOverPG := ownedAndMissing(1, "movie:tmdb:578", "movie:tmdb:862", "movie:tmdb:329")
	mpaOverPG.Policy.Audience.Ceiling = schedule.Rating("PG")
	mpaOverPG.Lineup[0].OfficialRating = "PG"
	mpaOverPG.Acquisitions[0].OfficialRating = "G"
	mpaOverPG.Acquisitions[1].OfficialRating = "PG-13"
	excludedModern := withMovieDates(ownedAndMissing(1, "movie:tmdb:194", "movie:tmdb:129"), schedule.Range{From: 2001, To: 2099})
	excludedModernPadded := withMovieDates(ownedAndMissing(2, "movie:tmdb:194", "movie:tmdb:129", "movie:tmdb:346648", "movie:tmdb:496243"), schedule.Range{From: 2001, To: 2099})
	disjoint := withMovieDates(ownedAndMissing(2, "movie:tmdb:289", "movie:tmdb:346648", "movie:tmdb:496243"),
		schedule.Range{From: 1940, To: 1949}, schedule.Range{From: 2010, To: 2019})
	wrongDisjointAxis := withSeriesDates(ownedAndMissing(2, "movie:tmdb:289", "movie:tmdb:346648", "movie:tmdb:496243"),
		schedule.Range{From: 1940, To: 1949}, schedule.Range{From: 2010, To: 2019})
	french := withMovieDates(proposalWithKeys(t, "movie:tmdb:194"), schedule.Range{From: 2000, To: 2009})
	notFrench := withMovieDates(ownedAndMissing(0, "movie:tmdb:129"), schedule.Range{From: 2000, To: 2009})
	koreanMissing := withMovieDates(ownedAndMissing(0, "movie:tmdb:496243"), schedule.Range{From: 2010, To: 2019})
	koreanOwned := withMovieDates(proposalWithKeys(t, "movie:tmdb:496243"), schedule.Range{From: 2010, To: 2019})
	refineAdd := proposalWithKeys(t, "movie:tmdb:578", "movie:tmdb:601")
	refineDropsAdd := proposalWithKeys(t, "movie:tmdb:578")
	refineRemove := withMovieDates(proposalWithKeys(t, "movie:tmdb:194"), schedule.Range{From: 2000, To: 2009})
	refineKeepsRemoved := withMovieDates(ownedAndMissing(1, "movie:tmdb:194", "movie:tmdb:129"), schedule.Range{From: 2000, To: 2009})
	controls := []struct {
		name, caseID, failure string
		positive, negative    suggest.Proposal
	}{
		{"disjoint-wrong-date-axis", "exp-movie-epoch-disjoint", "date", disjoint, wrongDisjointAxis},
		{"director-role-producer-padding", "exp-movie-director-spielberg", "outside the acceptable set", spielberg, spielbergWithProducerCredit},
		{"cast-role-director-padding", "exp-movie-cast-tom-hanks", "outside the acceptable set", hanks, hanksWithWrongCast},
		{"language-substitution", "exp-movie-language-french", "outside the acceptable set", french, notFrench},
		{"comforting-includes-frightening", "exp-movie-mood-comforting", "outside the acceptable set", comforting, comfortingWithJaws},
		{"tense-includes-comforting", "exp-movie-mood-tense", "outside the acceptable set", tense, tenseWithPaddington},
		{"rating-above-pg", "exp-movie-director-spielberg-pg", "above the forbidden ceiling", pg, overPG},
		{"mpa-rating-above-pg", "exp-movie-audience-pg-or-lower", "above the forbidden ceiling", mpaPG, mpaOverPG},
		{"multiple-exclusions-padded", "exp-movie-exclude-modern-two", "outside the acceptable set", excludedModern, excludedModernPadded},
		{"missing-title-marked-owned", "exp-movie-region-south-korean", "acquisitions", koreanMissing, koreanOwned},
		{"refine-drops-requested-addition", "exp-movie-refine-add", "required grounded key", refineAdd, refineDropsAdd},
		{"refine-keeps-requested-removal", "exp-movie-refine-remove", "outside the acceptable set", refineRemove, refineKeepsRemoved},
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
				t.Fatal("causal control requires exactly one authored movie request")
			}
			// These scripted controls isolate outcome grading. Production-path tests
			// above independently assert the model's observed Catalog operation.
			selected[0].ExpectedToolOperation = ""
			good := NewRunner(scriptedGenerator{proposal: control.positive}, config).Run(context.Background(), selected)
			if !good.Results[0].Passed() || !good.Assessment.Passed || good.Certified {
				t.Fatalf("independently valid movie positive failed: %+v", good.Results[0])
			}
			bad := NewRunner(scriptedGenerator{proposal: control.negative}, config).Run(context.Background(), selected)
			if control.failure == "date" {
				if bad.Results[0].PolicyAccurate || bad.Assessment.Passed || bad.Certified {
					t.Fatalf("wrong movie date policy escaped strict assessment: %+v", bad)
				}
				return
			}
			if bad.Results[0].Passed() || !strings.Contains(strings.Join(bad.Results[0].Failures, " "), control.failure) || bad.Certified {
				t.Fatalf("deliberately wrong movie answer escaped its target: %+v", bad.Results[0])
			}
		})
	}
}

func TestQueryExpansionEveryAuthoredRequestUsesProductionSuggestion(t *testing.T) {
	corpus, err := LoadEmbeddedQueryExpansionCorpus()
	if err != nil {
		t.Fatal(err)
	}
	if len(corpus.Cases) != 119 {
		t.Fatalf("cumulative executable corpus has %d requests, want 119", len(corpus.Cases))
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
			provider := testkit.NewLLM(queryExpansionResponses(t, corpus.Cases[i])...)
			generator, observer, err := NewEmbeddedQueryExpansionGenerator(provider)
			if err != nil {
				t.Fatal(err)
			}
			card := NewRunner(generator, config).WithObserver(observer).Run(context.Background(), []Case{c})
			if !card.Results[0].Passed() || !card.Assessment.Passed || card.Certified || !card.DevelopmentCorpus {
				last := ""
				if len(provider.LastMessages) > 0 {
					last = provider.LastMessages[len(provider.LastMessages)-1].Content
				}
				t.Fatalf("authored request: stage=%s failures=%v assessment=%v selected=%d providerCalls=%d last=%q", card.Results[0].FailureStage, card.Results[0].Failures, card.Assessment.Failures, card.Results[0].SelectedCount, provider.Calls, last)
			}
			if corpus.Cases[i].FixtureCase == "mcu-reviewed" {
				sourcePresented := false
				for _, message := range provider.LastMessages {
					sourcePresented = sourcePresented || strings.Contains(message.Content, "UNTRUSTED REFERENCE DATA")
				}
				if !sourcePresented {
					t.Fatal("MCU request passed without reviewed source evidence reaching final selection")
				}
			}
		})
	}
}

// Provider replies are authored independently of the manifest's allowed keys,
// required anchors, minimum counts and date oracles. Only request text is read
// to locate the exact wire-protocol date span.
func queryExpansionResponses(t *testing.T, c QueryPilotCase) []llm.Response {
	t.Helper()
	if c.FixtureCase == "mcu-reviewed" {
		return queryExpansionMCUResponses(t, c)
	}
	if strings.HasPrefix(c.FixtureCase, "history-era-reviewed") {
		return queryExpansionHistoryResponses(t, c)
	}
	if strings.HasPrefix(c.FixtureCase, "movie-reviewed") {
		return queryExpansionMovieResponses(t, c)
	}
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

func queryExpansionMovieResponses(t *testing.T, c QueryPilotCase) []llm.Response {
	t.Helper()
	ids := []int{346648}
	args := map[string]any{"media_type": "movie", "keywords": []any{"comforting"}}
	type dateInterval struct {
		marker   string
		from, to int
	}
	var intervals []dateInterval
	setInterval := func(marker string, from, to int) {
		intervals = []dateInterval{{marker: marker, from: from, to: to}}
	}
	switch c.ID {
	case "exp-movie-exclude-jurassic":
		ids, args = []int{578, 601}, map[string]any{"media_type": "movie", "creators": []any{"Steven Spielberg"}}
	case "exp-movie-exclude-spielberg-two":
		ids, args = []int{578}, map[string]any{"media_type": "movie", "creators": []any{"Steven Spielberg"}}
	case "exp-movie-exclude-toy-story":
		ids, args = []int{13}, map[string]any{"media_type": "movie", "cast": []any{"Tom Hanks"}}
		setInterval("1990s", 1990, 1999)
	case "exp-movie-exclude-modern-two":
		ids, args = []int{194, 129}, map[string]any{"media_type": "movie"}
		setInterval("after 2000", 2001, 2099)
	case "exp-movie-thin-library-2010s":
		ids, args = []int{346648}, map[string]any{"media_type": "movie"}
		setInterval("2010s", 2010, 2019)
	case "exp-movie-thin-acquisition-korean":
		ids, args = []int{496243}, map[string]any{"media_type": "movie", "genres": []any{"Drama", "Thriller"}, "origin_country": "KR"}
		setInterval("2010s", 2010, 2019)
	case "exp-movie-epoch-1940s":
		ids, args = []int{289}, map[string]any{"media_type": "movie", "genres": []any{"Romance"}}
		setInterval("1940s", 1940, 1949)
	case "exp-movie-epoch-1950s":
		ids, args = []int{872}, map[string]any{"media_type": "movie", "genres": []any{"Comedy", "Music"}}
		setInterval("1950s", 1950, 1959)
	case "exp-movie-epoch-1960s":
		ids, args = []int{62}, map[string]any{"media_type": "movie", "genres": []any{"Science Fiction"}}
		setInterval("sixties", 1960, 1969)
	case "exp-movie-epoch-1980s":
		ids, args = []int{105}, map[string]any{"media_type": "movie", "genres": []any{"Comedy", "Science Fiction"}}
		setInterval("eighties", 1980, 1989)
	case "exp-movie-epoch-inclusive":
		ids, args = []int{329, 13, 862}, map[string]any{"media_type": "movie", "genres": []any{"Adventure", "Drama", "Animation"}}
		setInterval("1993 through 1995", 1993, 1995)
	case "exp-movie-epoch-disjoint":
		ids, args = []int{289, 346648, 496243}, map[string]any{"media_type": "movie", "genres": []any{"Drama"}}
		intervals = []dateInterval{{marker: "1940s", from: 1940, to: 1949}, {marker: "2010s", from: 2010, to: 2019}}
	case "exp-movie-epoch-before-1970":
		ids, args = []int{289, 872, 62}, map[string]any{"media_type": "movie", "genres": []any{"Drama"}}
		setInterval("before 1970", 1900, 1969)
	case "exp-movie-epoch-2000s-international":
		ids, args = []int{194, 129}, map[string]any{"media_type": "movie", "genres": []any{"Drama"}}
		setInterval("2000s", 2000, 2009)
	case "exp-movie-epoch-2010s-international":
		ids, args = []int{346648, 496243}, map[string]any{"media_type": "movie", "genres": []any{"Drama"}}
		setInterval("2010s", 2010, 2019)
	case "exp-movie-epoch-after-2000":
		ids, args = []int{194, 129, 346648, 496243}, map[string]any{"media_type": "movie", "genres": []any{"Drama"}}
		setInterval("after 2000", 2001, 2099)
	case "exp-movie-epoch-1990s-not-1980s":
		ids, args = []int{329, 13, 862}, map[string]any{"media_type": "movie", "genres": []any{"Drama"}}
		setInterval("1990s", 1990, 1999)
	case "exp-movie-language-french":
		ids, args = []int{194}, map[string]any{"media_type": "movie", "genres": []any{"Comedy", "Romance"}, "original_language": "fr"}
		setInterval("2000s", 2000, 2009)
	case "exp-movie-region-south-korean":
		ids, args = []int{496243}, map[string]any{"media_type": "movie", "genres": []any{"Drama", "Thriller"}, "origin_country": "KR"}
		setInterval("2010s", 2010, 2019)
	case "exp-movie-refine-add":
		ids, args = []int{578, 601}, map[string]any{"media_type": "movie", "creators": []any{"Steven Spielberg"}}
	case "exp-movie-refine-remove":
		ids, args = []int{194}, map[string]any{"media_type": "movie", "genres": []any{"Comedy"}}
		setInterval("2000s", 2000, 2009)
	case "exp-movie-mood-tense":
		ids, args = []int{578, 496243}, map[string]any{"media_type": "movie", "keywords": []any{"tense", "threatening"}}
	case "exp-movie-mood-dark-satire":
		ids, args = []int{496243}, map[string]any{"media_type": "movie", "keywords": []any{"dark", "satirical", "thriller"}}
	case "exp-movie-mood-tense-1970s":
		ids, args = []int{578}, map[string]any{"media_type": "movie", "keywords": []any{"tense", "frightening"}}
		setInterval("1970s", 1970, 1979)
	case "exp-movie-mood-dark-korean-2010s":
		ids, args = []int{496243}, map[string]any{"media_type": "movie", "keywords": []any{"dark"}, "genres": []any{"Thriller"}, "origin_country": "KR"}
		setInterval("2010s", 2010, 2019)
	case "exp-movie-mood-comforting-british-2010s":
		ids, args = []int{346648}, map[string]any{"media_type": "movie", "keywords": []any{"comforting"}, "genres": []any{"Family", "Comedy"}, "origin_country": "GB"}
		setInterval("2010s", 2010, 2019)
	case "exp-movie-director-spielberg", "exp-movie-director-spielberg-not-produced":
		ids, args = []int{578, 601, 329}, map[string]any{"media_type": "movie", "creators": []any{"Steven Spielberg"}}
	case "exp-movie-director-spielberg-1980s":
		ids, args = []int{601}, map[string]any{"media_type": "movie", "creators": []any{"Steven Spielberg"}}
		setInterval("1980s", 1980, 1989)
	case "exp-movie-director-spielberg-1990s":
		ids, args = []int{329}, map[string]any{"media_type": "movie", "creators": []any{"Steven Spielberg"}, "genres": []any{"Science Fiction"}}
		setInterval("1990s", 1990, 1999)
	case "exp-movie-director-zemeckis":
		ids, args = []int{105, 13}, map[string]any{"media_type": "movie", "creators": []any{"Robert Zemeckis"}}
	case "exp-movie-director-zemeckis-1980s":
		ids, args = []int{105}, map[string]any{"media_type": "movie", "creators": []any{"Robert Zemeckis"}}
		setInterval("eighties", 1980, 1989)
	case "exp-movie-cast-tom-hanks":
		ids, args = []int{13, 862}, map[string]any{"media_type": "movie", "cast": []any{"Tom Hanks"}}
		setInterval("1990s", 1990, 1999)
	case "exp-movie-cast-tom-hanks-animation":
		ids, args = []int{862}, map[string]any{"media_type": "movie", "cast": []any{"Tom Hanks"}, "genres": []any{"Animation"}}
	case "exp-movie-people-hanks-zemeckis":
		ids, args = []int{13}, map[string]any{"media_type": "movie", "cast": []any{"Tom Hanks"}, "creators": []any{"Robert Zemeckis"}}
	case "exp-movie-director-spielberg-pg":
		ids, args = []int{578, 601}, map[string]any{"media_type": "movie", "creators": []any{"Steven Spielberg"}}
	case "exp-movie-audience-pg-or-lower":
		ids, args = []int{578, 862}, map[string]any{"media_type": "movie", "origin_country": "US"}
	case "exp-movie-audience-g-animated-1990s":
		ids, args = []int{862}, map[string]any{"media_type": "movie", "genres": []any{"Animation"}, "origin_country": "US"}
		setInterval("1990s", 1990, 1999)
	case "exp-movie-audience-spielberg-pg-1970s":
		ids, args = []int{578}, map[string]any{"media_type": "movie", "creators": []any{"Steven Spielberg"}}
		setInterval("1970s", 1970, 1979)
	case "exp-movie-audience-empty-excluded-toy-story":
		args = map[string]any{"media_type": "movie", "genres": []any{"Animation"}, "origin_country": "US"}
		setInterval("1990s", 1990, 1999)
	case "exp-movie-audience-empty-spielberg-1990s":
		args = map[string]any{"media_type": "movie", "creators": []any{"Steven Spielberg"}}
		setInterval("1990s", 1990, 1999)
	}
	meaning := map[string]any{"kind": "none", "anchors": []any{}, "axes": []any{}}
	if len(intervals) > 0 {
		anchors := make([]any, 0, len(intervals))
		wireIntervals := make([]any, 0, len(intervals))
		for i, interval := range intervals {
			start := strings.Index(c.Description, interval.marker)
			if start < 0 {
				t.Fatalf("independent movie provider script has no date span %q", interval.marker)
			}
			anchors = append(anchors, map[string]any{"field": "description", "start": start, "end": start + len(interval.marker)})
			wireIntervals = append(wireIntervals, map[string]any{"anchor": i, "start": interval.from, "end": interval.to})
		}
		meaning = map[string]any{
			"kind":    "constraints",
			"anchors": anchors,
			"axes": []any{map[string]any{
				"kind": "movie_release", "combine": "any",
				"intervals": wireIntervals,
			}},
		}
	}
	args["dateMeaning"] = meaning
	picks := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		picks = append(picks, map[string]any{"mediaType": "movie", "key": fmt.Sprintf("movie:tmdb:%d", id)})
	}
	return []llm.Response{
		testkit.ToolCallResponse("catalog_search", args),
		testkit.FinalResponse(fmt.Sprintf(`{"picks":%s,"dateMeaning":%s}`, mustJSON(t, picks), mustJSON(t, meaning))),
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	blob, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(blob)
}

func queryExpansionHistoryResponses(t *testing.T, c QueryPilotCase) []llm.Response {
	t.Helper()
	meaning := map[string]any{"kind": "none", "anchors": []any{}, "axes": []any{}}
	if c.ID == "exp-history-airing-2000s" || c.ID == "exp-history-airing-1996-1999" {
		marker, from, to := "2000s", 2000, 2009
		if c.ID == "exp-history-airing-1996-1999" {
			marker, from, to = "1996-1999", 1996, 1999
		}
		start := strings.Index(c.Description, marker)
		if start < 0 {
			t.Fatal("independent History provider script has no date span")
		}
		meaning = map[string]any{
			"kind":    "constraints",
			"anchors": []any{map[string]any{"field": "description", "start": start, "end": start + len(marker)}},
			"axes": []any{map[string]any{
				"kind": "series_airing", "combine": "any",
				"intervals": []any{map[string]any{"anchor": 0, "start": from, "end": to}},
			}},
		}
	}
	final, err := json.Marshal(map[string]any{
		"picks":       []any{map[string]any{"mediaType": "series", "key": "series:tmdb:6145"}},
		"dateMeaning": meaning,
	})
	if err != nil {
		t.Fatal(err)
	}
	return []llm.Response{
		testkit.ToolCallResponse("catalog_search", map[string]any{
			"media_type": "series", "network": "History", "dateMeaning": meaning,
		}),
		testkit.FinalResponse(string(final)),
	}
}

func queryExpansionMCUResponses(t *testing.T, c QueryPilotCase) []llm.Response {
	t.Helper()
	ids := []int{1726, 1724, 10138, 10195, 1771, 24428}
	switch c.ID {
	case "exp-mcu-without-hulk":
		ids = []int{1726, 10138, 10195, 1771, 24428}
	case "exp-mcu-without-iron-man-2":
		ids = []int{1726, 1724, 10195, 1771, 24428}
	case "exp-mcu-without-thor":
		ids = []int{1726, 1724, 10138, 1771, 24428}
	case "exp-mcu-without-avengers":
		ids = []int{1726, 1724, 10138, 10195, 1771}
	case "exp-mcu-owned", "exp-mcu-no-acquisitions":
		ids = []int{1726, 10138, 10195}
	case "exp-mcu-release-2010s":
		ids = []int{10138, 10195, 1771, 24428}
	}
	meaning := map[string]any{"kind": "none", "anchors": []any{}, "axes": []any{}}
	marker, from, to := "", 0, 0
	switch c.ID {
	case "exp-mcu-release-years":
		marker, from, to = "2008 through 2012", 2008, 2012
	case "exp-mcu-release-years-conversation":
		marker, from, to = "2008 and 2012", 2008, 2012
	case "exp-mcu-release-2010s":
		marker, from, to = "2010 through 2012", 2010, 2012
	}
	if marker != "" {
		start := strings.Index(c.Description, marker)
		if start < 0 {
			t.Fatal("independent MCU provider script has no date span")
		}
		meaning = map[string]any{"kind": "constraints", "anchors": []any{map[string]any{"field": "description", "start": start, "end": start + len(marker)}}, "axes": []any{map[string]any{"kind": "movie_release", "combine": "any", "intervals": []any{map[string]any{"anchor": 0, "start": from, "end": to}}}}}
	}
	args := map[string]any{
		"media_type": "movie", "mode": "collection",
		"titles":      []any{"Iron Man", "The Incredible Hulk", "Iron Man 2", "Thor", "Captain America: The First Avenger", "Avengers Assemble"},
		"dateMeaning": meaning,
	}
	picks := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		picks = append(picks, map[string]any{"mediaType": "movie", "key": fmt.Sprintf("movie:tmdb:%d", id)})
	}
	final, err := json.Marshal(map[string]any{"picks": picks, "dateMeaning": meaning})
	if err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(wire, &args); err != nil {
		t.Fatal(err)
	}
	return []llm.Response{testkit.ToolCallResponse("catalog_search", args), testkit.FinalResponse(string(final))}
}
