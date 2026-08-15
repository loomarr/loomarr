package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mantonx/loomarr/internal/images"
)

func TestRunWritesTheRepositoryCertificationReport(t *testing.T) {
	worker, err := filepath.Abs(filepath.Join("..", "..", "target", "debug", "loomarr-image"))
	if err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(t.TempDir(), "image-certification.json")
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{
		"--worker", worker,
		"--report", reportPath,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run = %d; stderr = %s", code, stderr.String())
	}
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var report images.CertificationReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if !report.Passed || report.Summary.Passed != 7 || report.Summary.Refused != 2 {
		t.Errorf("report = %+v", report.Summary)
	}
	if !strings.Contains(stdout.String(), reportPath) || !strings.Contains(stdout.String(), "7 passed, 2 refused") {
		t.Errorf("stdout = %q", stdout.String())
	}
}
