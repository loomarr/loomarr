package app

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/jobs"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

// ⚠ **A job's ceiling must not become every other job's ceiling.**
//
// `Job.Timeout` fixed jobs being SIGKILLed at 60 seconds, and by doing so created a worse problem:
// on SQLite there is ONE worker slot (`MaxWorkers: 1`, because the store holds `MaxOpenConns(1)`),
// so a job now permitted to run for 30 minutes holds it for 30 minutes. Measured live 2026-08-12:
// a `filler-pipeline` pass ran 01:50:11Z → 02:20:47Z and every other job was starved for the whole
// span — channel maintenance, `images-fetch` and `seerr-queue-poll` all missed 02:00:00Z,
// `library-scan` and `reconcile` sat at 01:55:00Z, and a manually triggered `filler-sync` did not
// execute until the worker freed.
//
// The fix is a second queue, and these are the two tests that hold it in place.

// The starvation proof, against real River and real SQLite.
//
// ⚠ **Deterministic without a clock or a sleep.** The hog blocks on an unbuffered channel, so
// receiving from `started` establishes happens-before: the hog is provably INSIDE `Work`, holding
// a slot, at the moment the cheap job is triggered. "Cheap ran while hog was running" is therefore
// a causal relation, not a timing window that a slow CI box could invert. The only deadline is the
// failure bound.
func TestRiverQueues_ALongJobDoesNotStarveTheCheapOnes(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)

	started := make(chan struct{})
	release := make(chan struct{})
	// ⚠ Closed FIRST so the hog can never outlive the test, whatever else fails below.
	defer close(release)
	fired := make(chan struct{}, 1)

	// Far-future crons: only an explicit Trigger runs these, so no periodic insert can confound
	// which job occupied which slot.
	reg := jobs.NewRegistry().
		Add(jobs.Job{
			Name: "hog", Group: jobs.GroupSystem, Title: "Hog", Description: "a long media job.",
			DefaultCron: "0 0 5 1 1 *",
			Timeout:     jobs.LongJobTimeout,
			Run: func(ctx context.Context) error {
				close(started)
				select {
				case <-release:
				case <-ctx.Done():
				}
				return nil
			},
		}).
		Add(jobs.Job{
			Name: "cheap", Group: jobs.GroupSystem, Title: "Cheap", Description: "a cheap sweep.",
			DefaultCron: "0 0 5 1 1 *",
			// A short explicit ceiling must preserve cancellation without putting health-like work
			// behind the long media queue.
			Timeout: 10 * time.Second,
			Run:     func(context.Context) error { fired <- struct{}{}; return nil },
		})

	log := slog.New(slog.DiscardHandler)
	s := jobs.New(st, reg, nil, time.Now, log)
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

	// The hog is inside Work, holding a slot. A cheap sweep must still run.
	if err := s.Trigger(ctx, "cheap"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-fired:
	case <-time.After(20 * time.Second):
		t.Fatal("a cheap job could not run while a long job held a worker — one job's 30-minute " +
			"ceiling is every other job's 30-minute outage")
	}
}

// ⚠ **The job that says whether Loomarr is alive never waits behind another job.**
//
// Measured live 2026-11-18: `channel-maintenance` spent 53 s inside its Emby retry budget
// (01:40:02 → 01:40:55Z) on the single default worker. `system-health` could not start, the
// database check went stale at its 65 s mark, `/v1/readyz` answered 503, and the reverse proxy took
// Loomarr off the network: a viewer's stream got 503s and stopped. The same happened at 01:30. Any
// default job may legitimately run for River's 60 s, so a shared worker can always outlast the
// freshness window; the watchdog needs a worker of its own.
func TestRiverQueues_AStuckDefaultJobDoesNotDelayTheWatchdog(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)

	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	fired := make(chan struct{}, 1)

	reg := jobs.NewRegistry().
		Add(jobs.Job{
			Name: "maintenance", Group: jobs.GroupSystem, Title: "Maintenance",
			Description: "an ordinary job waiting on a busy media server.",
			DefaultCron: "0 0 5 1 1 *",
			Run: func(ctx context.Context) error {
				close(started)
				select {
				case <-release:
				case <-ctx.Done():
				}
				return nil
			},
		}).
		Add(jobs.Job{
			Name: "health", Group: jobs.GroupSystem, Title: "Health", Description: "the health watchdog.",
			DefaultCron: "0 0 5 1 1 *", Timeout: 10 * time.Second, Watchdog: true,
			Run: func(context.Context) error { fired <- struct{}{}; return nil },
		})

	log := slog.New(slog.DiscardHandler)
	s := jobs.New(st, reg, nil, time.Now, log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.SeedRegistry(ctx)
	if _, err := s.StartRiver(ctx, store.DialectOf(st), store.PoolOf(st), log); err != nil {
		t.Fatalf("StartRiver: %v", err)
	}

	if err := s.Trigger(ctx, "maintenance"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(20 * time.Second):
		t.Fatal("the ordinary job never started; nothing is being proved")
	}
	if err := s.Trigger(ctx, "health"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-fired:
	case <-time.After(20 * time.Second):
		t.Fatal("the health watchdog waited behind an ordinary job; readiness would go stale and " +
			"the proxy would take Loomarr off the network")
	}
}

// The routing invariant is pinned in `internal/platform/jobs` (queues_test.go), where `queueFor` and
// `riverQueues` live — test-only exports would be a worse trade than an internal test.
