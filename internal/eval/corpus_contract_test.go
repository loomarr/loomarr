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

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/suggest"
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
	scores []judgeScores
	next   int
}

type scriptedObserver struct {
	begins int
	value  Observation
}

func (o *scriptedObserver) Begin() { o.begins++ }

func (o *scriptedObserver) Snapshot(error) Observation { return o.value }

func (j *sequenceJudge) Score(context.Context, Case, suggest.Proposal) judgeScores {
	score := j.scores[j.next]
	j.next++
	return score
}

func (g *sequenceGenerator) Suggest(context.Context, suggest.Intent) (suggest.Proposal, error) {
	proposal := g.proposals[g.next]
	g.next++
	return proposal, nil
}

func TestRunnerRejectsProposalMissingExactRequiredKey(t *testing.T) {
	runner := NewRunner(scriptedGenerator{proposal: suggest.Proposal{
		Lineup: []suggest.ProposalItem{{MediaType: provision.Movie, TMDBID: 680, Name: "Pulp Fiction"}},
	}}, RunnerConfig{Trials: 1, Profile: "hermetic", Provider: "fixture", Model: "fixture-v1"})

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
}

func TestRunnerReportsWorstMedianAndBestQualityAcrossTrials(t *testing.T) {
	proposal := suggest.Proposal{Lineup: []suggest.ProposalItem{{
		MediaType: provision.Movie, TMDBID: 603, Name: "The Matrix",
	}}}
	judge := &sequenceJudge{scores: []judgeScores{
		{Overall: 0.5, Relevance: 0.4, Serendipity: 0.8},
		{Overall: 0.8, Relevance: 0.9, Serendipity: 0.3},
		{Overall: 0.7, Relevance: 0.7, Serendipity: 0.6},
	}}
	runner := NewRunner(scriptedGenerator{proposal: proposal}, RunnerConfig{Trials: 3}).WithJudge(judge)

	card := runner.Run(context.Background(), []Case{{
		Name: "quality_distribution", JudgeRubric: "Relevant science fiction with defensible variety",
	}})

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

func TestRunnerRejectsForbiddenIdentityMediaMixDiversityAndStructuralBudget(t *testing.T) {
	observer := &scriptedObserver{value: Observation{ToolCalls: 3, CandidatesSurfaced: 25}}
	runner := NewRunner(scriptedGenerator{proposal: suggest.Proposal{Lineup: []suggest.ProposalItem{{
		MediaType: provision.Movie, TMDBID: 603, Name: "The Matrix", Genres: []string{"Science Fiction"},
	}}}}, RunnerConfig{}).WithObserver(observer)
	card := runner.Run(context.Background(), []Case{{Name: "bounded_mix",
		ForbidKeys: []provision.Key{"movie:tmdb:603"}, MinMovies: 2, MinSeries: 1,
		MinDistinctGenres: 2, MaxToolCalls: 2, MaxCandidatesSurfaced: 24,
	}})
	if card.Certified || len(card.Results[0].Failures) != 6 {
		t.Fatalf("result = %+v, want six independent hard failures", card.Results[0])
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
	failures := deterministicChecks(Case{NoFabrication: true}, suggest.Proposal{}, errors.New("llm chat: unavailable"))
	if len(failures) != 1 {
		t.Fatalf("provider failure under no-fabrication gate = %v, want one failure", failures)
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
	scorecard := Scorecard{
		SchemaVersion: scorecardSchemaVersion, CorpusVersion: corpusVersion,
		Provider: "openai", Model: "example/model", Results: []Result{{
			Case: "holiday", RelevanceScore: 0.8, SerendipityScore: 0.7,
		}},
	}
	blob, err := json.Marshal(scorecard)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(blob), os.Getenv("LLM_API_KEY")) {
		t.Fatal("scorecard metadata contains the LLM credential")
	}
	for _, field := range []string{"relevanceScore", "serendipityScore"} {
		if !strings.Contains(string(blob), field) {
			t.Errorf("scorecard is missing quality dimension %q: %s", field, blob)
		}
	}
}
