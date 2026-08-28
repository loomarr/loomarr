//go:build eval

package eval

import (
	"context"
	"slices"
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/suggest"
)

func TestDurableNegativeConstraintPinsExactSawIdentity(t *testing.T) {
	c := corpusCase(t, "explicit_negative_constraint")
	if !slices.Contains(c.ForbidKeys, provision.Key("movie:tmdb:176")) {
		t.Fatalf("durable negative constraint ForbidKeys = %v, want exact Saw identity", c.ForbidKeys)
	}
	if c.TitleEvidence != TitleEvidenceGrounded || !slices.Contains(c.ForbidTitles, "Saw") ||
		!slices.Contains(c.ForbidTitleTerms, "Saw") {
		t.Fatalf("durable negative title contract = scope %q exact %v heuristic %v",
			c.TitleEvidence, c.ForbidTitles, c.ForbidTitleTerms)
	}
}

func TestDurableNamedIncludePinsExactGroundedTitle(t *testing.T) {
	c := corpusCase(t, "must_include_grounding")
	if c.TitleEvidence != TitleEvidenceGrounded || !slices.Contains(c.RequireTitles, "The Matrix") {
		t.Fatalf("durable named include title contract = scope %q required %v", c.TitleEvidence, c.RequireTitles)
	}
}

func TestActionMarathonCorpusRejectsAnySeriesIdentity(t *testing.T) {
	c := corpusCase(t, "template_action_marathon")
	proposal := suggest.Proposal{
		Lineup: []suggest.ProposalItem{
			{MediaType: provision.Movie, TMDBID: 603, Name: "The Matrix", Genres: []string{"Action"}, OfficialRating: "PG-13"},
			{MediaType: provision.Series, TMDBID: 456, Name: "The Simpsons", Genres: []string{"Action"}, OfficialRating: "TV-PG"},
		},
		Scores: suggest.Scores{ThemeFit: 0.9},
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
		MediaType: provision.Movie, TMDBID: 680, Name: "Pulp Fiction",
	}}}}, RunnerConfig{}).Run(context.Background(), []Case{{
		Name: "required_exact_title", TitleEvidence: TitleEvidenceGrounded,
		RequireTitles: []string{"The Matrix"},
	}})

	result := card.Results[0]
	if card.Certified || result.FailureStage != FailureStageDeterministic ||
		!slices.Contains(result.Failures, `required grounded title "The Matrix" is missing`) {
		t.Fatalf("missing exact grounded title result = %+v", result)
	}
}

func TestRunnerRejectsForbiddenExactGroundedTitle(t *testing.T) {
	card := NewRunner(scriptedGenerator{proposal: suggest.Proposal{Lineup: []suggest.ProposalItem{{
		MediaType: provision.Movie, TMDBID: 176, Name: "  sAw\t",
	}}}}, RunnerConfig{}).Run(context.Background(), []Case{{
		Name: "forbidden_exact_title", TitleEvidence: TitleEvidenceGrounded,
		ForbidTitles: []string{"Saw"},
	}})

	result := card.Results[0]
	if card.Certified || result.FailureStage != FailureStageDeterministic ||
		!slices.Contains(result.Failures, `forbidden grounded title "Saw" is present`) {
		t.Fatalf("forbidden exact grounded title result = %+v", result)
	}
}

func TestRunnerExactGroundedTitlesDoNotMatchNearOrSubstringTitles(t *testing.T) {
	proposal := suggest.Proposal{Lineup: []suggest.ProposalItem{
		{MediaType: provision.Movie, TMDBID: 215, Name: "Saw II"},
		{MediaType: provision.Movie, TMDBID: 999001, Name: "Seesaw"},
	}}
	cases := []Case{
		{Name: "near_is_not_required", TitleEvidence: TitleEvidenceGrounded, RequireTitles: []string{"Saw"}},
		{Name: "substring_is_not_forbidden", TitleEvidence: TitleEvidenceGrounded, ForbidTitles: []string{"Saw"}},
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
