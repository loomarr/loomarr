package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillerrelease"
)

func TestRunWritesHoldReport(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.json")
	outPath := filepath.Join(dir, "report.json")
	if err := os.WriteFile(manifestPath, []byte(`{"schema_version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	wantTime := time.Date(2026, 9, 20, 19, 0, 0, 0, time.UTC)

	exitCode := run([]string{
		"-root", dir,
		"-manifest", manifestPath,
		"-out", outPath,
		"-generated-at", wantTime.Format(time.RFC3339),
	}, &stdout, &stderr, time.Now)

	if exitCode != exitHold {
		t.Fatalf("exit code = %d, want %d; stderr: %s", exitCode, exitHold, stderr.String())
	}
	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	var report fillerrelease.Report
	if err := json.Unmarshal(content, &report); err != nil {
		t.Fatal(err)
	}
	if report.Verdict != fillerrelease.VerdictHold {
		t.Fatalf("Verdict = %q, want HOLD", report.Verdict)
	}
	if !report.GeneratedAt.Equal(wantTime) {
		t.Fatalf("GeneratedAt = %v, want %v", report.GeneratedAt, wantTime)
	}
	if stdout.String() == "" {
		t.Fatal("stdout is empty")
	}
}

func TestRunUnreadableManifestIsExecutionError(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer

	exitCode := run([]string{
		"-root", dir,
		"-manifest", filepath.Join(dir, "missing.json"),
		"-out", filepath.Join(dir, "report.json"),
	}, &stdout, &stderr, time.Now)

	if exitCode != exitError {
		t.Fatalf("exit code = %d, want %d", exitCode, exitError)
	}
	if stderr.String() == "" {
		t.Fatal("stderr is empty")
	}
}

func TestRunMissingEvidenceRootStillWritesHoldReport(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.json")
	outPath := filepath.Join(dir, "report.json")
	if err := os.WriteFile(manifestPath, []byte(`{"schema_version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer

	exitCode := run([]string{
		"-root", filepath.Join(dir, "missing-root"),
		"-manifest", manifestPath,
		"-out", outPath,
	}, &stdout, &stderr, time.Now)

	if exitCode != exitHold {
		t.Fatalf("exit code = %d, want %d; stderr: %s", exitCode, exitHold, stderr.String())
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("hold report was not written: %v", err)
	}
}
