package jobs

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/metrics"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// fakeStore is an in-memory ScheduleStore with a controllable clock-independent view; the
// scheduler drives `now` via a fake clock, so we assert due-selection + lease behavior
// deterministically without real time.
type fakeStore struct {
	mu   sync.Mutex
	rows map[string]store.ScheduledJob
}

func newFakeStore() *fakeStore { return &fakeStore{rows: map[string]store.ScheduledJob{}} }

// ⚠ Upsert PRESERVES `paused`, exactly as the SQL does by omitting it from ON CONFLICT DO
// UPDATE. A fake that let the flag through would pass tests the database fails: this call runs
// after every execution, so carrying paused would clear it on the next run of a paused job.
// ⚠ It HONOURS cancellation, and that is what makes the timeout case testable at all. A fake that
// ignored `ctx` is why the record-through-a-dead-context bug was invisible: the write that records
// a job's outcome was made through the very context whose expiry caused that outcome, so a job
// that timed out never persisted its result — and against this double it looked perfect.
func (f *fakeStore) UpsertScheduledJob(ctx context.Context, j store.ScheduledJob) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	j.Paused = f.rows[j.Name].Paused
	f.rows[j.Name] = j
	return nil
}

func (f *fakeStore) SetScheduledJobPaused(_ context.Context, name string, paused bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	row := f.rows[name]
	row.Name, row.Paused = name, paused
	f.rows[name] = row
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

// A registered job with no state row is seeded due-now; executing it records the outcome and
// the next cron tick.
func TestReconcileSeedsAndExecuteRecords(t *testing.T) {
	st := newFakeStore()
	clk := &fakeClock{t: time.Unix(1000, 0)}
	var ran atomic.Int64
	reg := NewRegistry().Add(Job{
		Name: "a", Group: GroupSystem, Title: "A", Description: "test job a.", DefaultCron: everyMinute, Run: func(context.Context) error { ran.Add(1); return nil },
	})
	s := New(st, reg, nil, clk.now, testLog())
	ctx := context.Background()

	s.reconcileRegistry(ctx) // seeds "a" due-now
	if j, _ := st.GetScheduledJob(ctx, "a"); j.NextRun != clk.now() {
		t.Fatalf("seeded next_run = %v, want now", j.NextRun)
	}
	s.execute(ctx, s.jobs["a"], false)
	if ran.Load() != 1 {
		t.Fatalf("job ran %d times, want 1", ran.Load())
	}

	// After running, next_run is the next cron tick — strictly after now, within a minute.
	j, _ := st.GetScheduledJob(ctx, "a")
	if got := j.NextRun.Sub(clk.now()); got <= 0 || got > time.Minute {
		t.Errorf("post-run next_run offset = %v, want (0, 1m] from the every-minute cron", got)
	}
	if j.LastResult != "ok" {
		t.Errorf("last_result = %q, want ok", j.LastResult)
	}
}

func TestExecutePublishesJobLifecycleToGenerationMetrics(t *testing.T) {
	st := newFakeStore()
	clk := &fakeClock{t: time.Unix(1000, 0)}
	reg := NewRegistry().Add(Job{
		Name: "health-check", Group: GroupSystem, Title: "Check health",
		Description: "Checks dependencies.", DefaultCron: everyMinute,
		Run: func(context.Context) error { return nil },
	})
	recorder := metrics.New(metrics.Options{})
	scheduler := New(st, reg, nil, clk.now, testLog()).WithObserver(recorder)
	scheduler.execute(t.Context(), scheduler.jobs["health-check"], false)

	wants := []string{
		`loomarr_scheduler_job_executions_total{job="health-check",result="success",trigger="scheduled"} 1`,
		`loomarr_scheduler_job_duration_seconds_count{job="health-check"} 1`,
		`loomarr_scheduler_jobs_running{job="health-check"} 0`,
		`loomarr_scheduler_job_last_success_timestamp_seconds{job="health-check"} 1000`,
	}
	scrape := httptest.NewRecorder()
	recorder.Handler().ServeHTTP(scrape, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := scrape.Body.String()
	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Errorf("generation scrape does not contain %q", want)
		}
	}
}

func TestExecutePublishesBoundedFailureKinds(t *testing.T) {
	cases := []struct {
		name string
		run  func(context.Context) error
		ctx  func() context.Context
	}{
		{name: "error", run: func(context.Context) error { return errors.New("private failure") }, ctx: context.Background},
		{name: "panic", run: func(context.Context) error { panic("private panic") }, ctx: context.Background},
		{name: "timeout", run: func(context.Context) error { return nil }, ctx: func() context.Context {
			ctx, cancel := context.WithDeadline(context.Background(), time.Unix(1, 0))
			defer cancel()
			return ctx
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			job := Job{Name: "failure-check", Group: GroupSystem, Title: "Check failure",
				Description: "Exercises one failure kind.", DefaultCron: everyMinute, Run: tc.run}
			recorder := metrics.New(metrics.Options{})
			scheduler := New(newFakeStore(), NewRegistry().Add(job), nil,
				func() time.Time { return time.Unix(1000, 0) }, testLog()).WithObserver(recorder)
			scheduler.execute(tc.ctx(), job, true)

			scrape := httptest.NewRecorder()
			recorder.Handler().ServeHTTP(scrape, httptest.NewRequest(http.MethodGet, "/metrics", nil))
			want := `loomarr_scheduler_job_executions_total{job="failure-check",result="` + tc.name + `",trigger="manual"} 1`
			if !strings.Contains(scrape.Body.String(), want) {
				t.Errorf("generation scrape does not contain %q", want)
			}
			if strings.Contains(scrape.Body.String(), "private failure") || strings.Contains(scrape.Body.String(), "private panic") {
				t.Fatal("generation scrape leaked a job error")
			}
		})
	}
}

// An erroring job records last_result=error and never wedges the others.
func TestErroringJobRecordsErrorAndIsolated(t *testing.T) {
	st := newFakeStore()
	clk := &fakeClock{t: time.Unix(1000, 0)}
	var okRan atomic.Int64
	reg := NewRegistry().
		Add(Job{Name: "bad", Group: GroupSystem, Title: "Bad", Description: "test job bad.", DefaultCron: everyMinute, Run: func(context.Context) error { return errors.New("boom") }}).
		Add(Job{Name: "good", Group: GroupSystem, Title: "Good", Description: "test job good.", DefaultCron: everyMinute, Run: func(context.Context) error { okRan.Add(1); return nil }})
	s := New(st, reg, nil, clk.now, testLog())
	ctx := context.Background()

	s.execute(ctx, s.jobs["bad"], false)
	s.execute(ctx, s.jobs["good"], false)
	bad, _ := st.GetScheduledJob(ctx, "bad")
	if bad.LastResult != "error" || bad.LastError == "" {
		t.Errorf("erroring job recorded result %q error %q, want error with text", bad.LastResult, bad.LastError)
	}
	if okRan.Load() != 1 {
		t.Errorf("good job ran %d times, want 1", okRan.Load())
	}
}

// ⚠ **A job that runs out of time must still RECORD that it did** (§10 V51g). The result write
// used the same context the job ran under, so a job killed by its own deadline failed to persist
// `last_result`, `last_error` and `next_run` — the Tasks page kept showing the previous outcome
// and nothing anywhere said the job had been timing out.
//
// Observed live: `filler-pipeline` logged "scheduled job failed" immediately followed by
// "scheduler: record job result … context deadline exceeded", once every two minutes.
func TestTimedOutJobStillRecordsItsResult(t *testing.T) {
	st := newFakeStore()
	clk := &fakeClock{t: time.Unix(1000, 0)}
	// The job cancels the context it was handed, then reports it — what an exec'd ffmpeg or
	// whisper does when the deadline lands mid-run.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reg := NewRegistry().Add(Job{
		Name: "slow", Group: GroupSystem, Title: "Slow", Description: "test job slow.", DefaultCron: everyMinute,
		Run: func(c context.Context) error { cancel(); return c.Err() },
	})
	s := New(st, reg, nil, clk.now, testLog())

	s.execute(ctx, s.jobs["slow"], false)

	j, _ := st.GetScheduledJob(context.Background(), "slow")
	if j.LastResult != "error" || j.LastError == "" {
		t.Error("a job killed by its deadline recorded no error — the record died with the context")
	}
	if j.NextRun.IsZero() {
		t.Error("no next run scheduled; the job's own timeout erased its schedule")
	}
}

// A panicking job is contained (recorded as error), not a scheduler crash.
func TestPanicIsContained(t *testing.T) {
	st := newFakeStore()
	clk := &fakeClock{t: time.Unix(1000, 0)}
	reg := NewRegistry().Add(Job{Name: "panics", Group: GroupSystem, Title: "Panics", Description: "test job panics.", DefaultCron: everyMinute, Run: func(context.Context) error { panic("kaboom") }})
	s := New(st, reg, nil, clk.now, testLog())
	ctx := context.Background()

	s.execute(ctx, s.jobs["panics"], false)
	if j, _ := st.GetScheduledJob(ctx, "panics"); j.LastResult != "error" {
		t.Errorf("panicking job recorded result %q, want error", j.LastResult)
	}
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
		Add(Job{Name: "a", Group: GroupSystem, Title: "A", Description: "test job a.", DefaultCron: everyMinute, ScheduleKey: "job.a.schedule", Run: func(context.Context) error { return nil }}).
		Add(Job{Name: "b", Group: GroupSystem, Title: "B", Description: "test job b.", DefaultCron: "0 0 3 * * *", ScheduleKey: "job.b.schedule", Run: func(context.Context) error { return nil }}).
		Add(Job{Name: "c", Group: GroupSystem, Title: "C", Description: "test job c.", DefaultCron: everyMinute, ScheduleKey: "job.c.schedule", Run: func(context.Context) error { return nil }})
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
		Add(Job{Name: "first", Group: GroupAcquisitions, Description: "test job first.", Title: "First", DefaultCron: everyMinute, Run: func(context.Context) error { return nil }}).
		Add(Job{Name: "second", Group: GroupSystem, Description: "test job second.", Title: "Second", DefaultCron: everyMinute, Run: func(context.Context) error { return nil }})
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
