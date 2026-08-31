//go:build eval

package eval

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/suggest"
)

type scriptedGenerator struct {
	proposal suggest.Proposal
	err      error
}

type providerGenerator struct {
	provider llm.Provider
	proposal suggest.Proposal
	calls    int
}

func (g providerGenerator) Suggest(ctx context.Context, _ suggest.Intent) (suggest.Proposal, error) {
	calls := g.calls
	if calls <= 0 {
		calls = 1
	}
	for range calls {
		if _, err := g.provider.Chat(ctx, nil, llm.ChatOptions{}); err != nil {
			return suggest.Proposal{}, err
		}
	}
	return g.proposal, nil
}

func (g scriptedGenerator) Suggest(context.Context, suggest.Intent) (suggest.Proposal, error) {
	return g.proposal, g.err
}

type sequenceGenerator struct {
	proposals []suggest.Proposal
	next      int
}

type sequenceErrorGenerator struct {
	errors []error
	next   int
}

func (g *sequenceErrorGenerator) Suggest(context.Context, suggest.Intent) (suggest.Proposal, error) {
	err := g.errors[g.next]
	g.next++
	return suggest.Proposal{}, err
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
	if result.FailureStage != FailureStageGeneration || card.FailureCounts[FailureStageGeneration] != 1 {
		t.Fatalf("generation failure accounting = stage %q counts %v", result.FailureStage, card.FailureCounts)
	}
}

func TestRunnerAggregatesRetrievalAndGenerationFailuresSeparately(t *testing.T) {
	generator := &sequenceErrorGenerator{errors: []error{
		suggest.ErrNoGroundedTitles,
		errors.New("llm chat: upstream unavailable"),
	}}
	observer := &observedProvider{}
	card := NewRunner(generator, RunnerConfig{}).WithObserver(observer).Run(context.Background(), []Case{
		{Name: "retrieval"}, {Name: "generation"},
	})

	if card.Certified || len(card.Results) != 2 {
		t.Fatalf("failure aggregation = %+v", card)
	}
	if card.Results[0].FailureStage != FailureStageRetrieval || card.FailureCounts[FailureStageRetrieval] != 1 {
		t.Fatalf("retrieval accounting = stage %q counts %v", card.Results[0].FailureStage, card.FailureCounts)
	}
	if card.Results[1].FailureStage != FailureStageGeneration || card.FailureCounts[FailureStageGeneration] != 1 {
		t.Fatalf("generation accounting = stage %q counts %v", card.Results[1].FailureStage, card.FailureCounts)
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
