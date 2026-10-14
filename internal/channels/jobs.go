package channels

import (
	"context"

	"github.com/loomarr/loomarr/internal/scheduler"
)

// Job returns the channel sweep as a scheduler job (§9, §18.1) — the periodic reconcile
// that refreshes every channel's durable desired state and projects Tunarr-backed channels.
//
// ⚠ Sweep returns a COUNT and no error: it is best-effort by design, reconciling what it
// can and leaving the rest for the next tick. The job reports success accordingly — a
// partial sweep is not a failed one, and marking it failed would put a red row on the Tasks
// page for normal operation.
func (r *Runner) Job() scheduler.Job {
	return scheduler.Job{
		Name: "channel-maintenance", Group: scheduler.GroupChannels, Title: "Maintain live channels",
		Description: "Rebuilds upcoming schedules and updates each channel's selected playout backend.",
		DefaultCron: "0 */10 * * * *", ScheduleKey: "job.channel_maintenance.schedule",
		Run: func(ctx context.Context) error { r.Sweep(ctx); return nil },
	}
}
