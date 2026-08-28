//go:build eval

package eval

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
)

type scriptedGenerator struct {
	proposal suggest.Proposal
	err      error
}

func (g scriptedGenerator) Suggest(context.Context, suggest.Intent) (suggest.Proposal, error) {
	return g.proposal, g.err
}

type sequenceGenerator struct {
	proposals []suggest.Proposal
	next      int
}

type sequenceJudge struct {
	scores []JudgeScores
	next   int
}

type scriptedObserver struct {
	begins int
	value  Observation
}

func (o *scriptedObserver) Begin() { o.begins++ }

func (o *scriptedObserver) Snapshot(error) Observation { return o.value }

func (j *sequenceJudge) Score(context.Context, JudgeEvidence) (JudgeScores, error) {
	score := j.scores[j.next]
	j.next++
	return score, nil
}

func (g *sequenceGenerator) Suggest(context.Context, suggest.Intent) (suggest.Proposal, error) {
	proposal := g.proposals[g.next]
	g.next++
	return proposal, nil
}

func TestRunnerRejectsProposalMissingExactRequiredKey(t *testing.T) {
	runner := NewRunner(scriptedGenerator{proposal: suggest.Proposal{
		Lineup: []suggest.ProposalItem{{MediaType: provision.Movie, TMDBID: 680, Name: "Pulp Fiction"}},
	}}, RunnerConfig{Trials: 1, Profile: "hermetic", Generator: ModelIdentity{Provider: "fixture", Model: "fixture-v1"}})

	scorecard := runner.Run(context.Background(), []Case{{
		Name: "must_include_matrix",
		Intent: Intent{
			Description: "sci-fi movies",
			MustInclude: []string{"The Matrix"},
		},
		RequireKeys: []provision.Key{"movie:tmdb:603"},
	}})

	if scorecard.Certified {
		t.Fatal("scorecard certified a proposal that omitted the exact required title")
	}
	if len(scorecard.Results) != 1 || len(scorecard.Results[0].Failures) != 1 {
		t.Fatalf("result = %+v, want one exact-identity failure", scorecard.Results)
	}
	if got := scorecard.Results[0].Failures[0]; !strings.Contains(got, "movie:tmdb:603") {
		t.Fatalf("failure = %q, want missing required key", got)
	}
	result := scorecard.Results[0]
	if result.FailureStage != FailureStageDeterministic || scorecard.FailureCounts[FailureStageDeterministic] != 1 {
		t.Fatalf("deterministic failure accounting = stage %q counts %v", result.FailureStage, scorecard.FailureCounts)
	}
}

func TestNamedIncludeCorpusCasePinsTheMatrixIdentity(t *testing.T) {
	for _, c := range Corpus {
		if c.Name != "must_include_grounding" {
			continue
		}
		if len(c.RequireKeys) != 1 || c.RequireKeys[0] != "movie:tmdb:603" {
			t.Fatalf("required keys = %v, want exact Matrix identity", c.RequireKeys)
		}
		return
	}
	t.Fatal("must_include_grounding case is missing")
}

func TestRunnerChecksConcreteProgramsAfterScheduleFiltering(t *testing.T) {
	runner := NewRunner(scriptedGenerator{proposal: suggest.Proposal{
		Lineup: []suggest.ProposalItem{{
			MediaType: provision.Series, TMDBID: 456, Name: "The Simpsons",
			InLibrary: true, LibraryItemID: "show-simpsons", SeasonMin: 1, SeasonMax: 1,
		}},
	}}, RunnerConfig{Trials: 1}).WithMaterializer(NewFixtureScheduleMaterializer(map[provision.Key]FixtureTitle{
		"series:tmdb:456": {Episodes: []schedule.ResolvedProgram{
			{LibraryItemID: "ep-s01e01", Title: "Simpsons Roasting on an Open Fire", DurationMs: 22 * 60 * 1000, Season: 1, Episode: 1},
			{LibraryItemID: "ep-s04e12", Title: "Marge vs. the Monorail", DurationMs: 22 * 60 * 1000, Season: 4, Episode: 12},
		}},
	}))

	scorecard := runner.Run(context.Background(), []Case{{
		Name: "classic_episode_outcome",
		RequireScheduledPrograms: []string{
			"series:tmdb:456:s04e12",
		},
	}})

	if scorecard.Certified {
		t.Fatal("scorecard certified an episode removed by the final schedule's season filter")
	}
	if got := scorecard.Results[0].Failures; len(got) != 1 || !strings.Contains(got[0], "series:tmdb:456:s04e12") {
		t.Fatalf("failures = %v, want the missing scheduled episode identity", got)
	}
	result := scorecard.Results[0]
	if result.FailureStage != FailureStageSchedule || scorecard.FailureCounts[FailureStageSchedule] != 1 {
		t.Fatalf("schedule failure accounting = stage %q counts %v", result.FailureStage, scorecard.FailureCounts)
	}
}

func TestRunnerReportsRepeatedTrialPassRate(t *testing.T) {
	matrix := suggest.Proposal{Lineup: []suggest.ProposalItem{{
		MediaType: provision.Movie, TMDBID: 603, Name: "The Matrix",
	}}}
	unrelated := suggest.Proposal{Lineup: []suggest.ProposalItem{{
		MediaType: provision.Movie, TMDBID: 680, Name: "Pulp Fiction",
	}}}
	generator := &sequenceGenerator{proposals: []suggest.Proposal{matrix, unrelated, matrix}}
	runner := NewRunner(generator, RunnerConfig{Trials: 3})

	card := runner.Run(context.Background(), []Case{{
		Name: "repeat_stability", RequireKeys: []provision.Key{"movie:tmdb:603"},
	}})

	if card.Certified {
		t.Fatal("one failed trial must fail certification")
	}
	if len(card.Results) != 3 || card.Results[0].Trial != 1 || card.Results[2].Trial != 3 {
		t.Fatalf("trials = %+v, want three numbered results", card.Results)
	}
	if len(card.Cases) != 1 || card.Cases[0].Passed != 2 || card.Cases[0].Trials != 3 || card.Cases[0].PassRate != 2.0/3.0 {
		t.Fatalf("case summary = %+v, want 2/3 pass rate", card.Cases)
	}
	if card.FailureCounts[FailureStageDeterministic] != 1 {
		t.Fatalf("repeated-trial failure counts = %v, want one deterministic trial", card.FailureCounts)
	}
}

func TestRunnerReportsLowScoreAsThresholdResultAndQualityRange(t *testing.T) {
	proposal := suggest.Proposal{Lineup: []suggest.ProposalItem{{
		MediaType: provision.Movie, TMDBID: 603, Name: "The Matrix",
	}}}
	judge := &sequenceJudge{scores: []JudgeScores{
		{Overall: 0.5, Relevance: 0.4, Serendipity: 0.8},
		{Overall: 0.8, Relevance: 0.9, Serendipity: 0.3},
		{Overall: 0.7, Relevance: 0.7, Serendipity: 0.6},
	}}
	runner := NewRunner(scriptedGenerator{proposal: proposal}, RunnerConfig{Trials: 3}).WithJudge(judge)

	card := runner.Run(context.Background(), []Case{{
		Name: "quality_distribution", JudgeRubric: "Relevant science fiction with defensible variety",
		MinRelevanceScore: 0.5,
	}})

	if card.Certified {
		t.Fatal("scorecard certified a valid relevance score below its declared floor")
	}
	if got := card.Results[0]; got.JudgeError != "" || len(got.Failures) != 1 || !strings.Contains(got.Failures[0], "relevance score 0.40 < required 0.50") {
		t.Fatalf("low-score result = %+v, want an ordinary threshold failure", got)
	}
	if result := card.Results[0]; result.FailureStage != FailureStageJudge || card.FailureCounts[FailureStageJudge] != 1 {
		t.Fatalf("low-score failure accounting = stage %q counts %v", result.FailureStage, card.FailureCounts)
	}
	if len(card.Cases) != 1 {
		t.Fatalf("case summaries = %d, want one", len(card.Cases))
	}
	got := card.Cases[0]
	if got.Relevance != (ScoreRange{Min: 0.4, Median: 0.7, Max: 0.9}) {
		t.Fatalf("relevance range = %+v", got.Relevance)
	}
	if got.Serendipity != (ScoreRange{Min: 0.3, Median: 0.6, Max: 0.8}) {
		t.Fatalf("serendipity range = %+v", got.Serendipity)
	}
}

func TestRunnerFailsRubricWhenJudgeIsMissing(t *testing.T) {
	runner := NewRunner(scriptedGenerator{proposal: suggest.Proposal{
		Lineup: []suggest.ProposalItem{{MediaType: provision.Movie, TMDBID: 603, Name: "The Matrix"}},
	}}, RunnerConfig{})

	card := runner.Run(context.Background(), []Case{{
		Name: "judge_required", JudgeRubric: "Relevant science fiction",
	}})

	if card.Certified {
		t.Fatal("scorecard certified a rubric without a configured judge")
	}
	result := card.Results[0]
	if got := result.Failures; len(got) != 1 || !strings.Contains(got[0], "judge is not configured") {
		t.Fatalf("failures = %v, want missing-judge failure", got)
	}
	if result.FailureStage != FailureStageJudge || card.FailureCounts[FailureStageJudge] != 1 {
		t.Fatalf("judge failure accounting = stage %q counts %v", result.FailureStage, card.FailureCounts)
	}
}

func TestRunnerFailsRubricWhenJudgeReturnsError(t *testing.T) {
	provider := testkit.NewLLM()
	provider.Delay = time.Second
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := NewRunner(scriptedGenerator{proposal: suggest.Proposal{
		Lineup: []suggest.ProposalItem{{MediaType: provision.Movie, TMDBID: 603, Name: "The Matrix"}},
	}}, RunnerConfig{}).WithJudge(modelJudge{provider: provider})

	card := runner.Run(ctx, []Case{{
		Name: "judge_error", JudgeRubric: "Relevant science fiction",
	}})

	if card.Certified {
		t.Fatal("scorecard certified a rubric after the judge returned an error")
	}
	result := card.Results[0]
	if got := result.Failures; len(got) != 1 || !strings.Contains(got[0], "judge call failed") {
		t.Fatalf("failures = %v, want judge-call failure", got)
	}
	if result.FailureStage != FailureStageJudge || card.FailureCounts[FailureStageJudge] != 1 {
		t.Fatalf("judge failure accounting = stage %q counts %v", result.FailureStage, card.FailureCounts)
	}
}

func TestRunnerFailsRubricWhenJudgeOutputIsUnparseable(t *testing.T) {
	provider := testkit.NewLLM(testkit.FinalResponse("definitely not judge JSON"))
	runner := NewRunner(scriptedGenerator{proposal: suggest.Proposal{
		Lineup: []suggest.ProposalItem{{MediaType: provision.Movie, TMDBID: 603, Name: "The Matrix"}},
	}}, RunnerConfig{}).WithJudge(modelJudge{provider: provider})

	card := runner.Run(context.Background(), []Case{{
		Name: "judge_unparseable", JudgeRubric: "Relevant science fiction",
	}})

	if card.Certified {
		t.Fatal("scorecard certified a rubric after unparseable judge output")
	}
	result := card.Results[0]
	if !strings.Contains(result.JudgeError, "judge output unparseable") {
		t.Fatalf("judge error = %q, want unparseable-output error", result.JudgeError)
	}
	if result.JudgeScore < 0 || result.RelevanceScore < 0 || result.SerendipityScore < 0 {
		t.Fatalf("judge scores = %.2f/%.2f/%.2f, want no negative failure sentinel",
			result.JudgeScore, result.RelevanceScore, result.SerendipityScore)
	}
	if result.FailureStage != FailureStageJudge || card.FailureCounts[FailureStageJudge] != 1 {
		t.Fatalf("judge failure accounting = stage %q counts %v", result.FailureStage, card.FailureCounts)
	}
}

func TestJudgeScoreRejectsMissingRequiredScoreFields(t *testing.T) {
	proposal := suggest.Proposal{Lineup: []suggest.ProposalItem{{
		MediaType: provision.Movie, TMDBID: 603, Name: "The Matrix",
	}}}
	caseUnderTest := Case{Intent: Intent{Description: "science fiction"}, JudgeRubric: "Relevant science fiction"}
	tests := map[string]string{
		"overall":     `{"relevance":0.8,"serendipity":0.6,"reason":"A grounded assessment."}`,
		"relevance":   `{"overall":0.8,"serendipity":0.6,"reason":"A grounded assessment."}`,
		"serendipity": `{"overall":0.8,"relevance":0.8,"reason":"A grounded assessment."}`,
	}
	for name, response := range tests {
		t.Run(name, func(t *testing.T) {
			var scorer Judge = modelJudge{provider: testkit.NewLLM(testkit.FinalResponse(response))}
			if _, err := scorer.Score(context.Background(), NewJudgeEvidence(caseUnderTest, proposal, Observation{}, nil)); err == nil {
				t.Fatalf("Judge.Score accepted output missing %s: %s", name, response)
			}
		})
	}
}

func TestJudgeScoreRejectsOutOfRangeScores(t *testing.T) {
	proposal := suggest.Proposal{Lineup: []suggest.ProposalItem{{
		MediaType: provision.Movie, TMDBID: 603, Name: "The Matrix",
	}}}
	caseUnderTest := Case{Intent: Intent{Description: "science fiction"}, JudgeRubric: "Relevant science fiction"}
	tests := map[string]string{
		"overall below zero":     `{"overall":-0.1,"relevance":0.8,"serendipity":0.6,"reason":"A grounded assessment."}`,
		"overall above one":      `{"overall":1.1,"relevance":0.8,"serendipity":0.6,"reason":"A grounded assessment."}`,
		"relevance below zero":   `{"overall":0.8,"relevance":-0.1,"serendipity":0.6,"reason":"A grounded assessment."}`,
		"relevance above one":    `{"overall":0.8,"relevance":1.1,"serendipity":0.6,"reason":"A grounded assessment."}`,
		"serendipity below zero": `{"overall":0.8,"relevance":0.8,"serendipity":-0.1,"reason":"A grounded assessment."}`,
		"serendipity above one":  `{"overall":0.8,"relevance":0.8,"serendipity":1.1,"reason":"A grounded assessment."}`,
	}
	for name, response := range tests {
		t.Run(name, func(t *testing.T) {
			var scorer Judge = modelJudge{provider: testkit.NewLLM(testkit.FinalResponse(response))}
			if _, err := scorer.Score(context.Background(), NewJudgeEvidence(caseUnderTest, proposal, Observation{}, nil)); err == nil {
				t.Fatalf("Judge.Score accepted out-of-range output: %s", response)
			}
		})
	}
}

func TestJudgeScoreRejectsMissingOrBlankReason(t *testing.T) {
	proposal := suggest.Proposal{Lineup: []suggest.ProposalItem{{
		MediaType: provision.Movie, TMDBID: 603, Name: "The Matrix",
	}}}
	caseUnderTest := Case{Intent: Intent{Description: "science fiction"}, JudgeRubric: "Relevant science fiction"}
	tests := map[string]string{
		"missing": `{"overall":0.8,"relevance":0.8,"serendipity":0.6}`,
		"blank":   `{"overall":0.8,"relevance":0.8,"serendipity":0.6,"reason":"   "}`,
	}
	for name, response := range tests {
		t.Run(name, func(t *testing.T) {
			var scorer Judge = modelJudge{provider: testkit.NewLLM(testkit.FinalResponse(response))}
			if _, err := scorer.Score(context.Background(), NewJudgeEvidence(caseUnderTest, proposal, Observation{}, nil)); err == nil {
				t.Fatalf("Judge.Score accepted %s reason: %s", name, response)
			}
		})
	}
}

func TestRunnerRecordsStructuralObservationForEveryTrial(t *testing.T) {
	observer := &scriptedObserver{value: Observation{
		ToolCalls: 2, CandidatesSurfaced: 24, GroundingStage: "grounded",
	}}
	runner := NewRunner(scriptedGenerator{proposal: suggest.Proposal{}}, RunnerConfig{Trials: 2}).WithObserver(observer)

	card := runner.Run(context.Background(), []Case{{Name: "observed"}})

	if observer.begins != 2 {
		t.Fatalf("observer begins = %d, want one reset per trial", observer.begins)
	}
	for _, result := range card.Results {
		if result.ToolCalls != 2 || result.CandidatesSurfaced != 24 || result.GroundingStage != "grounded" {
			t.Fatalf("observation = %+v", result.Observation)
		}
	}
}

func TestRunnerRecordsGeneratorAndJudgeIdentitiesIndependently(t *testing.T) {
	generator := ModelIdentity{Provider: "ollama", Model: "qwen3:14b"}
	judge := ModelIdentity{Provider: "openai", Model: "openai/gpt-5.6"}
	runner := NewRunner(scriptedGenerator{proposal: suggest.Proposal{}}, RunnerConfig{
		Generator: generator,
		Judge:     judge,
	})

	card := runner.Run(context.Background(), []Case{{Name: "identity"}})

	if card.SchemaVersion != 4 {
		t.Fatalf("schema version = %d, want 4", card.SchemaVersion)
	}
	if card.Generator != generator || card.Judge != judge {
		t.Fatalf("identities = generator %+v judge %+v", card.Generator, card.Judge)
	}
	wantStages := []FailureStage{
		FailureStageProposal, FailureStageDeterministic, FailureStageStructuralBudget,
		FailureStageSchedule, FailureStageJudge,
	}
	if len(card.FailureCounts) != len(wantStages) || card.Results[0].FailureStage != "" {
		t.Fatalf("passing scorecard failure shape = stage %q counts %v", card.Results[0].FailureStage, card.FailureCounts)
	}
	for _, stage := range wantStages {
		if card.FailureCounts[stage] != 0 {
			t.Errorf("passing scorecard count[%q] = %d, want 0", stage, card.FailureCounts[stage])
		}
	}
	blob, err := json.Marshal(card)
	if err != nil {
		t.Fatal(err)
	}
	var shape map[string]json.RawMessage
	if err := json.Unmarshal(blob, &shape); err != nil {
		t.Fatal(err)
	}
	for _, legacy := range []string{"provider", "model"} {
		if _, ok := shape[legacy]; ok {
			t.Errorf("scorecard retained ambiguous legacy field %q: %s", legacy, blob)
		}
	}
}

func TestRunnerRejectsForbiddenIdentityMediaMixDiversityAndStructuralBudget(t *testing.T) {
	observer := &scriptedObserver{value: Observation{ToolCalls: 3, CandidatesSurfaced: 25}}
	proposal := suggest.Proposal{Lineup: []suggest.ProposalItem{{
		MediaType: provision.Movie, TMDBID: 603, Name: "The Matrix", Genres: []string{"Science Fiction"},
	}}}
	runner := NewRunner(scriptedGenerator{proposal: proposal}, RunnerConfig{}).WithObserver(observer)
	card := runner.Run(context.Background(), []Case{{Name: "bounded_mix",
		ForbidKeys: []provision.Key{"movie:tmdb:603"}, MinMovies: 2, MinSeries: 1,
		MinDistinctGenres: 2, MaxToolCalls: 2, MaxCandidatesSurfaced: 24,
	}})
	if card.Certified || len(card.Results[0].Failures) != 6 {
		t.Fatalf("result = %+v, want six independent hard failures", card.Results[0])
	}
	if result := card.Results[0]; result.FailureStage != FailureStageDeterministic || card.FailureCounts[FailureStageDeterministic] != 1 || card.FailureCounts[FailureStageStructuralBudget] != 0 {
		t.Fatalf("mixed failure accounting = stage %q counts %v", result.FailureStage, card.FailureCounts)
	}

	budgetCard := NewRunner(scriptedGenerator{proposal: proposal}, RunnerConfig{}).
		WithObserver(observer).
		Run(context.Background(), []Case{{Name: "budget_only", MaxToolCalls: 2, MaxCandidatesSurfaced: 24}})
	if budgetCard.Certified || len(budgetCard.Results[0].Failures) != 2 {
		t.Fatalf("budget-only result = %+v, want two structural-budget failures", budgetCard.Results[0])
	}
	if result := budgetCard.Results[0]; result.FailureStage != FailureStageStructuralBudget || budgetCard.FailureCounts[FailureStageStructuralBudget] != 1 {
		t.Fatalf("structural-budget accounting = stage %q counts %v", result.FailureStage, budgetCard.FailureCounts)
	}
}

func TestRunnerCertifiesCuratedEpisodesAndAtomicMovieSequence(t *testing.T) {
	series := provision.Key("series:tmdb:456")
	movieA := provision.Key("movie:tmdb:1")
	movieB := provision.Key("movie:tmdb:2")
	proposal := suggest.Proposal{Lineup: []suggest.ProposalItem{
		{MediaType: provision.Series, TMDBID: 456, Name: "The Simpsons", EpisodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeHighlights}},
		{MediaType: provision.Movie, TMDBID: 1, Name: "First", Year: 1981},
		{MediaType: provision.Movie, TMDBID: 2, Name: "Second", Year: 1984},
	}}
	runner := NewRunner(scriptedGenerator{proposal: proposal}, RunnerConfig{}).WithMaterializer(NewFixtureScheduleMaterializer(map[provision.Key]FixtureTitle{
		series: {Episodes: []schedule.ResolvedProgram{
			{LibraryItemID: "low", DurationMs: 1, Season: 1, Episode: 1, CommunityRating: 1},
			{LibraryItemID: "e2", DurationMs: 1, Season: 1, Episode: 2, CommunityRating: 9},
			{LibraryItemID: "e3", DurationMs: 1, Season: 1, Episode: 3, CommunityRating: 8},
			{LibraryItemID: "e4", DurationMs: 1, Season: 1, Episode: 4, CommunityRating: 7},
			{LibraryItemID: "e5", DurationMs: 1, Season: 1, Episode: 5, CommunityRating: 6},
		}},
		movieA: {LibraryItemID: "movie-a", DurationMs: 1, CollectionID: 99},
		movieB: {LibraryItemID: "movie-b", DurationMs: 1, CollectionID: 99},
	}))
	card := runner.Run(context.Background(), []Case{{Name: "viewer_outcome",
		ForbidScheduledPrograms:  []string{"series:tmdb:456:s01e01"},
		RequireScheduledSequence: []string{"movie:tmdb:1", "movie:tmdb:2"},
	}})
	if !card.Certified {
		t.Fatalf("viewer outcome failed: %+v", card.Results[0].Failures)
	}
}

func TestDeterministicChecksCountGroundedTitlesAcrossOwnership(t *testing.T) {
	caseUnderTest := Case{MinGrounded: 2}
	proposal := suggest.Proposal{
		Lineup:       []suggest.ProposalItem{{MediaType: provision.Movie, TMDBID: 1, Name: "Owned"}},
		Acquisitions: []suggest.ProposalItem{{MediaType: provision.Movie, TMDBID: 2, Name: "Discovered"}},
	}
	if failures := deterministicChecks(caseUnderTest, proposal, nil); len(failures) != 0 {
		t.Fatalf("ownership-neutral grounded count failed: %v", failures)
	}
	caseUnderTest.MinGrounded = 3
	if failures := deterministicChecks(caseUnderTest, proposal, nil); len(failures) != 1 {
		t.Fatalf("grounded shortfall failures = %v, want one", failures)
	}
}

func TestObservationClassifiesProviderFailureSeparately(t *testing.T) {
	observed := &observedProvider{obs: Observation{ToolCalls: 1, CandidatesSurfaced: 12}}
	if stage := observed.Snapshot(errors.New("llm chat: hosted provider unavailable")).GroundingStage; stage != "provider_error" {
		t.Fatalf("grounding stage = %q, want provider_error", stage)
	}
}

func TestObservationClassifiesInvalidModelOutputAsGenerationFailure(t *testing.T) {
	observed := &observedProvider{obs: Observation{ToolCalls: 2, CandidatesSurfaced: 24}}
	err := errors.New("suggester: model output not valid after 2 repairs")
	if stage := observed.Snapshot(err).GroundingStage; stage != "generation_error" {
		t.Fatalf("grounding stage = %q, want generation_error", stage)
	}
}

func TestNoFabricationDoesNotTurnProviderFailureIntoPass(t *testing.T) {
	runner := NewRunner(scriptedGenerator{err: errors.New("llm chat: unavailable")}, RunnerConfig{})
	card := runner.Run(context.Background(), []Case{{Name: "provider_failure", NoFabrication: true}})

	if card.Certified {
		t.Fatal("scorecard certified a provider failure under the no-fabrication gate")
	}
	result := card.Results[0]
	if len(result.Failures) != 1 {
		t.Fatalf("provider failure under no-fabrication gate = %v, want one failure", result.Failures)
	}
	if result.FailureStage != FailureStageProposal || card.FailureCounts[FailureStageProposal] != 1 {
		t.Fatalf("proposal failure accounting = stage %q counts %v", result.FailureStage, card.FailureCounts)
	}
}

// The frontend owns the shipped product data while this package owns the live
// certification corpus. Pin their exact starter ids/descriptions/constraints
// together so either side changing cannot silently certify a different request.
func TestCorpusContainsExactShippedStarterIntents(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "..", "web", "packages", "core", "src", "templates", "templates.ts"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	wantIDs := []string{"saturday-cartoons", "cozy-mystery", "late-night-scifi", "action-marathon"}
	seen := make(map[string]bool, len(wantIDs))
	for _, c := range Corpus {
		if c.TemplateID == "" {
			continue
		}
		seen[c.TemplateID] = true
		for _, exact := range []string{c.TemplateID, c.Intent.Description, c.Intent.Era, c.Intent.Tone} {
			if exact != "" && !strings.Contains(text, exact) {
				t.Errorf("template %q certification value %q is absent from shipped templates", c.TemplateID, exact)
			}
		}
	}
	for _, id := range wantIDs {
		if !seen[id] {
			t.Errorf("shipped starter template %q has no exact certification case", id)
		}
	}
}

func TestDeterministicChecksRejectNegativeConstraintViolations(t *testing.T) {
	caseUnderTest := Case{
		MinLineup: 1, ForbidGenres: []string{"horror"}, ForbidTitleTerms: []string{"Saw"},
	}
	proposal := suggest.Proposal{Lineup: []suggest.ProposalItem{
		{MediaType: provision.Movie, TMDBID: 1, Name: "Saw II", Genres: []string{"Horror"}},
	}}
	failures := deterministicChecks(caseUnderTest, proposal, nil)
	if len(failures) != 2 {
		t.Fatalf("negative constraint failures = %v, want genre + title", failures)
	}
}

func TestDeterministicChecksRejectWrongOrdering(t *testing.T) {
	proposal := suggest.Proposal{Policy: schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Ordering: schedule.OrderSequential}}}
	failures := deterministicChecks(Case{ExpectOrdering: "syndication"}, proposal, nil)
	if len(failures) != 1 {
		t.Fatalf("ordering failures = %v, want one", failures)
	}
}

func TestScorecardShapeExcludesCredentials(t *testing.T) {
	t.Setenv("LLM_API_KEY", "never-serialize-this-secret")
	t.Setenv("LOOMARR_EVAL_JUDGE_API_KEY", "never-serialize-this-judge-secret")
	scorecard := Scorecard{
		SchemaVersion: scorecardSchemaVersion, CorpusVersion: corpusVersion,
		Generator: ModelIdentity{Provider: "openai", Model: "example/generator"},
		Judge:     ModelIdentity{Provider: "openai", Model: "example/judge"}, Results: []Result{{
			Case: "holiday", RelevanceScore: 0.8, SerendipityScore: 0.7,
		}},
	}
	blob, err := json.Marshal(scorecard)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{os.Getenv("LLM_API_KEY"), os.Getenv("LOOMARR_EVAL_JUDGE_API_KEY")} {
		if strings.Contains(string(blob), secret) {
			t.Fatal("scorecard metadata contains an LLM credential")
		}
	}
	for _, field := range []string{"relevanceScore", "serendipityScore"} {
		if !strings.Contains(string(blob), field) {
			t.Errorf("scorecard is missing quality dimension %q: %s", field, blob)
		}
	}
}
