//go:build eval

package eval

import (
	"context"
	"slices"
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/suggest"
)

func TestActionMarathonCorpusRejectsAnySeriesIdentity(t *testing.T) {
	c := corpusCase(t, "template_action_marathon")
	proposal := suggest.Proposal{
		Lineup: []suggest.ProposalItem{
			{MediaType: provision.Movie, TMDBID: 90000603, Name: "Marigold Protocol", Genres: []string{"Action"}, OfficialRating: "PG-13"},
			{MediaType: provision.Series, TMDBID: 90000456, Name: "The Mudpuddles", Genres: []string{"Action"}, OfficialRating: "TV-PG"},
		},
		Scores: suggest.Scores{ThemeFit: new(0.9)},
	}
	judge := &sequenceJudge{scores: []JudgeScores{{
		Overall: 0.9, Relevance: 0.9, Serendipity: 0.8, Reason: "Otherwise strong.",
	}}}
	card := NewRunner(scriptedGenerator{proposal: proposal}, RunnerConfig{}).
		WithJudge(judge).
		Run(context.Background(), []Case{c})

	result := card.Results[0]
	if card.Certified || result.FailureStage != FailureStageDeterministic {
		t.Fatalf("mixed-media action marathon certified: %+v", result)
	}
}

func TestRunnerRejectsMissingRequiredExactGroundedTitle(t *testing.T) {
	card := NewRunner(scriptedGenerator{proposal: suggest.Proposal{Lineup: []suggest.ProposalItem{{
		MediaType: provision.Movie, TMDBID: 90000680, Name: "Brass Diner",
	}}}}, RunnerConfig{}).Run(context.Background(), []Case{{
		Name: "required_exact_title", TitleEvidence: TitleEvidenceGrounded,
		RequireTitles: []string{"Marigold Protocol"},
	}})

	result := card.Results[0]
	if card.Certified || result.FailureStage != FailureStageDeterministic ||
		!slices.Contains(result.Failures, `required grounded title "Marigold Protocol" is missing`) {
		t.Fatalf("missing exact grounded title result = %+v", result)
	}
}

func TestRunnerRejectsForbiddenExactGroundedTitle(t *testing.T) {
	card := NewRunner(scriptedGenerator{proposal: suggest.Proposal{Lineup: []suggest.ProposalItem{{
		MediaType: provision.Movie, TMDBID: 90000176, Name: "  lARk\t",
	}}}}, RunnerConfig{}).Run(context.Background(), []Case{{
		Name: "forbidden_exact_title", TitleEvidence: TitleEvidenceGrounded,
		ForbidTitles: []string{"Lark"},
	}})

	result := card.Results[0]
	if card.Certified || result.FailureStage != FailureStageDeterministic ||
		!slices.Contains(result.Failures, `forbidden grounded title "Lark" is present`) {
		t.Fatalf("forbidden exact grounded title result = %+v", result)
	}
}

func TestRunnerExactGroundedTitlesDoNotMatchNearOrSubstringTitles(t *testing.T) {
	proposal := suggest.Proposal{Lineup: []suggest.ProposalItem{
		{MediaType: provision.Movie, TMDBID: 90000215, Name: "Lark II"},
		{MediaType: provision.Movie, TMDBID: 90999001, Name: "Skylark"},
	}}
	cases := []Case{
		{Name: "near_is_not_required", TitleEvidence: TitleEvidenceGrounded, RequireTitles: []string{"Lark"}},
		{Name: "substring_is_not_forbidden", TitleEvidence: TitleEvidenceGrounded, ForbidTitles: []string{"Lark"}},
	}
	card := NewRunner(scriptedGenerator{proposal: proposal}, RunnerConfig{}).Run(context.Background(), cases)

	if card.Results[0].Passed() || !card.Results[1].Passed() {
		t.Fatalf("exact near-title results = %+v", card.Results)
	}
}

func corpusCase(t *testing.T, name string) Case {
	t.Helper()
	for _, c := range Corpus {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("corpus case %q not found", name)
	return Case{}
}

func TestRequiredThemeFloorRejectsUnknownAndPartialEvidence(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value *float64
		pass  bool
	}{
		{"unknown", nil, false}, {"partial", new(0.25), false}, {"at floor", new(0.5), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			card := NewRunner(scriptedGenerator{proposal: suggest.Proposal{
				Lineup: []suggest.ProposalItem{{MediaType: provision.Movie, TMDBID: 90000603, Name: "Marigold Protocol"}},
				Scores: suggest.Scores{Version: 1, ThemeFit: tc.value},
			}}, RunnerConfig{}).Run(context.Background(), []Case{{Name: "theme_floor", MinThemeFit: 0.5}})
			if card.Results[0].Passed() != tc.pass {
				t.Fatalf("required floor result: %+v", card.Results[0])
			}
		})
	}
}
