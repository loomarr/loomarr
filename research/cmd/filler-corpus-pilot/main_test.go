package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/fillercorpus"
)

func TestRunRejectsLabelBearingInput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "lane.json")
	if err := os.WriteFile(input, []byte(`{"schemaVersion":1,"truth":"eligible"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"--out", filepath.Join(dir, "out.json"), "--snapshot-at", "2026-08-26T12:00:00Z", "--locked-at", "2026-08-26T13:00:00Z"}
	for range fillercorpus.PilotAuthorities {
		args = append(args, "--lane", input)
	}
	var stderr bytes.Buffer
	if code := run(args, ioDiscard{}, &stderr); code != 1 || !strings.Contains(stderr.String(), "unknown field") {
		t.Fatalf("code/stderr = %d %s", code, stderr.String())
	}
}

func TestRunRequiresExplicitPaths(t *testing.T) {
	if code := run(nil, ioDiscard{}, ioDiscard{}); code != 2 {
		t.Fatalf("code = %d", code)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
