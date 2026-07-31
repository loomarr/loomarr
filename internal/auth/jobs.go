package auth

import (
	"context"
	"time"

	"github.com/mantonx/loomarr/internal/scheduler"
)

// Job returns the expired-session sweep as a scheduler job (§11, §18.1).
//
// Sessions are rows, not stateless JWTs, so expiry has to be collected rather than merely
// observed — this is what stops the table growing forever with dead rows.
func (s *SessionSweeper) Job(now func() time.Time) scheduler.Job {
	if now == nil {
		now = time.Now
	}
	return scheduler.Job{
		Name: "session-sweep", Title: "Clear expired sessions",
		Description: "Removes sign-in sessions that have already expired. Nobody is signed out early by this.",
		DefaultCron: "0 0 * * * *", ScheduleKey: "job.session_sweep.schedule",
		Run: func(ctx context.Context) error { _, err := s.Sweep(ctx, now()); return err },
	}
}
