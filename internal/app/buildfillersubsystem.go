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
	"github.com/loomarr/loomarr/internal/app/playoutresolve"
	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/defectfloor"
	"github.com/loomarr/loomarr/internal/filler/enrichment"
	"github.com/loomarr/loomarr/internal/filler/fetch"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/filler/research"
	"github.com/loomarr/loomarr/internal/filler/source"
	archiveprovider "github.com/loomarr/loomarr/internal/filler/source/archive"
	youtubeprovider "github.com/loomarr/loomarr/internal/filler/source/youtube"
	"github.com/loomarr/loomarr/internal/filler/station"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/media/creative"
	"github.com/loomarr/loomarr/internal/media/defects"
	"github.com/loomarr/loomarr/internal/media/images"
	"github.com/loomarr/loomarr/internal/media/tools"
	"github.com/loomarr/loomarr/internal/platform/diagnostics"
	"github.com/loomarr/loomarr/internal/platform/events"
	"github.com/loomarr/loomarr/internal/platform/httpx"
	"github.com/loomarr/loomarr/internal/platform/jobs"
	"github.com/loomarr/loomarr/internal/platform/llm"
	"github.com/loomarr/loomarr/internal/platform/metrics"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/storagegovernor"
	"github.com/loomarr/loomarr/internal/playout/tunarr"
)

type fillerBuild struct {
	service  api.FillerService
	defects  *defectfloor.DefectReports
	preview  api.PodPreviewer
	taxonomy api.TaxonomyEditor
	research api.FillerResearchService
	// providers is every provider a filler source can name; the API publishes it and the fetcher
	// and the service adapter dispatch through it.
	providers *source.Registry
	// intakeLanguages is the language gate's live wanted set, for split review to compare against.
	intakeLanguages func(context.Context) (string, error)
}

// compilationGate reads `filler.acquisition.compilations` against the same single-clip ceiling
// probe marks compilations by, both live on every call (#1773).
func compilationGate(set resolved) filler.CompilationGate {
	return filler.CompilationGate{
		Take: func() bool { return settings.FillerAcquisitionCompilations.Get(set) },
		Over: func() time.Duration { return settings.FillerAutosplitMaxDuration.Get(set) },
	}
}

func buildFillerSubsystem(
	st records.Store,
	set resolved,
	layout cliplibrary.Layout,
	log *slog.Logger,
	libraryClient *library.Client,
	eventBus *events.Bus,
	emitter *eventEmitter,
	registry *jobs.Registry,
	playoutResolver *playoutresolve.Resolver,
	channelService api.ChannelService,
	processDiagnostics *diagnostics.ProcessManager,
	storageGovernor *storagegovernor.Governor,
	metricRecorder *metrics.Recorder,
	imageService *images.Service,
	playbackHeadroom filler.PlaybackHeadroom,
	owner *generationLifecycle,
	holidaysChanged func(context.Context),
	calendar func() schedule.Calendar,
	interactiveLLM *llm.InteractiveGate,
) fillerBuild {
	var result fillerBuild
	if st == nil {
		return result
	}
	result.research = newFillerResearchSettingsAdapter(st, set, metricRecorder)
	result.intakeLanguages = intakeLanguages(st, set)
	result.defects = defectfloor.NewDefectReports(fillerDefectStoreAdapter{st}, time.Now)
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
	if recovered, err := fetch.RecoverAcquisitionArtifacts(artifactRecoveryCtx, layout.WatchDir(), layout.ClipDir(), st, time.Now); err != nil {
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
	ytDlpPath := resolveTool(settings.IngestYtdlpPath.Get(set), "yt-dlp")
	// Each Archive.org runtime is read once and kept beside the clips (#1773); with no filler
	// folder there is nothing to download into, and the cache lives in memory only.
	runtimesPath := ""
	if dir := layout.ClipDir(); dir != "" {
		runtimesPath = filepath.Join(dir, remoteRuntimesFile)
	}
	providers := fillerSourceProviders(ytDlpPath,
		cachedArchiveCatalog{Catalog: fetch.NewArchiveDownloader(), runtimes: newRemoteRuntimeCache(runtimesPath, log)},
		// The compilation gate can only hold back a runtime it knows (#1773).
		func() bool { return !settings.FillerAcquisitionCompilations.Get(set) }, log)
	result.providers = providers
	adapter := fillerServiceAdapter{
		providers: providers,
		syncer:    syncer, fetcher: fetcher,
		bus: eventBus, log: log, newID: newID, timeout: settings.IngestTimeout.Get(set),
		start: owner.startInteractiveOperation, operations: st,
		sources: st, acquisitions: st, readiness: st, now: time.Now,
		home:     set.homeGeography,
		splitter: splitter, splitClips: fillerSplitStoreAdapter{st: st, wake: wake},
		storage: storageGovernor, storagePath: layout.ClipDir(),
		storageAutomatic: func() bool { return settings.FillerStorageLibraryBudgetGb.Get(set) == 0 },
		storageCleanup:   fetch.NewAcquisitionCleaner(layout.WatchDir(), st, time.Now),
	}

	pods := buildPodAdapter(st, set, calendar, log).WithMetrics(metricRecorder)
	result.preview = podPreviewAdapter{store: st, pods: pods}
	adapter.pool = result.preview.Pool
	if playoutResolver != nil {
		playoutResolver.SetPods(result.preview)
	}
	if engine, ok := channelService.(*channels.Engine); ok {
		engine.WithPods(pods)
		log.Info("filler pod assembler wired into the scheduler")
	}

	registry.Add(fillerSyncJob(syncer))
	log.Info("filler catalog sync registered", "dir", layout.ClipDir(),
		"every", settings.FillerSyncEvery.Get(set))
	ffmpegPath := settings.PlayoutFFmpegPath.Get(set)
	judgeVision := &hotVisionProvider{set: set, log: log, metrics: metricRecorder}
	defectTools := buildFillerMediaTools(set, metricRecorder)
	defectJudge := defectfloor.NewVisionDefectJudge(judgeVision, defectTools,
		fillerDefectJudgeLedger{st: st, set: set}, judgeVision.defectJudgeEndpoint, time.Now, log)
	defectFloor := defectfloor.NewDefectFloor(
		fillerDefectStoreAdapter{st}, cliplibrary.FFprobeNextTo(ffmpegPath),
		func(ctx context.Context, file string, width, height int, hasAudio bool) (defects.Readings, error) {
			return tools.MeasureDefects(ctx, ffmpegPath, file, width, height, hasAudio)
		},
		func(ctx context.Context, file string, durationMs int64) (creative.Fingerprint, error) {
			return defectTools.Fingerprint(ctx, file, 0, durationMs)
		}, defectJudge, layout.ClipDir(), time.Now, log,
	)
	pipeline, transcribeStage, visionStage := buildPipeline(st, set, layout, log, emitter, splitter, wake,
		processDiagnostics, storageGovernor, metricRecorder, playbackHeadroom, playoutResolver,
		defectFloor.CheckReplacement, interactiveLLM)
	stations := station.NewLazy(st.ListBroadcastStations)
	enrichmentSignals := fillerEnrichmentSignals{store: st, files: layout.FS(), stations: stations}
	enrichmentRunner := enrichment.NewRunner(
		fillerEnrichmentRepository{st: st},
		enrichmentSignals.Load,
		func() int { return settings.FillerPipelineMaxClips.Get(set) }, time.Now,
	)
	enrichmentCoordinator := enrichment.NewCoordinator(
		enrichmentRunner, fillerEnrichmentRepository{st: st},
		func() enrichment.TextSelection { return activeFillerTextSelection(set, metricRecorder) },
		enrichmentSignals.Load,
		func() int { return settings.FillerPipelineMaxClips.Get(set) }, time.Now,
	).WithCapabilities(
		enrichment.NewCapabilityRunner(
			enrichment.CapabilityTranscript, fillerEnrichmentRepository{st: st},
			func() enrichment.CapabilitySelection { return activeFillerTranscriptCapability(set) },
			fillerMediaExecutor{store: st, transcript: transcribeStage,
				kind: enrichment.CapabilityTranscript}.Run,
			func() int { return settings.FillerPipelineMaxWhisper.Get(set) }, time.Now,
		),
		enrichment.NewCapabilityRunner(
			enrichment.CapabilityVision, fillerEnrichmentRepository{st: st},
			func() enrichment.CapabilitySelection { return activeFillerVisionCapability(set) },
			fillerMediaExecutor{store: st, vision: visionStage,
				kind: enrichment.CapabilityVision}.Run,
			func() int { return settings.FillerPipelineMaxVision.Get(set) }, time.Now,
		),
	)
	selection := activeFillerTextSelection(set, metricRecorder)
	var researchRunner *research.Runner
	if selection.Provider != nil && strings.TrimSpace(selection.Model) != "" {
		const researchUserAgent = "Loomarr/1.0 (https://github.com/loomarr/loomarr)"
		structuredClient := httpx.NewNamedObserved(httpx.TargetFillerStructuredResearch, httpx.TimeoutReference, metricRecorder)
		wiki, wikiErr := research.NewMediaWiki(research.MediaWikiConfig{
			Client: structuredClient, UserAgent: researchUserAgent,
		})
		wikidata, wikidataErr := research.NewWikidata(research.WikidataConfig{
			Client: structuredClient, UserAgent: researchUserAgent,
		})
		archive, archiveErr := research.NewArchive(research.ArchiveConfig{
			Client: structuredClient, UserAgent: researchUserAgent,
		})
		loc, locErr := research.NewLOC(research.LOCConfig{
			Client: structuredClient, UserAgent: researchUserAgent,
		})
		wikidataSource, wikidataSwitchErr := research.NewSwitchable(wikidata,
			func() bool { return settings.FillerResearchWikidataEnabled.Get(set) })
		wikipediaSource, wikipediaSwitchErr := research.NewSwitchable(wiki,
			func() bool { return settings.FillerResearchWikipediaEnabled.Get(set) })
		archiveSource, archiveSwitchErr := research.NewSwitchable(archive,
			func() bool { return settings.FillerResearchArchiveEnabled.Get(set) })
		locSource, locSwitchErr := research.NewSwitchable(loc,
			func() bool { return settings.FillerResearchLocEnabled.Get(set) })
		retriever, retrievalErr := research.NewFederated(
			wikidataSource, wikipediaSource, archiveSource, locSource,
		)
		if err := errors.Join(wikiErr, wikidataErr, archiveErr, locErr, wikidataSwitchErr,
			wikipediaSwitchErr, archiveSwitchErr, locSwitchErr, retrievalErr); err != nil {
			log.Warn("could not construct filler context retrieval", "err", err)
		} else {
			web := research.NewWeb(func() research.WebConfig {
				return research.WebConfig{
					Provider:     research.WebProvider(settings.FillerResearchWebProvider.Get(set)),
					BraveAPIKey:  settings.FillerResearchBraveAPIKey.Get(set),
					SearXNGURL:   settings.FillerResearchSearxngURL.Get(set),
					MonthlyLimit: settings.FillerResearchMonthlyLimit.Get(set),
				}
			}, st, research.WebOptions{
				Client: httpx.NewNamedObserved(httpx.TargetFillerWebSearch, httpx.TimeoutReference, metricRecorder),
				Now:    time.Now,
			})
			researcher := research.New(retriever, selection.Provider, selection.ProviderName,
				selection.Model, time.Now).WithFallback(web).WithStations(stations.ResolveDuring(log))
			researchRunner = research.NewRunner(fillerResearchRepository{st: st}, researcher,
				fillerResearchSignals{files: layout.FS()}.Load,
				func() int {
					structuredEnabled := settings.FillerResearchWikidataEnabled.Get(set) ||
						settings.FillerResearchWikipediaEnabled.Get(set) ||
						settings.FillerResearchArchiveEnabled.Get(set) ||
						settings.FillerResearchLocEnabled.Get(set)
					return fillerResearchRunLimit(settings.FillerResearchEnabled.Get(set), structuredEnabled,
						settings.FillerResearchWebProvider.Get(set), settings.FillerPipelineMaxClips.Get(set))
				})
		}
	}
	local := &station.LocalResolver{Stations: stations,
		RadiusKM: func() int { return settings.FillerLocalRadiusKm.Get(set) }, Log: log}
	// The places' policy: the household's geography, region fallback included, so a fallback
	// country's national clips stay usable.
	placePolicy := func() pod.Policy {
		return pod.Policy{LocalStations: local.Resolve, Geography: set.homeGeography()}
	}
	details := fillerDetailRunner{enrichment: enrichmentCoordinator, research: researchRunner,
		retire: &fillerGeographyRetirement{clips: st, store: st, log: log, now: time.Now, policy: placePolicy}}
	registry.Add(fillerPipelineJob(newFillerPipelineDriver(pipeline, details, log)))
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
	registry.Add(fillerSplitSweepJob(filler.NewSplitSweeper(
		fillerSweepStoreAdapter{st}, layout.ClipDir(),
		func() time.Duration { return settings.FillerSplitReviewWindow.Get(set) }, time.Now, log,
	)))
	registry.Add(fillerIdentityJob(filler.NewCreativeIdentity(
		fillerIdentityStoreAdapter{st}, buildFillerMediaTools(set, metricRecorder), layout.ClipDir(),
		func() float64 {
			// `filler.target_lufs` is a string whose empty value turns correction off; with no
			// target, NaN makes the loudness rung a tie for every pair rather than ranking toward 0.
			lufs, err := strconv.ParseFloat(settings.FillerTargetLufs.Get(set), 64)
			if err != nil {
				return math.NaN()
			}
			return lufs
		}, time.Now, log,
	)))
	registry.Add(fillerDefectsJob(defectFloor))
	autoFetch := filler.NewFetcher(
		fetchStoreAdapter{
			st:         st,
			fetchEvery: func() time.Duration { return settings.FillerFetchEvery.Get(set) },
			home:       set.homeGeography,
		},
		providers, adapter,
		filler.FetchLimits{
			MaxPerRun:         func() int { return settings.FillerFetchMaxPerRun.Get(set) },
			MaxProviderPerRun: func() int { return 50 },
			MaxCatalogClips:   func() int { return settings.FillerFetchMaxCatalogClips.Get(set) },
			MinDuration:       func() time.Duration { return settings.FillerMinDuration.Get(set) },
			MaxDuration:       func() time.Duration { return settings.FillerAutosplitMaxDuration.Get(set) },
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
		windows := make(map[string][]pod.EraRange, len(pool.Channels))
		for _, ch := range pool.Channels {
			windows[ch.ChannelID] = ch.Report.EraWindows
		}
		now := time.Now()
		spacing := time.Duration(settings.FillerCooldownSeconds.Get(set)) * time.Second
		gaps := filler.CoverageGapsFrom(pool, pod.SizePool(airings, windows, spacing, now), now.Year())
		if !gaps.Met() {
			// A short pass also looks for the stations near each place, in the eras it is short of
			// (#2059 decision 9). Without them it still fetches; it just doesn't look for local.
			if near, err := nearbyPlaceStations(ctx, st, placePolicy()); err != nil {
				log.Warn("filler auto-fetch: nearby stations unavailable; not searching for them", "err", err)
			} else if len(near) > 0 {
				if directory, err := stations.Get(ctx); err != nil {
					log.Warn("filler auto-fetch: broadcast stations unavailable; not searching for them", "err", err)
				} else {
					gaps.Stations = nearbyCallSigns(near, gaps.Eras, directory)
				}
			}
		}
		return gaps, nil
	})
	autoFetch.WithOpenSearch(openArchiveSearch{archive: fetch.NewArchiveDownloader(), channels: st,
		policy: placePolicy, stations: stations, languages: result.intakeLanguages})
	adapter.autoFetch = autoFetch
	result.service = adapter
	registry.Add(fillerFetchJob(autoFetch, localCuratedSources{store: st, policy: placePolicy, log: log}))
	log.Info("filler auto-fetch registered", "every", settings.FillerFetchEvery.Get(set),
		"max_per_run", settings.FillerFetchMaxPerRun.Get(set))
	return result
}

// fillerSourceProviders is every provider a filler source can name. YouTube and archive.org are
// fetched; the folder and the media library are scanned where they are. A source whose provider is
// missing here is inert, so adding a provider is its package plus one line below. The order is the
// order the source list shows them in.
func fillerSourceProviders(
	ytDlpPath string, archiveCatalog archiveprovider.Catalog, archiveRuntimes func() bool, log *slog.Logger,
) *source.Registry {
	youtube := youtubeprovider.New(youtubeprovider.YtDlp(fetch.NewYouTubeEnumerator(ytDlpPath)))
	if ytDlpPath != "" {
		youtube.WithFinder(fetch.NewYouTubeSourceFinder(ytDlpPath))
	}
	providers, err := source.NewRegistry(
		archiveprovider.New(archiveprovider.Config{Catalog: archiveCatalog, Runtimes: archiveRuntimes, Log: log}),
		youtube,
		// Library before Folder: the add form offers local providers in registry order, and a
		// media-server library is the common case.
		source.Library(),
		source.Folder(),
	)
	if err != nil {
		// The IDs are fixed above, so this is a programming error, never an installation's.
		panic(err)
	}
	return providers
}

func fillerResearchRunLimit(enabled, structuredEnabled bool, webProvider string, pipelineLimit int) int {
	provider := strings.TrimSpace(webProvider)
	if !enabled || (!structuredEnabled && (provider == "" || provider == string(research.WebProviderNone))) {
		return 0
	}
	return max(0, pipelineLimit)
}
