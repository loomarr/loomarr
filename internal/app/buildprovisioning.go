package app

import (
	"log/slog"
	"time"

	"github.com/loomarr/loomarr/internal/activity"
	"github.com/loomarr/loomarr/internal/diagnostics"
	"github.com/loomarr/loomarr/internal/jobs"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/metrics"
	"github.com/loomarr/loomarr/internal/provisionbackstop"
	"github.com/loomarr/loomarr/internal/quality"
	"github.com/loomarr/loomarr/internal/retention"
	"github.com/loomarr/loomarr/internal/settings"
	"github.com/loomarr/loomarr/internal/store"
)

// buildProvisioning registers acquisition, availability, retention, and queue work and returns
// the one episode refresher consumed later by channel maintenance.
func buildProvisioning(
	st store.Store,
	set resolved,
	libraryClient *library.Client,
	emitter *eventEmitter,
	registry *jobs.Registry,
	activityRecorder *activity.Recorder,
	processDiagnostics *diagnostics.ProcessManager,
	log *slog.Logger,
	metricRecorder *metrics.Recorder,
) *provisionbackstop.EpisodeRefresh {
	if st == nil {
		return nil
	}
	reconciler := provisionbackstop.NewDynamic(st, set.requesterFor(metricRecorder), func() provisionbackstop.LibraryLookup {
		return libraryClient.Snapshot()
	}, emitter, provisionbackstop.Config{
		RequestTTL: settings.RequestTTL.Get(set), DownloadingTTL: settings.DownloadingTTL.Get(set),
	}, time.Now, log).WithQualityRecorder(quality.NewAcquisitionRecorder(st, log))
	registry.Add(reconciler.Job())
	registry.Add(retention.New(st, retention.Windows{
		Proposals:   func() time.Duration { return settings.ProposalsRetention.Get(set) },
		Jobs:        func() time.Duration { return settings.JobsRetention.Get(set) },
		Activity:    func() time.Duration { return settings.ActivityRetention.Get(set) },
		Diagnostics: func() time.Duration { return settings.DiagnosticsRetention.Get(set) },
		DiagnosticsMaxBytes: func() int64 {
			return int64(settings.DiagnosticsMaxStorageMb.Get(set)) * 1024 * 1024
		},
	}, time.Now, log).WithDiagnostics(processDiagnostics).Job())

	libraryScan := provisionbackstop.NewLibraryScan(
		st, libraryClient, emitter, settings.JobLibraryScanLookback.Get(set), time.Now, log,
	).WithActivity(activityRecorder).WithQualityRecorder(quality.NewAcquisitionRecorder(st, log))
	registry.AddAll(libraryScan.Jobs())

	episodeRefresh := provisionbackstop.NewDynamicEpisodeRefresh(st, func() provisionbackstop.EpisodeResolver {
		return episodeResolver(libraryClient.Snapshot())
	}, func() time.Duration {
		return settings.EpisodesMaxAge.Get(set)
	}, time.Now, log)

	if arr := set.arrRequester(metricRecorder); arr != nil {
		queuePoll := provisionbackstop.NewQueuePoll(st, arr, emitter, settings.DownloadingTTL.Get(set), time.Now, log)
		registry.Add(queuePoll.Job("arr-queue-poll", "Poll Sonarr and Radarr",
			"Updates acquisition progress and detects stalled downloads from Sonarr and Radarr."))
	} else if seerr := set.seerrRequester(metricRecorder); seerr != nil {
		queuePoll := provisionbackstop.NewQueuePoll(st, seerr, emitter, settings.DownloadingTTL.Get(set), time.Now, log)
		registry.Add(queuePoll.Job("seerr-queue-poll", "Poll Seerr acquisitions",
			"Updates requested titles from the coarse acquisition states reported by Seerr."))
	}
	return episodeRefresh
}
