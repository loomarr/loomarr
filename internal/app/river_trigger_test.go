package app

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/scheduler"
	"github.com/mantonx/loomarr/internal/store"
)

// ⚠ **The regression guard for a 50× Run-now slowdown, and it is subtle enough to reintroduce.**
//
// River has two insert paths that are FIVE SECONDS apart: a job inserted as AVAILABLE is picked
// up on the fetch poll (~100ms), while one inserted as SCHEDULED waits for the job-scheduler
// maintenance pass (`JobSchedulerIntervalDefault = 5s`). ANY future `ScheduledAt` puts the job
// on the slow path — including `now + 10ms`, which is exactly what a first cut used to "keep
// the insert clear of a periodic insert landing in the same instant".
//
// That made every Run-now take ~4.97s, measured consistently. It reads as a broken button, and
// it made the poll-availability safety proof (TestScanAvailability_NoWebhook — the gate that
// authorized deleting the inbound webhook) flaky under load.
//
// The threshold is deliberately loose (2s): this asserts "not on the 5s maintenance cycle", not
// a performance number, so it will not go flaky on a slow CI box. A regression lands at ~5s and
// fails clearly; the healthy path measures ~100ms with 20× of headroom.
func TestRiverTrigger_IsNotOnTheSlowSchedulePath(t *testing.T) {
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/trigger.db", true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	fired := make(chan time.Time, 4)
	reg := scheduler.NewRegistry().Add(scheduler.Job{
		Name: "probe", Group: scheduler.GroupSystem, Title: "Probe", Description: "probe job.",
		// Far-future cron: only an explicit Trigger can run this, so the measurement can
		// never accidentally time a periodic firing.
		DefaultCron: "0 0 5 1 1 *",
		Run:         func(context.Context) error { fired <- time.Now(); return nil },
	})
	log := slog.New(slog.DiscardHandler)
	s := scheduler.New(st, reg, nil, time.Now, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.SeedRegistry(ctx)
	if _, err := s.StartRiver(ctx, st, store.PoolOf(st), log); err != nil {
		t.Fatalf("StartRiver: %v", err)
	}

	// Twice: the first insert also pays any one-time warm-up, and the bug showed on BOTH, so
	// a single sample could be explained away as startup cost.
	for i := range 2 {
		start := time.Now()
		if err := s.Trigger(ctx, "probe"); err != nil {
			t.Fatalf("trigger %d: %v", i, err)
		}
		select {
		case at := <-fired:
			if took := at.Sub(start); took > 2*time.Second {
				t.Errorf("trigger %d reached the worker in %v — that is River's 5s job-scheduler "+
					"cycle, so the insert carries a future ScheduledAt. Run now must insert as "+
					"available (no InsertOpts.ScheduledAt).", i, took)
			}
		case <-time.After(20 * time.Second):
			t.Fatalf("trigger %d never reached a worker", i)
		}
	}
}
