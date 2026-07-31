package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ScheduledJob is the persisted runtime state of one named, recurring background job
// (§18.1). It is NOT config — which jobs exist and how often come from the code registry +
// settings; this row records last-run/result and the next-run lease used to coordinate ticks
// across replicas. Rows are reconciled from the code registry on boot.
type ScheduledJob struct {
	Name       string
	LastRun    time.Time
	LastResult string // "ok" | "error" | ""
	LastError  string
	NextRun    time.Time
	UpdatedAt  time.Time
	// Paused stops the job running on its schedule (§18.1). Operator-set and durable, so it
	// survives restarts — distinct from DisabledReason, which states an environment fact the
	// operator cannot change and which offers no Resume.
	//
	// ⚠ Written ONLY by SetScheduledJobPaused, never by UpsertScheduledJob: that upsert runs
	// after every execution, so carrying `paused` in its DO UPDATE list would clear the flag
	// on the next run of any job someone paused.
	Paused bool
}

const scheduledJobSelect = `SELECT name, last_run, last_result, last_error, next_run, updated_at, paused FROM scheduled_jobs`

// UpsertScheduledJob writes a job's state, creating the row on first sight (boot registry
// reconcile) and updating it after each run. Identity is `name`.
func (s *sqlStore) UpsertScheduledJob(ctx context.Context, j ScheduledJob) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`INSERT INTO scheduled_jobs (name, last_run, last_result, last_error, next_run, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (name) DO UPDATE SET
		   last_run=excluded.last_run, last_result=excluded.last_result,
		   last_error=excluded.last_error, next_run=excluded.next_run, updated_at=excluded.updated_at`),
		j.Name, epoch(j.LastRun), j.LastResult, j.LastError, epoch(j.NextRun), epoch(j.UpdatedAt))
	if err != nil {
		return fmt.Errorf("upsert scheduled job %s: %w", j.Name, err)
	}
	return nil
}

// SetScheduledJobPaused sets (or clears) a job's pause flag, creating the row if the job has
// not run yet — an operator can pause a task before its first execution.
//
// ⚠ Deliberately its OWN statement rather than a field on UpsertScheduledJob. That upsert runs
// after every execution, so carrying `paused` through it would clear the flag on the next run
// of any paused job — silently, and worst at exactly the moment someone had just paused
// something. Keeping the write here means the only thing that can change pause is an operator
// asking for it.
func (s *sqlStore) SetScheduledJobPaused(ctx context.Context, name string, paused bool) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`INSERT INTO scheduled_jobs (name, last_run, last_result, last_error, next_run, updated_at, paused)
		 VALUES (?, 0, '', '', 0, ?, ?)
		 ON CONFLICT (name) DO UPDATE SET paused=excluded.paused, updated_at=excluded.updated_at`),
		name, epoch(time.Now()), paused)
	if err != nil {
		return fmt.Errorf("set scheduled job %s paused: %w", name, err)
	}
	return nil
}

// ListScheduledJobs returns all job state rows (for the Tasks API/UI), name-ordered.
func (s *sqlStore) ListScheduledJobs(ctx context.Context) ([]ScheduledJob, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(scheduledJobSelect+` ORDER BY name`))
	if err != nil {
		return nil, fmt.Errorf("list scheduled jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanScheduledJobs(rows)
}

// GetScheduledJob returns one job's state, or ErrNotFound.
func (s *sqlStore) GetScheduledJob(ctx context.Context, name string) (ScheduledJob, error) {
	return scanScheduledJob(s.db.QueryRowContext(ctx, s.ph(scheduledJobSelect+` WHERE name = ?`), name))
}

// ClaimDueScheduledJobs leases every job whose next_run is due (§18.1), advancing next_run to
// now+lease so a concurrent tick/replica won't re-run it before this run reschedules it. The
// caller runs each returned job then Upserts its real next_run (now + effective interval).
func (s *sqlStore) ClaimDueScheduledJobs(ctx context.Context, now time.Time, lease time.Duration) ([]ScheduledJob, error) {
	rows, err := s.db.QueryContext(ctx, s.scheduledJobClaimSQL, epoch(now.Add(lease)), epoch(now))
	if err != nil {
		return nil, fmt.Errorf("claim due scheduled jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanScheduledJobs(rows)
}

func scanScheduledJob(sc scannable) (ScheduledJob, error) {
	var (
		j                           ScheduledJob
		lastRun, nextRun, updatedAt int64
	)
	err := sc.Scan(&j.Name, &lastRun, &j.LastResult, &j.LastError, &nextRun, &updatedAt, &j.Paused)
	if err == sql.ErrNoRows {
		return ScheduledJob{}, ErrNotFound
	}
	if err != nil {
		return ScheduledJob{}, err
	}
	j.LastRun = fromEpoch(lastRun)
	j.NextRun = fromEpoch(nextRun)
	j.UpdatedAt = fromEpoch(updatedAt)
	return j, nil
}

func scanScheduledJobs(rows *sql.Rows) ([]ScheduledJob, error) {
	var out []ScheduledJob
	for rows.Next() {
		j, err := scanScheduledJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}
