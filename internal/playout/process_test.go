package playout

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCombinedProgressPreservesDiagnostics(t *testing.T) {
	p := &Process{}
	var samples []Progress
	p.readCombined(io.NopCloser(strings.NewReader(strings.Join([]string{
		"frame=12",
		"fps=29.97",
		"stream_0_0_q=-0.0",
		"bitrate=2048.0kbits/s",
		"total_size=889311",
		"out_time_us=3456000",
		"out_time_ms=3456000",
		"out_time=-00:00:03.456000",
		"dup_frames=0",
		"drop_frames=0",
		"speed=1.25x",
		"progress=continue",
		"frame=12 fps=29.97 q=-0.0 size=868KiB time=00:00:03.45 bitrate=2058.6kbits/s",
		"Decoder warning: recovered damaged frame",
	}, "\n"))), func(sample Progress) {
		samples = append(samples, sample)
	})

	if len(samples) != 1 {
		t.Fatalf("samples = %d, want 1", len(samples))
	}
	if got, want := samples[0], (Progress{Frame: 12, OutTimeMS: 3456, Speed: 1.25}); got != want {
		t.Errorf("sample = %+v, want %+v", got, want)
	}
	if got := p.LastError(); got != "Decoder warning: recovered damaged frame" {
		t.Errorf("last diagnostic = %q", got)
	}
}

func TestProcessProgressTransport(t *testing.T) {
	sampleCh := make(chan Progress, 1)
	proc, err := Start(context.Background(), os.Args[0], []string{
		"-test.run=^TestProcessTreeHelper$", "--", "progress",
	}, nil, func(sample Progress) {
		sampleCh <- sample
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = proc.Stdout.Close() }()
	if err := proc.Wait(); err != nil {
		t.Fatalf("progress helper: %v", err)
	}
	var sample Progress
	select {
	case sample = <-sampleCh:
	case <-time.After(time.Second):
		t.Fatal("progress callback was not delivered")
	}
	if got, want := sample, (Progress{Frame: 12, OutTimeMS: 3456, Speed: 1.25}); got != want {
		t.Errorf("sample = %+v, want %+v", got, want)
	}
	deadline := time.Now().Add(time.Second)
	for proc.LastError() == "" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := proc.LastError(); got != "Decoder warning: recovered damaged frame" {
		t.Errorf("last diagnostic = %q", got)
	}
}

// TestProcessTreeHelper is re-executed by TestProcessTreeLifecycle. The parent
// spawns one descendant and waits forever; cancelling the supervised parent must
// remove both, on Unix through a process group and on Windows through a Job Object.
func TestProcessTreeHelper(t *testing.T) {
	args := helperArgs(os.Args)
	if len(args) == 0 {
		return
	}
	switch args[0] {
	case "parent", "parent-exit":
		if len(args) != 2 {
			os.Exit(2)
		}
		child := exec.Command(os.Args[0], "-test.run=^TestProcessTreeHelper$", "--", "child")
		if err := child.Start(); err != nil {
			os.Exit(3)
		}
		if err := os.WriteFile(args[1], []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
			_ = child.Process.Kill()
			os.Exit(4)
		}
		if args[0] == "parent-exit" {
			return
		}
		_ = child.Wait()
	case "child":
		time.Sleep(24 * time.Hour)
	case "progress":
		writeHelperProgress(strings.Join([]string{
			"frame=12",
			"fps=29.97",
			"stream_0_0_q=-0.0",
			"bitrate=2048.0kbits/s",
			"total_size=889311",
			"out_time_us=3456000",
			"out_time_ms=3456000",
			"out_time=-00:00:03.456000",
			"dup_frames=0",
			"drop_frames=0",
			"speed=1.25x",
			"progress=end",
		}, "\n") + "\n")
		_, _ = io.WriteString(os.Stderr, "Decoder warning: recovered damaged frame\n")
	default:
		os.Exit(5)
	}
}

func TestProcessTreeNaturalExitSweepsDescendants(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	proc, err := Start(context.Background(), os.Args[0], []string{
		"-test.run=^TestProcessTreeHelper$", "--", "parent-exit", pidFile,
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = proc.Stdout.Close() }()
	if err := proc.Wait(); err != nil {
		t.Fatalf("naturally exited parent: %v", err)
	}

	childPID := waitForHelperPID(t, pidFile)
	deadline := time.Now().Add(5 * time.Second)
	for processRunning(childPID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if processRunning(childPID) {
		t.Errorf("descendant pid %d survived its parent's natural exit", childPID)
	}
}

func TestProcessTreeLifecycle(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithCancel(context.Background())
	proc, err := Start(ctx, os.Args[0], []string{
		"-test.run=^TestProcessTreeHelper$", "--", "parent", pidFile,
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = proc.Stdout.Close() }()

	childPID := waitForHelperPID(t, pidFile)

	cancel()
	if err := proc.Wait(); err != nil {
		t.Fatalf("cancelled process tree: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for processRunning(childPID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if processRunning(childPID) {
		t.Errorf("descendant pid %d survived process-tree cancellation", childPID)
	}
}

func waitForHelperPID(t *testing.T, pidFile string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		body, err := os.ReadFile(pidFile)
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(body)))
			if parseErr != nil {
				t.Fatalf("parse child pid: %v", parseErr)
			}
			return pid
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("helper did not publish its child pid")
	return 0
}

func helperArgs(args []string) []string {
	for i, arg := range args {
		if arg == "--" {
			return args[i+1:]
		}
	}
	return nil
}
