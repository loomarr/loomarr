package app

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/riverqueue/river"

	"github.com/loomarr/loomarr/internal/scheduler"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

type noopPreparedRunner struct{}

func (noopPreparedRunner) Run(context.Context) error { return nil }

type signalPreparedRunner struct{ ran chan struct{} }

func (r signalPreparedRunner) Run(context.Context) error { r.ran <- struct{}{}; return nil }

// A prepare pass only plans, launches planner-owned workers and returns, so it declares a short
// ceiling. A ceiling above River's default would route it to the single-worker `long` queue.
func TestPreparedPlayoutJobDeclaresAShortPassCeiling(t *testing.T) {
	job := preparedPlayoutJob(noopPreparedRunner{}, "")
	if job.Name != "playout-prepare" || job.ScheduleKey != "job.playout_prepare.schedule" ||
		job.DefaultCron != "0 * * * * *" {
		t.Fatalf("job = %+v", job)
	}
	if job.Timeout <= 0 || job.Timeout > river.JobTimeoutDefault {
		t.Fatalf("Timeout = %v, want a short explicit ceiling within River's default %v so the pass "+
			"stays off the long queue", job.Timeout, river.JobTimeoutDefault)
	}
}

// Live 2026-09-25: playout-prepare sat overdue for minutes while filler-pipeline (making serial LLM
// calls) held the one `long` worker, because the pass declared LongJobTimeout. Real River, real
// SQLite, the real job definition: with the long queue occupied the pass must still run.
func TestPreparedPlayoutJobRunsWhileTheLongQueueIsOccupied(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	ran := make(chan struct{}, 1)

	prepare := preparedPlayoutJob(signalPreparedRunner{ran: ran}, "")
	prepare.DefaultCron = "0 0 5 1 1 *" // only an explicit Trigger runs it
	reg := scheduler.NewRegistry().
		Add(scheduler.Job{
			Name: "hog", Group: scheduler.GroupSystem, Title: "Hog", Description: "a long media job.",
			DefaultCron: "0 0 5 1 1 *", Timeout: scheduler.LongJobTimeout,
			Run: func(ctx context.Context) error {
				close(started)
				select {
				case <-release:
				case <-ctx.Done():
				}
				return nil
			},
		}).
		Add(prepare)

	log := slog.New(slog.DiscardHandler)
	s := scheduler.New(st, reg, nil, time.Now, log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.SeedRegistry(ctx)
	if _, err := s.StartRiver(ctx, store.DialectOf(st), store.PoolOf(st), log); err != nil {
		t.Fatalf("StartRiver: %v", err)
	}
	if err := s.Trigger(ctx, "hog"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(20 * time.Second):
		t.Fatal("the long job never started; nothing is being proved about starvation")
	}
	if err := s.Trigger(ctx, "playout-prepare"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ran:
	case <-time.After(20 * time.Second):
		t.Fatal("playout-prepare could not run while a long job held the long queue")
	}
}
