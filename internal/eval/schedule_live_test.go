//go:build eval

package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/tmdb"
)

type intentRecordingGenerator struct {
	proposal suggest.Proposal
	intents  []suggest.Intent
}

func (g *intentRecordingGenerator) Suggest(_ context.Context, intent suggest.Intent) (suggest.Proposal, error) {
	g.intents = append(g.intents, intent)
	return g.proposal, nil
}

func TestLiveScheduleCertificationRejectsUnknownCaseName(t *testing.T) {
	prepared := LiveScheduleCertification{Cases: []Case{{Name: "known"}}}
	if _, err := prepared.Case("unknown"); err == nil {
		t.Fatal("unknown live schedule case returned assertion-free Case instead of an error")
	}
}

func TestPreparedLiveScheduleCasesRequireJudgedMaterializedOutcomes(t *testing.T) {
	media, tmdbServer, snapshot := liveScheduleEvidenceFixture(t)
	prepared, err := PrepareLiveScheduleCertification(context.Background(), snapshot,
		library.New(library.Emby, media.URL, media.AdminToken, "eval-test"),
		tmdb.NewWithBase(tmdbServer.URL, "test-key"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Cases) != liveScheduleCaseCount {
		t.Fatalf("prepared live cases = %d", len(prepared.Cases))
	}
	for _, c := range prepared.Cases {
		if c.JudgeRubric == "" || c.MinJudgeScore <= 0 || c.MinRelevanceScore <= 0 || c.MinSerendipityScore <= 0 {
			t.Errorf("live case %q judge contract = rubric %q floors %.2f/%.2f/%.2f",
				c.Name, c.JudgeRubric, c.MinJudgeScore, c.MinRelevanceScore, c.MinSerendipityScore)
		}
	}
}

func TestRunnerSendsCanonicalLiveSeriesIntentThroughGenerationAndSchedule(t *testing.T) {
	media, tmdbServer, snapshot := liveScheduleEvidenceFixture(t)
	prepared, err := PrepareLiveScheduleCertification(context.Background(), snapshot,
		library.New(library.Emby, media.URL, media.AdminToken, "eval-test"),
		tmdb.NewWithBase(tmdbServer.URL, "test-key"),
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name        string
		description string
		proposal    suggest.Proposal
	}{
		{
			name: "schedule_classic_simpsons_highlights", description: "Classic Simpsons reruns from the golden era, curated for variety",
			proposal: suggest.Proposal{Policy: schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Ordering: schedule.OrderSyndication}}, Lineup: []suggest.ProposalItem{{
				MediaType: provision.Series, TMDBID: 456, Name: "The Simpsons", InLibrary: true,
				LibraryItemID: "show-simpsons", EpisodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeHighlights},
			}}},
		},
		{
			name: "schedule_owned_simpsons_christmas", description: "Christmas episodes of The Simpsons already in my library",
			proposal: suggest.Proposal{Policy: schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{
				Seasonal: schedule.SeasonalPolicy{Mode: schedule.SeasonalExclusive, Holidays: []string{"christmas"}},
			}}, Lineup: []suggest.ProposalItem{{
				MediaType: provision.Series, TMDBID: 456, Name: "The Simpsons", InLibrary: true,
				LibraryItemID: "show-simpsons", EpisodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeHoliday, Holidays: []string{"christmas"}},
			}}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := requireLiveScheduleCase(t, prepared, tc.name)
			if c.Intent.Description != tc.description {
				t.Fatalf("prepared semantic request = %q, want %q", c.Intent.Description, tc.description)
			}
			generator := &intentRecordingGenerator{proposal: tc.proposal}
			card := NewRunner(generator, RunnerConfig{}).
				WithMaterializer(prepared.Materializer).
				WithJudge(&sequenceJudge{scores: []JudgeScores{{
					Overall: 0.9, Relevance: 0.9, Serendipity: 0.8, Reason: "The concrete schedule satisfies the request.",
				}}}).
				Run(context.Background(), []Case{c})
			if !card.Certified || len(generator.intents) != 1 || generator.intents[0].Description != tc.description {
				t.Fatalf("semantic request did not cross Runner generation: intents=%+v result=%+v", generator.intents, card.Results[0])
			}
			for _, required := range c.RequireScheduledPrograms {
				if !slices.Contains(card.Results[0].ScheduledPrograms, required) {
					t.Errorf("materialized schedule %v omitted pinned required %q", card.Results[0].ScheduledPrograms, required)
				}
			}
			for _, forbidden := range c.ForbidScheduledPrograms {
				if slices.Contains(card.Results[0].ScheduledPrograms, forbidden) {
					t.Errorf("materialized schedule %v included pinned forbidden %q", card.Results[0].ScheduledPrograms, forbidden)
				}
			}
		})
	}
}

func requireLiveScheduleCase(t *testing.T, prepared LiveScheduleCertification, name string) Case {
	t.Helper()
	c, err := prepared.Case(name)
	if err != nil {
		t.Fatal(err)
	}
	return c
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

func TestRunnerRejectsForbiddenEmittedEpisode(t *testing.T) {
	c := scheduleCorpusCase(t, "schedule_classic_simpsons_highlights")
	proposal := suggest.Proposal{Policy: schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{
		Ordering: schedule.OrderSyndication,
	}}, Lineup: []suggest.ProposalItem{{
		MediaType: provision.Series, TMDBID: 456, Name: "The Simpsons", InLibrary: true,
		LibraryItemID:    "show-simpsons",
		EpisodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeHighlights},
	}}}
	episodes := simpsonsHighlightEpisodes()
	episodes[0].CommunityRating = 10
	for episode := 9; episode <= 20; episode++ {
		episodes = append(episodes, schedule.ResolvedProgram{
			LibraryItemID: fmt.Sprintf("simpsons-s01e%02d", episode),
			Title:         fmt.Sprintf("Episode %d", episode), DurationMs: 22 * 60 * 1000,
			Season: 1, Episode: episode, CommunityRating: 1,
		})
	}
	runner := NewRunner(scriptedGenerator{proposal: proposal}, RunnerConfig{}).
		WithMaterializer(NewFixtureScheduleMaterializer(map[provision.Key]FixtureTitle{
			"series:tmdb:456": {Episodes: episodes},
		}))

	card := runner.Run(context.Background(), []Case{c})
	if card.Certified {
		t.Fatal("certified a final schedule containing a forbidden episode")
	}
	result := card.Results[0]
	if result.FailureStage != FailureStageSchedule || card.FailureCounts[FailureStageSchedule] != 1 {
		t.Fatalf("schedule failure accounting = stage %q counts %v failures %v", result.FailureStage, card.FailureCounts, result.Failures)
	}
	if len(result.Failures) != 1 || !strings.Contains(result.Failures[0], "series:tmdb:456:s01e01") {
		t.Fatalf("failures = %v, want forbidden emitted episode", result.Failures)
	}
}

func TestRunnerRejectsWrongRequiredSequence(t *testing.T) {
	c := scheduleCorpusCase(t, "schedule_movie_franchise_release_order")
	proposal := suggest.Proposal{Lineup: []suggest.ProposalItem{
		{MediaType: provision.Movie, TMDBID: 89, Name: "Indiana Jones and the Last Crusade", Year: 1989, InLibrary: true, LibraryItemID: "last-crusade"},
		{MediaType: provision.Movie, TMDBID: 87, Name: "Indiana Jones and the Temple of Doom", Year: 1984, InLibrary: true, LibraryItemID: "temple"},
		{MediaType: provision.Movie, TMDBID: 85, Name: "Raiders of the Lost Ark", Year: 1981, InLibrary: true, LibraryItemID: "raiders"},
	}}
	runner := NewRunner(scriptedGenerator{proposal: proposal}, RunnerConfig{}).
		WithMaterializer(NewFixtureScheduleMaterializer(map[provision.Key]FixtureTitle{
			"movie:tmdb:85": {LibraryItemID: "raiders", DurationMs: 115 * 60 * 1000, CollectionID: -1},
			"movie:tmdb:87": {LibraryItemID: "temple", DurationMs: 118 * 60 * 1000, CollectionID: -1},
			"movie:tmdb:89": {LibraryItemID: "last-crusade", DurationMs: 127 * 60 * 1000, CollectionID: -1},
		}))
	card := runner.Run(context.Background(), []Case{c})
	if card.Certified || card.Results[0].FailureStage != FailureStageSchedule || card.FailureCounts[FailureStageSchedule] != 1 {
		t.Fatalf("wrong final sequence result = %+v counts %v", card.Results[0], card.FailureCounts)
	}
}

func TestRunnerEmitsReversedInterleavedFranchiseAtomicallyInReleaseOrder(t *testing.T) {
	c := scheduleCorpusCase(t, "schedule_movie_franchise_release_order")
	proposal := suggest.Proposal{Lineup: []suggest.ProposalItem{
		{MediaType: provision.Movie, TMDBID: 89, Name: "Indiana Jones and the Last Crusade", Year: 1989, InLibrary: true, LibraryItemID: "last-crusade"},
		{MediaType: provision.Movie, TMDBID: 603, Name: "The Matrix", Year: 1999, InLibrary: true, LibraryItemID: "matrix"},
		{MediaType: provision.Movie, TMDBID: 87, Name: "Indiana Jones and the Temple of Doom", Year: 1984, InLibrary: true, LibraryItemID: "temple"},
		{MediaType: provision.Movie, TMDBID: 85, Name: "Raiders of the Lost Ark", Year: 1981, InLibrary: true, LibraryItemID: "raiders"},
	}}
	runner := NewRunner(scriptedGenerator{proposal: proposal}, RunnerConfig{}).
		WithMaterializer(NewFixtureScheduleMaterializer(map[provision.Key]FixtureTitle{
			"movie:tmdb:85":  {LibraryItemID: "raiders", DurationMs: 115 * 60 * 1000, CollectionID: 84},
			"movie:tmdb:87":  {LibraryItemID: "temple", DurationMs: 118 * 60 * 1000, CollectionID: 84},
			"movie:tmdb:89":  {LibraryItemID: "last-crusade", DurationMs: 127 * 60 * 1000, CollectionID: 84},
			"movie:tmdb:603": {LibraryItemID: "matrix", DurationMs: 136 * 60 * 1000, CollectionID: -1},
		}))
	card := runner.Run(context.Background(), []Case{c})
	if !card.Certified {
		t.Fatalf("atomic release order failed: %+v", card.Results[0])
	}
	if got := card.Results[0].ScheduledPrograms; !containsSequence(got, c.RequireScheduledSequence) {
		t.Fatalf("final emitted schedule = %v, want atomic release sequence %v", got, c.RequireScheduledSequence)
	}
}

func TestRunnerMaterializesOwnedHolidayEpisodesOnly(t *testing.T) {
	c := scheduleCorpusCase(t, "schedule_owned_simpsons_christmas")
	proposal := suggest.Proposal{Policy: schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{
		Seasonal: schedule.SeasonalPolicy{Mode: schedule.SeasonalExclusive, Holidays: []string{"christmas"}},
	}}, Lineup: []suggest.ProposalItem{{
		MediaType: provision.Series, TMDBID: 456, Name: "The Simpsons", InLibrary: true,
		LibraryItemID: "show-simpsons", EpisodeSelection: schedule.EpisodeSelection{
			Mode: schedule.EpisodeHoliday, Holidays: []string{"christmas"},
		},
	}}}
	runner := NewRunner(scriptedGenerator{proposal: proposal}, RunnerConfig{}).
		WithMaterializer(NewFixtureScheduleMaterializer(map[provision.Key]FixtureTitle{
			"series:tmdb:456": {Episodes: simpsonsHolidayEpisodes()},
		}))
	card := runner.Run(context.Background(), []Case{c})
	if !card.Certified {
		t.Fatalf("owned holiday schedule failed: %+v", card.Results[0])
	}
}

func TestRunnerRejectsWrongHolidayEpisodeOutcome(t *testing.T) {
	c := scheduleCorpusCase(t, "schedule_owned_simpsons_christmas")
	proposal := suggest.Proposal{Policy: schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{
		Seasonal: schedule.SeasonalPolicy{Mode: schedule.SeasonalExclusive, Holidays: []string{"christmas"}},
	}}, Lineup: []suggest.ProposalItem{{
		MediaType: provision.Series, TMDBID: 456, Name: "The Simpsons", InLibrary: true,
		LibraryItemID: "show-simpsons", EpisodeSelection: schedule.EpisodeSelection{
			Mode: schedule.EpisodeHoliday, Holidays: []string{"christmas"},
		},
	}}}
	episodes := simpsonsHolidayEpisodes()
	episodes[0].Title = "The Only Christmas Episode"
	episodes[1].Title = "A Regular Tuesday"
	episodes[2].Title = "A Regular Wednesday"
	episodes[3].Title = "Home for Winter"
	episodes[3].Overview = ""
	runner := NewRunner(scriptedGenerator{proposal: proposal}, RunnerConfig{}).
		WithMaterializer(NewFixtureScheduleMaterializer(map[provision.Key]FixtureTitle{
			"series:tmdb:456": {Episodes: episodes},
		}))

	card := runner.Run(context.Background(), []Case{c})
	result := card.Results[0]
	if card.Certified || result.FailureStage != FailureStageSchedule || card.FailureCounts[FailureStageSchedule] != 1 {
		t.Fatalf("wrong holiday outcome = %+v counts %v", result, card.FailureCounts)
	}
	if !slices.Contains(result.ScheduledPrograms, "series:tmdb:456:s02e01") {
		t.Fatalf("emitted schedule = %v, want deliberately wrong holiday episode", result.ScheduledPrograms)
	}
}

func TestRunnerChecksExactTitlesAgainstMaterializedScheduleEvidence(t *testing.T) {
	proposal := suggest.Proposal{Policy: schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{
		Seasonal: schedule.SeasonalPolicy{Mode: schedule.SeasonalExclusive, Holidays: []string{"christmas"}},
	}}, Lineup: []suggest.ProposalItem{{
		MediaType: provision.Series, TMDBID: 456, Name: "The Simpsons",
		EpisodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeHoliday, Holidays: []string{"christmas"}},
	}}}
	runner := NewRunner(scriptedGenerator{proposal: proposal}, RunnerConfig{}).
		WithMaterializer(NewFixtureScheduleMaterializer(map[provision.Key]FixtureTitle{
			"series:tmdb:456": {Episodes: []schedule.ResolvedProgram{{
				LibraryItemID: "near", Title: "A Springfield Christmas Eve", DurationMs: 1,
				Season: 2, Episode: 2, Tags: []string{"christmas"},
			}}},
		}))
	card := runner.Run(context.Background(), []Case{{
		Name: "scheduled_exact_title", TitleEvidence: TitleEvidenceScheduled,
		RequireTitles: []string{"A Springfield Christmas"},
	}})

	result := card.Results[0]
	if card.Certified || result.FailureStage != FailureStageSchedule ||
		!slices.Contains(result.Failures, `required scheduled title "A Springfield Christmas" is missing`) {
		t.Fatalf("scheduled exact title result = %+v", result)
	}
}

func simpsonsHolidayEpisodes() []schedule.ResolvedProgram {
	return []schedule.ResolvedProgram{
		{LibraryItemID: "ordinary", Title: "A Regular Tuesday", DurationMs: 1, Season: 2, Episode: 1},
		{LibraryItemID: "xmas-1", Title: "A Springfield Christmas (1)", DurationMs: 1, Season: 2, Episode: 2},
		{LibraryItemID: "xmas-2", Title: "A Springfield Christmas (2)", DurationMs: 1, Season: 2, Episode: 3},
		{LibraryItemID: "overview", Title: "Home for Winter", Overview: "The family meets Santa", DurationMs: 1, Season: 2, Episode: 4},
	}
}

func TestRunnerLiveScheduleMaterializerUsesLibraryAndTMDBTestServers(t *testing.T) {
	media := testkit.NewMediaServer(t)
	media.ItemMetadata = map[string]testkit.ItemMetadata{
		"movie-raiders": {RunTimeMs: 115 * 60 * 1000},
		"movie-temple":  {RunTimeMs: 118 * 60 * 1000},
		"movie-crusade": {RunTimeMs: 127 * 60 * 1000},
	}
	media.SetEpisodeItems(
		testkit.EpisodeStub{LibraryItemID: "ordinary", Name: "A Regular Tuesday", RunTimeMs: 22 * 60 * 1000, Season: 2, Episode: 1},
		testkit.EpisodeStub{LibraryItemID: "xmas", Name: "A Springfield Christmas", RunTimeMs: 22 * 60 * 1000,
			Season: 2, Episode: 2, ProductionYear: 1990, OfficialRating: "TV-PG", CommunityRating: 8.4,
			Overview: "Springfield gathers for Christmas.", Tags: []string{"christmas", "holiday"}},
	)
	tmdbServer := testkit.NewTMDB(t)
	tmdbServer.AddMovie(85, "Raiders of the Lost Ark", 1981, nil, "")
	tmdbServer.AddMovie(87, "Indiana Jones and the Temple of Doom", 1984, nil, "")
	tmdbServer.AddMovie(89, "Indiana Jones and the Last Crusade", 1989, nil, "")
	tmdbServer.SetCollectionID(85, 84)
	tmdbServer.SetCollectionID(87, 84)
	tmdbServer.SetCollectionID(89, 84)

	proposal := suggest.Proposal{Policy: schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{
		Seasonal: schedule.SeasonalPolicy{Mode: schedule.SeasonalExclusive, Holidays: []string{"christmas"}},
	}}, Lineup: []suggest.ProposalItem{
		{MediaType: provision.Movie, TMDBID: 89, Name: "Indiana Jones and the Last Crusade", Year: 1989, InLibrary: true, LibraryItemID: "movie-crusade"},
		{MediaType: provision.Series, TMDBID: 456, Name: "The Simpsons", InLibrary: true, LibraryItemID: "show-simpsons",
			EpisodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeHoliday, Holidays: []string{"christmas"}},
		},
		{MediaType: provision.Movie, TMDBID: 87, Name: "Indiana Jones and the Temple of Doom", Year: 1984, InLibrary: true, LibraryItemID: "movie-temple"},
		{MediaType: provision.Movie, TMDBID: 85, Name: "Raiders of the Lost Ark", Year: 1981, InLibrary: true, LibraryItemID: "movie-raiders"},
	}}
	judge := newSemanticRecordingJudge(func(evidence JudgeEvidence) error {
		index := slices.IndexFunc(evidence.ScheduledPrograms, func(program MaterializedProgram) bool {
			return program.Identity == "series:tmdb:456:s02e02"
		})
		if index < 0 {
			return fmt.Errorf("materialized holiday episode is missing: %+v", evidence.ScheduledPrograms)
		}
		program := evidence.ScheduledPrograms[index]
		if program.Title != "A Springfield Christmas" || program.Year != 1990 ||
			program.CommunityRating != 8.4 || program.Overview != "Springfield gathers for Christmas." ||
			!slices.Equal(program.Tags, []string{"christmas", "holiday"}) {
			return fmt.Errorf("materialized holiday facts changed: %+v", program)
		}
		return nil
	})
	runner := NewRunner(scriptedGenerator{proposal: proposal}, RunnerConfig{}).WithMaterializer(
		NewLiveScheduleMaterializer(
			library.New(library.Emby, media.URL, media.AdminToken, "eval-test"),
			tmdb.NewWithBase(tmdbServer.URL, "test-key"),
		),
	).WithJudge(judge)
	card := runner.Run(context.Background(), []Case{{
		Name: "live_hydration", MinLineup: 4,
		JudgeRubric:              "Assess the emitted holiday episode and franchise sequence",
		RequireScheduledPrograms: []string{"series:tmdb:456:s02e02", "movie:tmdb:85", "movie:tmdb:87", "movie:tmdb:89"},
		ForbidScheduledPrograms:  []string{"series:tmdb:456:s02e01"},
		RequireScheduledSequence: []string{"movie:tmdb:85", "movie:tmdb:87", "movie:tmdb:89"},
	}})
	if !card.Certified || judge.CallCount() != 1 {
		t.Fatalf("live materialized schedule failed: result=%+v judge calls=%d errors=%v",
			card.Results[0], judge.CallCount(), judge.ValidationErrors())
	}
}

func TestEvalCasesRequireExplicitLiveScheduleCertification(t *testing.T) {
	if _, _, err := evalCases(true, false, nil); err == nil {
		t.Fatal("certification accepted omission of the live schedule corpus")
	}
	exploratory, notice, err := evalCases(false, false, nil)
	if err != nil || len(exploratory) != len(Corpus) || !strings.Contains(notice, "schedule-outcome corpus omitted") {
		t.Fatalf("exploratory cases=%d notice=%q err=%v", len(exploratory), notice, err)
	}
	liveCases := []Case{{Name: "curated"}, {Name: "holiday"}, {Name: "franchise"}}
	certification, notice, err := evalCases(true, true, liveCases)
	if err != nil || notice != "" || len(certification) != len(Corpus)+len(liveCases) {
		t.Fatalf("certification cases=%d notice=%q err=%v", len(certification), notice, err)
	}
}

func TestPrepareLiveScheduleCertificationDerivesEpisodesFromVerifiedSnapshot(t *testing.T) {
	media, tmdbServer, snapshot := liveScheduleEvidenceFixture(t)
	path := filepath.Join(t.TempDir(), "schedule-evidence.json")
	blob, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err = LoadScheduleEvidence(path)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareLiveScheduleCertification(context.Background(), snapshot,
		library.New(library.Emby, media.URL, media.AdminToken, "eval-test"),
		tmdb.NewWithBase(tmdbServer.URL, "test-key"),
	)
	if err != nil {
		t.Fatal(err)
	}
	holiday := requireLiveScheduleCase(t, prepared, "schedule_owned_simpsons_christmas")
	if !slices.Contains(holiday.RequireScheduledPrograms, "series:tmdb:456:s01e01") {
		t.Fatalf("verified Christmas premiere not required: %+v", holiday)
	}
	if slices.Contains(holiday.ForbidScheduledPrograms, "series:tmdb:456:s01e01") {
		t.Fatalf("verified Christmas premiere was copied from synthetic forbidden evidence: %+v", holiday)
	}
	fixture := scheduleCorpusCase(t, "schedule_owned_simpsons_christmas")
	if slices.Equal(holiday.RequireScheduledPrograms, fixture.RequireScheduledPrograms) {
		t.Fatal("synthetic fixture expectations masqueraded as live certification evidence")
	}
}

func TestPrepareLiveScheduleCertificationUsesProductionStructuralBounds(t *testing.T) {
	media, tmdbServer, snapshot := liveScheduleEvidenceFixture(t)
	prepared, err := PrepareLiveScheduleCertification(context.Background(), snapshot,
		library.New(library.Emby, media.URL, media.AdminToken, "eval-test"),
		tmdb.NewWithBase(tmdbServer.URL, "test-key"),
	)
	if err != nil {
		t.Fatal(err)
	}
	bounds := suggest.ProductionBounds()
	for _, c := range prepared.Cases {
		if c.MaxToolCalls != bounds.MaxToolCalls || c.MaxCandidatesSurfaced != bounds.MaxCandidatesSurfaced {
			t.Errorf("live case %q structural budgets = %d/%d, want production %d/%d",
				c.Name, c.MaxToolCalls, c.MaxCandidatesSurfaced,
				bounds.MaxToolCalls, bounds.MaxCandidatesSurfaced)
		}
	}
}

func TestPrepareLiveScheduleCertificationRejectsDriftBeforeInference(t *testing.T) {
	media, tmdbServer, snapshot := liveScheduleEvidenceFixture(t)
	snapshot.Curated.Episodes[0].CommunityRating = 10
	_, err := PrepareLiveScheduleCertification(context.Background(), snapshot,
		library.New(library.Emby, media.URL, media.AdminToken, "eval-test"),
		tmdb.NewWithBase(tmdbServer.URL, "test-key"),
	)
	if err == nil || !strings.Contains(err.Error(), "drifted from Library episode evidence") {
		t.Fatalf("drift preflight error = %v", err)
	}
}

func TestRunnerChecksVerifiedLiveHolidayOutcome(t *testing.T) {
	media, tmdbServer, snapshot := liveScheduleEvidenceFixture(t)
	prepared, err := PrepareLiveScheduleCertification(context.Background(), snapshot,
		library.New(library.Emby, media.URL, media.AdminToken, "eval-test"),
		tmdb.NewWithBase(tmdbServer.URL, "test-key"),
	)
	if err != nil {
		t.Fatal(err)
	}
	holidayCase := requireLiveScheduleCase(t, prepared, "schedule_owned_simpsons_christmas")
	if holidayCase.JudgeRubric == "" || holidayCase.MinJudgeScore <= 0 || holidayCase.MinRelevanceScore <= 0 {
		t.Fatalf("live holiday judge contract = rubric %q floors %.2f/%.2f", holidayCase.JudgeRubric, holidayCase.MinJudgeScore, holidayCase.MinRelevanceScore)
	}
	judge := newSemanticRecordingJudge(func(evidence JudgeEvidence) error {
		index := slices.IndexFunc(evidence.ScheduledPrograms, func(program MaterializedProgram) bool {
			return program.Identity == "series:tmdb:456:s01e01"
		})
		if index < 0 {
			return fmt.Errorf("pinned holiday episode is missing: %+v", evidence.ScheduledPrograms)
		}
		program := evidence.ScheduledPrograms[index]
		if program.Title != "Simpsons Roasting on an Open Fire" || program.Year != 1989 ||
			program.CommunityRating != 6.1 || program.Overview != "The Simpsons celebrate Christmas together." {
			return fmt.Errorf("pinned holiday evidence changed: %+v", program)
		}
		return nil
	})
	proposal := suggest.Proposal{Policy: schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{
		Seasonal: schedule.SeasonalPolicy{Mode: schedule.SeasonalExclusive, Holidays: []string{"christmas"}},
	}}, Lineup: []suggest.ProposalItem{{
		MediaType: provision.Series, TMDBID: 456, Name: "The Simpsons", InLibrary: true,
		LibraryItemID: "show-simpsons", EpisodeSelection: schedule.EpisodeSelection{
			Mode: schedule.EpisodeHoliday, Holidays: []string{"christmas"},
		},
	}}}
	card := NewRunner(scriptedGenerator{proposal: proposal}, RunnerConfig{}).
		WithMaterializer(prepared.Materializer).
		WithJudge(judge).
		Run(context.Background(), []Case{holidayCase})
	if !card.Certified || !slices.Contains(card.Results[0].ScheduledPrograms, "series:tmdb:456:s01e01") {
		t.Fatalf("verified live holiday result = %+v", card.Results[0])
	}
	if judge.CallCount() != 1 {
		t.Fatalf("live holiday schedule did not reach Judge.Score: calls=%d errors=%v",
			judge.CallCount(), judge.ValidationErrors())
	}
}

func TestRunnerDoesNotRecomputePinnedScheduleOracle(t *testing.T) {
	media, tmdbServer, snapshot := liveScheduleEvidenceFixture(t)
	snapshot.Curated.RequiredPrograms[0] = "series:tmdb:456:s01e01"
	snapshot.Curated.ForbiddenPrograms[0] = "series:tmdb:456:s01e02"
	prepared, err := PrepareLiveScheduleCertification(context.Background(), snapshot,
		library.New(library.Emby, media.URL, media.AdminToken, "eval-test"),
		tmdb.NewWithBase(tmdbServer.URL, "test-key"),
	)
	if err != nil {
		t.Fatalf("independent pinned oracle should pass evidence validation: %v", err)
	}
	proposal := suggest.Proposal{Policy: schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{
		Ordering: schedule.OrderSyndication,
	}}, Lineup: []suggest.ProposalItem{{
		MediaType: provision.Series, TMDBID: 456, Name: "The Simpsons", InLibrary: true,
		LibraryItemID: "show-simpsons", EpisodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeHighlights},
	}}}
	card := NewRunner(scriptedGenerator{proposal: proposal}, RunnerConfig{}).
		WithMaterializer(prepared.Materializer).
		Run(context.Background(), []Case{requireLiveScheduleCase(t, prepared, "schedule_classic_simpsons_highlights")})
	if card.Certified || card.Results[0].FailureStage != FailureStageSchedule {
		t.Fatalf("production scheduler recomputed its own oracle: %+v", card.Results[0])
	}
}

func TestRunnerRejectsUndeclaredLiveScheduleLineupKey(t *testing.T) {
	media, tmdbServer, snapshot := liveScheduleEvidenceFixture(t)
	prepared, err := PrepareLiveScheduleCertification(context.Background(), snapshot,
		library.New(library.Emby, media.URL, media.AdminToken, "eval-test"),
		tmdb.NewWithBase(tmdbServer.URL, "test-key"),
	)
	if err != nil {
		t.Fatal(err)
	}
	proposal := suggest.Proposal{Policy: schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{
		Ordering: schedule.OrderSyndication,
	}}, Lineup: []suggest.ProposalItem{
		{MediaType: provision.Series, TMDBID: 456, Name: "The Simpsons", InLibrary: true,
			LibraryItemID: "show-simpsons", EpisodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeHighlights}},
		{MediaType: provision.Movie, TMDBID: 85, Name: "Raiders of the Lost Ark", Year: 1981,
			InLibrary: true, LibraryItemID: "raiders"},
	}}
	card := NewRunner(scriptedGenerator{proposal: proposal}, RunnerConfig{}).
		WithMaterializer(prepared.Materializer).
		Run(context.Background(), []Case{requireLiveScheduleCase(t, prepared, "schedule_classic_simpsons_highlights")})
	result := card.Results[0]
	if card.Certified || result.FailureStage != FailureStageSchedule || card.FailureCounts[FailureStageSchedule] != 1 {
		t.Fatalf("undeclared live lineup result = %+v counts %v", result, card.FailureCounts)
	}
	if len(result.Failures) != 1 || !strings.Contains(result.Failures[0], "undeclared lineup key movie:tmdb:85") {
		t.Fatalf("undeclared live lineup failures = %v", result.Failures)
	}
}

func TestLoadScheduleEvidenceRejectsMissingPrerequisite(t *testing.T) {
	if _, err := LoadScheduleEvidence(filepath.Join(t.TempDir(), "missing-snapshot.json")); err == nil {
		t.Fatal("missing live evidence was accepted")
	}
	if _, _, err := evalCases(true, false, nil); err == nil {
		t.Fatal("required certification accepted no live schedule prerequisite")
	}
}

func TestLoadScheduleEvidenceRejectsMalformedTrailingJSON(t *testing.T) {
	_, _, snapshot := liveScheduleEvidenceFixture(t)
	blob, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "trailing.json")
	if err := os.WriteFile(path, append(blob, '{'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadScheduleEvidence(path); err == nil {
		t.Fatal("malformed trailing JSON was accepted")
	}
}

func TestPrepareLiveScheduleCertificationRejectsWhitespaceSnapshotID(t *testing.T) {
	_, _, snapshot := liveScheduleEvidenceFixture(t)
	snapshot.SnapshotID = "   "
	if _, err := PrepareLiveScheduleCertification(context.Background(), snapshot, nil, nil); err == nil {
		t.Fatal("whitespace snapshot id was accepted")
	}
}

func TestPrepareLiveScheduleCertificationRejectsSeriesKeySubstitution(t *testing.T) {
	for _, caseName := range []string{"curated", "holiday"} {
		t.Run(caseName, func(t *testing.T) {
			media, tmdbServer, snapshot := liveScheduleEvidenceFixture(t)
			evidence := &snapshot.Curated
			if caseName == "holiday" {
				evidence = &snapshot.Holiday
			}
			evidence.Key = "series:tmdb:999"
			for i := range evidence.RequiredPrograms {
				evidence.RequiredPrograms[i] = strings.Replace(evidence.RequiredPrograms[i], "series:tmdb:456", "series:tmdb:999", 1)
			}
			for i := range evidence.ForbiddenPrograms {
				evidence.ForbiddenPrograms[i] = strings.Replace(evidence.ForbiddenPrograms[i], "series:tmdb:456", "series:tmdb:999", 1)
			}

			_, err := PrepareLiveScheduleCertification(context.Background(), snapshot,
				library.New(library.Emby, media.URL, media.AdminToken, "eval-test"),
				tmdb.NewWithBase(tmdbServer.URL, "test-key"),
			)
			if err == nil {
				t.Fatal("live schedule evidence accepted a substituted series Key")
			}
		})
	}
}

func TestPrepareLiveScheduleCertificationRejectsSeriesLibraryIdentitySubstitution(t *testing.T) {
	media, tmdbServer, snapshot := liveScheduleEvidenceFixture(t)
	snapshot.Curated.LibraryItemID = "unrelated-series"
	snapshot.Holiday.LibraryItemID = "unrelated-series"

	_, err := PrepareLiveScheduleCertification(context.Background(), snapshot,
		library.New(library.Emby, media.URL, media.AdminToken, "eval-test"),
		tmdb.NewWithBase(tmdbServer.URL, "test-key"),
	)
	if err == nil {
		t.Fatal("live schedule evidence accepted a series Library item unrelated to TMDB 456")
	}
}

func TestPrepareLiveScheduleCertificationRejectsMovieLibraryIdentitySubstitution(t *testing.T) {
	media, tmdbServer, snapshot := liveScheduleEvidenceFixture(t)
	snapshot.Franchise.Movies[0].LibraryItemID = "unrelated-movie"
	media.ItemMetadata["unrelated-movie"] = testkit.ItemMetadata{RunTimeMs: 115 * 60 * 1000}

	_, err := PrepareLiveScheduleCertification(context.Background(), snapshot,
		library.New(library.Emby, media.URL, media.AdminToken, "eval-test"),
		tmdb.NewWithBase(tmdbServer.URL, "test-key"),
	)
	if err == nil {
		t.Fatal("live schedule evidence accepted a movie Library item unrelated to TMDB 85")
	}
}

func liveScheduleEvidenceFixture(t *testing.T) (*testkit.MediaServer, *testkit.TMDB, ScheduleEvidenceSnapshot) {
	t.Helper()
	media := testkit.NewMediaServer(t)
	media.SetSearchItems(
		testkit.SearchStub{LibraryItemID: "show-simpsons", Name: "The Simpsons", Type: "Series", TMDBID: 456},
		testkit.SearchStub{LibraryItemID: "raiders", Name: "Raiders of the Lost Ark", Type: "Movie", TMDBID: 85},
		testkit.SearchStub{LibraryItemID: "temple", Name: "Indiana Jones and the Temple of Doom", Type: "Movie", TMDBID: 87},
		testkit.SearchStub{LibraryItemID: "crusade", Name: "Indiana Jones and the Last Crusade", Type: "Movie", TMDBID: 89},
	)
	episodeStubs := make([]testkit.EpisodeStub, 0, 8)
	evidence := make([]ScheduleEpisodeEvidence, 0, 8)
	for i, rating := range []float64{6.1, 9.4, 6.3, 8.9, 6.0, 9.1, 6.2, 8.8} {
		title := fmt.Sprintf("Verified Episode %d", i+1)
		if i == 0 {
			title = "Simpsons Roasting on an Open Fire"
		}
		overview := ""
		if i == 0 {
			overview = "The Simpsons celebrate Christmas together."
		}
		id := fmt.Sprintf("verified-s01e%02d", i+1)
		episodeStubs = append(episodeStubs, testkit.EpisodeStub{
			LibraryItemID: id, Name: title, RunTimeMs: 22 * 60 * 1000,
			Season: 1, Episode: i + 1, ProductionYear: 1989 + i/4, CommunityRating: rating, Overview: overview,
		})
		evidence = append(evidence, ScheduleEpisodeEvidence{
			LibraryItemID: id, Title: title, DurationMs: 22 * 60 * 1000,
			Season: 1, Episode: i + 1, Year: 1989 + i/4, CommunityRating: rating, Overview: overview,
		})
	}
	media.SetEpisodeItems(episodeStubs...)
	media.ItemMetadata = map[string]testkit.ItemMetadata{
		"raiders": {RunTimeMs: 115 * 60 * 1000},
		"temple":  {RunTimeMs: 118 * 60 * 1000},
		"crusade": {RunTimeMs: 127 * 60 * 1000},
	}
	tmdbServer := testkit.NewTMDB(t)
	for _, movie := range []struct {
		id, year int
		name     string
	}{
		{85, 1981, "Raiders of the Lost Ark"},
		{87, 1984, "Indiana Jones and the Temple of Doom"},
		{89, 1989, "Indiana Jones and the Last Crusade"},
	} {
		tmdbServer.AddMovie(movie.id, movie.name, movie.year, nil, "")
		tmdbServer.SetCollectionID(movie.id, 84)
	}
	curated := ScheduleSeriesEvidence{
		Key: "series:tmdb:456", Name: "The Simpsons", LibraryItemID: "show-simpsons", Episodes: evidence,
		RequiredPrograms:  []string{"series:tmdb:456:s01e02", "series:tmdb:456:s01e04", "series:tmdb:456:s01e06", "series:tmdb:456:s01e08"},
		ForbiddenPrograms: []string{"series:tmdb:456:s01e01", "series:tmdb:456:s01e03", "series:tmdb:456:s01e05", "series:tmdb:456:s01e07"},
	}
	holiday := ScheduleSeriesEvidence{
		Key: "series:tmdb:456", Name: "The Simpsons", LibraryItemID: "show-simpsons", Episodes: evidence,
		RequiredPrograms:  []string{"series:tmdb:456:s01e01"},
		ForbiddenPrograms: []string{"series:tmdb:456:s01e02", "series:tmdb:456:s01e03", "series:tmdb:456:s01e04", "series:tmdb:456:s01e05", "series:tmdb:456:s01e06", "series:tmdb:456:s01e07", "series:tmdb:456:s01e08"},
	}
	return media, tmdbServer, ScheduleEvidenceSnapshot{
		SchemaVersion: 1, SnapshotID: "library-2026-08-27",
		Curated: curated, Holiday: holiday,
		Franchise: ScheduleFranchiseEvidence{Movies: []ScheduleMovieEvidence{
			{Key: "movie:tmdb:85", Name: "Raiders of the Lost Ark", LibraryItemID: "raiders", DurationMs: 115 * 60 * 1000, CollectionID: 84},
			{Key: "movie:tmdb:87", Name: "Indiana Jones and the Temple of Doom", LibraryItemID: "temple", DurationMs: 118 * 60 * 1000, CollectionID: 84},
			{Key: "movie:tmdb:89", Name: "Indiana Jones and the Last Crusade", LibraryItemID: "crusade", DurationMs: 127 * 60 * 1000, CollectionID: 84},
		},
			RequiredSequence: []string{"movie:tmdb:85", "movie:tmdb:87", "movie:tmdb:89"},
		},
	}
}

func scheduleCorpusCase(t *testing.T, name string) Case {
	t.Helper()
	for _, c := range fixtureScheduleCorpus {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("schedule corpus case %q is missing", name)
	return Case{}
}

func simpsonsHighlightEpisodes() []schedule.ResolvedProgram {
	ratings := []float64{6.1, 9.4, 6.3, 8.9, 6.0, 9.1, 6.2, 8.8}
	episodes := make([]schedule.ResolvedProgram, 0, len(ratings))
	for i, rating := range ratings {
		episodes = append(episodes, schedule.ResolvedProgram{
			LibraryItemID: fmt.Sprintf("simpsons-s01e%02d", i+1),
			Title:         fmt.Sprintf("Episode %d", i+1), DurationMs: 22 * 60 * 1000,
			Season: 1, Episode: i + 1, CommunityRating: rating,
		})
	}
	return episodes
}
