//go:build !windows

package proctree

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestWaitSweepsDescendantsAfterNaturalParentExit(t *testing.T) {
	pidFile := t.TempDir() + "/child.pid"
	cmd := exec.Command("sh", "-c", `sleep 60 & echo $! > "$1"; exit 7`, "sh", pidFile)
	s, err := Start(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Wait(); err == nil {
		t.Fatal("natural parent failure was suppressed")
	}

	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read descendant pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("parse descendant pid: %v", err)
	}
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("descendant %d survived its naturally exited parent", pid)
}
