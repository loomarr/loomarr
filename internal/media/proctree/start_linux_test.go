package proctree

import (
	"os/exec"
	"testing"
)

// #2020: on Linux a LowPriority child runs at BackgroundNice from its first instruction. The renice
// that follows Start is not what nices it: a child that read its own priority at once, as the
// downloader test's fake yt-dlp does, saw nice 0 when that renice lost the race on a loaded host.
// So the start alone, with no renice after it, must already have niced the child, and must leave
// every later ordinary child at normal priority.
//
// The baseline is an ordinary child's, not this process's: when the lowered thread happens to be
// the main thread, Go parks it rather than ending it, and ps then reports the process at its nice
// although no goroutine runs there again.
func TestStartProcessNicesTheChildBeforeItRuns(t *testing.T) {
	start := func(o options) int {
		t.Helper()
		cmd := exec.Command("sleep", "30")
		if err := startProcess(cmd, o); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		return niceOf(t, cmd.Process.Pid)
	}
	baseline := start(options{})
	if baseline >= BackgroundNice {
		t.Skipf("test process already runs at nice %d", baseline)
	}
	for range 5 {
		if got := start(options{lowPriority: true}); got != BackgroundNice {
			t.Fatalf("low-priority child nice = %d, want %d", got, BackgroundNice)
		}
		if got := start(options{}); got != baseline {
			t.Fatalf("an ordinary child after a low-priority one runs at nice %d, want %d", got, baseline)
		}
	}
}
