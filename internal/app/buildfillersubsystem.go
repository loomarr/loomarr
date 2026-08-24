package app

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/diagnostics"
	"github.com/loomarr/loomarr/internal/events"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/programmer"
	"github.com/loomarr/loomarr/internal/scheduler"
	"github.com/loomarr/loomarr/internal/store"
)

type fillerBuild struct {
	service api.FillerService
	preview api.PodPreviewer
}

func buildFillerSubsystem(
	st store.Store,
	set resolved,
	layout filler.Layout,
	log *slog.Logger,
	libraryClient *library.Client,
	eventBus *events.Bus,
	emitter *eventEmitter,
	jobs *scheduler.Registry,
	playoutResolver *playoutResolver,
	channelService api.ChannelService,
	processDiagnostics *diagnostics.ProcessManager,
) fillerBuild {
	var result fillerBuild
	if st == nil {
		return result
	}
	// Background acquisition workers are process-owned in the single-replica beta. Any queued or
	// running rows visible before this process accepts requests belonged to the previous process
	// and can no longer complete; close them instead of promising work that does not exist.
	recoveryCtx, recoveryCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if n, err := st.RecoverInterruptedAcquisitionRuns(recoveryCtx, time.Now().UTC()); err != nil {
		log.Warn("could not recover interrupted filler acquisitions", "err", err)
	} else if n > 0 {
		log.Info("recovered interrupted filler acquisitions", "runs", n)
	}
	recoveryCancel()

	if dir := layout.ClipDir(); dir != "" {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			log.Warn("could not create the filler drop-folder; the catalog scan will report it",
				"dir", dir, "err", err)
		}
		if watch := layout.WatchDir(); watch != "" {
			if err := os.MkdirAll(watch, 0o750); err != nil {
				log.Warn("could not create the filler watch folder; incoming clips cannot be accepted",
					"dir", watch, "err", err)
			}
		}
	}

	fillerProgrammer := programmer.NewDynamic(set.tunarrConfig())
	wake := &fillerChannelWake{st: st, channels: channelService, log: log}
	syncer := buildSyncer(st, set, layout, log, fillerProgrammer, libraryClient)
	taggerProvider, tagger := buildTagger(st, set, layout, log, wake)
	fetcher := buildFetcher(set, layout, log)
	splitter := buildSplitter(st, set, layout, log, wake)
	adapter := fillerServiceAdapter{
		syncer: syncer, tagger: tagger, fetcher: fetcher,
		bus: eventBus, log: log, newID: newID, timeout: set.dur("ingest.timeout"),
		sources: st, acquisitions: st, readiness: st, now: time.Now,
		splitter: splitter, splitClips: fillerSplitStoreAdapter{st: st, wake: wake},
	}

	pods := buildPodAdapter(st, set, log)
	result.preview = podPreviewAdapter{store: st, pods: pods}
	adapter.pool = result.preview.Pool
	if playoutResolver != nil {
		playoutResolver.pods = result.preview
	}
	if engine, ok := channelService.(*channels.Engine); ok {
		engine.WithPods(pods)
		log.Info("filler pod assembler wired into the scheduler")
	}

	jobs.Add(fillerSyncJob(syncer))
	log.Info("filler catalog sync registered", "dir", layout.ClipDir(),
		"every", set.dur("filler.sync_every"), "ai_tagging", set.boolv("filler.ai_tagging"))
	pipeline := buildPipeline(st, set, layout, log, emitter, splitter, taggerProvider, wake, processDiagnostics)
	jobs.Add(fillerPipelineJob(pipeline))
	adapter.pipeline = pipeline
	adapter.afterIngest = func(ctx context.Context) error {
		if _, err := syncer.Sync(ctx); err != nil {
			return err
		}
		// Enrol only. Running a whole pipeline pass here could make one completed download wait on
		// an unrelated transcode/Whisper backlog; the scheduled job remains the stage driver.
		_, err := pipeline.EnrolMissing(ctx)
		return err
	}
	jobs.Add(fillerSplitSweepJob(filler.NewSplitSweeper(
		fillerSweepStoreAdapter{st}, layout.ClipDir(),
		func() time.Duration { return set.dur("filler.split.review_window") }, time.Now, log,
	)))
	autoFetch := filler.NewFetcher(
		fetchStoreAdapter{st: st, fetchEvery: func() time.Duration { return set.dur("filler.fetch.every") }},
		archiveDiscoverAdapter{}, adapter, layout.ClipDir(),
		filler.FetchLimits{
			MaxPerRun:       func() int { return set.intv("filler.fetch.max_per_run") },
			MaxCatalogClips: func() int { return set.intv("filler.fetch.max_catalog_clips") },
			MaxDiskGB:       func() int { return set.intv("filler.fetch.max_disk_gb") },
		}, log,
	).WithEnabled(func() bool { return set.dur("filler.fetch.every") > 0 })
	adapter.autoFetch = autoFetch
	result.service = adapter
	jobs.Add(fillerFetchJob(autoFetch))
	log.Info("filler auto-fetch registered", "every", set.dur("filler.fetch.every"),
		"max_per_run", set.intv("filler.fetch.max_per_run"))
	return result
}
