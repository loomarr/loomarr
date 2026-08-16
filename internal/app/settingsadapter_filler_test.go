package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProbeWritableDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := probeWritableDirectory(dir); err != nil {
		t.Fatalf("writable temp directory failed probe: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("probe left artifacts behind: %v", entries)
	}

	missing := filepath.Join(dir, "missing")
	if err := probeWritableDirectory(missing); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("missing directory probe = %v, want actionable missing-path error", err)
	}

	file := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := probeWritableDirectory(file); err == nil || !strings.Contains(err.Error(), "not a folder") {
		t.Fatalf("regular file probe = %v, want not-a-folder error", err)
	}
}
