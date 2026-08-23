//go:build eval

package eval

import (
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
