package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

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
	log *slog.Logger,
	applyBackend func(context.Context) error,
	convergeMediaSource func(context.Context) error,
) jobs.Job {
	job := runner.Job()
	job.Name = "channel-maintenance"
	job.Title = "Maintain live channels"
	job.Description = "Keeps Tunarr connected to your media server, rebuilds upcoming schedules, " +
		"and converges each channel's playout backend."
	job.ScheduleKey = settings.JobChannelMaintenanceSchedule.Key()
	job.Run = func(ctx context.Context) error {
		var run maintenanceRun
		began := time.Now()
		var mediaSourceErr error
		if convergeMediaSource != nil {
			mediaSourceErr = convergeMediaSource(ctx)
		}
		run.MediaSource = time.Since(began)
		began = time.Now()
		var transitionErr error
		if applyBackend != nil {
			transitionErr = applyBackend(ctx)
		}
		run.Backend = time.Since(began)
		run.Sweep = runner.Sweep(ctx)
		run.log(log)
		return errors.Join(mediaSourceErr, transitionErr)
	}
	return job
}

// slowChannelMaintenance is a third of the one-minute budget the job runs under. A run past it
// logs where its time went at Info (#4428); a quicker one logs it at Debug.
const slowChannelMaintenance = 20 * time.Second

// maintenanceRun is where one channel-maintenance run spent its time, step by step.
type maintenanceRun struct {
	MediaSource, Backend time.Duration
	Sweep                channels.SweepReport
}

func (m maintenanceRun) log(log *slog.Logger) {
	total := m.MediaSource + m.Backend + m.Sweep.Claim + m.Sweep.Reconcile
	level := slog.LevelDebug
	if total >= slowChannelMaintenance {
		level = slog.LevelInfo
	}
	log.Log(context.Background(), level, "channel maintenance timing",
		"total_ms", total.Milliseconds(),
		"media_source_ms", m.MediaSource.Milliseconds(), "backend_ms", m.Backend.Milliseconds(),
		"claim_ms", m.Sweep.Claim.Milliseconds(), "reconcile_ms", m.Sweep.Reconcile.Milliseconds(),
		"claimed", m.Sweep.Claimed, "reconciled", m.Sweep.Reconciled,
		"slowest_channel", m.Sweep.SlowestChannel, "slowest_ms", m.Sweep.Slowest.Milliseconds())
}
