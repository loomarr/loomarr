// Command loomarr is the single-binary media-channel builder (design §2, §21).
// Phase 1 wires config, structured logging, and an HTTP server exposing /healthz
// and /readyz with graceful shutdown. Subsystems are added in later phases.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/auth"
	"github.com/mantonx/loomarr/internal/catalog"
	"github.com/mantonx/loomarr/internal/channels"
	"github.com/mantonx/loomarr/internal/config"
	"github.com/mantonx/loomarr/internal/events"
	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/ingest"
	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/llm"
	"github.com/mantonx/loomarr/internal/programmer"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/reconcile"
	"github.com/mantonx/loomarr/internal/requester"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/setup"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
	"github.com/mantonx/loomarr/internal/tmdb"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := newLogger(cfg.LogLevel)
	slog.SetDefault(log)
	log.Info("loomarr starting", "listen", cfg.ListenAddr, "log_level", cfg.LogLevel)

	// Open the store (§5): backend chosen by DATABASE_URL scheme, migrations run
	// on startup when AUTO_MIGRATE. If unset, run without a store for now (later
	// phases require it; readiness reflects the truth either way).
	var st store.Store
	if cfg.DatabaseURL != "" {
		st, err = store.Open(context.Background(), cfg.DatabaseURL, cfg.AutoMigrate)
		if err != nil {
			return err // downgrade guard / bad scheme / unreachable DB fail fast
		}
		defer func() { _ = st.Close() }()
		log.Info("store opened", "auto_migrate", cfg.AutoMigrate)
	} else {
		log.Warn("no DATABASE_URL set — running without a store (not ready)")
	}

	// Readiness is true only once the store is connected + migrated (§17).
	ready := func() (bool, string) {
		if st == nil {
			return false, "no store configured"
		}
		if _, err := st.GetSetting(context.Background(), "healthcheck_probe"); err != nil && err != store.ErrNotFound {
			return false, "store unreachable: " + err.Error()
		}
		return true, "ok"
	}

	// Wire the ingest webhook handler when the store + library are configured
	// (§6, Phase 6). Requires a store, the library flavor, and WEBHOOK_SECRET.
	var ingestHandler http.Handler
	if st != nil && cfg.LibraryFlavor != "" && cfg.WebhookSecret != "" {
		flavor, ferr := library.ParseFlavor(cfg.LibraryFlavor)
		if ferr != nil {
			return ferr
		}
		deviceID := instanceDeviceID(context.Background(), st)
		lib := library.New(flavor, cfg.LibraryURL, cfg.LibraryToken, deviceID)
		ingestHandler = ingest.New(st, lib, cfg.WebhookSecret, cfg.DownloadingTTL, time.Now, log)
		log.Info("ingest webhook mounted", "path", "/hooks/arr")
	}

	// Start the provisioning reconciler + janitor loop (§7) when the store and a
	// requester/library are configured. It runs until ctx (shutdown) is done.
	rootCtx, cancelRoot := context.WithCancel(context.Background())
	defer cancelRoot()
	if st != nil && cfg.LibraryFlavor != "" && cfg.SeerrURL != "" {
		flavor, _ := library.ParseFlavor(cfg.LibraryFlavor)
		lib := library.New(flavor, cfg.LibraryURL, cfg.LibraryToken, instanceDeviceID(rootCtx, st))
		req := requester.NewSeerr(cfg.SeerrURL, cfg.SeerrAPIKey)
		rec := reconcile.New(st, req, lib, nil, reconcile.Config{
			RequestTTL: cfg.RequestTTL, DownloadingTTL: cfg.DownloadingTTL,
		}, time.Now, log)
		// Janitor sweeps expired sessions (§5, §11); jobs/proposals join in P11.
		janitor := reconcile.NewJanitor(log, time.Now, auth.NewSessionSweeper(st))
		runner := reconcile.NewRunner(rec, janitor, cfg.ReconcileEvery, log)
		go runner.Run(rootCtx)
	}

	// Scheduler + Tunarr (§9, Phase 10): the channel reconcile engine + periodic
	// sweep, plus the Live TV wiring connector (guide-refresh poker). Wired when a
	// store, Tunarr, and the media-server library are configured.
	var channelSvc api.ChannelService
	var liveTVSvc api.LiveTVService
	if st != nil && cfg.TunarrURL != "" && cfg.LibraryFlavor != "" {
		flavor, _ := library.ParseFlavor(cfg.LibraryFlavor)
		lib := library.New(flavor, cfg.LibraryURL, cfg.LibraryToken, instanceDeviceID(rootCtx, st))
		prog := programmer.New(cfg.TunarrURL, cfg.TunarrAPIKey, cfg.TunarrTranscodeID)
		connector := setup.NewLiveTVConnector(lib, setup.TunarrURLsFrom(cfg.TunarrURL))
		liveTVSvc = liveTVAdapter{connector}

		// Program duration comes from the media server (§9/§10): give the scheduler
		// a resolver so program slots carry a real runtime before the Tunarr push.
		avail := channels.NewStoreAvailability(rootCtx, st, lib.ItemDurationMs)
		engine := channels.New(st, prog, avail, connector, channels.Config{
			// Pending-slot policy defaults to pod-fill (§9); the interstitial-card
			// alternative is future config. SCHED_BACKFILL gates reshuffle-vs-stable
			// placement, handled inside the engine, not the placeholder kind.
			Policy: schedule.PodFill, ReconcileTTL: cfg.ChannelReconcileEvery,
		}, time.Now, log)
		channelSvc = engine

		sweep := channels.NewRunner(engine, st, cfg.ChannelReconcileEvery, 2*cfg.ChannelReconcileEvery, 50, time.Now, log)
		go sweep.Run(rootCtx)
		log.Info("channel scheduler started", "tunarr", cfg.TunarrURL, "sweep_every", cfg.ChannelReconcileEvery)
	}

	// Suggester + search (§8, Phase 11): the catalog boundary (library + TMDB),
	// the LLM provider, the grounded suggester, and the worker pool. Wired when a
	// store, the library, and the LLM are configured. The catalog + search share
	// one impl (§7.2).
	var suggestSvc api.SuggestService
	var searchSvc api.SearchService
	eventBus := events.NewBus()
	if st != nil && cfg.LibraryFlavor != "" && cfg.LLMURL != "" {
		flavor, _ := library.ParseFlavor(cfg.LibraryFlavor)
		lib := library.New(flavor, cfg.LibraryURL, cfg.LibraryToken, instanceDeviceID(rootCtx, st))
		var tmdbClient *tmdb.Client
		if cfg.TMDBAPIKey != "" {
			tmdbClient = tmdb.New(cfg.TMDBAPIKey)
		}
		// catalog.New accepts nil corpora; a nil *tmdb.Client would be a non-nil
		// interface, so pass the concrete nil only when configured.
		var tmdbSearcher catalog.TMDBSearcher
		var validator suggest.Validator
		if tmdbClient != nil {
			tmdbSearcher = tmdbClient
			validator = tmdbClient
		} else {
			validator = noopValidator{} // no TMDB: acquisitions can't be re-validated, so treat as exists
		}
		cat := catalog.New(lib, tmdbSearcher, nil) // clip corpus wired in Phase 12
		searchSvc = searchAdapter{cat}

		provider := llm.NewOllama(cfg.LLMURL, cfg.LLMModel) // Anthropic provider is opt-in (Phase 11 default ollama)
		sug := suggest.New(provider, cat, validator, cfg.SuggestMaxAcquire)
		svc := suggest.NewService(st, sug, suggest.Config{
			Workers: cfg.JobWorkers, Timeout: cfg.JobTimeout, CacheTTL: 24 * time.Hour,
		}, newID, time.Now, log)
		suggestSvc = submitAdapter{svc}
		go svc.Run(rootCtx)
		log.Info("suggester started", "provider", provider.Name(), "workers", cfg.JobWorkers, "tmdb", tmdbClient != nil)
	}

	// Filler & commercials (§10, Phase 12): catalog sync from the media server's
	// filler library + a periodic sweep, the AI-tagging job, and the pod assembler
	// wired into the channel scheduler. Wired when a store, the library, and a
	// FILLER_LIBRARY are configured. The pod adapter (if the scheduler is up) turns
	// filler gaps into matched ad pods.
	var fillerSvc api.FillerService
	if st != nil && cfg.LibraryFlavor != "" && cfg.FillerLibrary != "" {
		flavor, _ := library.ParseFlavor(cfg.LibraryFlavor)
		lib := library.New(flavor, cfg.LibraryURL, cfg.LibraryToken, instanceDeviceID(rootCtx, st))
		syncer := filler.NewSyncer(fillerListerAdapter{lib}, fillerStoreAdapter{st}, cfg.FillerLibrary, time.Now, log)

		var tagger *filler.Tagger
		if cfg.FillerAITagging && cfg.LLMURL != "" {
			provider := llm.NewOllama(cfg.LLMURL, cfg.LLMModel)
			tagger = filler.NewTagger(fillerTagStoreAdapter{st}, provider, time.Now, log)
		}
		fillerSvc = fillerServiceAdapter{syncer: syncer, tagger: tagger}

		// Pod assembler → scheduler (§10). If the channel engine is up, teach it to
		// fill filler gaps with matched pods.
		if engine, ok := channelSvc.(*channels.Engine); ok {
			podPolicy := filler.Policy{}
			engine.WithPods(filler.NewPodAdapter(clipCatalogAdapter{st}, podPolicy, log))
			log.Info("filler pod assembler wired into the scheduler")
		}

		// Periodic filler catalog sync alongside the reconciler (§10).
		go runFillerSync(rootCtx, syncer, cfg.FillerSyncEvery, log)
		log.Info("filler catalog sync started", "library", cfg.FillerLibrary, "every", cfg.FillerSyncEvery, "ai_tagging", cfg.FillerAITagging)
	}

	// API server (§7): titles + ops + OpenAPI/docs + backup + auth/users (§11).
	// Session auth (cookie) with API_TOKEN Bearer break-glass; SQLite gets a
	// backup streamer, Postgres passes nil (→ 501).
	var backup api.BackupStreamer
	if st != nil {
		if b := store.SQLiteBackuper(st); b != nil {
			backup = b
		}
	}
	authorizer := api.Authorizer(api.NewTokenAuthorizer(cfg.APIToken))
	var loginSvc api.LoginService
	var sessMgr api.SessionManager
	var userSync api.UserSyncer
	if st != nil && cfg.LibraryFlavor != "" {
		flavor, _ := library.ParseFlavor(cfg.LibraryFlavor)
		lib := library.New(flavor, cfg.LibraryURL, cfg.LibraryToken, instanceDeviceID(rootCtx, st))
		mgr := auth.NewManager(st, cfg.SessionTTL, time.Now)
		limiter := auth.NewRateLimiter(0.2, 5) // ~5 attempts, refill 1/5s (§11)
		loginSvc = auth.NewLoginService(lib, st, mgr, limiter, time.Now)
		sessMgr = mgr
		userSync = auth.NewUserSync(lib, st, time.Now)
		authorizer = api.NewSessionAuthorizer(mgr, cfg.APIToken)
	}
	srv := &http.Server{
		Addr: cfg.ListenAddr,
		Handler: api.Router(log, api.Options{
			Store:        st,
			Auth:         authorizer,
			Log:          log,
			BackupSQLite: backup,
			Ingest:       ingestHandler,
			Ready:        ready,
			Login:        loginSvc,
			Sessions:     sessMgr,
			UserSync:     userSync,
			CookieSecure: cfg.CookieSecure,
			Channels:     channelSvc,
			LiveTV:       liveTVSvc,
			Suggest:      suggestSvc,
			Search:       searchSvc,
			Events:       eventBus,
			Filler:       fillerSvc,
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Run the server; surface a listen error via this channel.
	serverErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// Block until a signal or a server error.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		log.Info("shutdown signal received")
	}

	// Signal the reconciler/janitor loop to stop (§7) alongside the HTTP drain,
	// so background work halts cleanly rather than being abandoned at exit.
	cancelRoot()

	// Graceful drain.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	log.Info("loomarr stopped cleanly")
	return nil
}

// liveTVAdapter adapts setup.LiveTVConnector to the api.LiveTVService interface
// (Connect returns a struct there, a tuple here). Thin, wiring-only.
type liveTVAdapter struct{ c *setup.LiveTVConnector }

func (a liveTVAdapter) Connect(ctx context.Context) (bool, bool, error) {
	res, err := a.c.Connect(ctx)
	return res.TunerAdded, res.ListingAdded, err
}
func (a liveTVAdapter) Wired(ctx context.Context) (bool, error) { return a.c.Wired(ctx) }

// submitAdapter maps suggest.Service to api.SuggestService (the API interface
// takes flat args to stay dependency-light; here we rebuild the Intent).
type submitAdapter struct{ svc *suggest.Service }

func (a submitAdapter) Submit(ctx context.Context, desc, era, tone string, inc, exc []string, max int, createdBy string) (string, error) {
	return a.svc.Submit(ctx, suggest.Intent{
		Description: desc, Era: era, Tone: tone,
		MustInclude: inc, MustExclude: exc, MaxAcquire: max,
	}, createdBy)
}

// searchAdapter maps catalog.Catalog to api.SearchService (converts candidates
// to the API's dependency-light shape).
type searchAdapter struct{ cat *catalog.Catalog }

func (a searchAdapter) Search(ctx context.Context, q, scope string, limit int) ([]api.SearchCandidate, error) {
	cands, err := a.cat.Search(ctx, q, catalog.ParseScope(scope), limit)
	if err != nil {
		return nil, err
	}
	out := make([]api.SearchCandidate, 0, len(cands))
	for _, c := range cands {
		out = append(out, api.SearchCandidate{
			MediaType: string(c.MediaType), TMDBID: c.TMDBID, TVDBID: c.TVDBID,
			Name: c.Name, Year: c.Year, InLibrary: c.InLibrary, LibraryItemID: c.LibraryItemID,
		})
	}
	return out, nil
}

// noopValidator is the acquisition validator when TMDB isn't configured: it can't
// re-check existence, so it treats every id as existing. The grounding guarantee
// (the id was surfaced by the catalog tool) still holds; only the belt-and-
// suspenders TMDB exists-check is skipped. Configure TMDB_API_KEY to enable it.
type noopValidator struct{}

func (noopValidator) Exists(context.Context, provision.MediaType, int) (bool, error) {
	return true, nil
}

// --- filler bridging adapters (§10) ---
// These translate between the store's Clip/FillerClip types and the filler
// package's port types, keeping filler free of a store/library import (the domain
// stays pure; main does the wiring).

// fillerListerAdapter bridges library.ListFillerClips → filler.Lister.
type fillerListerAdapter struct{ lib *library.Client }

func (a fillerListerAdapter) ListFillerClips(ctx context.Context, libraryID string) ([]filler.RawClip, error) {
	raw, err := a.lib.ListFillerClips(ctx, libraryID)
	if err != nil {
		return nil, err
	}
	out := make([]filler.RawClip, len(raw))
	for i, r := range raw {
		out[i] = filler.RawClip{LibraryItemID: r.LibraryItemID, Name: r.Name, DurationMs: r.DurationMs, Kind: r.Kind, Era: r.Era}
	}
	return out, nil
}

// fillerStoreAdapter bridges the store's clip methods → filler.Store (the sync).
type fillerStoreAdapter struct{ st store.Store }

func (a fillerStoreAdapter) UpsertClip(ctx context.Context, c filler.StoreClip) error {
	return a.st.UpsertClip(ctx, store.Clip{Clip: c.Clip, UpdatedAt: c.UpdatedAt})
}
func (a fillerStoreAdapter) GetClip(ctx context.Context, id string) (filler.StoreClip, bool, error) {
	c, err := a.st.GetClip(ctx, id)
	if err == store.ErrNotFound {
		return filler.StoreClip{}, false, nil
	}
	if err != nil {
		return filler.StoreClip{}, false, err
	}
	return filler.StoreClip{Clip: c.Clip, UpdatedAt: c.UpdatedAt}, true, nil
}
func (a fillerStoreAdapter) DeleteClipsNotIn(ctx context.Context, keep []string) (int, error) {
	return a.st.DeleteClipsNotIn(ctx, keep)
}

// fillerTagStoreAdapter bridges the store → filler.TagStore (the AI-tagging job).
type fillerTagStoreAdapter struct{ st store.Store }

func (a fillerTagStoreAdapter) ListUntaggedCommercials(ctx context.Context) ([]filler.StoreClip, error) {
	clips, err := a.st.ListUntaggedCommercials(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]filler.StoreClip, len(clips))
	for i, c := range clips {
		out[i] = filler.StoreClip{Clip: c.Clip, UpdatedAt: c.UpdatedAt}
	}
	return out, nil
}
func (a fillerTagStoreAdapter) UpdateClipTags(ctx context.Context, id string, era int, audience, category string, aiTagged bool, updatedAt time.Time) error {
	return a.st.UpdateClipTags(ctx, id, era, audience, category, aiTagged, updatedAt)
}

// clipCatalogAdapter bridges the store → filler.CatalogReader (pod assembly).
type clipCatalogAdapter struct{ st store.Store }

func (a clipCatalogAdapter) AllClips(ctx context.Context) ([]filler.Clip, error) {
	clips, err := a.st.ListClips(ctx, store.ClipFilter{})
	if err != nil {
		return nil, err
	}
	out := make([]filler.Clip, len(clips))
	for i, c := range clips {
		out[i] = c.Clip
	}
	return out, nil
}

// fillerServiceAdapter bridges filler.Syncer/Tagger → api.FillerService.
type fillerServiceAdapter struct {
	syncer *filler.Syncer
	tagger *filler.Tagger
}

func (a fillerServiceAdapter) Sync(ctx context.Context) (int, int, int, int, error) {
	res, err := a.syncer.Sync(ctx)
	return res.Total, res.Added, res.Updated, res.Pruned, err
}
func (a fillerServiceAdapter) Tag(ctx context.Context) (int, int, int, int, error) {
	if a.tagger == nil {
		return 0, 0, 0, 0, nil // AI tagging disabled (FILLER_AI_TAGGING=false)
	}
	res, err := a.tagger.Run(ctx)
	return res.Considered, res.Tagged, res.Partial, res.Skipped, err
}

// runFillerSync runs the periodic filler catalog sync until ctx is done (§10).
func runFillerSync(ctx context.Context, syncer *filler.Syncer, every time.Duration, log *slog.Logger) {
	if every <= 0 {
		every = 15 * time.Minute
	}
	t := time.NewTicker(every)
	defer t.Stop()
	if _, err := syncer.Sync(ctx); err != nil {
		log.Warn("initial filler sync", "err", err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := syncer.Sync(ctx); err != nil {
				log.Warn("filler sync", "err", err)
			}
		}
	}
}

// newID generates a random 128-bit hex id for jobs/proposals.
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "id-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b[:])
}

// instanceDeviceID returns a stable per-install id (§11) used as the media
// server DeviceId, so Loomarr appears as one device. Generated + stored on first
// call; read thereafter. Best-effort — falls back to a constant if the store errs.
func instanceDeviceID(ctx context.Context, st store.Store) string {
	const key = "instance_id"
	if v, err := st.GetSetting(ctx, key); err == nil && v != "" {
		return v
	}
	id := "loomarr-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	if err := st.SetSetting(ctx, key, id); err != nil {
		return "loomarr"
	}
	return id
}

// newLogger builds a JSON slog logger at the configured level (§14, §17).
func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
