package playout

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/diagnostics"
	"github.com/loomarr/loomarr/internal/testkit/playoutprocessfixture"
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

func TestProcessWaitPreservesUnreadFiniteOutput(t *testing.T) {
	for _, mode := range []string{"prepared-success", "prepared-failure"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			proc, err := Start(ctx, os.Args[0], []string{
				"-test.run=^TestProcessTreeHelper$", "--", mode,
			}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = proc.Stdout.Close() }()
			// A lifecycle observer can reap a short child before its media consumer
			// reads the final buffered bytes. Reaping must preserve those bytes.
			waitErr := proc.Wait()
			if (waitErr != nil) != (mode == "prepared-failure") {
				t.Fatalf("natural child exit: %v", waitErr)
			}
			got, err := io.ReadAll(proc.Stdout)
			if err != nil || string(got) != playoutprocessfixture.PreparedPrefix {
				t.Fatalf("unread finite output after Wait: %q, error: %v", got, err)
			}
		})
	}
}

type processDiagnosticsSink struct {
	mu     sync.Mutex
	runs   map[string]diagnostics.ProcessRun
	events []diagnostics.Record
}

func (s *processDiagnosticsSink) UpsertDiagnosticProcessRun(_ context.Context, run diagnostics.ProcessRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runs == nil {
		s.runs = map[string]diagnostics.ProcessRun{}
	}
	s.runs[run.ID] = run
	return nil
}
func (s *processDiagnosticsSink) AppendDiagnosticEvents(_ context.Context, events []diagnostics.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, events...)
	return nil
}
func (*processDiagnosticsSink) ListDiagnosticRetentionCandidates(context.Context, time.Time, int) ([]diagnostics.RetentionCandidate, error) {
	return nil, nil
}
func (*processDiagnosticsSink) DeleteDiagnosticEvent(context.Context, string) (bool, error) {
	return false, nil
}
func (*processDiagnosticsSink) DeleteDiagnosticProcessRun(context.Context, string) (bool, error) {
	return false, nil
}
func (*processDiagnosticsSink) DiagnosticRetainedBytes(context.Context) (int64, error) { return 0, nil }

func TestStartObservedPersistsProgressStderrAndExit(t *testing.T) {
	sink := &processDiagnosticsSink{}
	recorder := diagnostics.New(sink, diagnostics.Options{FlushInterval: time.Millisecond})
	manager := diagnostics.NewProcessManager(sink, recorder, diagnostics.ProcessOptions{
		OutputDir: t.TempDir(), FlushInterval: time.Millisecond,
		Version: func(context.Context, string) string { return "test ffmpeg version" },
	})
	proc, err := StartObserved(t.Context(), os.Args[0], []string{
		"-test.run=^TestProcessTreeHelper$", "--", "progress",
	}, nil, nil, manager, diagnostics.ProcessSpec{
		Purpose: "playout_program", ChannelID: "channel-a", ScheduleBlockID: "block-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := proc.Wait(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if err := manager.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := recorder.Close(ctx); err != nil {
		t.Fatal(err)
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	run := sink.runs[proc.ProcessRunID()]
	if run.Status != diagnostics.ProcessSucceeded || run.ChannelID != "channel-a" || run.ScheduleBlockID != "block-a" {
		t.Fatalf("observed run = %#v", run)
	}
	if run.FirstError != "Decoder warning: recovered damaged frame" || run.OutputBytes == 0 {
		t.Fatalf("observed output metadata = %#v", run)
	}
	progress := false
	for _, event := range sink.events {
		if event.Event == "process.progress" && event.ProcessRunID == run.ID {
			progress = true
		}
	}
	if !progress {
		t.Fatalf("progress events = %+v", sink.events)
	}
}

func TestStartObservedEarlyCancellationFinishesDiagnostics(t *testing.T) {
	const attempts = 20
	for attempt := 0; attempt < attempts; attempt++ {
		t.Run(strconv.Itoa(attempt), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			registrationBlocked := make(chan struct{})
			allowRegistration := make(chan struct{})
			var registrationOnce sync.Once
			var allowOnce sync.Once
			unblockRegistration := func() { allowOnce.Do(func() { close(allowRegistration) }) }

			sink := &processDiagnosticsSink{}
			recorder := diagnostics.New(sink, diagnostics.Options{FlushInterval: time.Millisecond})
			manager := diagnostics.NewProcessManager(sink, recorder, diagnostics.ProcessOptions{
				OutputDir: t.TempDir(), FlushInterval: time.Millisecond,
				Version: func(context.Context, string) string { return "test ffmpeg version" },
				Now: func() time.Time {
					registrationOnce.Do(func() {
						close(registrationBlocked)
						<-allowRegistration
					})
					return time.Now()
				},
			})
			type startResult struct {
				proc *Process
				err  error
			}
			started := make(chan startResult, 1)
			var proc *Process
			go func() {
				proc, err := StartObserved(ctx, os.Args[0], []string{
					"-test.run=^TestProcessTreeHelper$", "--", "parent", filepath.Join(t.TempDir(), "child.pid"),
				}, nil, nil, manager, diagnostics.ProcessSpec{Purpose: "playout_program"})
				started <- startResult{proc: proc, err: err}
			}()
			var cleanupOnce sync.Once
			var cleanupErr error
			cleanup := func() error {
				cleanupOnce.Do(func() {
					unblockRegistration()
					cancel()
					closeCtx, closeCancel := context.WithTimeout(context.Background(), 2*time.Second)
					defer closeCancel()
					if proc == nil {
						select {
						case result := <-started:
							proc = result.proc
							cleanupErr = errors.Join(cleanupErr, result.err)
						case <-closeCtx.Done():
							cleanupErr = errors.Join(cleanupErr, errors.New("process startup did not finish during cleanup"))
						}
					}
					if proc != nil {
						waited := make(chan error, 1)
						go func() { waited <- proc.Wait() }()
						select {
						case err := <-waited:
							cleanupErr = errors.Join(cleanupErr, err)
						case <-closeCtx.Done():
							cleanupErr = errors.Join(cleanupErr, errors.New("process did not stop during cleanup"))
						}
					}
					cleanupErr = errors.Join(cleanupErr, manager.Close(closeCtx))
					cleanupErr = errors.Join(cleanupErr, recorder.Close(closeCtx))
				})
				return cleanupErr
			}
			defer func() {
				if err := cleanup(); err != nil {
					t.Error(err)
				}
			}()

			select {
			case <-registrationBlocked:
			case <-time.After(2 * time.Second):
				t.Fatal("diagnostic registration did not reach the controlled boundary")
			}
			// This is an ordinary cancellation while Begin is blocked, not a direct
			// Stop before StartObserved returns. The loop makes the former ordering's
			// scheduler race repeatable without changing Context's contract.
			cancel()
			time.Sleep(20 * time.Millisecond)
			unblockRegistration()

			var result startResult
			select {
			case result = <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("process startup did not return after diagnostic registration")
			}
			if result.err != nil {
				t.Fatal(result.err)
			}
			proc = result.proc
			if err := cleanup(); err != nil {
				t.Fatal(err)
			}
			sink.mu.Lock()
			defer sink.mu.Unlock()
			run := sink.runs[result.proc.ProcessRunID()]
			if run.Status != diagnostics.ProcessCancelled {
				t.Fatalf("cancelled process diagnostic = %#v, want status %q", run, diagnostics.ProcessCancelled)
			}
		})
	}
}

func TestStartObservedCancellationFinishesRecordedDiagnostics(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sink := &processDiagnosticsSink{}
	recorder := diagnostics.New(sink, diagnostics.Options{FlushInterval: time.Millisecond})
	manager := diagnostics.NewProcessManager(sink, recorder, diagnostics.ProcessOptions{
		OutputDir: t.TempDir(), FlushInterval: time.Millisecond,
		Version: func(context.Context, string) string { return "test ffmpeg version" },
	})
	proc, err := StartObserved(ctx, os.Args[0], []string{
		"-test.run=^TestProcessTreeHelper$", "--", "parent", filepath.Join(t.TempDir(), "child.pid"),
	}, nil, nil, manager, diagnostics.ProcessSpec{Purpose: "playout_program"})
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		proc.Stop()
		_ = proc.Wait()
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer closeCancel()
		if err := manager.Close(closeCtx); err != nil {
			t.Error(err)
		}
		if err := recorder.Close(closeCtx); err != nil {
			t.Error(err)
		}
	})

	runID := proc.ProcessRunID()
	waitForObservedRun(t, sink, runID, func(run diagnostics.ProcessRun) bool { return run.ID != "" })
	cancel()
	waitForObservedRun(t, sink, runID, func(run diagnostics.ProcessRun) bool {
		return run.Status == diagnostics.ProcessCancelled
	})
}

func waitForObservedRun(t *testing.T, sink *processDiagnosticsSink, runID string, ready func(diagnostics.ProcessRun) bool) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		sink.mu.Lock()
		run := sink.runs[runID]
		sink.mu.Unlock()
		if ready(run) {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("process diagnostic %q did not reach the expected state: %#v", runID, run)
		case <-tick.C:
		}
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
	case "prepared-success", "prepared-failure", "prepared-stalled":
		playoutprocessfixture.RunPrepared(args[0])
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

func TestWaitForHelperPIDWaitsForCompletePublication(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	if err := os.WriteFile(pidFile, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		time.Sleep(25 * time.Millisecond)
		done <- os.WriteFile(pidFile, []byte("1234"), 0o600)
	}()
	if got := waitForHelperPID(t, pidFile); got != 1234 {
		t.Fatalf("pid = %d, want 1234", got)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func waitForHelperPID(t *testing.T, pidFile string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		body, err := os.ReadFile(pidFile)
		if err == nil {
			published := strings.TrimSpace(string(body))
			if published == "" {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			pid, parseErr := strconv.Atoi(published)
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
