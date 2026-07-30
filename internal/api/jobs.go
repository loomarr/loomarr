package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

// JobService backs the background-job scheduler routes (§18.1). Satisfied by an adapter over
// *scheduler.Scheduler in the composition root (the api package doesn't import scheduler).
type JobService interface {
	// List returns the current status of every registered job, in a stable order.
	List(ctx context.Context) ([]JobView, error)
	// Trigger forces a job to run off-cycle ("Run now"). Returns ErrJobNotFound for an
	// unknown name.
	Trigger(ctx context.Context, name string) error
	// SetPaused pauses or resumes a job's SCHEDULE (§18.1). A paused job stays listed and
	// keeps its cron, but never runs when due. ErrJobNotFound for an unknown name;
	// ErrJobDisabled for one this backend cannot run at all.
	SetPaused(ctx context.Context, name string, paused bool) error
}

// ErrJobNotFound is returned by JobService.Trigger for an unregistered job name.
var ErrJobNotFound = huma.Error404NotFound("no such job") // sentinel; handler re-wraps friendly

// ErrJobDisabled is returned by JobService.Trigger for a job this backend cannot run (e.g.
// `backup` on Postgres). A 409 rather than a 404: the job exists and is listed, it just
// cannot run here — a 404 would send an admin looking for a name that is on their screen.
var ErrJobDisabled = huma.Error409Conflict("job disabled") // sentinel; handler re-wraps friendly

// JobView is the API/UI read model for one scheduled job (§18.1). All timing is BE-authored;
// the FE renders these values verbatim (it does not compute countdowns). Schedule is a cron
// expression; scheduleKey is the settings key the "Modify Job" modal PATCHes to change it.
type JobView struct {
	Name        string    `json:"name" doc:"Stable job id (also the run/trigger key)"`
	Title       string    `json:"title" doc:"Human label for the Tasks page"`
	Description string    `json:"description" doc:"One plain sentence saying what running this job does"`
	Schedule    string    `json:"schedule" doc:"Effective cron expression (settings override or default)"`
	ScheduleKey string    `json:"scheduleKey" doc:"Settings key to PATCH to change the schedule"`
	LastRun     time.Time `json:"lastRun,omitempty" doc:"When the job last started (zero = never)"`
	LastResult  string    `json:"lastResult" doc:"ok | error | '' (never run)"`
	LastError   string    `json:"lastError,omitempty" doc:"Error detail when lastResult is error"`
	NextRun     time.Time `json:"nextRun,omitempty" doc:"When the job is next due"`
	Running     bool      `json:"running" doc:"True while the job is currently executing"`
	Paused      bool      `json:"paused" doc:"True when the operator paused the schedule; the job stays listed but never runs when due"`
	// DisabledReason is non-empty when this backend cannot run the job at all — it is
	// listed so its absence is never inferred, but it never runs and Run-now 409s.
	DisabledReason string `json:"disabledReason,omitempty" doc:"Why this job cannot run here; empty when it can"`
}

func (s *Server) registerJobs(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "jobs-list", Method: http.MethodGet, Path: "/v1/jobs",
		Summary:     "List background jobs",
		Description: "Admin only. The scheduler's named jobs with their effective interval, last/next run, result, and running state (§18.1). All timing is server-authored.",
		Tags:        []string{"jobs"},
	}, RoleAdmin), s.jobsList)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "jobs-run", Method: http.MethodPost, Path: "/v1/jobs/{name}/run",
		Summary:       "Run a job now",
		Description:   "Admin only. Triggers a scheduled job off-cycle (§18.1). Idempotent-ish: a job already due/running is not double-run (the scheduler lease guards it). A job this backend cannot run (a non-empty disabledReason) returns 409.",
		Tags:          []string{"jobs"},
		DefaultStatus: http.StatusAccepted,
	}, RoleAdmin), s.jobsRun)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "jobs-pause", Method: http.MethodPost, Path: "/v1/jobs/{name}/pause",
		Summary: "Pause or resume a job",
		Description: "Admin only. Pausing stops a job running on its schedule (§18.1); it stays listed, keeps its cron, and reports no next run. " +
			"Run now still works on a paused job — pause stops the schedule, not the task. " +
			"A job this backend cannot run at all (a non-empty disabledReason) returns 409: that is an environment fact, not an operator preference.",
		Tags: []string{"jobs"},
	}, RoleAdmin), s.jobsPause)
}

type jobsPauseInput struct {
	Name string `path:"name" doc:"Job name (the stable id from GET /v1/jobs)"`
	Body struct {
		Paused bool `json:"paused" doc:"true to pause the schedule, false to resume"`
	}
}

func (s *Server) jobsPause(ctx context.Context, in *jobsPauseInput) (*struct{}, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.jobs == nil {
		return nil, errNotImplemented("Scheduler unavailable", "The job scheduler isn't running (no store configured).")
	}
	if err := s.jobs.SetPaused(ctx, in.Name, in.Body.Paused); err != nil {
		if err == ErrJobNotFound {
			return nil, errNotFound("Job not found", "There's no scheduled job with that name.")
		}
		if err == ErrJobDisabled {
			return nil, huma.Error409Conflict("This job can't run on this backend, so it can't be paused or resumed.")
		}
		return nil, apiErrWithCause(http.StatusBadGateway, "Couldn't change the job",
			"Loomarr couldn't update that job. Try again in a moment.", err)
	}
	return &struct{}{}, nil
}

type jobsListOutput struct {
	Body struct {
		Jobs []JobView `json:"jobs"`
	}
}

func (s *Server) jobsList(ctx context.Context, _ *struct{}) (*jobsListOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.jobs == nil {
		return nil, errNotImplemented("Scheduler unavailable", "The job scheduler isn't running (no store configured).")
	}
	list, err := s.jobs.List(ctx)
	if err != nil {
		return nil, apiErrWithCause(http.StatusBadGateway, "Couldn't list jobs", "Loomarr couldn't read the scheduled jobs. Try again in a moment.", err)
	}
	out := &jobsListOutput{}
	out.Body.Jobs = list
	return out, nil
}

type jobsRunInput struct {
	Name string `path:"name" doc:"The job's stable id (from GET /v1/jobs)"`
}

func (s *Server) jobsRun(ctx context.Context, in *jobsRunInput) (*struct{}, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.jobs == nil {
		return nil, errNotImplemented("Scheduler unavailable", "The job scheduler isn't running (no store configured).")
	}
	if err := s.jobs.Trigger(ctx, in.Name); err != nil {
		if err == ErrJobNotFound {
			return nil, errNotFound("Job not found", "There's no scheduled job with that name.")
		}
		if err == ErrJobDisabled {
			return nil, huma.Error409Conflict("This job isn't available on this backend, so it can't be run.")
		}
		return nil, apiErrWithCause(http.StatusBadGateway, "Couldn't run the job", "Loomarr couldn't trigger that job. Try again in a moment.", err)
	}
	return &struct{}{}, nil
}
