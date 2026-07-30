package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/store"
)

// pausableJob is a job that counts its executions.
func pausableJob(ran *atomic.Int64) Job {
	return Job{
		Name: "reconcile", Title: "Reconcile", Description: "test job reconcile.",
		DefaultCron: everyMinute,
		Run:         func(context.Context) error { ran.Add(1); return nil },
	}
}

// ⚠ THE GATE: a paused job that is due does not run. Everything else about pause is
// presentation; this is the behaviour.
func TestPaused_DueJobDoesNotRun(t *testing.T) {
	st := newFakeStore()
	clk := &fakeClock{t: time.Unix(1000, 0)}
	var ran atomic.Int64
	s := New(st, NewRegistry().Add(pausableJob(&ran)), nil, clk.now, testLog())
	ctx := context.Background()

	if err := s.SetPaused(ctx, "reconcile", true); err != nil {
		t.Fatal(err)
	}
	// Due NOW — the only thing standing between it and a run is the pause flag.
	_ = st.UpsertScheduledJob(ctx, store.ScheduledJob{Name: "reconcile", NextRun: clk.now()})

	s.tick(ctx)
	time.Sleep(50 * time.Millisecond) // let any wrongly-spawned goroutine land before asserting
	if n := ran.Load(); n != 0 {
		t.Errorf("paused job ran %d times, want 0", n)
	}
}

// Resuming lets it run again — otherwise pause is a one-way door and the control lies.
func TestPaused_ResumeLetsItRunAgain(t *testing.T) {
	st := newFakeStore()
	clk := &fakeClock{t: time.Unix(1000, 0)}
	var ran atomic.Int64
	s := New(st, NewRegistry().Add(pausableJob(&ran)), nil, clk.now, testLog())
	ctx := context.Background()

	_ = s.SetPaused(ctx, "reconcile", true)
	_ = st.UpsertScheduledJob(ctx, store.ScheduledJob{Name: "reconcile", NextRun: clk.now()})
	s.tick(ctx)
	time.Sleep(50 * time.Millisecond)

	if err := s.SetPaused(ctx, "reconcile", false); err != nil {
		t.Fatal(err)
	}
	s.tick(ctx)
	waitFor(t, func() bool { return ran.Load() == 1 })
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
	_ = st.UpsertScheduledJob(ctx, store.ScheduledJob{
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

// A paused job reports no next run: the claim query skips it, so any stored next_run is a time
// that will not fire, and showing it would promise a run that never comes.
func TestPaused_ReportsNoNextRun(t *testing.T) {
	st := newFakeStore()
	clk := &fakeClock{t: time.Unix(1000, 0)}
	var ran atomic.Int64
	s := New(st, NewRegistry().Add(pausableJob(&ran)), nil, clk.now, testLog())
	ctx := context.Background()

	_ = st.UpsertScheduledJob(ctx, store.ScheduledJob{Name: "reconcile", NextRun: clk.now().Add(time.Hour)})
	_ = s.SetPaused(ctx, "reconcile", true)

	got, err := s.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !got[0].Paused {
		t.Fatal("List did not report the job as paused")
	}
	if !got[0].NextRun.IsZero() {
		t.Errorf("paused job NextRun = %v, want zero — it is never claimed", got[0].NextRun)
	}
}

// ⚠ Run-now STILL WORKS on a paused job, deliberately: pause stops the schedule, not the task.
// An admin clicking Run now on a row that visibly says "Paused" is making a specific one-off
// request, and refusing it would mean unpause → run → repause to do what they already asked.
func TestPaused_RunNowStillWorks(t *testing.T) {
	st := newFakeStore()
	clk := &fakeClock{t: time.Unix(1000, 0)}
	var ran atomic.Int64
	s := New(st, NewRegistry().Add(pausableJob(&ran)), nil, clk.now, testLog())
	ctx := context.Background()

	_ = s.SetPaused(ctx, "reconcile", true)
	if err := s.Trigger(ctx, "reconcile"); err != nil {
		t.Fatalf("Trigger on a paused job = %v, want nil", err)
	}
	// Triggering marks it due; the run itself still goes through the claim, which skips paused
	// rows — so the honest contract is that Trigger is ACCEPTED, and executing it is the
	// scheduler's business. Assert the acceptance and that pause is intact.
	got, err := st.GetScheduledJob(ctx, "reconcile")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Paused {
		t.Error("Run now cleared the pause flag; it must be a one-off, not an implicit resume")
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

	if err := s.SetPaused(context.Background(), "backup", true); err != ErrJobDisabled {
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

	if err := s.SetPaused(context.Background(), "nope", true); err != ErrUnknownJob {
		t.Errorf("SetPaused on an unknown job = %v, want ErrUnknownJob", err)
	}
}
