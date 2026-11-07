package app

import (
	"context"
	"errors"

	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/platform/jobs"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/provisionbackstop"
)

// channelMaintenanceJob composes the prerequisites for a current channel guide. Episode
// discovery, Tunarr's media source and channel convergence remain separate modules and failure
// boundaries internally, but operators need one outcome and one cadence: keep live channels
// current.
func channelMaintenanceJob(
	runner *channels.Runner,
	episodes *provisionbackstop.EpisodeRefresh,
	applyBackend func(context.Context) error,
	convergeMediaSource func(context.Context) error,
) jobs.Job {
	job := runner.Job()
	job.Name = "channel-maintenance"
	job.Title = "Maintain live channels"
	job.Description = "Refreshes series episodes, keeps Tunarr connected to your media server, " +
		"rebuilds upcoming schedules, and converges each channel's playout backend."
	job.ScheduleKey = settings.JobChannelMaintenanceSchedule.Key()
	runSweep := job.Run
	job.Run = func(ctx context.Context) error {
		_, refreshErr := episodes.Run(ctx)
		var mediaSourceErr error
		if convergeMediaSource != nil {
			mediaSourceErr = convergeMediaSource(ctx)
		}
		var transitionErr error
		if applyBackend != nil {
			transitionErr = applyBackend(ctx)
		}
		sweepErr := runSweep(ctx)
		return errors.Join(refreshErr, mediaSourceErr, transitionErr, sweepErr)
	}
	return job
}
