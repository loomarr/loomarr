//go:build eval

package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/suggest"
)

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

func TestScorecardShapeExcludesCredentials(t *testing.T) {
	t.Setenv("LLM_API_KEY", "never-serialize-this-secret")
	scorecard := Scorecard{
		SchemaVersion: scorecardSchemaVersion, CorpusVersion: corpusVersion,
		Provider: "openai", Model: "example/model", Results: []Result{},
	}
	blob, err := json.Marshal(scorecard)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(blob), os.Getenv("LLM_API_KEY")) {
		t.Fatal("scorecard metadata contains the LLM credential")
	}
}
