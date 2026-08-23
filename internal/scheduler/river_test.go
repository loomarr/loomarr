package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/loomarr/loomarr/internal/store"
)

type historyNotifier chan string

func (n historyNotifier) JobChanged(name string) { n <- name }

func TestLiveCronGateReadsTheCurrentSettingAndEmitsEachTickOnce(t *testing.T) {
	cron := "0 */5 * * * *"
	job := Job{
		Name: "probe", Group: GroupSystem, Title: "Probe", Description: "Runs the probe.",
		DefaultCron: cron, ScheduleKey: "job.probe.schedule",
		Run: func(context.Context) error { return nil },
	}
	s := New(newFakeStore(), NewRegistry().Add(job), func(_, _ string) string { return cron }, time.Now, testLog())
	gate := &liveCronGate{scheduler: s, job: job}
	current := time.Date(2026, time.August, 14, 12, 5, 5, 0, time.UTC)

	if !gate.due(current) {
		t.Fatal("five-minute cron was not due just after its tick")
	}
	if gate.due(current.Add(time.Second)) {
		t.Fatal("same cron tick was emitted twice across overlapping poll windows")
	}

	cron = "0 */15 * * * *"
	if gate.due(time.Date(2026, time.August, 14, 12, 10, 5, 0, time.UTC)) {
		t.Fatal("changed fifteen-minute cron fired before its next tick")
	}
	if !gate.due(time.Date(2026, time.August, 14, 12, 15, 5, 0, time.UTC)) {
		t.Fatal("changed fifteen-minute cron did not apply at its next tick")
	}
	if got, want := gate.Next(current), current.Add(liveCronPollInterval); !got.Equal(want) {
		t.Fatalf("next live schedule poll = %v, want %v", got, want)
	}
}

func TestRiverWorkerManualRunBypassesPause(t *testing.T) {
	st := newFakeStore()
	var ran atomic.Int64
	job := Job{
		Name: "probe", Group: GroupSystem, Title: "Probe", Description: "Runs the probe.", DefaultCron: everyMinute,
		Run: func(context.Context) error { ran.Add(1); return nil },
	}
	s := New(st, NewRegistry().Add(job), nil, time.Now, testLog())
	if err := s.SetPaused(context.Background(), job.Name, true); err != nil {
		t.Fatal(err)
	}
	w := riverWorker{s: s}

	if err := w.Work(context.Background(), &river.Job[jobArgs]{Args: jobArgs{Name: job.Name}}); err != nil {
		t.Fatal(err)
	}
	if got := ran.Load(); got != 0 {
		t.Fatalf("periodic run of paused job executed %d times, want 0", got)
	}

	if err := w.Work(context.Background(), &river.Job[jobArgs]{Args: jobArgs{Name: job.Name, Manual: true}}); err != nil {
		t.Fatal(err)
	}
	if got := ran.Load(); got != 1 {
		t.Fatalf("manual run of paused job executed %d times, want 1", got)
	}
}

func TestPeriodicInsertOptsCoalescesOnlyActiveScheduledTicks(t *testing.T) {
	opts := periodicInsertOpts(queueTestJob("probe", 0))
	if !opts.UniqueOpts.ByArgs {
		t.Fatal("periodic insert is not unique by job args")
	}
	want := map[rivertype.JobState]bool{
		rivertype.JobStateAvailable: true,
		rivertype.JobStatePending:   true,
		rivertype.JobStateRunning:   true,
		rivertype.JobStateRetryable: true,
		rivertype.JobStateScheduled: true,
	}
	for _, state := range opts.UniqueOpts.ByState {
		delete(want, state)
	}
	if len(want) != 0 {
		t.Fatalf("periodic uniqueness is missing active states: %v", want)
	}
	for _, state := range opts.UniqueOpts.ByState {
		if state == rivertype.JobStateCompleted || state == rivertype.JobStateDiscarded {
			t.Fatalf("periodic uniqueness includes terminal state %q; the next tick would be swallowed", state)
		}
	}
}

func TestJobArgsUseOneRiverKindPerNamedTask(t *testing.T) {
	if got, want := (jobArgs{Name: "reconcile"}).Kind(), "loomarr_job:reconcile"; got != want {
		t.Fatalf("named kind = %q, want %q", got, want)
	}
	if got := (jobArgs{}).Kind(); got != legacyJobKind {
		t.Fatalf("legacy empty-name kind = %q, want %q", got, legacyJobKind)
	}
}

func TestSummarizeJobHistoryCountsOnlyTrustworthyRunsInTheWindow(t *testing.T) {
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	row := func(start time.Time, duration time.Duration, result string, manual bool) *rivertype.JobRow {
		finish := start.Add(duration)
		args, err := json.Marshal(jobArgs{Name: "probe", Manual: manual})
		if err != nil {
			t.Fatal(err)
		}
		metadata, err := json.Marshal(map[string]any{"output": jobExecutionOutput{
			StartedAt: start, FinishedAt: finish, DurationMs: duration.Milliseconds(), Result: result,
			Error: map[bool]string{true: "probe failed"}[result == "error"],
		}})
		if err != nil {
			t.Fatal(err)
		}
		return &rivertype.JobRow{EncodedArgs: args, Metadata: metadata}
	}

	rows := []*rivertype.JobRow{
		row(now.Add(-time.Hour), 3*time.Second, "error", true),
		row(now.Add(-2*time.Hour), time.Second, "ok", false),
		{EncodedArgs: []byte(`{"name":"probe"}`), Metadata: []byte(`{}`)}, // old row: no outcome
		row(now.Add(-25*time.Hour), 9*time.Second, "error", false),        // outside the window
	}
	history := summarizeJobHistory(now, rows, false)

	if history.RunCount != 2 || history.FailureCount != 1 || history.AverageDurationMs != 2000 {
		t.Fatalf("summary = %+v, want 2 runs, 1 failure, 2000ms average", history)
	}
	if len(history.Recent) != 2 || !history.Recent[0].Manual || history.Recent[0].Error != "probe failed" {
		t.Fatalf("recent = %+v, want newest manual failure first", history.Recent)
	}
	if !history.WindowStart.Equal(now.Add(-24 * time.Hour)) {
		t.Fatalf("window start = %v, want %v", history.WindowStart, now.Add(-24*time.Hour))
	}
}

func TestRiverHistoryRecordsTheLoomarrOutcome(t *testing.T) {
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/history.db", true)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	log := slog.New(slog.DiscardHandler)
	ran := make(chan struct{}, 1)
	changed := make(historyNotifier, 1)
	s := New(st, NewRegistry().Add(Job{
		Name: "probe", Group: GroupSystem, Title: "Probe", Description: "Runs the probe.",
		DefaultCron: "0 0 5 1 1 *",
		Run: func(context.Context) error {
			ran <- struct{}{}
			return errors.New("media server unavailable")
		},
	}), nil, time.Now, log).WithNotifier(changed)
	s.SeedRegistry(ctx)
	wait, err := s.StartRiver(ctx, st, store.PoolOf(st), log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		waitCtx, waitCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer waitCancel()
		_ = wait(waitCtx)
		_ = st.Close()
	})

	if err := s.Trigger(ctx, "probe"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ran:
	case <-time.After(20 * time.Second):
		t.Fatal("triggered job did not run")
	}
	select {
	case name := <-changed:
		if name != "probe" {
			t.Fatalf("completion notification = %q, want probe", name)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("completed job did not notify the Tasks page")
	}

	// The notification is deliberately post-finalization: history must contain the run by
	// the time an SSE consumer reacts to this signal and refetches.
	history, err := s.History(ctx, "probe")
	if err != nil {
		t.Fatal(err)
	}
	if history.RunCount != 1 || history.FailureCount != 1 || len(history.Recent) != 1 ||
		!history.Recent[0].Manual || history.Recent[0].Error != "media server unavailable" {
		t.Fatalf("history after completion notification = %+v, want one manual failure", history)
	}
}
