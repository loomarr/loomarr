package scheduler

import (
	"errors"
	"fmt"
)

// ErrUnknownJob is returned by Trigger when the requested job name isn't registered.
var ErrUnknownJob = errors.New("unknown job")

// ErrJobDisabled is returned by Trigger for a job this build/backend cannot run (e.g.
// `backup` on Postgres). Distinct from ErrUnknownJob: the job exists and is listed, it
// just cannot be run here — which is a 409, not a 404.
var ErrJobDisabled = errors.New("job is not available on this backend")

// Registry collects the code-defined jobs before the scheduler is built. The composition
// root (internal/app) collects them: reg.AddAll(reconcile.Jobs(...)).AddAll(…). Order is
// preserved so the Tasks page lists jobs in a stable, intentional order.
//
// ⚠ **A job that owns work should declare its own Job.** The cron default, the settings key
// and the body belong next to the code they schedule, not in the composition root — see the
// `Jobs()` constructors on the subsystem packages. The root's remaining task is to decide
// WHICH sets apply to this build, which is genuinely its job (SQLite vs Postgres backup).
type Registry struct {
	jobs   []Job
	sealed bool
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{} }

// Add appends a job. Returns the registry for chaining. Panics on a duplicate or empty name —
// a programming error, caught at boot rather than silently shadowing a job.
//
// ⚠ **Panics if the registry is SEALED**, and that is the point of Seal existing. `New`
// snapshots the jobs into a map, so an Add afterwards used to be accepted here and then
// silently never scheduled — while `Jobs()` still listed it, so `GET /v1/jobs` showed a
// task that could never run. Verified: registry reported 2 jobs, scheduler ran 1, nothing
// logged. A boot-time panic naming the job is the honest failure.
func (r *Registry) Add(j Job) *Registry {
	if r.sealed {
		panic(fmt.Sprintf("scheduler: job %q registered after the scheduler was built — "+
			"New() snapshots the registry, so this job would never run", j.Name))
	}
	if j.Name == "" {
		panic("scheduler: job with empty name")
	}
	// ⚠ Group, Title and Description are REQUIRED, enforced here rather than left to review.
	// An operator on the Tasks page deciding whether to run or pause a task needs to know
	// what it does; a nullable field is one where the next job ships without one and the row
	// reads as a bare identifier. Boot-time panic naming the job is the honest failure —
	// the same posture as a duplicate name.
	if !j.Group.valid() {
		panic(fmt.Sprintf("scheduler: job %q has invalid Group %q", j.Name, j.Group))
	}
	if j.Title == "" {
		panic(fmt.Sprintf("scheduler: job %q has no Title — the Tasks page would show a bare id", j.Name))
	}
	if j.Description == "" {
		panic(fmt.Sprintf("scheduler: job %q has no Description — an operator cannot judge "+
			"whether to run or pause a task they cannot read", j.Name))
	}
	for _, existing := range r.jobs {
		if existing.Name == j.Name {
			panic(fmt.Sprintf("scheduler: duplicate job name %q", j.Name))
		}
	}
	r.jobs = append(r.jobs, j)
	return r
}

// AddAll appends every job in js, in order. The shape a subsystem's `Jobs()` returns.
//
// A nil or empty slice is fine and means "this build registers none" — a subsystem that is
// not wired (no store, no provider) returns nothing rather than making the root ask.
func (r *Registry) AddAll(js []Job) *Registry {
	for _, j := range js {
		r.Add(j)
	}
	return r
}

// Jobs returns the registered jobs in registration order.
func (r *Registry) Jobs() []Job { return append([]Job(nil), r.jobs...) }

// seal closes the registry to further Adds. Called by New, because that is the moment the
// job set stops being able to change: anything added later is invisible to the scheduler.
func (r *Registry) seal() { r.sealed = true }

// panicError wraps a recovered panic value so a job that panics becomes a normal job error
// (last_result="error") instead of crashing the scheduler goroutine.
type panicError struct{ v any }

func (e *panicError) Error() string { return fmt.Sprintf("panic: %v", e.v) }
