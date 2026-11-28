package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	catalogapi "github.com/loomarr/loomarr/internal/api/catalog"
	channelsapi "github.com/loomarr/loomarr/internal/api/channels"
	guideapi "github.com/loomarr/loomarr/internal/api/guide"
	libraryapi "github.com/loomarr/loomarr/internal/api/library"
	proposalsapi "github.com/loomarr/loomarr/internal/api/proposals"
	"github.com/loomarr/loomarr/internal/api/system"
	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/channels/edit"
	"github.com/loomarr/loomarr/internal/channels/growth"
	"github.com/loomarr/loomarr/internal/channels/recurate"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/library/mediaserver"
	"github.com/loomarr/loomarr/internal/library/resolvers"
	"github.com/loomarr/loomarr/internal/library/titlesearch"
	"github.com/loomarr/loomarr/internal/library/tmdb"
	"github.com/loomarr/loomarr/internal/media/images"
	"github.com/loomarr/loomarr/internal/media/images/rustgen"
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
	"github.com/loomarr/loomarr/internal/playout/resolve"
	"github.com/loomarr/loomarr/internal/proposals/accept"
	"github.com/loomarr/loomarr/internal/proposals/suggest"
	"github.com/loomarr/loomarr/internal/proposals/webreference"
	"github.com/loomarr/loomarr/internal/proposals/workflow"
)

type suggestionBuild struct {
	suggest          api.SuggestService
	progress         proposalsapi.ProposalProgress // the same *suggest.Service, read for live generation progress
	workflow         proposalsapi.ProposalWorkflow
	durableWorkflow  *workflow.Workflow
	search           catalogapi.SearchService
	movieCollections libraryapi.MovieCollectionService
	collections      libraryapi.CollectionService
	ideaLibrary      channelsapi.IdeaLibrary
	systemLLM        system.SystemLLMService
	icons            channelsapi.IconService
	images           *images.Service
	imageWorker      *rustgen.Generator // the certified worker images uses; it also decodes subtitles
	imageFetcher     *images.Fetcher
	timelineThumbs   guideapi.TimelineThumbResolver
}

func buildSuggestions(
	rootCtx context.Context,
	st records.Store,
	set resolved,
	overrides Overrides,
	eventBus *events.Bus,
	emitter *eventEmitter,
	registry *jobs.Registry,
	fillerLayout cliplibrary.Layout,
	activityRecorder *activity.Recorder,
	log *slog.Logger,
	libraryClient *mediaserver.Client,
	tmdbClient *tmdb.Client,
	channelService guideapi.ChannelService,
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

	result.durableWorkflow = workflow.New(st, newID, time.Now, suggest.IntentHash)
	result.workflow = result.durableWorkflow

	var err error
	result.images, result.imageWorker, err = newImageService(
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
		rootCtx, registry, result.images, st, fillerLayout, set, activityRecorder, log,
	)
	result.timelineThumbs = resolve.NewTimelineThumbs(rootCtx, tmdbClient, result.images, result.imageFetcher, log)
	if engine, ok := channelService.(*channels.Engine); ok {
		engine.WithFranchises(resolvers.NewFranchises(tmdbClient))
	}

	catalogService := titlesearch.New(titlesearch.LibrarySearch{Titles: st, Facts: st}, tmdbClient).WithPresenceSource(func() titlesearch.LibraryPresence {
		return titlesearch.LibraryOwnership{Titles: st}
	}).WithEvidence(st)
	// A household member's search, collection rosters and channel ideas read only the titles a
	// member reads (docs/design/library.md "Who reads the Library"); the suggester, recurate and
	// every server-side reader keep catalogService and st.
	memberTitles := st.MemberTitles()
	memberCatalog := titlesearch.New(titlesearch.LibrarySearch{Titles: memberTitles, Facts: st}, tmdbClient).WithPresenceSource(func() titlesearch.LibraryPresence {
		return titlesearch.LibraryOwnership{Titles: memberTitles}
	}).WithEvidence(st)
	result.search = searchAdapter{cat: catalogService, member: memberCatalog}
	collectionResolver := tmdb.NewCollectionResolver(tmdbClient).WithPresenceSource(func() titlesearch.LibraryPresence {
		return titlesearch.LibraryOwnership{Titles: st}
	})
	memberCollectionResolver := tmdb.NewCollectionResolver(tmdbClient).WithPresenceSource(func() titlesearch.LibraryPresence {
		return titlesearch.LibraryOwnership{Titles: memberTitles}
	})
	result.movieCollections = movieCollectionAdapter{resolver: collectionResolver, member: memberCollectionResolver}
	result.collections = libraryCollections{own: st, items: st}
	result.ideaLibrary = ideaAudiences{
		all:    &libraryIdeaItems{lib: libraryIdeaTitles{titles: st, facts: st}, now: time.Now},
		member: &libraryIdeaItems{lib: libraryIdeaTitles{titles: memberTitles, facts: st}, now: time.Now},
	}
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
	service = service.WithAutoCurate(recurate.NewCurator(st, approver, recurate.NewSettingsThresholds(set), log))
	registry.Add(recurate.NewRunner(st, service, log).WithAdjacency(catalogService).Job())
	// Channels grow from the line their seeds draw (#4333), through the same approve → bind path.
	// A model is configured when the live selection names one; then unclear fits wait for review.
	hasModel := func() bool {
		if sw, ok := provider.(*llm.Swappable); ok {
			return sw.Model() != ""
		}
		return provider != nil
	}
	registry.Add(growth.NewFill(st, result.durableWorkflow, approver, hasModel, time.Now, log).Job())
	// The model reviews what channels grew by, in the background, yielding to a person waiting on it.
	var reconciler edit.Reconciler
	if r, ok := channelService.(edit.Reconciler); ok {
		reconciler = r
	}
	if review, ok := growthReview(st, provider, reconciler, interactiveLLM.Busy, set, log); ok {
		registry.Add(review.Job())
	}

	result.suggest = service
	result.progress = service
	result.systemLLM = systemLLM
	owner.goRun(func(ctx context.Context) { service.Run(ctx) })
	log.Info("suggester started", "provider", provider.Name(), "workers", settings.JobWorkers.Get(set),
		"tmdb_configured", settings.TMDBAPIKey.Get(set) != "")
	return result, nil
}
