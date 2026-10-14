package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRequiresBoundInputs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "fixed generated-at") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestSourcePathRefusesEscape(t *testing.T) {
	if _, err := sourcePath(t.TempDir(), "../outside.mp4"); err == nil {
		t.Fatal("source path escape accepted")
	}
}

func TestSourcePathRefusesSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.mp4")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "inside.mp4")); err != nil {
		t.Fatal(err)
	}
	if _, err := sourcePath(root, "inside.mp4"); err == nil {
		t.Fatal("source symlink escape accepted")
	}
}

func TestPublishIsImmutable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "measurements.json")
	if err := publish(path, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := publish(path, []byte("second")); err == nil {
		t.Fatal("existing measurement overwritten")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "first" {
		t.Fatalf("published data=%q err=%v", data, err)
	}
}
