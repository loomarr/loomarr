package app

import (
	"context"
	"errors"

	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/reconcile"
	"github.com/loomarr/loomarr/internal/scheduler"
)

// channelMaintenanceJob composes the two prerequisites for a current channel guide. Episode
// discovery and channel convergence remain separate modules and failure boundaries internally,
// but operators need one outcome and one cadence: keep live channels current.
func channelMaintenanceJob(
	runner *channels.Runner,
	episodes *reconcile.EpisodeRefresh,
	applyBackend func(context.Context) error,
) scheduler.Job {
	job := runner.Job()
	job.Name = "channel-maintenance"
	job.Title = "Maintain live channels"
	job.Description = "Refreshes series episodes, rebuilds upcoming schedules, and converges each channel's playout backend."
	job.ScheduleKey = "job.channel_maintenance.schedule"
	runSweep := job.Run
	job.Run = func(ctx context.Context) error {
		_, refreshErr := episodes.Run(ctx)
		var transitionErr error
		if applyBackend != nil {
			transitionErr = applyBackend(ctx)
		}
		sweepErr := runSweep(ctx)
		return errors.Join(refreshErr, transitionErr, sweepErr)
	}
	return job
}
