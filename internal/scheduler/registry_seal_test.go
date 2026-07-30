package scheduler_test

import (
	"context"
	"strings"
	"testing"

	"github.com/mantonx/loomarr/internal/scheduler"
)

// ⚠ **THE SEAL (§18.1).** `New` snapshots the registry into a map, so a job added afterwards
// is accepted by the registry, listed by Jobs(), and NEVER SCHEDULED.
//
// That was the live hazard before this: the registry reported 2 jobs while the scheduler ran
// 1, nothing logged, and `GET /v1/jobs` reads the scheduler — so an operator saw a Tasks page
// that looked complete and a job that silently never ran. A boot-time panic naming the job is
// the honest failure.
func TestRegistry_AddAfterNewPanics(t *testing.T) {
	reg := scheduler.NewRegistry()
	reg.Add(scheduler.Job{Name: "early", Title: "Early", Description: "test job early.", DefaultCron: "0 0 * * * *",
		Run: func(context.Context) error { return nil }})
	_ = scheduler.New(nil, reg, nil, nil, nil)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("a job registered AFTER the scheduler was built did not panic — " +
				"it would be listed by Jobs() and never run")
		}
		// The message must name the job: "something panicked at boot" is not actionable.
		if msg, _ := r.(string); !strings.Contains(msg, "late") {
			t.Errorf("panic = %q, want it to name the offending job", r)
		}
	}()
	reg.Add(scheduler.Job{Name: "late", Title: "Late", Description: "test job late.", DefaultCron: "0 0 * * * *",
		Run: func(context.Context) error { return nil }})
}

// The seal must not fire on the NORMAL path — every job registered before New is kept, or
// the guard would be indistinguishable from a registry that rejects everything.
func TestRegistry_AddsBeforeNewAllSurvive(t *testing.T) {
	reg := scheduler.NewRegistry()
	reg.AddAll([]scheduler.Job{
		{Name: "a", Title: "A", Description: "test job a.", DefaultCron: "0 0 * * * *", Run: func(context.Context) error { return nil }},
		{Name: "b", Title: "B", Description: "test job b.", DefaultCron: "0 0 * * * *", Run: func(context.Context) error { return nil }},
	})
	reg.Add(scheduler.Job{Name: "c", Title: "C", Description: "test job c.", DefaultCron: "0 0 * * * *",
		Run: func(context.Context) error { return nil }})
	_ = scheduler.New(nil, reg, nil, nil, nil)

	if got := len(reg.Jobs()); got != 3 {
		t.Errorf("registry has %d jobs, want 3 — the seal must not drop pre-New registrations", got)
	}
}

// ⚠ Title and Description are REQUIRED, enforced at registration rather than left to review.
// The Tasks page is where an operator decides whether to run or pause a task; a row showing a
// bare id, or a title with nothing explaining what it does, cannot support that decision. An
// optional field is one where the next job ships without it and nobody notices until the page
// looks wrong — so the failure is a boot-time panic naming the job, like a duplicate name.
func TestRegistry_RequiresTitleAndDescription(t *testing.T) {
	for _, tc := range []struct {
		name string
		job  scheduler.Job
		want string
	}{
		{"no title", scheduler.Job{Name: "x", Description: "d", DefaultCron: "0 0 * * * *"}, "no Title"},
		{"no description", scheduler.Job{Name: "x", Title: "X", DefaultCron: "0 0 * * * *"}, "no Description"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("Add(%+v) did not panic — the job would reach the Tasks page unreadable", tc.job)
				}
				// The message must name the job; a bare "invalid job" sends the reader hunting.
				if msg, _ := r.(string); !strings.Contains(msg, tc.want) || !strings.Contains(msg, `"x"`) {
					t.Errorf("panic = %q, want it to mention %q and the job name", msg, tc.want)
				}
			}()
			scheduler.NewRegistry().Add(tc.job)
		})
	}
}

// AddAll on an empty/nil slice is a no-op: a subsystem that is not wired returns no jobs
// rather than making the composition root ask whether it has any.
func TestRegistry_AddAllTolerating(t *testing.T) {
	reg := scheduler.NewRegistry()
	reg.AddAll(nil)
	reg.AddAll([]scheduler.Job{})
	if got := len(reg.Jobs()); got != 0 {
		t.Errorf("registry has %d jobs, want 0", got)
	}
}
