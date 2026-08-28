//go:build eval

package eval

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestJudgeScoreRendersGroundedPolicyAndStructuralEvidence(t *testing.T) {
	t.Setenv("LLM_API_KEY", "PROMPT_MUST_NOT_CONTAIN_CREDENTIAL")
	t.Setenv("LLM_URL", "https://private-provider.invalid/v1")
	provider := testkit.NewLLM(testkit.FinalResponse(
		`{"overall":0.9,"relevance":0.95,"serendipity":0.7,"reason":"PRIVATE_PROVIDER_RESPONSE_PAYLOAD"}`,
	))
	proposal := suggest.Proposal{
		Lineup: []suggest.ProposalItem{{
			MediaType: provision.Movie, TMDBID: 603, Name: "The Matrix", Year: 1999,
			InLibrary: true, LibraryItemID: "library-matrix", Source: "library-search",
			Rationale: "Grounded cyberpunk anchor", Confidence: 0.93,
			Genres: []string{"Action", "Science Fiction"}, OfficialRating: "R",
		}},
		Policy: schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{
			Scope: schedule.ScopePolicy{Era: &schedule.Range{From: 1980, To: 1999}},
			Audience: schedule.AudiencePolicy{
				Ceiling: schedule.Rating("TV-14"), Unrated: schedule.UnratedExclude,
			},
			Ordering: schedule.OrderSyndication,
			Seasonal: schedule.SeasonalPolicy{
				Mode: schedule.SeasonalExclusive, Holidays: []string{"halloween"},
			},
		}},
	}
	evidence := NewJudgeEvidence(Case{
		Intent:      Intent{Description: "late-90s cyberpunk movies"},
		JudgeRubric: "Grounded, era-accurate science fiction",
	}, proposal, Observation{
		ToolCalls: 3, TitleCalls: 1, GenreCalls: 1, KeywordCalls: 1,
		CandidatesSurfaced: 17, GroundingStage: "grounded",
	}, nil)

	var scorer Judge = modelJudge{provider: provider}
	if _, err := scorer.Score(context.Background(), evidence); err != nil {
		t.Fatalf("Judge.Score returned error: %v", err)
	}

	prompt := provider.Prompt()
	for _, required := range []string{
		"late-90s cyberpunk movies",
		"Grounded, era-accurate science fiction",
		`"key":"movie:tmdb:603"`,
		`"name":"The Matrix"`,
		`"year":1999`,
		`"source":"library-search"`,
		`"rationale":"Grounded cyberpunk anchor"`,
		`"confidence":0.93`,
		`"genres":["Action","Science Fiction"]`,
		`"rating":"R"`,
		`"era":{"from":1980,"to":1999}`,
		`"ceiling":"TV-14"`,
		`"unrated":"exclude"`,
		`"ordering":"syndication"`,
		`"seasonal":{"mode":"exclusive","holidays":["halloween"]}`,
		`"toolCalls":3`,
		`"titleCalls":1`,
		`"genreCalls":1`,
		`"keywordCalls":1`,
		`"candidatesSurfaced":17`,
		`"groundingStage":"grounded"`,
	} {
		if !strings.Contains(prompt, required) {
			t.Errorf("judge prompt missing %q:\n%s", required, prompt)
		}
	}
	for _, forbidden := range []string{
		"PROMPT_MUST_NOT_CONTAIN_CREDENTIAL",
		"https://private-provider.invalid/v1",
		"PRIVATE_PROVIDER_RESPONSE_PAYLOAD",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Errorf("judge prompt contains private provider data %q", forbidden)
		}
	}
}

func TestJudgeScoreKeepsAcquisitionOwnershipDistinct(t *testing.T) {
	provider := testkit.NewLLM(testkit.FinalResponse(
		`{"overall":0.8,"relevance":0.8,"serendipity":0.8,"reason":"Ownership is explicit."}`,
	))
	evidence := NewJudgeEvidence(Case{
		Intent: Intent{Description: "paired titles"}, JudgeRubric: "Respect ownership",
	}, suggest.Proposal{
		Lineup: []suggest.ProposalItem{{
			MediaType: provision.Movie, TMDBID: 603, Name: "Library title", InLibrary: true,
		}},
		Acquisitions: []suggest.ProposalItem{{
			MediaType: provision.Movie, TMDBID: 550, Name: "Acquisition title", InLibrary: false,
		}},
	}, Observation{}, nil)

	var scorer Judge = modelJudge{provider: provider}
	if _, err := scorer.Score(context.Background(), evidence); err != nil {
		t.Fatalf("Judge.Score returned error: %v", err)
	}

	prompt := provider.Prompt()
	for _, required := range []string{
		`"key":"movie:tmdb:603","name":"Library title","ownership":"library"`,
		`"key":"movie:tmdb:550","name":"Acquisition title","ownership":"acquisition"`,
	} {
		if !strings.Contains(prompt, required) {
			t.Errorf("judge prompt missing distinct ownership %q:\n%s", required, prompt)
		}
	}
}

func TestRunnerGivesJudgeScheduleMaterializerEvidenceNotProposalTitles(t *testing.T) {
	provider := testkit.NewLLM(testkit.FinalResponse(
		`{"overall":0.9,"relevance":0.9,"serendipity":0.8,"reason":"The scheduled evidence is concrete."}`,
	))
	proposal := suggest.Proposal{Lineup: []suggest.ProposalItem{{
		MediaType: provision.Movie, TMDBID: 603,
		Name: "PROPOSAL TITLE IS NOT SCHEDULE EVIDENCE", InLibrary: true,
	}}}
	runner := NewRunner(scriptedGenerator{proposal: proposal}, RunnerConfig{}).
		WithMaterializer(NewFixtureScheduleMaterializer(map[provision.Key]FixtureTitle{
			"movie:tmdb:603": {
				LibraryItemID: "library-matrix", DurationMs: 120 * 60 * 1000,
			},
		})).
		WithJudge(modelJudge{provider: provider})

	card := runner.Run(context.Background(), []Case{{
		Name:                     "materialized_judge_evidence",
		Intent:                   Intent{Description: "science-fiction movies"},
		JudgeRubric:              "Judge the programs that will actually air",
		RequireScheduledPrograms: []string{"movie:tmdb:603"},
	}})
	if !card.Certified {
		t.Fatalf("Runner.Run failed: %+v", card.Results)
	}

	_, scheduleEvidence, found := strings.Cut(provider.Prompt(), "MATERIALIZED SCHEDULE SAMPLE:\n")
	if !found {
		t.Fatalf("judge prompt has no materialized schedule section:\n%s", provider.Prompt())
	}
	if !strings.Contains(scheduleEvidence, `"movie:tmdb:603"`) {
		t.Fatalf("schedule evidence = %q, want materialized program identity", scheduleEvidence)
	}
	if strings.Contains(scheduleEvidence, proposal.Lineup[0].Name) {
		t.Fatalf("schedule evidence used proposal title instead of materializer output: %q", scheduleEvidence)
	}
}

func TestJudgeScoreExcludesEvidenceBeyondEveryNamedCap(t *testing.T) {
	lineup := make([]suggest.ProposalItem, JudgeMaxItemsPerOwnership+1)
	acquisitions := make([]suggest.ProposalItem, JudgeMaxItemsPerOwnership+1)
	for i := range lineup {
		lineup[i] = suggest.ProposalItem{
			MediaType: provision.Movie, TMDBID: 1000 + i,
			Name: fmt.Sprintf("lineup-%02d", i), InLibrary: true,
		}
		acquisitions[i] = suggest.ProposalItem{
			MediaType: provision.Movie, TMDBID: 2000 + i,
			Name: fmt.Sprintf("acquisition-%02d", i),
		}
	}
	lineup[JudgeMaxItemsPerOwnership].Name = "LINEUP_BEYOND_CAP"
	acquisitions[JudgeMaxItemsPerOwnership].Name = "ACQUISITION_BEYOND_CAP"
	lineup[0].Rationale = strings.Repeat("x", JudgeMaxTextRunes) + "TEXT_BEYOND_CAP"
	lineup[0].Genres = make([]string, JudgeMaxGenresPerItem+1)
	for i := range lineup[0].Genres {
		lineup[0].Genres[i] = fmt.Sprintf("genre-%02d", i)
	}
	lineup[0].Genres[JudgeMaxGenresPerItem] = "GENRE_BEYOND_CAP"

	collections := make([]string, JudgeMaxPolicyValues+1)
	for i := range collections {
		collections[i] = fmt.Sprintf("collection-%02d", i)
	}
	collections[JudgeMaxPolicyValues] = "POLICY_BEYOND_CAP"
	programs := make([]string, JudgeMaxScheduledPrograms+1)
	for i := range programs {
		programs[i] = fmt.Sprintf("movie:tmdb:%d", 3000+i)
	}
	programs[JudgeMaxScheduledPrograms] = "SCHEDULE_BEYOND_CAP"

	provider := testkit.NewLLM(testkit.FinalResponse(
		`{"overall":0.8,"relevance":0.8,"serendipity":0.8,"reason":"The bounded sample is sufficient."}`,
	))
	evidence := NewJudgeEvidence(Case{
		Intent: Intent{Description: "bounded evidence"}, JudgeRubric: "Use the bounded facts",
	}, suggest.Proposal{
		Lineup: lineup, Acquisitions: acquisitions,
		Policy: schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{
			Scope: schedule.ScopePolicy{Collections: collections},
		}},
	}, Observation{}, programs)

	var scorer Judge = modelJudge{provider: provider}
	if _, err := scorer.Score(context.Background(), evidence); err != nil {
		t.Fatalf("Judge.Score returned error: %v", err)
	}
	prompt := provider.Prompt()
	for _, retained := range []string{
		fmt.Sprintf("lineup-%02d", JudgeMaxItemsPerOwnership-1),
		fmt.Sprintf("acquisition-%02d", JudgeMaxItemsPerOwnership-1),
		fmt.Sprintf("genre-%02d", JudgeMaxGenresPerItem-1),
		fmt.Sprintf("collection-%02d", JudgeMaxPolicyValues-1),
		fmt.Sprintf("movie:tmdb:%d", 3000+JudgeMaxScheduledPrograms-1),
	} {
		if !strings.Contains(prompt, retained) {
			t.Errorf("judge prompt dropped in-cap evidence %q", retained)
		}
	}
	for _, excluded := range []string{
		"LINEUP_BEYOND_CAP",
		"ACQUISITION_BEYOND_CAP",
		"GENRE_BEYOND_CAP",
		"POLICY_BEYOND_CAP",
		"SCHEDULE_BEYOND_CAP",
		"TEXT_BEYOND_CAP",
	} {
		if strings.Contains(prompt, excluded) {
			t.Errorf("judge prompt included out-of-cap evidence %q", excluded)
		}
	}
}
