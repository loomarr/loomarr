//go:build eval

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	eval "github.com/loomarr/loomarr/internal/eval"
)

func TestRunWritesMachineAndHumanPlannerComparison(t *testing.T) {
	dir := t.TempDir()
	selection := eval.CertificationSelection{QualityMargin: 0.02, Weights: eval.CertificationQualityWeights{
		GroundedCompletion: 0.20, CorrectToolOperation: 0.20, SchemaValidity: 0.10,
		PolicyAccuracy: 0.15, ProposalQuality: 0.25, Recovery: 0.10,
	}}
	writeCard := func(name string, quality float64, footprint int64) string {
		t.Helper()
		path := filepath.Join(dir, name+".json")
		card := eval.Scorecard{
			SchemaVersion: 10, CorpusVersion: "planner-certification-v3", Certified: true,
			Generator: eval.ModelIdentity{Provider: "ollama", Model: name},
			Contract: &eval.CertificationContract{
				CorpusVersion: "planner-certification-v3", CatalogFixtureSHA256: "fixture",
				PromptVersion: "prompt", ToolSchemaVersion: "tool", ScorerVersion: "scorer", Selection: selection,
			},
			Assessment: &eval.CertificationAssessment{
				Passed: true, GroundedCompletionRate: quality, CorrectToolOperationRate: quality,
				SchemaValidityRate: quality, PolicyAccuracyRate: quality, ProposalQualityRate: quality, RecoveryRate: quality,
				Performance: eval.PerformanceSummary{ResourceStatus: "measured", PeakRAMBytes: footprint},
			},
		}
		blob, err := json.Marshal(card)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, blob, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	comparisonPath := filepath.Join(dir, "comparison.json")
	summaryPath := filepath.Join(dir, "comparison.md")
	var stderr bytes.Buffer
	code := run([]string{
		"--out", comparisonPath, "--summary", summaryPath,
		writeCard("gemma", 0.98, 10<<30), writeCard("qwen", 0.97, 7<<30),
	}, &stderr)
	if code != 0 {
		t.Fatalf("run = %d; stderr = %s", code, stderr.String())
	}
	comparisonBlob, err := os.ReadFile(comparisonPath)
	if err != nil {
		t.Fatal(err)
	}
	var comparison eval.PlannerModelComparison
	if err := json.Unmarshal(comparisonBlob, &comparison); err != nil {
		t.Fatal(err)
	}
	if comparison.Preferred.Model != "qwen" {
		t.Fatalf("preferred model = %+v, want qwen", comparison.Preferred)
	}
	summary, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(summary), "Preferred: `qwen`") {
		t.Fatalf("comparison summary:\n%s", summary)
	}
}

func TestRunWritesEvidenceAndFailsWhenNoCandidateIsEligible(t *testing.T) {
	dir := t.TempDir()
	selection := eval.CertificationSelection{QualityMargin: 0.02, Weights: eval.CertificationQualityWeights{
		GroundedCompletion: 0.20, CorrectToolOperation: 0.20, SchemaValidity: 0.10,
		PolicyAccuracy: 0.15, ProposalQuality: 0.25, Recovery: 0.10,
	}}
	paths := make([]string, 0, 2)
	for _, model := range []string{"gemma", "qwen"} {
		path := filepath.Join(dir, model+".json")
		card := eval.Scorecard{
			SchemaVersion: 10, CorpusVersion: "planner-certification-v3",
			Generator: eval.ModelIdentity{Provider: "ollama", Model: model},
			Contract: &eval.CertificationContract{CorpusVersion: "planner-certification-v3", CatalogFixtureSHA256: "fixture",
				PromptVersion: "prompt", ToolSchemaVersion: "tool", ScorerVersion: "scorer", Selection: selection},
			Assessment: &eval.CertificationAssessment{GroundedCompletionRate: 0.5, CorrectToolOperationRate: 0.5,
				SchemaValidityRate: 0.5, PolicyAccuracyRate: 0.5, ProposalQualityRate: 0.5, RecoveryRate: 0.5},
		}
		blob, err := json.Marshal(card)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, blob, 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	comparisonPath := filepath.Join(dir, "comparison.json")
	summaryPath := filepath.Join(dir, "comparison.md")
	var stderr bytes.Buffer
	code := run([]string{"--out", comparisonPath, "--summary", summaryPath, paths[0], paths[1]}, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "no candidate cleared") {
		t.Fatalf("run = %d; stderr = %s", code, stderr.String())
	}
	for _, path := range []string{comparisonPath, summaryPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing no-eligible evidence %s: %v", path, err)
		}
	}
}
