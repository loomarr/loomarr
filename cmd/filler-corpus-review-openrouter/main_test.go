package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRecoversCrashStaleLockOnlyWithExactDigest(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	out := filepath.Join(t.TempDir(), "completed-review")
	checkpointDir := out + ".private"
	if err := os.Mkdir(checkpointDir, 0o700); err != nil {
		t.Fatal(err)
	}
	stale := []byte("crash-stale-active-run\n")
	if err := os.WriteFile(filepath.Join(checkpointDir, "active-run.lock"), stale, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(stale)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run([]string{"--out", out, "--recover-lock-sha256", hex.EncodeToString(digest[:])}, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "recovered crash-stale lock") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(checkpointDir, "active-run.lock")); !os.IsNotExist(err) {
		t.Fatalf("active lock remains: %v", err)
	}
}

func TestRunRequiresPaidReviewContract(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "OPENROUTER_API_KEY") || !strings.Contains(stderr.String(), "--snapshot") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
