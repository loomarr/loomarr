package app

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/clipfetch"
	"github.com/loomarr/loomarr/internal/creative"
	"github.com/loomarr/loomarr/internal/defects"
	"github.com/loomarr/loomarr/internal/diagnostics"
	"github.com/loomarr/loomarr/internal/events"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillerdecision"
	"github.com/loomarr/loomarr/internal/fillerenrichment"
	"github.com/loomarr/loomarr/internal/fillerresearch"
	"github.com/loomarr/loomarr/internal/fillerstore"
	"github.com/loomarr/loomarr/internal/httpx"
	"github.com/loomarr/loomarr/internal/images"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/mediatools"
	"github.com/loomarr/loomarr/internal/metrics"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/scheduler"
	"github.com/loomarr/loomarr/internal/storagegovernor"
	"github.com/loomarr/loomarr/internal/tunarr"
)

type fillerBuild struct {
	service   api.FillerService
	decisions *fillerdecision.Service
	defects   *filler.DefectReports
	preview   api.PodPreviewer
	taxonomy  api.TaxonomyEditor
	research  api.FillerResearchService
	// intakeLanguages is the language gate's live wanted set, for split review to compare against.
	intakeLanguages func(context.Context) (string, error)
}

// compilationGate reads `filler.acquisition.compilations` against the same single-clip ceiling
// probe marks compilations by, both live on every call (#1773).
func compilationGate(set resolved) filler.CompilationGate {
	return filler.CompilationGate{
		Take: func() bool { return set.boolv("filler.acquisition.compilations") },
		Over: func() time.Duration { return set.dur("filler.autosplit.max_duration") },
	}
}

func buildFillerSubsystem(
	st fillerstore.Store,
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
	storageGovernor *storagegovernor.Governor,
	metricRecorder *metrics.Recorder,
	imageService *images.Service,
	playbackHeadroom filler.PlaybackHeadroom,
	owner *generationLifecycle,
	holidaysChanged func(context.Context),
	calendar func() schedule.Calendar,
) fillerBuild {
	var result fillerBuild
	if st == nil {
		return result
	}
	result.research = newFillerResearchSettingsAdapter(st, set, metricRecorder)
	result.intakeLanguages = intakeLanguages(st, set)
	decisionService, err := fillerdecision.New(st)
	if err != nil {
		log.Error("could not construct filler decision service", "err", err)
	} else {
		result.decisions = decisionService
	}
	result.defects = filler.NewDefectReports(fillerDefectStoreAdapter{st}, time.Now)
	// Background acquisition workers are process-owned in the single-replica beta. Any queued or
	// running rows visible before this process accepts requests belonged to the previous process
	// and can no longer complete; close them instead of promising work that does not exist.
	recoveryCtx, recoveryCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if n, err := st.RecoverInterruptedAcquisitionRuns(recoveryCtx, time.Now().UTC()); err != nil {
		log.Warn("could not recover interrupted filler acquisitions", "err", err)
	} else if n > 0 {
		log.Info("recovered interrupted filler acquisitions", "runs", n)
	}
	if n, err := st.RecoverInterruptedInteractiveOperations(recoveryCtx, time.Now().UTC()); err != nil {
		log.Warn("could not recover interrupted interactive operations", "err", err)
	} else if n > 0 {
		log.Info("recovered interrupted interactive operations", "operations", n)
	}
	if n, err := st.RecoverInterruptedSpokenSafetyRuns(recoveryCtx, time.Now().UTC()); err != nil {
		log.Warn("could not recover interrupted spoken-safety runs", "err", err)
	} else if n > 0 {
		log.Info("recovered interrupted spoken-safety runs", "runs", n)
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
	artifactRecoveryCtx, artifactRecoveryCancel := context.WithTimeout(context.Background(), 30*time.Second)
	if recovered, err := clipfetch.RecoverAcquisitionArtifacts(artifactRecoveryCtx, layout.WatchDir(), layout.ClipDir(), st, time.Now); err != nil {
		log.Warn("could not recover filler acquisition artifacts", "err", err)
	} else if recovered.Published > 0 || recovered.Repair > 0 || recovered.Pending > 0 {
		log.Info("recovered filler acquisition artifacts",
			"published", recovered.Published, "repair", recovered.Repair, "pending", recovered.Pending)
	}
	artifactRecoveryCancel()

	fillerProgrammer := tunarr.NewDynamicObserved(set.tunarrConfig(), metricRecorder)
	wake := &fillerChannelWake{st: st, channels: channelService, log: log}
	result.taxonomy = taxonomyEditor{store: st, wake: wake, holidaysChanged: holidaysChanged}
	syncer := buildSyncer(st, set, layout, log, fillerProgrammer, libraryClient, storageGovernor)
	fetcher := buildFetcher(set, layout, log, st, storageGovernor)
	splitter := buildSplitter(st, set, layout, log, wake, metricRecorder, storageGovernor, imageService)
	ytDlpPath := resolveTool(set.str("ingest.ytdlp_path"), "yt-dlp")
	adapter := fillerServiceAdapter{
		syncer: syncer, fetcher: fetcher,
		bus: eventBus, log: log, newID: newID, timeout: set.dur("ingest.timeout"),
		start: owner.startInteractiveOperation, operations: st,
		sources: st, acquisitions: st, readiness: st, now: time.Now,
		archiveFinder: clipfetch.NewArchiveSourceFinder(),
		home:          set.homeGeography,
		splitter:      splitter, splitClips: fillerSplitStoreAdapter{st: st, wake: wake},
		storage: storageGovernor, storagePath: layout.ClipDir(),
		storageAutomatic: func() bool { return set.intv("filler.storage.library_budget_gb") == 0 },
		storageCleanup:   clipfetch.NewAcquisitionCleaner(layout.WatchDir(), st, time.Now),
	}
	if ytDlpPath != "" {
		adapter.youtubeFinder = clipfetch.NewYouTubeSourceFinder(ytDlpPath)
	}

	pods := buildPodAdapter(st, set, calendar, log).WithMetrics(metricRecorder)
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
		"every", set.dur("filler.sync_every"))
	pipeline, transcribeStage, visionStage := buildPipeline(st, set, layout, log, emitter, splitter, wake,
		processDiagnostics, storageGovernor, metricRecorder, playbackHeadroom)
	stations := &stationDirectory{list: st.ListBroadcastStations}
	enrichmentSignals := fillerEnrichmentSignals{store: st, files: layout.FS(), stations: stations}
	enrichment := fillerenrichment.NewRunner(
		fillerEnrichmentRepository{st: st},
		enrichmentSignals.Load,
		func() int { return set.intv("filler.pipeline.max_clips") }, time.Now,
	)
	enrichmentCoordinator := fillerenrichment.NewCoordinator(
		enrichment, fillerEnrichmentRepository{st: st},
		func() fillerenrichment.TextSelection { return activeFillerTextSelection(set, metricRecorder) },
		enrichmentSignals.Load,
		func() int { return set.intv("filler.pipeline.max_clips") }, time.Now,
	).WithCapabilities(
		fillerenrichment.NewCapabilityRunner(
			fillerenrichment.CapabilityTranscript, fillerEnrichmentRepository{st: st},
			func() fillerenrichment.CapabilitySelection { return activeFillerTranscriptCapability(set) },
			fillerMediaExecutor{store: st, transcript: transcribeStage,
				kind: fillerenrichment.CapabilityTranscript}.Run,
			func() int { return set.intv("filler.pipeline.max_whisper") }, time.Now,
		),
		fillerenrichment.NewCapabilityRunner(
			fillerenrichment.CapabilityVision, fillerEnrichmentRepository{st: st},
			func() fillerenrichment.CapabilitySelection { return activeFillerVisionCapability(set) },
			fillerMediaExecutor{store: st, vision: visionStage,
				kind: fillerenrichment.CapabilityVision}.Run,
			func() int { return set.intv("filler.pipeline.max_vision") }, time.Now,
		),
	)
	selection := activeFillerTextSelection(set, metricRecorder)
	var researchRunner *fillerresearch.Runner
	if selection.Provider != nil && strings.TrimSpace(selection.Model) != "" {
		const researchUserAgent = "Loomarr/1.0 (https://github.com/loomarr/loomarr)"
		structuredClient := httpx.NewNamedObserved("filler_structured_research", httpx.TimeoutReference, metricRecorder)
		wiki, wikiErr := fillerresearch.NewMediaWiki(fillerresearch.MediaWikiConfig{
			Client: structuredClient, UserAgent: researchUserAgent,
		})
		wikidata, wikidataErr := fillerresearch.NewWikidata(fillerresearch.WikidataConfig{
			Client: structuredClient, UserAgent: researchUserAgent,
		})
		archive, archiveErr := fillerresearch.NewArchive(fillerresearch.ArchiveConfig{
			Client: structuredClient, UserAgent: researchUserAgent,
		})
		loc, locErr := fillerresearch.NewLOC(fillerresearch.LOCConfig{
			Client: structuredClient, UserAgent: researchUserAgent,
		})
		wikidataSource, wikidataSwitchErr := fillerresearch.NewSwitchable(wikidata,
			func() bool { return set.boolOn("filler.research.wikidata_enabled") })
		wikipediaSource, wikipediaSwitchErr := fillerresearch.NewSwitchable(wiki,
			func() bool { return set.boolOn("filler.research.wikipedia_enabled") })
		archiveSource, archiveSwitchErr := fillerresearch.NewSwitchable(archive,
			func() bool { return set.boolOn("filler.research.archive_enabled") })
		locSource, locSwitchErr := fillerresearch.NewSwitchable(loc,
			func() bool { return set.boolOn("filler.research.loc_enabled") })
		retriever, retrievalErr := fillerresearch.NewFederated(
			wikidataSource, wikipediaSource, archiveSource, locSource,
		)
		if err := errors.Join(wikiErr, wikidataErr, archiveErr, locErr, wikidataSwitchErr,
			wikipediaSwitchErr, archiveSwitchErr, locSwitchErr, retrievalErr); err != nil {
			log.Warn("could not construct filler context retrieval", "err", err)
		} else {
			web := fillerresearch.NewWeb(func() fillerresearch.WebConfig {
				return fillerresearch.WebConfig{
					Provider:     fillerresearch.WebProvider(set.str("filler.research.web_provider")),
					BraveAPIKey:  set.str("filler.research.brave_api_key"),
					SearXNGURL:   set.str("filler.research.searxng_url"),
					MonthlyLimit: set.intv("filler.research.monthly_limit"),
				}
			}, st, fillerresearch.WebOptions{
				Client: httpx.NewNamedObserved("filler_web_search", httpx.TimeoutReference, metricRecorder),
				Now:    time.Now,
			})
			researcher := fillerresearch.New(retriever, selection.Provider, selection.ProviderName,
				selection.Model, time.Now).WithFallback(web).WithStations(stations.resolveDuring(log))
			researchRunner = fillerresearch.NewRunner(fillerResearchRepository{st: st}, researcher,
				fillerResearchSignals{files: layout.FS()}.Load,
				func() int {
					structuredEnabled := set.boolOn("filler.research.wikidata_enabled") ||
						set.boolOn("filler.research.wikipedia_enabled") ||
						set.boolOn("filler.research.archive_enabled") ||
						set.boolOn("filler.research.loc_enabled")
					return fillerResearchRunLimit(set.boolOn("filler.research.enabled"), structuredEnabled,
						set.str("filler.research.web_provider"), set.intv("filler.pipeline.max_clips"))
				})
		}
	}
	local := &localStations{stations: stations,
		radiusKM: func() int { return set.intv("filler.local_radius_km") }, log: log}
	// The places' policy: the household's geography, region fallback included, so a fallback
	// country's national clips stay usable.
	placePolicy := func() filler.Policy {
		return filler.Policy{LocalStations: local.resolve, Geography: set.homeGeography()}
	}
	details := fillerDetailRunner{enrichment: enrichmentCoordinator, research: researchRunner,
		retire: &fillerGeographyRetirement{clips: st, store: st, log: log, now: time.Now, policy: placePolicy}}
	jobs.Add(fillerPipelineJob(newFillerPipelineDriver(pipeline, details, log)))
	adapter.pipeline = pipeline
	if decisionService != nil {
		decisionService.WithDiagnosticRecovery(adapter)
	}
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
	jobs.Add(fillerIdentityJob(filler.NewCreativeIdentity(
		fillerIdentityStoreAdapter{st}, buildFillerMediaTools(set, metricRecorder), layout.ClipDir(),
		func() float64 {
			// `filler.target_lufs` is a string whose empty value turns correction off; with no
			// target, NaN makes the loudness rung a tie for every pair rather than ranking toward 0.
			lufs, err := strconv.ParseFloat(set.str("filler.target_lufs"), 64)
			if err != nil {
				return math.NaN()
			}
			return lufs
		}, time.Now, log,
	)))
	ffmpegPath := set.str("playout.ffmpeg_path")
	judgeVision := &hotVisionProvider{set: set, log: log, metrics: metricRecorder}
	defectTools := buildFillerMediaTools(set, metricRecorder)
	defectJudge := filler.NewVisionDefectJudge(judgeVision, defectTools,
		fillerDefectJudgeLedger{st: st, set: set}, judgeVision.defectJudgeEndpoint, time.Now, log)
	jobs.Add(fillerDefectsJob(filler.NewDefectFloor(
		fillerDefectStoreAdapter{st}, filler.FFprobeNextTo(ffmpegPath),
		func(ctx context.Context, file string, width, height int, hasAudio bool) (defects.Readings, error) {
			return mediatools.MeasureDefects(ctx, ffmpegPath, file, width, height, hasAudio)
		},
		func(ctx context.Context, file string, durationMs int64) (creative.Fingerprint, error) {
			return defectTools.Fingerprint(ctx, file, 0, durationMs)
		}, defectJudge, layout.ClipDir(), time.Now, log,
	)))
	// Each Archive.org runtime is read once and kept beside the clips (#1773); with no filler
	// folder there is nothing to download into, and the cache lives in memory only.
	runtimesPath := ""
	if dir := layout.ClipDir(); dir != "" {
		runtimesPath = filepath.Join(dir, remoteRuntimesFile)
	}
	sourceEnumerator := registeredSourceEnumerator{
		youtube: clipfetch.NewYouTubeEnumerator(ytDlpPath),
		archive: cachedArchiveCatalog{archiveCatalog: clipfetch.NewArchiveDownloader(), runtimes: newRemoteRuntimeCache(runtimesPath, log)},
		// The compilation gate can only hold back a runtime it knows (#1773).
		archiveRuntimes: func() bool { return !set.boolv("filler.acquisition.compilations") },
	}
	autoFetch := filler.NewFetcher(
		fetchStoreAdapter{
			st:         st,
			fetchEvery: func() time.Duration { return set.dur("filler.fetch.every") },
			home:       set.homeGeography,
		},
		sourceEnumerator, adapter,
		filler.FetchLimits{
			MaxPerRun:         func() int { return set.intv("filler.fetch.max_per_run") },
			MaxProviderPerRun: func() int { return 50 },
			MaxCatalogClips:   func() int { return set.intv("filler.fetch.max_catalog_clips") },
			MinDuration:       func() time.Duration { return set.dur("filler.min_duration") },
			MaxDuration:       func() time.Duration { return set.dur("filler.autosplit.max_duration") },
			Compilations:      compilationGate(set),
		}, log,
	).WithCoverageGaps(func(ctx context.Context) (filler.CoverageGaps, error) {
		// The same per-channel coverage readiness and the pool strip report (#749), against a
		// target sized from the household's watched evenings (#2035).
		pool, err := result.preview.Pool(ctx)
		if err != nil {
			return filler.CoverageGaps{}, err
		}
		airings, err := st.FillerAirings(ctx)
		if err != nil {
			return filler.CoverageGaps{}, err
		}
		windows := make(map[string][]filler.EraRange, len(pool.Channels))
		for _, ch := range pool.Channels {
			windows[ch.ChannelID] = ch.Report.EraWindows
		}
		now := time.Now()
		spacing := time.Duration(set.intv("filler.cooldown_seconds")) * time.Second
		gaps := filler.CoverageGapsFrom(pool, filler.SizePool(airings, windows, spacing, now), now.Year())
		if !gaps.Met() {
			// A short pass also looks for the stations near each place, in the eras it is short of
			// (#2059 decision 9). Without them it still fetches; it just doesn't look for local.
			if near, err := nearbyPlaceStations(ctx, st, placePolicy()); err != nil {
				log.Warn("filler auto-fetch: nearby stations unavailable; not searching for them", "err", err)
			} else if len(near) > 0 {
				if directory, err := stations.get(ctx); err != nil {
					log.Warn("filler auto-fetch: broadcast stations unavailable; not searching for them", "err", err)
				} else {
					gaps.Stations = nearbyCallSigns(near, gaps.Eras, directory)
				}
			}
		}
		return gaps, nil
	})
	autoFetch.WithOpenSearch(openArchiveSearch{archive: clipfetch.NewArchiveDownloader(), channels: st,
		policy: placePolicy, stations: stations, languages: result.intakeLanguages})
	adapter.autoFetch = autoFetch
	result.service = adapter
	jobs.Add(fillerFetchJob(autoFetch, localCuratedSources{store: st, policy: placePolicy, log: log}))
	log.Info("filler auto-fetch registered", "every", set.dur("filler.fetch.every"),
		"max_per_run", set.intv("filler.fetch.max_per_run"))
	return result
}

func fillerResearchRunLimit(enabled, structuredEnabled bool, webProvider string, pipelineLimit int) int {
	provider := strings.TrimSpace(webProvider)
	if !enabled || (!structuredEnabled && (provider == "" || provider == string(fillerresearch.WebProviderNone))) {
		return 0
	}
	return max(0, pipelineLimit)
}
