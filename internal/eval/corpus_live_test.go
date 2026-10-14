//go:build eval

package eval

import (
	"slices"
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
)

// These tests pin corpus.go, the live corpus that runs real channel intents against a real LLM,
// library and TMDB, so they name the real titles and ids those intents are about (#1592). They
// moved here unchanged: TestDurableNegativeConstraintPinsExactSawIdentity and
// TestDurableNamedIncludePinsExactGroundedTitle from corpus_behavior_test.go, and
// TestNamedIncludeCorpusCasePinsTheMatrixIdentity from runner_accounting_test.go.

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
