//go:build eval

package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	scorecardSchemaVersion = 2
	corpusVersion          = "2026-08-23.3"
)

type Scorecard struct {
	SchemaVersion int       `json:"schemaVersion"`
	CorpusVersion string    `json:"corpusVersion"`
	GeneratedAt   time.Time `json:"generatedAt"`
	Profile       string    `json:"profile"`
	Provider      string    `json:"provider"`
	Model         string    `json:"model"`
	Certified     bool      `json:"certified"`
	Results       []Result  `json:"results"`
}

// TestEvalCorpus runs the whole intent corpus through the REAL suggester against
// the REAL configured LLM + catalog, applying the deterministic hard gates and an
// optional judge score. It is gated behind the `eval` build tag so it never runs
// under `make check`. Run it with:
//
//	make eval        # or: go test -tags=eval -v -timeout 20m ./internal/eval/
//
// with LLM_* + LIBRARY_URL + LIBRARY_TOKEN + TMDB_API_KEY set. It skips (not fails)
// when the env isn't configured, so it's safe to leave in CI as a no-op until wired.
func TestEvalCorpus(t *testing.T) {
	required := os.Getenv("LOOMARR_EVAL_REQUIRED") == "1"
	if required && os.Getenv("LOOMARR_EVAL_OUT") == "" {
		t.Fatal("LOOMARR_EVAL_OUT is required in certification mode")
	}
	sug, _, observed, err := buildSuggester()
	if err != nil {
		if required {
			t.Fatalf("semantic certification is not configured: %v", err)
		}
		t.Skipf("eval not configured: %v", err)
	}
	// The judge uses the same configured provider by default. LOOMARR_EVAL_JUDGE can
	// point at a stronger model later; for now reuse the suggester's provider.
	judgeProvider := buildJudgeProvider()

	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Minute)
	defer cancel()

	var results []Result
	for _, c := range Corpus {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			observed.Begin()
			prop, gerr := sug.Suggest(ctx, mapIntent(c.Intent))
			res := Result{
				Case: c.Name, Lineup: len(prop.Lineup), Acquisitions: len(prop.Acquisitions),
				Ceiling: string(prop.Policy.Audience.Ceiling), ThemeFit: prop.Scores.ThemeFit,
				JudgeScore: -1, RelevanceScore: -1, SerendipityScore: -1,
			}
			res.Observation = observed.Snapshot(gerr)
			res.Failures = deterministicChecks(c, prop, gerr)

			// Judge only a proposal that cleared the hard gates + has a rubric.
			if res.Passed() && c.JudgeRubric != "" {
				scores := judge(ctx, judgeProvider, c, prop)
				res.JudgeScore, res.RelevanceScore = scores.Overall, scores.Relevance
				res.SerendipityScore, res.JudgeNote = scores.Serendipity, scores.Reason
				if c.MinJudgeScore > 0 && scores.Overall < c.MinJudgeScore && (required || scores.Overall >= 0) {
					res.Failures = append(res.Failures,
						fmt.Sprintf("judge score %.2f < required %.2f: %s", scores.Overall, c.MinJudgeScore, scores.Reason))
				}
				if c.MinRelevanceScore > 0 && scores.Relevance < c.MinRelevanceScore && (required || scores.Relevance >= 0) {
					res.Failures = append(res.Failures,
						fmt.Sprintf("relevance score %.2f < required %.2f: %s", scores.Relevance, c.MinRelevanceScore, scores.Reason))
				}
				if c.MinSerendipityScore > 0 && scores.Serendipity < c.MinSerendipityScore && (required || scores.Serendipity >= 0) {
					res.Failures = append(res.Failures,
						fmt.Sprintf("serendipity score %.2f < required %.2f: %s", scores.Serendipity, c.MinSerendipityScore, scores.Reason))
				}
			}
			results = append(results, res)

			for _, fail := range res.Failures {
				t.Errorf("%s", fail)
			}
			t.Logf("lineup=%d acq=%d ceiling=%q themeFit=%.2f judge=%.2f relevance=%.2f serendipity=%.2f stage=%s tools=%d candidates=%d (%s)",
				res.Lineup, res.Acquisitions, res.Ceiling, res.ThemeFit, res.JudgeScore,
				res.RelevanceScore, res.SerendipityScore, res.GroundingStage,
				res.ToolCalls, res.CandidatesSurfaced, res.JudgeNote)
		})
	}

	// Emit a scorecard artifact (stdout + optional file) so a run is inspectable.
	writeScorecard(t, results, required)
}

// writeScorecard prints a summary table and, when LOOMARR_EVAL_OUT is set, writes
// the JSON scorecard there for CI archiving / trend tracking.
func writeScorecard(t *testing.T, results []Result, required bool) {
	pass := 0
	var b strings.Builder
	b.WriteString("\n=== Eval scorecard ===\n")
	for _, r := range results {
		status := "PASS"
		if !r.Passed() {
			status = "FAIL"
		} else {
			pass++
		}
		fmt.Fprintf(&b, "  [%s] %-36s lineup=%d acq=%d ceiling=%-6s themeFit=%.2f stage=%-15s tools=%d candidates=%d judge=%.2f rel=%.2f ser=%.2f\n",
			status, r.Case, r.Lineup, r.Acquisitions, r.Ceiling, r.ThemeFit,
			r.GroundingStage, r.ToolCalls, r.CandidatesSurfaced,
			r.JudgeScore, r.RelevanceScore, r.SerendipityScore)
		for _, f := range r.Failures {
			fmt.Fprintf(&b, "        ✗ %s\n", f)
		}
	}
	fmt.Fprintf(&b, "  ---\n  %d/%d cases passed\n", pass, len(results))
	t.Log(b.String())

	if out := os.Getenv("LOOMARR_EVAL_OUT"); out != "" {
		certified := len(results) == len(Corpus)
		for _, result := range results {
			certified = certified && result.Passed()
		}
		provider := os.Getenv("LLM_PROVIDER")
		if provider == "" {
			provider = "ollama"
		}
		scorecard := Scorecard{
			SchemaVersion: scorecardSchemaVersion, CorpusVersion: corpusVersion,
			GeneratedAt: time.Now().UTC(), Profile: os.Getenv("LOOMARR_EVAL_PROFILE"),
			Provider: provider, Model: os.Getenv("LLM_MODEL"),
			Certified: certified, Results: results,
		}
		blob, err := json.MarshalIndent(scorecard, "", "  ")
		if err != nil {
			t.Fatalf("marshal semantic scorecard: %v", err)
		}
		if err := os.WriteFile(out, blob, 0o644); err != nil {
			if required {
				t.Errorf("write required semantic scorecard to %s: %v", out, err)
			} else {
				t.Logf("could not write scorecard to %s: %v", out, err)
			}
		}
	}
}
