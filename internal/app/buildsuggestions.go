package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/mantonx/loomarr/internal/activity"
	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/buildinfo"
	"github.com/mantonx/loomarr/internal/catalog"
	"github.com/mantonx/loomarr/internal/channels"
	"github.com/mantonx/loomarr/internal/events"
	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/images"
	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/proposalworkflow"
	"github.com/mantonx/loomarr/internal/recurate"
	"github.com/mantonx/loomarr/internal/scheduler"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
	"github.com/mantonx/loomarr/internal/tmdb"
)

type suggestionBuild struct {
	suggest         api.SuggestService
	workflow        api.ProposalWorkflow
	durableWorkflow *proposalworkflow.Workflow
	search          api.SearchService
	collections     api.CollectionService
	systemLLM       api.SystemLLMService
	icons           api.IconService
	images          *images.Service
	imageFetcher    *images.Fetcher
	timelineThumbs  api.TimelineThumbResolver
}

func buildSuggestions(
	rootCtx context.Context,
	st store.Store,
	set resolved,
	overrides Overrides,
	eventBus *events.Bus,
	emitter *eventEmitter,
	jobs *scheduler.Registry,
	fillerLayout filler.Layout,
	activityRecorder *activity.Recorder,
	log *slog.Logger,
	libraryClient *library.Client,
	tmdbClient *tmdb.Client,
	channelService api.ChannelService,
	approver *suggest.Approver,
	owner *generationLifecycle,
) (suggestionBuild, error) {
	var result suggestionBuild
	if st == nil {
		return result, nil
	}

	result.durableWorkflow = proposalworkflow.New(st, newID, time.Now)
	result.workflow = result.durableWorkflow

	var err error
	result.images, err = newImageService(st, set, overrides.ImageWorkerExecutable, buildinfo.Get().Version)
	if err != nil {
		return suggestionBuild{}, err
	}
	result.imageFetcher = registerImageJobs(
		rootCtx, jobs, result.images, imageStore{st}, fillerLayout, set, activityRecorder, log,
	)
	result.timelineThumbs = timelineThumbResolver{
		tmdb: tmdbClient, images: result.images, fetch: result.imageFetcher,
	}
	if engine, ok := channelService.(*channels.Engine); ok {
		engine.WithFranchises(tmdbFranchises{tmdb: tmdbClient})
	}

	catalogService := catalog.New(libraryClient, tmdbClient).WithPresenceSource(func() catalog.LibraryPresence {
		return libraryPresence{lib: libraryClient.Snapshot()}
	})
	result.search = searchAdapter{catalogService}
	result.collections = libraryCollections{lib: libraryClient}
	result.icons = iconAdapter{
		store: st, tmdb: tmdbClient, images: result.images, fetch: result.imageFetcher, log: log,
	}

	provider, systemLLM := buildLLM(rootCtx, set, st, eventBus, log)
	if overrides.LLM != nil {
		provider = overrides.LLM
	}
	suggester := suggest.New(provider, catalogService, tmdbClient, set.intv("suggest.max_acquisitions"))
	suggester.WithRatings(tmdbClient)
	service := suggest.NewService(st, suggester, suggest.Config{
		Workers: set.intv("job.workers"), Timeout: set.dur("job.timeout"), CacheTTL: 24 * time.Hour,
	}, newID, time.Now, log).
		WithProgressEmitter(emitter).
		WithDurableWorkflow(result.durableWorkflow)
	service = service.WithAutoApprove(suggest.NewAutoApprover(
		st, approver, func(context.Context) int { return set.intv("suggest.max_acquisitions") }, log,
	))
	service = service.WithAutoCurate(recurate.NewCurator(st, approver, recurateThresholds{set}, log))
	jobs.Add(recurate.NewRunner(st, service, log).WithAdjacency(catalogService).Job())

	result.suggest = service
	result.systemLLM = systemLLM
	owner.goRun(func(ctx context.Context) { service.Run(ctx) })
	log.Info("suggester started", "provider", provider.Name(), "workers", set.intv("job.workers"),
		"tmdb_configured", set.str("tmdb.api_key") != "")
	return result, nil
}
