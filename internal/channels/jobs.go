package channels

import (
	"context"

	"github.com/mantonx/loomarr/internal/scheduler"
)

// Job returns the channel sweep as a scheduler job (§9, §18.1) — the periodic reconcile
// that keeps Tunarr matching Loomarr's desired state.
//
// ⚠ Sweep returns a COUNT and no error: it is best-effort by design, reconciling what it
// can and leaving the rest for the next tick. The job reports success accordingly — a
// partial sweep is not a failed one, and marking it failed would put a red row on the Tasks
// page for normal operation.
func (r *Runner) Job() scheduler.Job {
	return scheduler.Job{
		Name: "channel-sweep", Title: "Reconcile channels with Tunarr",
		DefaultCron: "0 */10 * * * *", ScheduleKey: "job.channel_sweep.schedule",
		Run: func(ctx context.Context) error { r.Sweep(ctx); return nil },
	}
}
