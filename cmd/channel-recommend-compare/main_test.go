package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/recommend"
)

func TestCommandWritesOfflineSharedModelDecision(t *testing.T) {
	dir := t.TempDir()
	cardPath := func(profile, model string, score float64) string {
		t.Helper()
		thresholds := recommend.Thresholds{MinRelevance: .8, MinNovelty: .75, MinDiversity: .7, MinCatalogFeasibility: .9, MinPolicySafety: 1, MinSchemaValidity: .98, MinAbstention: .8}
		card := recommend.Scorecard{
			SchemaVersion: 1, CorpusVersion: "channel-recommendation-v1", CorpusSHA256: "fixture",
			PromptVersion: "prompt", OutputSchema: "schema", ScorerVersion: "scorer",
			SelectionMetric: "mean_quality", SelectionMargin: .02, Thresholds: thresholds,
			Profile: profile, Provider: "ollama", Model: model, Certified: true,
			Budget: recommend.RunConfig{ExpectedCases: 1}, Results: []recommend.CaseRun{{CaseResult: recommend.CaseResult{Passed: true}}},
			Quality:           recommend.AggregateQuality{Quality: recommend.Quality{Relevance: score, Novelty: score, Diversity: score, CatalogFeasibility: score, PolicySafety: 1, Abstention: score}, SchemaValidity: 1},
			HardFailureCounts: map[string]int{}, Resources: recommend.ResourceUsage{AccountingComplete: true},
		}
		blob, err := json.Marshal(card)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, profile+".json")
		if err := os.WriteFile(path, blob, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	out := filepath.Join(dir, "comparison.json")
	summary := filepath.Join(dir, "comparison.md")
	var stderr bytes.Buffer
	code := run([]string{
		"--out", out, "--summary", summary, "--shared-profile", "shared-planner",
		cardPath("shared-planner", "qwen:9b", .90), cardPath("alternative", "gemma:12b", .91),
	}, &stderr)
	if code != 0 {
		t.Fatalf("run = %d; stderr = %s", code, stderr.String())
	}
	blob, err := os.ReadFile(summary)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(blob), "shared model is sufficient") {
		t.Fatalf("summary:\n%s", blob)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatal(err)
	}
}
