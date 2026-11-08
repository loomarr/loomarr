package app

import (
	"context"
	"errors"

	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/platform/jobs"
	"github.com/loomarr/loomarr/internal/platform/settings"
)

// channelMaintenanceJob composes the prerequisites for a current channel guide. Tunarr's media
// source, the playout backend and channel convergence remain separate modules and failure
// boundaries internally, but operators need one outcome and one cadence: keep live channels
// current.
func channelMaintenanceJob(
	runner *channels.Runner,
	applyBackend func(context.Context) error,
	convergeMediaSource func(context.Context) error,
) jobs.Job {
	job := runner.Job()
	job.Name = "channel-maintenance"
	job.Title = "Maintain live channels"
	job.Description = "Keeps Tunarr connected to your media server, rebuilds upcoming schedules, " +
		"and converges each channel's playout backend."
	job.ScheduleKey = settings.JobChannelMaintenanceSchedule.Key()
	runSweep := job.Run
	job.Run = func(ctx context.Context) error {
		var mediaSourceErr error
		if convergeMediaSource != nil {
			mediaSourceErr = convergeMediaSource(ctx)
		}
		var transitionErr error
		if applyBackend != nil {
			transitionErr = applyBackend(ctx)
		}
		sweepErr := runSweep(ctx)
		return errors.Join(mediaSourceErr, transitionErr, sweepErr)
	}
	return job
}
