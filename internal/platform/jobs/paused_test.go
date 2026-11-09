package jobs

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/jobstore"
	"github.com/riverqueue/river"
)

// pausableJob is a job that counts its executions.
func pausableJob(ran *atomic.Int64) Job {
	return Job{
		Name: "reconcile", Group: GroupSystem, Title: "Reconcile", Description: "test job reconcile.",
		DefaultCron: everyMinute,
		Run:         func(context.Context) error { ran.Add(1); return nil },
	}
}

// ⚠ THE GATE: a paused job's scheduled run does not execute, and resuming lets it run again —
// otherwise pause is a one-way door and the control lies. (Run-now on a paused job is covered by
// TestRiverWorkerManualRunBypassesPause.)
func TestPaused_ScheduledRunSkippedUntilResumed(t *testing.T) {
	st := newFakeStore()
	clk := &fakeClock{t: time.Unix(1000, 0)}
	var ran atomic.Int64
	s := New(st, NewRegistry().Add(pausableJob(&ran)), nil, clk.now, testLog())
	ctx := context.Background()
	w := riverWorker{s: s}
	scheduled := &river.Job[jobArgs]{Args: jobArgs{Name: "reconcile"}}

	if err := s.SetPaused(ctx, "reconcile", true); err != nil {
		t.Fatal(err)
	}
	if err := w.Work(ctx, scheduled); err != nil {
		t.Fatal(err)
	}
	if n := ran.Load(); n != 0 {
		t.Errorf("paused job ran %d times, want 0", n)
	}

	if err := s.SetPaused(ctx, "reconcile", false); err != nil {
		t.Fatal(err)
	}
	if err := w.Work(ctx, scheduled); err != nil {
		t.Fatal(err)
	}
	if n := ran.Load(); n != 1 {
		t.Errorf("resumed job ran %d times, want 1", n)
	}
}

// pauseReadFailsStore fails the pause-flag read with a database error.
type pauseReadFailsStore struct{ *fakeStore }

func (pauseReadFailsStore) GetScheduledJob(context.Context, string) (jobstore.ScheduledJob, error) {
	return jobstore.ScheduledJob{}, errors.New("database is locked")
}

// A failed pause-flag read fails open (the run proceeds: one skipped run is not worth silencing every
// job on a database hiccup) but says so; a job that has simply never run has no row and says nothing.
func TestPaused_ReadErrorFailsOpenAndIsLogged(t *testing.T) {
	var logs bytes.Buffer
	clk := &fakeClock{t: time.Unix(1000, 0)}
	var ran atomic.Int64
	s := New(pauseReadFailsStore{newFakeStore()}, NewRegistry().Add(pausableJob(&ran)), nil, clk.now,
		slog.New(slog.NewTextHandler(&logs, nil)))
	w := riverWorker{s: s}

	if err := w.Work(context.Background(), &river.Job[jobArgs]{Args: jobArgs{Name: "reconcile"}}); err != nil {
		t.Fatal(err)
	}
	if ran.Load() != 1 {
		t.Errorf("job ran %d times after a pause-flag read error, want 1 (fail open)", ran.Load())
	}
	if !strings.Contains(logs.String(), "level=WARN") || !strings.Contains(logs.String(), "database is locked") {
		t.Errorf("the pause-flag read error was not logged at Warn:\n%s", logs.String())
	}

	// No row yet is the ordinary first run: nothing to report.
	logs.Reset()
	fresh := New(newFakeStore(), NewRegistry().Add(pausableJob(&ran)), nil, clk.now, slog.New(slog.NewTextHandler(&logs, nil)))
	if err := (&riverWorker{s: fresh}).Work(context.Background(), &river.Job[jobArgs]{Args: jobArgs{Name: "reconcile"}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logs.String(), "pause") {
		t.Errorf("a never-run job logged about its pause flag:\n%s", logs.String())
	}
}

// ⚠ **The trap this design exists to avoid.** UpsertScheduledJob runs after EVERY execution.
// If `paused` rode along in it (the obvious way to write the column), the next run of any
// other job — or a state write for this one — would silently clear a pause the operator set.
// Asserted as an outcome rather than by inspecting SQL, so it holds for the fake and the real
// store alike.
func TestPaused_SurvivesAStateWrite(t *testing.T) {
	st := newFakeStore()
	clk := &fakeClock{t: time.Unix(1000, 0)}
	var ran atomic.Int64
	s := New(st, NewRegistry().Add(pausableJob(&ran)), nil, clk.now, testLog())
	ctx := context.Background()

	_ = s.SetPaused(ctx, "reconcile", true)
	// A perfectly ordinary state write, the shape the scheduler makes after every run.
	_ = st.UpsertScheduledJob(ctx, jobstore.ScheduledJob{
		Name: "reconcile", LastResult: "ok", LastRun: clk.now(), NextRun: clk.now().Add(time.Hour),
	})

	got, err := st.GetScheduledJob(ctx, "reconcile")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Paused {
		t.Error("a routine state write cleared the pause flag — paused must not ride in " +
			"UpsertScheduledJob's DO UPDATE list, or every run un-pauses the job")
	}
}

// A paused job reports no next run: the worker skips its scheduled runs, so any stored next_run is a time
// that will not fire, and showing it would promise a run that never comes.
func TestPaused_ReportsNoNextRun(t *testing.T) {
	st := newFakeStore()
	clk := &fakeClock{t: time.Unix(1000, 0)}
	var ran atomic.Int64
	s := New(st, NewRegistry().Add(pausableJob(&ran)), nil, clk.now, testLog())
	ctx := context.Background()

	_ = st.UpsertScheduledJob(ctx, jobstore.ScheduledJob{Name: "reconcile", NextRun: clk.now().Add(time.Hour)})
	_ = s.SetPaused(ctx, "reconcile", true)

	got, err := s.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !got[0].Paused {
		t.Fatal("List did not report the job as paused")
	}
	if !got[0].NextRun.IsZero() {
		t.Errorf("paused job NextRun = %v, want zero — its scheduled runs are skipped", got[0].NextRun)
	}
}

// A disabled job cannot be paused: pausing states a preference about a job that would
// otherwise run, and this one cannot run at all. Accepting it would leave the row in two
// contradictory states and imply Resume would make it work.
func TestPaused_DisabledJobRefuses(t *testing.T) {
	st := newFakeStore()
	clk := &fakeClock{t: time.Unix(1000, 0)}
	var ran atomic.Int64
	s := New(st, NewRegistry().Add(disabledJob(&ran)), nil, clk.now, testLog())

	if err := s.SetPaused(context.Background(), "backup", true); !errors.Is(err, ErrJobDisabled) {
		t.Errorf("SetPaused on a disabled job = %v, want ErrJobDisabled", err)
	}
}

// An unregistered name is unknown, not silently accepted — otherwise a typo writes a pause row
// for a job that does not exist.
func TestPaused_UnknownJobRefuses(t *testing.T) {
	st := newFakeStore()
	clk := &fakeClock{t: time.Unix(1000, 0)}
	var ran atomic.Int64
	s := New(st, NewRegistry().Add(pausableJob(&ran)), nil, clk.now, testLog())

	if err := s.SetPaused(context.Background(), "nope", true); !errors.Is(err, ErrUnknownJob) {
		t.Errorf("SetPaused on an unknown job = %v, want ErrUnknownJob", err)
	}
}
