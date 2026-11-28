package app

import (
	"log/slog"
	"time"

	"github.com/loomarr/loomarr/internal/library/mediaserver"
	"github.com/loomarr/loomarr/internal/library/provisionbackstop"
	"github.com/loomarr/loomarr/internal/library/resolvers"
	"github.com/loomarr/loomarr/internal/platform/activity"
	"github.com/loomarr/loomarr/internal/platform/diagnostics"
	"github.com/loomarr/loomarr/internal/platform/housekeeping"
	"github.com/loomarr/loomarr/internal/platform/jobs"
	"github.com/loomarr/loomarr/internal/platform/metrics"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/proposals/quality"
)

// buildProvisioning registers acquisition, availability, retention, and queue work.
func buildProvisioning(
	st provisioningStores,
	set resolved,
	libraryClient *mediaserver.Client,
	emitter *eventEmitter,
	registry *jobs.Registry,
	activityRecorder *activity.Recorder,
	processDiagnostics *diagnostics.ProcessManager,
	log *slog.Logger,
	metricRecorder *metrics.Recorder,
) {
	if st.titles == nil {
		return
	}
	reconciler := provisionbackstop.NewDynamic(st.titles, set.requesterFor(metricRecorder), func() provisionbackstop.LibraryLookup {
		return resolvers.NewOwnedLookup(st.library)
	}, emitter, provisionbackstop.Config{
		RequestTTL: settings.RequestTTL.Get(set), DownloadingTTL: settings.DownloadingTTL.Get(set),
	}, time.Now, log).WithQualityRecorder(quality.NewAcquisitionRecorder(st.observations, log))
	registry.Add(reconciler.Job())
	keeper := housekeeping.New(st.housekeeping, housekeeping.Windows{
		Proposals:   func() time.Duration { return settings.ProposalsRetention.Get(set) },
		Jobs:        func() time.Duration { return settings.JobsRetention.Get(set) },
		Activity:    func() time.Duration { return settings.ActivityRetention.Get(set) },
		Diagnostics: func() time.Duration { return settings.DiagnosticsRetention.Get(set) },
		DiagnosticsMaxBytes: func() int64 {
			return int64(settings.DiagnosticsMaxStorageMb.Get(set)) * 1024 * 1024
		},
	}, time.Now, log).WithDiagnostics(processDiagnostics)
	if st.tidy != nil {
		keeper.WithTidier(st.tidy)
	}
	registry.Add(keeper.Job())

	libraryScan := provisionbackstop.NewLibraryScan(
		st.titles, resolvers.NewScanner(st.library), emitter, settings.JobLibraryScanLookback.Get(set), time.Now, log,
	).WithActivity(activityRecorder).WithQualityRecorder(quality.NewAcquisitionRecorder(st.observations, log))
	registry.AddAll(libraryScan.Jobs())

	if arr := set.arrRequester(metricRecorder); arr != nil {
		queuePoll := provisionbackstop.NewQueuePoll(st.titles, arr, emitter, settings.DownloadingTTL.Get(set), time.Now, log)
		registry.Add(queuePoll.Job("arr-queue-poll", "Poll Sonarr and Radarr",
			"Updates acquisition progress and detects stalled downloads from Sonarr and Radarr."))
	} else if seerr := set.seerrRequester(metricRecorder); seerr != nil {
		queuePoll := provisionbackstop.NewQueuePoll(st.titles, seerr, emitter, settings.DownloadingTTL.Get(set), time.Now, log)
		registry.Add(queuePoll.Job("seerr-queue-poll", "Poll Seerr acquisitions",
			"Updates requested titles from the coarse acquisition states reported by Seerr."))
	}
}
