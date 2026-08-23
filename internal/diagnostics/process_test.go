package diagnostics

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type processSinkMemory struct {
	mu     sync.Mutex
	runs   map[string]ProcessRun
	events map[string]int64
}

func (s *processSinkMemory) UpsertDiagnosticProcessRun(_ context.Context, run ProcessRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runs == nil {
		s.runs = map[string]ProcessRun{}
	}
	s.runs[run.ID] = run
	return nil
}

func (s *processSinkMemory) ListDiagnosticRetentionCandidates(_ context.Context, before time.Time, limit int) ([]RetentionCandidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []RetentionCandidate
	for id, size := range s.events {
		result = append(result, RetentionCandidate{Kind: EvidenceEvent, ID: id, At: 1, SizeBytes: size})
	}
	for id, run := range s.runs {
		if run.Status == ProcessRunning || run.EndedAt == 0 {
			continue
		}
		if !before.IsZero() && run.EndedAt >= before.UnixMilli() {
			continue
		}
		result = append(result, RetentionCandidate{Kind: EvidenceProcessRun, ID: id, At: run.EndedAt, SizeBytes: run.SizeBytes, OutputRef: run.OutputRef})
	}
	slices.SortFunc(result, func(a, b RetentionCandidate) int {
		if a.At < b.At {
			return -1
		}
		if a.At > b.At {
			return 1
		}
		return strings.Compare(a.ID, b.ID)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *processSinkMemory) DeleteDiagnosticEvent(_ context.Context, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.events[id]; !ok {
		return false, nil
	}
	delete(s.events, id)
	return true, nil
}

func (s *processSinkMemory) DeleteDiagnosticProcessRun(_ context.Context, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok || run.Status == ProcessRunning {
		return false, nil
	}
	delete(s.runs, id)
	return true, nil
}

func (s *processSinkMemory) DiagnosticRetainedBytes(_ context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var total int64
	for _, size := range s.events {
		total += size
	}
	for _, run := range s.runs {
		total += run.SizeBytes
	}
	return total, nil
}

func (s *processSinkMemory) get(id string) ProcessRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runs[id]
}

func TestProcessManagerCapturesBoundedRedactedOutputAndLifecycle(t *testing.T) {
	dir := t.TempDir()
	sink := &processSinkMemory{}
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	manager := NewProcessManager(sink, nil, ProcessOptions{
		OutputDir: dir, InstanceID: "instance-a", PrefixBytes: 12, TailBytes: 48,
		FlushInterval: time.Hour, Now: func() time.Time { return now },
		Version: func(context.Context, string) string { return "ffmpeg version 8.0" },
	})
	handle := manager.Begin(ProcessSpec{
		Purpose: "playout_program", ChannelID: "channel-a", Executable: "/usr/bin/ffmpeg",
		Args: []string{"-headers", "Authorization: Bearer secret", "-i", "https://u:p@example.test/video?token=secret", "-c:v", "copy", "/private/output.ts"},
	})
	if handle == nil {
		t.Fatal("Begin returned nil")
	}
	handle.RecordOutput("opening")
	handle.RecordOutput("Authorization: Bearer do-not-store")
	handle.RecordOutput("failed reading /private/media/movie.mkv token=also-secret")
	handle.RecordOutput("request failed at https://example.test/private/movie.mkv?session=abc")
	handle.Finish(ProcessResult{Err: context.DeadlineExceeded, TerminationReason: "deadline"})

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if err := manager.Close(ctx); err != nil {
		t.Fatal(err)
	}

	run := sink.get(handle.ID())
	if run.Status != ProcessFailed || run.EndedAt == 0 || run.ExecutableVersion != "ffmpeg version 8.0" {
		t.Fatalf("run lifecycle = %#v", run)
	}
	if run.InstanceID != "instance-a" || run.ChannelID != "channel-a" || run.OutputRef == "" {
		t.Fatalf("run correlation = %#v", run)
	}
	for _, secret := range []string{"do-not-store", "also-secret", "u:p", "/private/", "movie.mkv", "session=abc"} {
		if strings.Contains(run.CommandSummary+run.FirstError+run.LastError, secret) {
			t.Fatalf("metadata contains %q: %#v", secret, run)
		}
	}
	if !strings.Contains(run.CommandSummary, "[redacted]") || !strings.Contains(run.CommandSummary, "[input]") {
		t.Fatalf("command summary did not expose redaction: %q", run.CommandSummary)
	}
	output, err := os.ReadFile(filepath.Join(dir, run.OutputRef))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(output), "do-not-store") || strings.Contains(string(output), "also-secret") || strings.Contains(string(output), "/private/") {
		t.Fatalf("output was not redacted: %q", output)
	}
	if run.DiscardedLines == 0 || !strings.Contains(string(output), "discarded") {
		t.Fatalf("bounded output did not report truncation: run=%#v output=%q", run, output)
	}
}

func TestCommandSummaryNeverPersistsInputURLsOrCompletePaths(t *testing.T) {
	got := commandSummary("/opt/tools/ffmpeg", []string{
		"-i", "https://alice:password@example.test/media/title.mkv?api_key=secret",
		"-vf", "scale=1280:720", "C:\\Media\\private\\out.ts",
	})
	for _, forbidden := range []string{"alice", "password", "title.mkv", "api_key", "secret", "C:\\Media"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("command summary %q contains %q", got, forbidden)
		}
	}
	if !strings.Contains(got, "[input]") || !strings.Contains(got, "[path]") {
		t.Fatalf("command summary = %q", got)
	}
}

func TestProcessManagerPurgeCoordinatesFilesRowsAndProtectsActiveRuns(t *testing.T) {
	dir := t.TempDir()
	old := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	sink := &processSinkMemory{events: map[string]int64{"event-old": 20}, runs: map[string]ProcessRun{
		"run-old":    {ID: "run-old", Purpose: "probe", Status: ProcessFailed, EndedAt: old.UnixMilli(), OutputRef: "run-old.log", SizeBytes: 40},
		"run-active": {ID: "run-active", Purpose: "playout", Status: ProcessRunning, StartedAt: old.UnixMilli(), OutputRef: "run-active.log", SizeBytes: 100},
	}}
	for _, name := range []string{"run-old.log", "run-active.log"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("diagnostic"), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	manager := NewProcessManager(sink, nil, ProcessOptions{OutputDir: dir})
	result, err := manager.Purge(t.Context(), old.Add(24*time.Hour), 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.Events != 1 || result.ProcessRuns != 1 || result.RetainedBytes != 100 {
		t.Fatalf("purge result = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(dir, "run-old.log")); !os.IsNotExist(err) {
		t.Fatalf("completed output still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "run-active.log")); err != nil {
		t.Fatalf("active output was removed: %v", err)
	}
	if _, ok := sink.runs["run-active"]; !ok {
		t.Fatal("active row was removed")
	}
	if err := manager.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestProcessManagerCloseCanContinueAfterCallerTimeout(t *testing.T) {
	sink := &processSinkMemory{}
	manager := NewProcessManager(sink, nil, ProcessOptions{OutputDir: t.TempDir(), Version: func(context.Context, string) string { return "" }})
	handle := manager.Begin(ProcessSpec{Purpose: "long_running", Executable: "ffmpeg"})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := manager.Close(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("first Close = %v, want cancelled", err)
	}
	handle.Finish(ProcessResult{})
	ctx, cancel = context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := manager.Close(ctx); err != nil {
		t.Fatalf("second Close = %v", err)
	}
}

func TestProcessManagerCachesExecutableVersionAcrossRuns(t *testing.T) {
	sink := &processSinkMemory{}
	var calls atomic.Int32
	manager := NewProcessManager(sink, nil, ProcessOptions{
		OutputDir: t.TempDir(), Version: func(context.Context, string) string {
			calls.Add(1)
			return "ffmpeg version cached"
		},
	})
	first := manager.Begin(ProcessSpec{Purpose: "first", Executable: "/usr/bin/ffmpeg"})
	second := manager.Begin(ProcessSpec{Purpose: "second", Executable: "/usr/bin/ffmpeg"})
	first.Finish(ProcessResult{})
	second.Finish(ProcessResult{})
	if err := manager.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("version resolver calls = %d, want 1", got)
	}
}
