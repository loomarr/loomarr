// Package scheduler runs Loomarr's recurring background work as named, tunable,
// on-demand JOBS (design §18.1) — the model Sonarr/Radarr/Overseerr expose as
// System → Tasks. One heartbeat loop replaces the previous scattering of bespoke
// time.Ticker goroutines: a code-defined registry of jobs, each with a default
// interval overridable via a settings key, all claimable through the same
// lease as titles/channels so a job never double-runs across replicas, and all
// triggerable on demand ("Run now").
package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/adhocore/gronx"

	"github.com/mantonx/loomarr/internal/store"
)

// Job is one named, recurring unit of work. The Run func is code; the SCHEDULE is a cron
// expression (6-field, seconds-leading) resolved from ScheduleKey (a settings key) falling
// back to DefaultCron — matching how Sonarr/Overseerr expose per-task schedules.
type Job struct {
	Name  string // stable id, e.g. "reconcile" — the API/UI key
	Title string // human label for the Tasks page, e.g. "Reconcile acquisitions"
	// Description is one plain sentence saying what running this job actually DOES, in the
	// operator's terms rather than the code's ("Checks in-flight downloads and moves finished
	// ones into your library"). Rendered under the title on the Tasks page.
	//
	// ⚠ REQUIRED — the registry panics without it. Someone deciding whether to run or pause a
	// task needs to know what it does, and an optional field is one where the next job ships
	// without one and its row reads as a bare identifier.
	Description string
	DefaultCron string // built-in cron schedule, e.g. "0 */5 * * * *"
	ScheduleKey string // settings key, e.g. "job.reconcile.schedule"; "" ⇒ always DefaultCron
	Run         func(ctx context.Context) error

	// DisabledReason, when non-empty, means this build/backend CANNOT run this job. It is
	// listed on the Tasks page carrying this reason, is never scheduled or claimed, and
	// refuses Trigger.
	//
	// ⚠ This is not an operator "off" switch and there is no enable control: it states a
	// fact about the environment (backup needs SQLite; WriteBackup does not exist on
	// Postgres), which no amount of clicking changes.
	//
	// It exists because the alternative — not registering the job at all — is also a
	// claim. An absent row is indistinguishable, from the Tasks page alone, from a job
	// that runs fine and has simply never failed; for backup that ambiguity means an
	// operator believing they are covered when they are not.
	DisabledReason string
}

// Disabled reports whether the job cannot run in this environment.
func (j Job) Disabled() bool { return j.DisabledReason != "" }

// JobStatus is the read model the Tasks API/UI renders.
type JobStatus struct {
	Name        string    `json:"name"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Schedule    string    `json:"schedule"`    // effective cron expression (settings override or default)
	ScheduleKey string    `json:"scheduleKey"` // settings key the modal PATCHes to change it
	LastRun     time.Time `json:"lastRun"`
	LastResult  string    `json:"lastResult"` // "ok" | "error" | ""
	LastError   string    `json:"lastError"`
	NextRun     time.Time `json:"nextRun"`
	Running     bool      `json:"running"`
	// DisabledReason is non-empty when this job cannot run in this environment. The UI
	// renders it in place of the schedule and offers neither Run-now nor Modify.
	DisabledReason string `json:"disabledReason,omitempty"`
}

// ScheduleStore is the persistence the scheduler needs (satisfied by store.Store). It uses
// store.ScheduledJob directly — the scheduler is app-level infrastructure, not a pure domain
// package, so importing store avoids a duplicate type + per-call adapter (same precedent as
// internal/reconcile importing store).
type ScheduleStore interface {
	UpsertScheduledJob(ctx context.Context, j store.ScheduledJob) error
	GetScheduledJob(ctx context.Context, name string) (store.ScheduledJob, error)
	ListScheduledJobs(ctx context.Context) ([]store.ScheduledJob, error)
	ClaimDueScheduledJobs(ctx context.Context, now time.Time, lease time.Duration) ([]store.ScheduledJob, error)
}

// Notifier receives a job-changed signal after each run, for the SSE bus (optional).
type Notifier interface{ JobChanged(name string) }

// ScheduleFn resolves a job's effective cron expression from its settings key, falling back
// to def when unset (so a schedule is edited in the settings registry, hot-applied).
type ScheduleFn func(key, def string) string

const (
	// heartbeat is how often the loop checks for due jobs. Short + fixed: jobs are due on
	// their own intervals, not this cadence. Mirrors the suggest worker pool's 2s poll.
	heartbeat = 5 * time.Second
	// leaseHorizon must comfortably exceed the longest job runtime so a still-running job
	// isn't re-claimed by the next tick before it reschedules itself.
	leaseHorizon = 30 * time.Minute
)

// Scheduler ticks a heartbeat, claims due jobs, runs them in bounded goroutines, records
// state, and services on-demand triggers.
type Scheduler struct {
	store    ScheduleStore
	jobs     map[string]Job
	order    []string // registration order, for stable List output
	schedule ScheduleFn
	cron     *gronx.Gronx
	now      func() time.Time
	log      *slog.Logger
	notifier Notifier

	wake chan struct{} // buffered signal to run a tick now (Trigger, startup)

	mu      sync.Mutex
	running map[string]bool // jobs currently executing
}

// New builds a scheduler over the given registry. schedule resolves each job's cron from its
// settings key (falling back to the job's default cron).
func New(st ScheduleStore, reg *Registry, schedule ScheduleFn, now func() time.Time, log *slog.Logger) *Scheduler {
	// ⚠ Sealed here because this is where the job set stops being able to change: the map
	// below is a SNAPSHOT, so a later reg.Add would be accepted, listed by Jobs(), and never
	// scheduled. Sealing turns that silent no-op into a panic naming the job.
	reg.seal()
	jobs := make(map[string]Job, len(reg.jobs))
	order := make([]string, 0, len(reg.jobs))
	for _, j := range reg.jobs {
		jobs[j.Name] = j
		order = append(order, j.Name)
	}
	return &Scheduler{
		store: st, jobs: jobs, order: order, schedule: schedule, cron: gronx.New(),
		now: now, log: log, wake: make(chan struct{}, 1), running: map[string]bool{},
	}
}

// WithNotifier wires an SSE notifier (optional). Returns s for chaining.
func (s *Scheduler) WithNotifier(n Notifier) *Scheduler { s.notifier = n; return s }

// effectiveCron is the job's configured cron expression, or its default when unset/invalid.
func (s *Scheduler) effectiveCron(j Job) string {
	cron := j.DefaultCron
	if j.ScheduleKey != "" && s.schedule != nil {
		if c := s.schedule(j.ScheduleKey, j.DefaultCron); c != "" {
			cron = c
		}
	}
	// Guard against a stored expression the settings layer somehow let through: a job with a
	// bad cron would never reschedule. Fall back to the code default (always valid).
	if !s.cron.IsValid(cron) {
		s.log.Warn("scheduler: invalid cron, using default", "job", j.Name, "cron", cron)
		return j.DefaultCron
	}
	return cron
}

// nextRun computes the job's next fire time from its effective cron, after `from`. A
// still-invalid default (a programming error) falls back to from+1h so the job isn't wedged.
func (s *Scheduler) nextRun(j Job, from time.Time) time.Time {
	next, err := gronx.NextTickAfter(s.effectiveCron(j), from, false)
	if err != nil {
		s.log.Error("scheduler: next tick", "job", j.Name, "err", err)
		return from.Add(time.Hour)
	}
	return next
}

// Run reconciles the state table against the registry, then loops on the heartbeat (and
// on-demand wakes) until ctx is cancelled. Registered jobs missing a state row are created
// due-now so a fresh install runs each once on startup (matching the old run-once-on-start
// behavior); unknown-in-code rows are left untouched.
func (s *Scheduler) Run(ctx context.Context) {
	s.reconcileRegistry(ctx)
	s.log.Info("scheduler started", "jobs", len(s.jobs))

	ticker := time.NewTicker(heartbeat)
	defer ticker.Stop()
	// Kick an immediate tick so due-on-start jobs run without waiting a heartbeat.
	s.signal()
	for {
		select {
		case <-ctx.Done():
			s.log.Info("scheduler stopped")
			return
		case <-ticker.C:
			s.tick(ctx)
		case <-s.wake:
			s.tick(ctx)
		}
	}
}

// reconcileRegistry ensures every code-defined job has a state row (created due-now on first
// sight), so the loop will run it. Rows for jobs no longer in code are ignored, not deleted.
func (s *Scheduler) reconcileRegistry(ctx context.Context) {
	existing := map[string]bool{}
	if rows, err := s.store.ListScheduledJobs(ctx); err == nil {
		for _, r := range rows {
			existing[r.Name] = true
		}
	}
	now := s.now()
	for _, name := range s.order {
		if existing[name] {
			continue
		}
		// A disabled job gets no state row, so it can never become due and the claim query
		// never sees it. Enforcing it here as well as in tick() is deliberate: this is the
		// only point that would otherwise write a due-now row, and a job that is disabled
		// today may have been seeded by an earlier boot that was not.
		if s.jobs[name].Disabled() {
			continue
		}
		if err := s.store.UpsertScheduledJob(ctx, store.ScheduledJob{Name: name, NextRun: now, UpdatedAt: now}); err != nil {
			s.log.Warn("scheduler: seed job state", "job", name, "err", err)
		}
	}
}

// tick claims all due jobs and runs each in a bounded goroutine. A job whose Run panics or
// errors records last_result=error but never wedges the loop or the other jobs.
func (s *Scheduler) tick(ctx context.Context) {
	due, err := s.store.ClaimDueScheduledJobs(ctx, s.now(), leaseHorizon)
	if err != nil {
		s.log.Error("scheduler: claim due jobs", "err", err)
		return
	}
	for _, row := range due {
		j, ok := s.jobs[row.Name]
		if !ok {
			continue // a state row with no code job (e.g. renamed) — skip; it won't be reclaimed until we reschedule, which we don't, so it stays leased far out. Acceptable: stale rows are inert.
		}
		// ⚠ Load-bearing, not belt-and-braces. reconcileRegistry declines to SEED a
		// disabled job, but a row can already exist from a boot where the job was enabled
		// — precisely what a SQLite → Postgres migration (§18, V11) produces: the same
		// install, the same `backup` row, a backend that can no longer run it. Without
		// this the first tick after that migration would claim the row and run a job whose
		// writer does not exist.
		if j.Disabled() {
			continue
		}
		go s.execute(ctx, j)
	}
}

// execute runs one job and persists the outcome + next run.
func (s *Scheduler) execute(ctx context.Context, j Job) {
	s.setRunning(j.Name, true)
	defer s.setRunning(j.Name, false)

	start := s.now()
	err := s.safeRun(ctx, j)
	now := s.now()

	result, errText := "ok", ""
	if err != nil {
		result, errText = "error", err.Error()
		s.log.Warn("scheduled job failed", "job", j.Name, "err", err)
	}
	next := s.nextRun(j, now)
	if uerr := s.store.UpsertScheduledJob(ctx, store.ScheduledJob{
		Name: j.Name, LastRun: start, LastResult: result, LastError: errText, NextRun: next, UpdatedAt: now,
	}); uerr != nil {
		s.log.Error("scheduler: record job result", "job", j.Name, "err", uerr)
	}
	if s.notifier != nil {
		s.notifier.JobChanged(j.Name)
	}
}

// safeRun runs a job's Run func, converting a panic into an error so one bad job can't crash
// the scheduler goroutine.
func (s *Scheduler) safeRun(ctx context.Context, j Job) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = &panicError{v: r}
		}
	}()
	return j.Run(ctx)
}

// Trigger forces a job to run off-cycle ("Run now"): mark it due, then wake the loop.
// Returns ErrUnknownJob if the name isn't registered.
func (s *Scheduler) Trigger(ctx context.Context, name string) error {
	j, ok := s.jobs[name]
	if !ok {
		return ErrUnknownJob
	}
	// ⚠ The refusal lives HERE, not only in the UI. The Tasks page hides Run-now for a
	// disabled job, but that is a hint — anything a client can be shown, a client can
	// skip, and this one would run a job the backend cannot support.
	if j.Disabled() {
		return ErrJobDisabled
	}
	now := s.now()
	cur, err := s.store.GetScheduledJob(ctx, name)
	if err != nil {
		cur = store.ScheduledJob{Name: name}
	}
	cur.Name, cur.NextRun, cur.UpdatedAt = name, now, now
	if err := s.store.UpsertScheduledJob(ctx, cur); err != nil {
		return err
	}
	s.signal()
	return nil
}

// List returns the current status of every registered job (registration order), joining the
// code registry (name/title/interval) with the state table (last/next run, result).
func (s *Scheduler) List(ctx context.Context) ([]JobStatus, error) {
	state := map[string]store.ScheduledJob{}
	rows, err := s.store.ListScheduledJobs(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		state[r.Name] = r
	}
	out := make([]JobStatus, 0, len(s.order))
	for _, name := range s.order {
		j := s.jobs[name]
		st := state[name]
		status := JobStatus{
			Name: j.Name, Title: j.Title, Description: j.Description,
			Schedule: s.effectiveCron(j), ScheduleKey: j.ScheduleKey,
			LastRun: st.LastRun, LastResult: st.LastResult, LastError: st.LastError,
			NextRun: st.NextRun, Running: s.isRunning(name),
			DisabledReason: j.DisabledReason,
		}
		// A disabled job has no next run — it is never claimed. Any NextRun in the state
		// row is a leftover from a boot where it WAS enabled (the SQLite → Postgres case),
		// and rendering it would promise a run that will never happen. Zeroing it here
		// means the read model cannot disagree with what the loop actually does.
		if j.Disabled() {
			status.NextRun = time.Time{}
		}
		out = append(out, status)
	}
	return out, nil
}

func (s *Scheduler) signal() {
	select {
	case s.wake <- struct{}{}:
	default: // a wake is already pending; one tick will pick up all due jobs
	}
}

func (s *Scheduler) setRunning(name string, v bool) {
	s.mu.Lock()
	s.running[name] = v
	s.mu.Unlock()
}

func (s *Scheduler) isRunning(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running[name]
}
