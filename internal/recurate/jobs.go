package recurate

import (
	"context"

	"github.com/mantonx/loomarr/internal/scheduler"
)

// Job returns the auto-recuration pass as a scheduler job (§18.1) — the per-channel opt-in
// that schedules a refine and lets the result flow through the normal approve → bind →
// reconcile pipeline.
func (r *Runner) Job() scheduler.Job {
	return scheduler.Job{
		Name: "channel-recurate", Title: "Re-curate self-updating channels",
		DefaultCron: "0 0 4 * * 0", ScheduleKey: "job.recurate.schedule",
		Run: func(ctx context.Context) error { _, err := r.Run(ctx); return err },
	}
}
