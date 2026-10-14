//go:build !windows

package proctree

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// niceOf reads a process's nice value through ps, which reports it the same way on Linux and
// macOS (the raw getpriority syscall does not: Linux returns 20-nice).
func niceOf(t *testing.T, pid int) int {
	t.Helper()
	out, err := exec.Command("ps", "-o", "ni=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		t.Skipf("ps unavailable: %v", err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("parse nice %q: %v", out, err)
	}
	return n
}

func TestStartLowPriorityNicesTheChild(t *testing.T) {
	baseline := niceOf(t, os.Getpid())
	if baseline >= BackgroundNice {
		t.Skipf("test process already runs at nice %d", baseline)
	}

	normal := exec.Command("sleep", "30")
	ns, err := Start(context.Background(), normal)
	if err != nil {
		t.Fatal(err)
	}
	defer ns.Stop()
	if got := niceOf(t, normal.Process.Pid); got != baseline {
		t.Fatalf("an ordinary child changed priority: nice %d, baseline %d", got, baseline)
	}

	low := exec.Command("sleep", "30")
	ls, err := Start(context.Background(), low, LowPriority())
	if err != nil {
		t.Fatal(err)
	}
	defer ls.Stop()
	if got := niceOf(t, low.Process.Pid); got != BackgroundNice {
		t.Fatalf("LowPriority child nice = %d, want %d", got, BackgroundNice)
	}
}
