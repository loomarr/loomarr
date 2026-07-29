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
// root (internal/app) builds it: reg.Add(Job{...}).Add(Job{...}). Order is preserved so the
// Tasks page lists jobs in a stable, intentional order.
type Registry struct{ jobs []Job }

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{} }

// Add appends a job. Returns the registry for chaining. Panics on a duplicate or empty name —
// a programming error, caught at boot rather than silently shadowing a job.
func (r *Registry) Add(j Job) *Registry {
	if j.Name == "" {
		panic("scheduler: job with empty name")
	}
	for _, existing := range r.jobs {
		if existing.Name == j.Name {
			panic(fmt.Sprintf("scheduler: duplicate job name %q", j.Name))
		}
	}
	r.jobs = append(r.jobs, j)
	return r
}

// Jobs returns the registered jobs in registration order.
func (r *Registry) Jobs() []Job { return append([]Job(nil), r.jobs...) }

// panicError wraps a recovered panic value so a job that panics becomes a normal job error
// (last_result="error") instead of crashing the scheduler goroutine.
type panicError struct{ v any }

func (e *panicError) Error() string { return fmt.Sprintf("panic: %v", e.v) }
