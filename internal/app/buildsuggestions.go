package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	proposalsapi "github.com/loomarr/loomarr/internal/api/proposals"
	"github.com/loomarr/loomarr/internal/api/system"
	"github.com/loomarr/loomarr/internal/app/playoutresolve"
	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/channels/recurate"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/media/images"
	"github.com/loomarr/loomarr/internal/moviecollections"
	"github.com/loomarr/loomarr/internal/platform/activity"
	"github.com/loomarr/loomarr/internal/platform/buildinfo"
	"github.com/loomarr/loomarr/internal/platform/diagnostics"
	"github.com/loomarr/loomarr/internal/platform/events"
	"github.com/loomarr/loomarr/internal/platform/holidayvocab"
	"github.com/loomarr/loomarr/internal/platform/httpx"
	"github.com/loomarr/loomarr/internal/platform/jobs"
	"github.com/loomarr/loomarr/internal/platform/llm"
	"github.com/loomarr/loomarr/internal/platform/metrics"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/proposals/accept"
	"github.com/loomarr/loomarr/internal/proposals/suggest"
	"github.com/loomarr/loomarr/internal/proposals/webreference"
	"github.com/loomarr/loomarr/internal/proposals/workflow"
	"github.com/loomarr/loomarr/internal/tmdb"
)

type suggestionBuild struct {
	suggest          api.SuggestService
	progress         proposalsapi.ProposalProgress // the same *suggest.Service, read for live generation progress
	workflow         proposalsapi.ProposalWorkflow
	durableWorkflow  *workflow.Workflow
	search           api.SearchService
	movieCollections api.MovieCollectionService
	collections      api.CollectionService
	ideaLibrary      api.IdeaLibrary
	systemLLM        system.SystemLLMService
	icons            api.IconService
	images           *images.Service
	imageFetcher     *images.Fetcher
	timelineThumbs   api.TimelineThumbResolver
}

func buildSuggestions(
	rootCtx context.Context,
	st store.Store,
	set resolved,
	overrides Overrides,
	eventBus *events.Bus,
	emitter *eventEmitter,
	registry *jobs.Registry,
	fillerLayout cliplibrary.Layout,
	activityRecorder *activity.Recorder,
	log *slog.Logger,
	libraryClient *library.Client,
	tmdbClient *tmdb.Client,
	channelService api.ChannelService,
	approver *accept.Approver,
	owner *generationLifecycle,
	metricRecorder *metrics.Recorder,
	holidays holidayvocab.Source,
	interactiveLLM *llm.InteractiveGate,
) (suggestionBuild, error) {
	var result suggestionBuild
	if st == nil {
		overrides.Startup.Complete(diagnostics.StartupCheckImageWorker, diagnostics.StartupSkipped,
			"database unavailable", "/settings/system/diagnostics", "")
		return result, nil
	}

	result.durableWorkflow = workflow.New(st, newID, time.Now)
	result.workflow = result.durableWorkflow

	var err error
	result.images, err = newImageService(
		st, set, overrides.ImageWorkerExecutable, buildinfo.Get().Version, metricRecorder,
	)
	if err != nil {
		overrides.Startup.Complete(diagnostics.StartupCheckImageWorker, diagnostics.StartupFailed,
			"required image worker certification failed", "/settings/system/diagnostics", "")
		return suggestionBuild{}, err
	}
	overrides.Startup.Complete(diagnostics.StartupCheckImageWorker, diagnostics.StartupPassed,
		"required worker certified", "", "")
	result.imageFetcher = registerImageJobs(
		rootCtx, registry, result.images, imageStore{st}, fillerLayout, set, activityRecorder, log,
	)
	result.timelineThumbs = playoutresolve.NewTimelineThumbs(rootCtx, tmdbClient, result.images, result.imageFetcher, log)
	if engine, ok := channelService.(*channels.Engine); ok {
		engine.WithFranchises(tmdbFranchises{tmdb: tmdbClient})
	}

	catalogService := catalog.New(catalog.LibrarySearch{Titles: st, Facts: st}, tmdbClient).WithPresenceSource(func() catalog.LibraryPresence {
		return catalog.LibraryOwnership{Titles: st}
	})
	result.search = searchAdapter{catalogService}
	collectionResolver := moviecollections.New(tmdbClient).WithPresenceSource(func() catalog.LibraryPresence {
		return catalog.LibraryOwnership{Titles: st}
	})
	result.movieCollections = movieCollectionAdapter{resolver: collectionResolver}
	result.collections = libraryCollections{own: st}
	result.ideaLibrary = &libraryIdeaItems{lib: libraryIdeaTitles{titles: st, facts: st}, now: time.Now}
	result.icons = iconAdapter{
		store: st, tmdb: tmdbClient, images: result.images, fetch: result.imageFetcher, log: log,
	}

	provider, systemLLM := buildLLM(rootCtx, set, st, eventBus, log, metricRecorder, owner)
	if overrides.LLM != nil {
		provider = overrides.LLM
	}
	suggester := suggest.New(provider, catalogService, tmdbClient, settings.SuggestMaxAcquisitions.Get(set))
	suggester.WithRatings(tmdbClient).
		WithInteractiveGate(interactiveLLM).
		WithCalendar(householdCalendar(set, holidays, log)).
		WithMovieCollections(collectionResolver).
		WithFeedback(discoveryFeedbackSource{store: st}).
		WithReferences(webreference.NewWeb(httpx.NewPublicNamedObserved(httpx.TargetReference, httpx.TimeoutReference, metricRecorder)))
	service := suggest.NewService(st, result.durableWorkflow, suggester, suggest.Config{
		Workers: settings.JobWorkers.Get(set), Timeout: settings.JobTimeout.Get(set), CacheTTL: 24 * time.Hour,
	}, time.Now, log).
		WithProgressEmitter(emitter).
		WithProposalNotifier(emitter).
		WithQualityRecorder(st)
	service = service.WithAutoApprove(accept.NewAutoApprover(
		st, approver, func(context.Context) int { return settings.SuggestMaxAcquisitions.Get(set) }, log,
	))
	service = service.WithAutoCurate(recurate.NewCurator(st, approver, recurateThresholds{set}, log))
	registry.Add(recurate.NewRunner(st, service, log).WithAdjacency(catalogService).Job())

	result.suggest = service
	result.progress = service
	result.systemLLM = systemLLM
	owner.goRun(func(ctx context.Context) { service.Run(ctx) })
	log.Info("suggester started", "provider", provider.Name(), "workers", settings.JobWorkers.Get(set),
		"tmdb_configured", settings.TMDBAPIKey.Get(set) != "")
	return result, nil
}
