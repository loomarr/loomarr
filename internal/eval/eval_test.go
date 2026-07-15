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
	sug, _, err := buildSuggester()
	if err != nil {
		t.Skipf("eval not configured: %v", err)
	}
	// The judge uses the same configured provider by default. LOOMARR_EVAL_JUDGE can
	// point at a stronger model later; for now reuse the suggester's provider.
	judgeProvider := buildProvider()

	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Minute)
	defer cancel()

	var results []Result
	for _, c := range Corpus {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			prop, gerr := sug.Suggest(ctx, mapIntent(c.Intent))
			res := Result{
				Case: c.Name, Lineup: len(prop.Lineup), Acquisitions: len(prop.Acquisitions),
				Ceiling: string(prop.Policy.Audience.Ceiling), ThemeFit: prop.Scores.ThemeFit,
				JudgeScore: -1,
			}
			res.Failures = deterministicChecks(c, prop, gerr)

			// Judge only a proposal that cleared the hard gates + has a rubric.
			if res.Passed() && c.JudgeRubric != "" {
				score, note := judge(ctx, judgeProvider, c, prop)
				res.JudgeScore, res.JudgeNote = score, note
				if score >= 0 && c.MinJudgeScore > 0 && score < c.MinJudgeScore {
					res.Failures = append(res.Failures,
						fmt.Sprintf("judge score %.2f < required %.2f: %s", score, c.MinJudgeScore, note))
				}
			}
			results = append(results, res)

			for _, fail := range res.Failures {
				t.Errorf("%s", fail)
			}
			t.Logf("lineup=%d acq=%d ceiling=%q themeFit=%.2f judge=%.2f (%s)",
				res.Lineup, res.Acquisitions, res.Ceiling, res.ThemeFit, res.JudgeScore, res.JudgeNote)
		})
	}

	// Emit a scorecard artifact (stdout + optional file) so a run is inspectable.
	writeScorecard(t, results)
}

// writeScorecard prints a summary table and, when LOOMARR_EVAL_OUT is set, writes
// the JSON scorecard there for CI archiving / trend tracking.
func writeScorecard(t *testing.T, results []Result) {
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
		fmt.Fprintf(&b, "  [%s] %-28s lineup=%d acq=%d ceiling=%-6s themeFit=%.2f judge=%.2f\n",
			status, r.Case, r.Lineup, r.Acquisitions, r.Ceiling, r.ThemeFit, r.JudgeScore)
		for _, f := range r.Failures {
			fmt.Fprintf(&b, "        ✗ %s\n", f)
		}
	}
	fmt.Fprintf(&b, "  ---\n  %d/%d cases passed\n", pass, len(results))
	t.Log(b.String())

	if out := os.Getenv("LOOMARR_EVAL_OUT"); out != "" {
		blob, _ := json.MarshalIndent(results, "", "  ")
		if err := os.WriteFile(out, blob, 0o644); err != nil {
			t.Logf("could not write scorecard to %s: %v", out, err)
		}
	}
}
