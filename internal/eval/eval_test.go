//go:build eval

package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/suggest"
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
	if os.Getenv("LOOMARR_EVAL_CONTRACT_ONLY") == "1" {
		t.Skip("live semantic corpus is disabled in the hermetic contract lane")
	}
	required := os.Getenv("LOOMARR_EVAL_REQUIRED") == "1"
	liveSchedule := os.Getenv("LOOMARR_EVAL_LIVE_SCHEDULE") == "1"
	if required && !liveSchedule {
		t.Fatal("LOOMARR_EVAL_LIVE_SCHEDULE=1 is required in certification mode")
	}
	if required && os.Getenv("LOOMARR_EVAL_OUT") == "" {
		t.Fatal("LOOMARR_EVAL_OUT is required in certification mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Minute)
	defer cancel()

	var prepared LiveScheduleCertification
	var clients evalClients
	var err error
	if liveSchedule {
		evidencePath := os.Getenv("LOOMARR_EVAL_SCHEDULE_EVIDENCE")
		if evidencePath == "" {
			t.Fatal("LOOMARR_EVAL_SCHEDULE_EVIDENCE is required when live schedule evaluation is enabled")
		}
		snapshot, loadErr := LoadScheduleEvidence(evidencePath)
		if loadErr != nil {
			t.Fatalf("live schedule evidence prerequisite: %v", loadErr)
		}
		clients, err = buildEvalClients()
		if err != nil {
			t.Fatalf("live schedule evidence prerequisite: %v", err)
		}
		prepared, err = PrepareLiveScheduleCertification(ctx, snapshot, clients.library, clients.tmdb)
		if err != nil {
			t.Fatalf("live schedule evidence prerequisite: %v", err)
		}
	}
	cases, omission, err := evalCases(required, liveSchedule, prepared.Cases)
	if err != nil {
		t.Fatal(err)
	}
	if omission != "" {
		t.Log(omission)
	}
	var sug *suggest.Suggester
	var materializer ScheduleMaterializer
	var observed *observedProvider
	if liveSchedule {
		sug, observed, err = buildSuggesterWithClients(clients)
		materializer = prepared.Materializer
	} else {
		sug, materializer, observed, err = buildSuggester()
	}
	if err != nil {
		if required {
			t.Fatalf("semantic certification is not configured: %v", err)
		}
		t.Skipf("eval not configured: %v", err)
	}
	trials := evalTrials(required)
	provider := os.Getenv("LLM_PROVIDER")
	if provider == "" {
		provider = "ollama"
	}
	generatorIdentity := ModelIdentity{Provider: provider, Model: os.Getenv("LLM_MODEL")}
	judgeIdentity := generatorIdentity
	if judgeModel := os.Getenv("LOOMARR_EVAL_JUDGE"); judgeModel != "" {
		judgeIdentity = ModelIdentity{
			Provider: firstNonEmpty(os.Getenv("LOOMARR_EVAL_JUDGE_PROVIDER"), provider),
			Model:    judgeModel,
		}
	}
	runner := NewRunner(sug, RunnerConfig{
		Trials: trials, Profile: os.Getenv("LOOMARR_EVAL_PROFILE"),
		Generator: generatorIdentity,
		Judge:     judgeIdentity,
	}).WithObserver(observed).WithJudge(modelJudge{provider: buildJudgeProvider()})
	if liveSchedule {
		runner = runner.WithMaterializer(materializer)
	}
	card := runner.Run(ctx, cases)
	if liveSchedule {
		card.CorpusVersion += "+" + prepared.SnapshotID
	}
	for _, res := range card.Results {
		for _, fail := range res.Failures {
			t.Errorf("%s trial %d: %s", res.Case, res.Trial, fail)
		}
		t.Logf("case=%s trial=%d lineup=%d acq=%d ceiling=%q themeFit=%.2f judge=%.2f relevance=%.2f serendipity=%.2f stage=%s tools=%d candidates=%d (%s)",
			res.Case, res.Trial, res.Lineup, res.Acquisitions, res.Ceiling, res.ThemeFit,
			res.JudgeScore, res.RelevanceScore, res.SerendipityScore, res.GroundingStage,
			res.ToolCalls, res.CandidatesSurfaced, res.JudgeNote)
	}

	// Emit a scorecard artifact (stdout + optional file) so a run is inspectable.
	writeScorecard(t, card, required)
}

func evalCases(required, liveSchedule bool, liveCases []Case) ([]Case, string, error) {
	if required && !liveSchedule {
		return nil, "", fmt.Errorf("LOOMARR_EVAL_LIVE_SCHEDULE=1 is required in certification mode")
	}
	cases := append([]Case(nil), Corpus...)
	if liveSchedule {
		if len(liveCases) != 3 {
			return nil, "", fmt.Errorf("live schedule evidence must produce curated, holiday, and franchise cases")
		}
		return append(cases, liveCases...), "", nil
	}
	return cases, "schedule-outcome corpus omitted; set LOOMARR_EVAL_LIVE_SCHEDULE=1 to materialize owned viewer outcomes", nil
}

func evalTrials(required bool) int {
	defaultTrials := 1
	if required {
		defaultTrials = 3
	}
	raw := os.Getenv("LOOMARR_EVAL_TRIALS")
	if raw == "" {
		return defaultTrials
	}
	trials, err := strconv.Atoi(raw)
	if err != nil || trials <= 0 {
		return defaultTrials
	}
	return trials
}

// writeScorecard prints a summary table and, when LOOMARR_EVAL_OUT is set, writes
// the JSON scorecard there for CI archiving / trend tracking.
func writeScorecard(t *testing.T, scorecard Scorecard, required bool) {
	results := scorecard.Results
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
