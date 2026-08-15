package app

import (
	"context"
	"errors"

	"github.com/mantonx/loomarr/internal/channels"
	"github.com/mantonx/loomarr/internal/reconcile"
	"github.com/mantonx/loomarr/internal/scheduler"
)

// channelMaintenanceJob composes the two prerequisites for a current channel guide. Episode
// discovery and Tunarr reconciliation remain separate modules and failure boundaries internally,
// but operators need one outcome and one cadence: keep live channels current.
func channelMaintenanceJob(runner *channels.Runner, episodes *reconcile.EpisodeRefresh) scheduler.Job {
	job := runner.Job()
	job.Name = "channel-maintenance"
	job.Title = "Maintain live channels"
	job.Description = "Refreshes series episodes, rebuilds upcoming schedules, and sends them to Tunarr."
	job.ScheduleKey = "job.channel_maintenance.schedule"
	runSweep := job.Run
	job.Run = func(ctx context.Context) error {
		_, refreshErr := episodes.Run(ctx)
		sweepErr := runSweep(ctx)
		return errors.Join(refreshErr, sweepErr)
	}
	return job
}
