package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

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
