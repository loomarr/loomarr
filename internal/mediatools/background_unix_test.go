//go:build !windows

package mediatools

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The filler batch tools must run niced (#1512 G5). A fake tool records its own nice value, which
// is the observable fact — an args-only test cannot see how the process is scheduled.
func TestBackgroundToolsRunAtLowPriority(t *testing.T) {
	if own, err := exec.Command("ps", "-o", "ni=", "-p", strconv.Itoa(os.Getpid())).Output(); err != nil || strings.TrimSpace(string(own)) != "0" {
		t.Skip("needs ps and a test process at nice 0")
	}
	dir := t.TempDir()
	report := filepath.Join(dir, "nice.txt")
	tool := filepath.Join(dir, "fake-tool")
	script := "#!/bin/sh\nps -o ni= -p $$ > " + report + "\n"
	if err := os.WriteFile(tool, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	if _, err := runDerivativeCommand(context.Background(), tool, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(raw)); got != "10" {
		t.Fatalf("derivative QC tool ran at nice %q, want 10", got)
	}

	_ = os.Remove(report)
	if _, err := runConditioningCommand(context.Background(), tool, 1<<10, false); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(report)
	if got := strings.TrimSpace(string(raw)); got != "10" {
		t.Fatalf("conditioning tool ran at nice %q, want 10", got)
	}
}
