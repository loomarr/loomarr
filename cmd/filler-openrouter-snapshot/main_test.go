package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillerbakeoff"
)

func TestRunRequiresInputsAndCredential(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "--models") {
		t.Fatalf("missing inputs: code=%d stderr=%s", code, stderr.String())
	}
	stderr.Reset()
	if code := run([]string{"--models", "vendor/model", "--out", "snapshot.json"}, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "OPENROUTER_API_KEY") {
		t.Fatalf("missing key: code=%d stderr=%s", code, stderr.String())
	}
}

func TestWriteSnapshotIsPrivateAndImmutable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	snapshot := fillerbakeoff.OpenRouterSnapshot{SchemaVersion: 1, RetrievedAt: time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)}
	if err := writeSnapshot(path, snapshot); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	if err := writeSnapshot(path, snapshot); err == nil {
		t.Fatal("snapshot was overwritten")
	}
}
