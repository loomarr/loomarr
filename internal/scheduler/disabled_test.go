package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/store"
)

// pgReason is the shape of a real DisabledReason (backup on Postgres).
const pgReason = "Loomarr does not back up PostgreSQL itself — use pg_dump on your usual schedule."

// disabledJob builds a job that records whether it was ever executed. A disabled job's Run
// is nil in production (nothing should call it); here it is a counter so a regression that
// DOES call it fails as a wrong count rather than a nil-pointer panic recovered into an
// ordinary job error.
func disabledJob(ran *atomic.Int64) Job {
	return Job{
		Name: "backup", Group: GroupBackup, Title: "Back up the database", Description: "test job backup.", DefaultCron: everyMinute,
		DisabledReason: pgReason,
		Run:            func(context.Context) error { ran.Add(1); return nil },
	}
}

// ⚠ THE GATE: a disabled job is listed — that is the entire point. An omitted row is
// indistinguishable, from the Tasks page alone, from a job that runs fine and has never
// failed, and for backup that ambiguity means believing you are covered when you are not.
func TestDisabledJob_IsListedWithItsReason(t *testing.T) {
	st := newFakeStore()
	clk := &fakeClock{t: time.Unix(1000, 0)}
	var ran atomic.Int64
	reg := NewRegistry().
		Add(Job{Name: "reconcile", Group: GroupSystem, Description: "test job reconcile.", Title: "Reconcile", DefaultCron: everyMinute, Run: func(context.Context) error { return nil }}).
		Add(disabledJob(&ran))
	s := New(st, reg, nil, clk.now, testLog())

	got, err := s.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("List returned %d jobs, want both: %+v", len(got), got)
	}
	if got[1].Name != "backup" || got[1].DisabledReason != pgReason {
		t.Errorf("disabled job status = %+v, want it listed carrying its reason", got[1])
	}
	// An enabled job must not acquire a reason — otherwise the flag says nothing.
	if got[0].DisabledReason != "" {
		t.Errorf("enabled job carries DisabledReason %q, want empty", got[0].DisabledReason)
	}
}

// A disabled job has no next run. Rendering one would promise a run that never happens.
func TestDisabledJob_ReportsNoNextRun(t *testing.T) {
	st := newFakeStore()
	clk := &fakeClock{t: time.Unix(1000, 0)}
	var ran atomic.Int64
	reg := NewRegistry().Add(disabledJob(&ran))
	s := New(st, reg, nil, clk.now, testLog())
	ctx := context.Background()

	// ⚠ A LEFTOVER row from a boot where the job WAS enabled — exactly what a
	// SQLite → Postgres migration (§18, V11) leaves behind: same install, same row, a
	// backend that can no longer run it.
	_ = st.UpsertScheduledJob(ctx, store.ScheduledJob{
		Name: "backup", LastResult: "ok", NextRun: clk.now().Add(time.Hour),
	})

	got, err := s.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !got[0].NextRun.IsZero() {
		t.Errorf("disabled job NextRun = %v, want zero — it is never claimed", got[0].NextRun)
	}
}

// A disabled job is never seeded a state row, so it can never become due.
func TestDisabledJob_IsNotSeeded(t *testing.T) {
	st := newFakeStore()
	clk := &fakeClock{t: time.Unix(1000, 0)}
	var ran atomic.Int64
	reg := NewRegistry().Add(disabledJob(&ran))
	s := New(st, reg, nil, clk.now, testLog())
	ctx := context.Background()

	s.reconcileRegistry(ctx)

	if _, err := st.GetScheduledJob(ctx, "backup"); err == nil {
		t.Error("a disabled job was seeded a state row; it can then become due and be claimed")
	}
}

// ⚠ THE SAFETY GATE: even with a due row already present, the loop must not run it. This is
// the SQLite → Postgres case — reconcileRegistry declines to seed, but the row is already
// there from before the migration.
func TestDisabledJob_IsNeverClaimedEvenWithADueRow(t *testing.T) {
	st := newFakeStore()
	clk := &fakeClock{t: time.Unix(1000, 0)}
	var ran atomic.Int64
	reg := NewRegistry().Add(disabledJob(&ran))
	s := New(st, reg, nil, clk.now, testLog())
	ctx := context.Background()

	// Due NOW, as a pre-migration boot would have left it.
	_ = st.UpsertScheduledJob(ctx, store.ScheduledJob{Name: "backup", NextRun: clk.now()})

	s.tick(ctx)

	// Give any wrongly-spawned goroutine time to land before asserting it did not.
	time.Sleep(50 * time.Millisecond)
	if n := ran.Load(); n != 0 {
		t.Errorf("disabled job ran %d times, want 0 — a backend that cannot back up must not try", n)
	}
	if j, err := st.GetScheduledJob(ctx, "backup"); err == nil && j.LastResult != "" {
		t.Errorf("disabled job recorded last_result %q, want none — it never ran", j.LastResult)
	}
}

// ⚠ Run-now is refused BY THE SERVER. The Tasks page hides the button, but that is a hint —
// anything a client can be shown, a client can skip.
func TestDisabledJob_TriggerIsRefused(t *testing.T) {
	st := newFakeStore()
	clk := &fakeClock{t: time.Unix(1000, 0)}
	var ran atomic.Int64
	reg := NewRegistry().Add(disabledJob(&ran))
	s := New(st, reg, nil, clk.now, testLog())
	ctx := context.Background()

	if err := s.Trigger(ctx, "backup"); err != ErrJobDisabled {
		t.Fatalf("Trigger of a disabled job = %v, want ErrJobDisabled", err)
	}
	// The refusal must not have marked it due as a side effect.
	if j, err := st.GetScheduledJob(ctx, "backup"); err == nil && !j.NextRun.IsZero() {
		t.Errorf("a refused Trigger still wrote next_run %v", j.NextRun)
	}
	s.tick(ctx)
	time.Sleep(50 * time.Millisecond)
	if n := ran.Load(); n != 0 {
		t.Errorf("job ran %d times after a refused Trigger, want 0", n)
	}
}

// ErrJobDisabled and ErrUnknownJob are different answers: one job is on the admin's screen,
// the other does not exist. Collapsing them would send an admin hunting for a name they can
// see.
func TestDisabledJob_UnknownStillReportsUnknown(t *testing.T) {
	st := newFakeStore()
	clk := &fakeClock{t: time.Unix(1000, 0)}
	var ran atomic.Int64
	s := New(st, NewRegistry().Add(disabledJob(&ran)), nil, clk.now, testLog())

	if err := s.Trigger(context.Background(), "nope"); err != ErrUnknownJob {
		t.Errorf("Trigger of an unregistered job = %v, want ErrUnknownJob", err)
	}
}

// An enabled job in the same registry is unaffected — the disabled one must not suppress
// its neighbours.
func TestDisabledJob_DoesNotAffectEnabledJobs(t *testing.T) {
	st := newFakeStore()
	clk := &fakeClock{t: time.Unix(1000, 0)}
	var disabledRan, enabledRan atomic.Int64
	reg := NewRegistry().
		Add(disabledJob(&disabledRan)).
		Add(Job{
			Name: "reconcile", Group: GroupSystem, Title: "Reconcile", Description: "test job reconcile.", DefaultCron: everyMinute,
			Run: func(context.Context) error { enabledRan.Add(1); return nil },
		})
	s := New(st, reg, nil, clk.now, testLog())
	ctx := context.Background()

	s.reconcileRegistry(ctx)
	s.tick(ctx)

	waitFor(t, func() bool {
		j, err := st.GetScheduledJob(ctx, "reconcile")
		return err == nil && j.LastResult != ""
	})
	if enabledRan.Load() != 1 {
		t.Errorf("enabled job ran %d times, want 1", enabledRan.Load())
	}
	if disabledRan.Load() != 0 {
		t.Errorf("disabled job ran %d times, want 0", disabledRan.Load())
	}
}
