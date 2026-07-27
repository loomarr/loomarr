package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/store"
)

// fakeStore is an in-memory ScheduleStore with a controllable clock-independent view; the
// scheduler drives `now` via a fake clock, so we assert due-selection + lease behavior
// deterministically without real time.
type fakeStore struct {
	mu   sync.Mutex
	rows map[string]store.ScheduledJob
}

func newFakeStore() *fakeStore { return &fakeStore{rows: map[string]store.ScheduledJob{}} }

func (f *fakeStore) UpsertScheduledJob(_ context.Context, j store.ScheduledJob) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows[j.Name] = j
	return nil
}
func (f *fakeStore) GetScheduledJob(_ context.Context, name string) (store.ScheduledJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	j, ok := f.rows[name]
	if !ok {
		return store.ScheduledJob{}, errors.New("not found")
	}
	return j, nil
}
func (f *fakeStore) ListScheduledJobs(_ context.Context) ([]store.ScheduledJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]store.ScheduledJob, 0, len(f.rows))
	for _, j := range f.rows {
		out = append(out, j)
	}
	return out, nil
}

// ClaimDueScheduledJobs returns rows whose NextRun <= now, leasing them to now+lease — the
// same contract as the SQL implementation.
func (f *fakeStore) ClaimDueScheduledJobs(_ context.Context, now time.Time, lease time.Duration) ([]store.ScheduledJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var due []store.ScheduledJob
	for name, j := range f.rows {
		if !j.NextRun.After(now) {
			leased := j
			leased.NextRun = now.Add(lease)
			f.rows[name] = leased
			due = append(due, j) // return the pre-lease row (matches SQL RETURNING of the row identity)
		}
	}
	return due, nil
}

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func testLog() *slog.Logger { return slog.New(slog.DiscardHandler) }

// everyMinute is a 6-field seconds-leading cron (top of every minute) used across tests as a
// simple recurring schedule whose next-tick is deterministic.
const everyMinute = "0 * * * * *"

// A registered job with no state row is seeded due-now and runs on the first tick.
func TestReconcileSeedsAndRuns(t *testing.T) {
	st := newFakeStore()
	clk := &fakeClock{t: time.Unix(1000, 0)}
	var ran atomic.Int64
	reg := NewRegistry().Add(Job{
		Name: "a", DefaultCron: everyMinute, Run: func(context.Context) error { ran.Add(1); return nil },
	})
	s := New(st, reg, nil, clk.now, testLog())
	ctx := context.Background()

	s.reconcileRegistry(ctx) // seeds "a" due-now
	if j, _ := st.GetScheduledJob(ctx, "a"); j.NextRun != clk.now() {
		t.Fatalf("seeded next_run = %v, want now", j.NextRun)
	}
	s.tick(ctx)
	// ⚠ Wait for the STORE WRITE, not the run counter. `ran` is incremented INSIDE the job
	// function, while next_run/last_result are written after it returns — so waiting on the
	// counter can observe a job that has run but not yet recorded, and the assertions below
	// then read the seeded 30m default and an empty last_result.
	//
	// A real flake, not a theoretical one: it failed CI on main and reproduces locally with
	// `-count=200 -cpu=1,2` (it needs contention, which is why a 24-core dev box never sees
	// it and a 4-core runner does).
	waitFor(t, func() bool {
		j, err := st.GetScheduledJob(ctx, "a")
		return err == nil && j.LastResult != ""
	})

	// After running, next_run is the next cron tick — strictly after now, within a minute.
	j, _ := st.GetScheduledJob(ctx, "a")
	if got := j.NextRun.Sub(clk.now()); got <= 0 || got > time.Minute {
		t.Errorf("post-run next_run offset = %v, want (0, 1m] from the every-minute cron", got)
	}
	if j.LastResult != "ok" {
		t.Errorf("last_result = %q, want ok", j.LastResult)
	}
}

// A job not yet due is NOT run.
func TestNotDueDoesNotRun(t *testing.T) {
	st := newFakeStore()
	clk := &fakeClock{t: time.Unix(1000, 0)}
	var ran atomic.Int64
	reg := NewRegistry().Add(Job{Name: "a", DefaultCron: everyMinute, Run: func(context.Context) error { ran.Add(1); return nil }})
	s := New(st, reg, nil, clk.now, testLog())
	// Seed it due in the future.
	_ = st.UpsertScheduledJob(context.Background(), store.ScheduledJob{Name: "a", NextRun: clk.now().Add(time.Hour)})

	s.tick(context.Background())
	time.Sleep(20 * time.Millisecond)
	if ran.Load() != 0 {
		t.Errorf("ran %d, want 0 (not due)", ran.Load())
	}
}

// The lease prevents a second tick from re-running a job while the first run is in flight.
func TestLeasePreventsDoubleRun(t *testing.T) {
	st := newFakeStore()
	clk := &fakeClock{t: time.Unix(1000, 0)}
	release := make(chan struct{})
	var starts int
	var mu sync.Mutex
	reg := NewRegistry().Add(Job{Name: "slow", DefaultCron: everyMinute, Run: func(context.Context) error {
		mu.Lock()
		starts++
		mu.Unlock()
		<-release // block so the job is "in flight" across the second tick
		return nil
	}})
	s := New(st, reg, nil, clk.now, testLog())
	ctx := context.Background()
	s.reconcileRegistry(ctx)

	s.tick(ctx) // claims + starts the job (leases next_run to now+30m)
	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return starts == 1 })
	s.tick(ctx) // job still running, leased into the future → not re-claimed
	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	got := starts
	mu.Unlock()
	if got != 1 {
		t.Errorf("job started %d times, want 1 (lease must block the re-claim)", got)
	}
	close(release)
}

// Trigger forces an off-cycle run even when the job isn't otherwise due.
func TestTriggerRunsOffCycle(t *testing.T) {
	st := newFakeStore()
	clk := &fakeClock{t: time.Unix(1000, 0)}
	var ran atomic.Int64
	reg := NewRegistry().Add(Job{Name: "a", DefaultCron: everyMinute, Run: func(context.Context) error { ran.Add(1); return nil }})
	s := New(st, reg, nil, clk.now, testLog())
	ctx := context.Background()
	// Not due for an hour.
	_ = st.UpsertScheduledJob(ctx, store.ScheduledJob{Name: "a", NextRun: clk.now().Add(time.Hour)})

	if err := s.Trigger(ctx, "a"); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	s.tick(ctx) // Trigger set next_run=now → now due
	waitFor(t, func() bool { return ran.Load() == 1 })

	if err := s.Trigger(ctx, "nope"); err != ErrUnknownJob {
		t.Errorf("trigger unknown = %v, want ErrUnknownJob", err)
	}
}

// An erroring job records last_result=error and never wedges the others.
func TestErroringJobRecordsErrorAndIsolated(t *testing.T) {
	st := newFakeStore()
	clk := &fakeClock{t: time.Unix(1000, 0)}
	var okRan atomic.Int64
	reg := NewRegistry().
		Add(Job{Name: "bad", DefaultCron: everyMinute, Run: func(context.Context) error { return errors.New("boom") }}).
		Add(Job{Name: "good", DefaultCron: everyMinute, Run: func(context.Context) error { okRan.Add(1); return nil }})
	s := New(st, reg, nil, clk.now, testLog())
	ctx := context.Background()
	s.reconcileRegistry(ctx)

	s.tick(ctx)
	waitFor(t, func() bool {
		bad, _ := st.GetScheduledJob(ctx, "bad")
		return bad.LastResult == "error" && okRan.Load() == 1
	})
	bad, _ := st.GetScheduledJob(ctx, "bad")
	if bad.LastError == "" {
		t.Error("erroring job should record last_error")
	}
}

// A panicking job is contained (recorded as error), not a scheduler crash.
func TestPanicIsContained(t *testing.T) {
	st := newFakeStore()
	clk := &fakeClock{t: time.Unix(1000, 0)}
	reg := NewRegistry().Add(Job{Name: "panics", DefaultCron: everyMinute, Run: func(context.Context) error { panic("kaboom") }})
	s := New(st, reg, nil, clk.now, testLog())
	ctx := context.Background()
	s.reconcileRegistry(ctx)

	s.tick(ctx)
	waitFor(t, func() bool {
		j, _ := st.GetScheduledJob(ctx, "panics")
		return j.LastResult == "error"
	})
}

// The effective cron reads the settings resolver, falling back to the default when unset,
// and to the default when the configured value is an invalid cron (guard).
func TestEffectiveCronUsesResolverThenDefault(t *testing.T) {
	st := newFakeStore()
	clk := &fakeClock{t: time.Unix(1000, 0)}
	resolver := func(key, def string) string {
		switch key {
		case "job.a.schedule":
			return "0 */15 * * * *" // configured override
		case "job.c.schedule":
			return "not a cron" // invalid override → falls back to default
		default:
			return "" // unset → caller falls back to default
		}
	}
	reg := NewRegistry().
		Add(Job{Name: "a", DefaultCron: everyMinute, ScheduleKey: "job.a.schedule", Run: func(context.Context) error { return nil }}).
		Add(Job{Name: "b", DefaultCron: "0 0 3 * * *", ScheduleKey: "job.b.schedule", Run: func(context.Context) error { return nil }}).
		Add(Job{Name: "c", DefaultCron: everyMinute, ScheduleKey: "job.c.schedule", Run: func(context.Context) error { return nil }})
	s := New(st, reg, resolver, clk.now, testLog())

	if got := s.effectiveCron(s.jobs["a"]); got != "0 */15 * * * *" {
		t.Errorf("a cron = %q, want configured override", got)
	}
	if got := s.effectiveCron(s.jobs["b"]); got != "0 0 3 * * *" {
		t.Errorf("b cron = %q, want default (unset)", got)
	}
	if got := s.effectiveCron(s.jobs["c"]); got != everyMinute {
		t.Errorf("c cron = %q, want default (invalid override guarded)", got)
	}
}

// List joins the registry with state, in registration order.
func TestListJoinsRegistryAndState(t *testing.T) {
	st := newFakeStore()
	clk := &fakeClock{t: time.Unix(1000, 0)}
	reg := NewRegistry().
		Add(Job{Name: "first", Title: "First", DefaultCron: everyMinute, Run: func(context.Context) error { return nil }}).
		Add(Job{Name: "second", Title: "Second", DefaultCron: everyMinute, Run: func(context.Context) error { return nil }})
	s := New(st, reg, nil, clk.now, testLog())
	ctx := context.Background()
	_ = st.UpsertScheduledJob(ctx, store.ScheduledJob{Name: "first", LastResult: "ok", NextRun: clk.now().Add(time.Hour)})

	got, err := s.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "first" || got[1].Name != "second" {
		t.Fatalf("list order/contents wrong: %+v", got)
	}
	if got[0].Title != "First" || got[0].LastResult != "ok" || got[0].Schedule != everyMinute {
		t.Errorf("first status wrong: %+v", got[0])
	}
}

// waitFor polls cond up to ~1s (the scheduler runs jobs in goroutines).
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}
