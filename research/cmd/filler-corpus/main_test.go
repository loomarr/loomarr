package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRequiresExplicitInputs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
}

func TestReadersRejectUnknownAndTrailingFields(t *testing.T) {
	dir := t.TempDir()
	unknown := filepath.Join(dir, "unknown.json")
	if err := os.WriteFile(unknown, []byte(`{"known":"yes","legacy":"no"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readJSON[struct {
		Known string `json:"known"`
	}](unknown); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown field error = %v", err)
	}

	trailing := filepath.Join(dir, "trailing.jsonl")
	if err := os.WriteFile(trailing, []byte("{\"known\":\"yes\"} {\"known\":\"again\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readJSONL[struct {
		Known string `json:"known"`
	}](trailing); err == nil || !strings.Contains(err.Error(), "trailing JSON value") {
		t.Fatalf("trailing value error = %v", err)
	}
}
