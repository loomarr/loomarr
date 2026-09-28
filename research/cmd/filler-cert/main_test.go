package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/fillereval"
)

func TestRunWritesNonCertifyingReportForSmallReplay(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.json")
	predictionsPath := filepath.Join(dir, "predictions.jsonl")
	reportPath := filepath.Join(dir, "report.json")
	manifest := fillereval.Manifest{SchemaVersion: fillereval.SchemaVersion, Kind: fillereval.CorpusDevelopmentSeed, CorpusVersion: "seed-v1", SliceGates: []fillereval.SliceGate{{Slice: "contract", MinCases: 1, MinAccuracy: 1}}, Cases: []fillereval.Case{{
		ID: "eligible", Split: fillereval.SplitDevelopment, Cluster: "eligible", Source: "synthetic",
		License: "CC0", Truth: fillereval.TruthEligible, ContentRole: "commercial", Slices: []string{"contract"},
	}}}
	writeTestJSON(t, manifestPath, manifest)
	prediction := fillereval.Prediction{
		CaseID: "eligible", Verdict: fillereval.VerdictAdmit, ContentRole: "commercial",
		Steps: []fillereval.InferenceStep{{EvaluationID: "eval-1", Role: "filler_text", Rung: "text", RequestedProvider: "fixture", RequestedModel: "fixture", ResolvedModel: "fixture", ResolvedProvider: "fixture", Modalities: []string{"text"}, Attempts: 1}},
	}
	data, err := json.Marshal(prediction)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(predictionsPath, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--manifest", manifestPath, "--predictions", predictionsPath, "--report", reportPath,
		"--split", "development",
		"--generated-at", "2026-08-25T13:00:00Z", "--max-requests", "1", "--max-spend-nano-usd", "1", "--max-concurrency", "1",
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d, want non-certifying 1; stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var report fillereval.Report
	reportData, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(reportData, &report); err != nil {
		t.Fatal(err)
	}
	if report.Certified || report.Metrics.Cases != 1 {
		t.Fatalf("report = %+v", report)
	}
}

func TestRunRequiresAllPaths(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestRunRequiresExplicitRunCeilings(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--manifest", "manifest.json", "--predictions", "predictions.jsonl", "--report", "report.json"}, &stdout, &stderr)
	if code != 2 || !bytes.Contains(stderr.Bytes(), []byte("--max-requests")) {
		t.Fatalf("exit = %d stderr = %s", code, stderr.String())
	}
}

func TestReadPredictionsRejectsRemovedScalarInferenceLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.jsonl")
	if err := os.WriteFile(path, []byte(`{"caseId":"case-1","verdict":"admit","role":"filler_text","attempts":1}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readPredictions(path); err == nil || !strings.Contains(err.Error(), `unknown field "role"`) {
		t.Fatalf("error = %v", err)
	}
}

func writeTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
