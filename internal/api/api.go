// Package api wires Loomarr's inbound HTTP surface (§7). The router is the
// stdlib net/http ServeMux (Go 1.22 patterns) with Huma v2 mounted via humago
// for the versioned /v1 API — code-first OpenAPI 3.1, one source of truth for
// spec + validation + docs (§7.1). No third-party router; the embedded
// same-origin SPA means no CORS layer.
package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2/adapters/humago"

	catalogapi "github.com/loomarr/loomarr/internal/api/catalog"
	channelsapi "github.com/loomarr/loomarr/internal/api/channels"
	fillerapi "github.com/loomarr/loomarr/internal/api/filler"
	guideapi "github.com/loomarr/loomarr/internal/api/guide"
	"github.com/loomarr/loomarr/internal/api/httpcore"
	imagesapi "github.com/loomarr/loomarr/internal/api/images"
	libraryapi "github.com/loomarr/loomarr/internal/api/library"
	opsapi "github.com/loomarr/loomarr/internal/api/ops"
	"github.com/loomarr/loomarr/internal/api/people"
	playoutapi "github.com/loomarr/loomarr/internal/api/playout"
	proposalsapi "github.com/loomarr/loomarr/internal/api/proposals"
	"github.com/loomarr/loomarr/internal/api/system"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/channels/viewing"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/platform/holidayvocab"
	"github.com/loomarr/loomarr/internal/platform/metrics"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/web"
)

// ReadyFunc reports readiness (DB + migrations; soft Tunarr) — §17.
type ReadyFunc func() (ready bool, detail string)

// Router builds the top-level handler from the given options.
func Router(log *slog.Logger, opts Options) http.Handler {
	mux := http.NewServeMux()
	recorder := opts.Metrics
	if recorder == nil {
		recorder = metrics.New(metrics.Options{Version: "dev", Database: "unknown"})
	}

	ready := opts.Ready
	if ready == nil {
		ready = func() (bool, string) { return true, "ok" }
	}

	// ⚠ The ops surface (liveness, readiness, Prometheus, the profiler, the API reference) now
	// registers with everything else, in registerOps — under /v1, with its bare paths kept as
	// hidden aliases. It was the last of the raw-mux world; see ops.go for why the aliases are
	// not optional and why moving these reverses a decision that was pinned on purpose.

	// The Huma API (§7.1): /v1 operations, /v1/openapi.{json,yaml}. Auth is applied
	// as Huma middleware so every /v1 op resolves a role (§7 authorization model).
	cfg := humaConfig()
	// Stamp every error response with the request's correlation id + log its full cause
	// (§7). A response Transformer is the one seam that sees EVERY error body — returned
	// StatusErrors AND huma's own validation failures — with the request context.
	cfg.Transformers = append(cfg.Transformers, httpcore.ErrorTransformer(log))
	humaAPI := humago.New(mux, cfg)
	srv := &Server{
		store: opts.Store, auth: opts.Auth, log: log, metrics: recorder,
		devices: opts.Devices, origin: httpcore.Origin{CookieSecure: opts.CookieSecure, TrustProxy: opts.TrustProxy}, devLogin: opts.DevLogin, demo: opts.Demo,
		channels: opts.Channels, livetv: opts.LiveTV, tunarrConnect: opts.TunarrConnect, arrDiscovery: opts.ArrDiscovery,
		suggest:   opts.Suggest,
		now:       opts.Now,
		viewing:   viewing.New(),
		systemLLM: opts.SystemLLM, activity: opts.Activity, sso: opts.SSO,
		settings:                 opts.Settings,
		notificationDestinations: opts.NotificationDestinations, backendTransition: opts.BackendTransition,
		webPushPublicKey:  opts.WebPushPublicKey,
		provision:         opts.Provision,
		liveSettings:      opts.LiveSettings,
		libraryConfigured: opts.LibraryConfigured,
		playoutCapability: opts.PlayoutCapability,
		playoutSecret:     opts.PlayoutSecret, playoutSecretCurrent: opts.PlayoutSecretCurrent,
		playoutResolver: opts.PlayoutResolver,
		playout:         opts.Playout,
		playoutFont:     opts.PlayoutFont,
		playoutTonemap:  opts.PlayoutTonemap, playoutGPUTonemap: opts.PlayoutGPUTonemap,
		reclaimVRAM:     opts.ReclaimVRAM,
		residentLLMVRAM: opts.ResidentLLMVRAM,
	}
	srv.registerMiddleware(humaAPI)
	catalogapi.New(catalogapi.Deps{
		Store: opts.Store, LiveSettings: opts.LiveSettings, Search: opts.Search,
	}).Register(humaAPI)
	people.New(people.Deps{
		Store: opts.Store, Log: log, Metrics: recorder, Origin: srv.origin,
		Login: opts.Login, Sessions: opts.Sessions, Passwords: opts.Passwords, UserSync: opts.UserSync,
		Invitations: opts.Invitations, InvitationDelivery: opts.InvitationDelivery,
		InvitationRedemption: opts.InvitationRedemption, PasswordRecovery: opts.PasswordRecovery,
		AccessPublicURL: opts.AccessPublicURL, Devices: opts.Devices, DeviceLimiter: opts.DeviceLimiter,
		DevLogin: opts.DevLogin, SSO: opts.SSO,
		ConfigInt: srv.configInt, LibraryUnconfigured: srv.libraryUnconfigured,
	}, false).Register(humaAPI)
	system.New(system.Deps{
		Log: log,
		// Stamped per handler build, so a restart (§9.2) resets the uptime About reports — a
		// package-level var would survive the rebuild and keep counting from the original boot.
		StartedAt: time.Now(), Ready: ready,
		StoreFacts:   storeFacts(opts.Store),
		Operations:   opts.Store,
		BackupSQLite: opts.BackupSQLite,
		Backups:      opts.Backups, Database: opts.Database, Encryption: opts.Encryption, Jobs: opts.Jobs,
		SystemLLM: opts.SystemLLM, Restart: opts.Restart, RestartDrift: opts.RestartDrift,
		PlayoutObserver: opts.PlayoutObserver, ConnectionChecks: srv.runConnectionChecks,
	}, false).Register(humaAPI)
	imageRoutes := imagesapi.New(imagesapi.Deps{Log: log, Images: opts.Images})
	channelDeps := channelsapi.Deps{
		Log: log, Store: opts.Store, LiveSettings: opts.LiveSettings, BackendCheckpoint: opts.BackendCheckpoint,
		Settings: opts.Settings, Suggest: opts.Suggest, Channels: opts.Channels, Binder: opts.Binder,
		TunerRescanner: opts.TunerRescanner, Icons: opts.Icons, Playout: opts.Playout, Activity: opts.Activity,
		Images: opts.Images, WatermarkPreview: opts.WatermarkPreview, Now: opts.Now,
		IdeaLibrary: opts.IdeaLibrary, Approver: opts.Approver, Viewing: srv.viewing,
		Calendar: func() schedule.Calendar { return srv.holidayCalendar() },
	}
	if opts.Images != nil {
		channelDeps.ImageRecords = imageRoutes.ImageDTOsByHash
	}
	channelsapi.New(channelDeps).Register(humaAPI)
	// §9.1 streaming routes (V47), the play-url op and the status projection: Huma-mounted, shared auth.
	playoutapi.New(playoutapi.Deps{
		Log: log, Store: opts.Store, LiveSettings: opts.LiveSettings,
		BackendCheckpoint: opts.BackendCheckpoint,
		Playout:           opts.Playout, PlayoutObserver: opts.PlayoutObserver, PlayoutGuide: opts.PlayoutGuide,
		PlayoutResolver: opts.PlayoutResolver, PlayoutCapability: opts.PlayoutCapability,
		PlayoutSecret: opts.PlayoutSecret, PlayoutSecretCurrent: opts.PlayoutSecretCurrent,
		Viewing: srv.viewing, Now: opts.Now, GPU: systemGPU(opts.SystemLLM), ResidentLLMVRAM: opts.ResidentLLMVRAM,
	}, false).Register(humaAPI)
	guideDeps := guideapi.Deps{
		Log: log, Store: opts.Store, LiveSettings: opts.LiveSettings, Now: opts.Now, Channels: opts.Channels,
		Pods: opts.Pods, Guide: opts.Guide, PlayoutGuide: opts.PlayoutGuide, TimelineThumbs: opts.TimelineThumbs,
	}
	if opts.Images != nil {
		guideDeps.Images = opts.Images
		guideDeps.ImageRecords = imageRoutes.ImageDTOsByHash
		guideDeps.LibraryImage = imageRoutes.LibraryImage
	}
	guideapi.New(guideDeps).Register(humaAPI)
	srv.registerSetup(humaAPI)
	proposalsapi.New(proposalsapi.Deps{
		Store: opts.Store, Suggest: opts.Suggest, Approver: opts.Approver, ProposalOutlook: opts.ProposalOutlook,
		ProposalWorkflow: opts.ProposalWorkflow, ProposalProgress: opts.ProposalProgress, Settings: opts.Settings,
		Activity: opts.Activity, DecisionQuality: opts.DecisionQuality, ProposalNotifications: opts.ProposalNotifications,
		Holidays: func() holidayvocab.Vocabulary { return srv.holidayCalendar().Vocab() },
	}).Register(humaAPI)
	libraryDeps := libraryapi.Deps{
		Store: opts.Store, LiveSettings: opts.LiveSettings, LibraryRoots: opts.LibraryRoots,
		LibraryHooks: opts.LibraryHooks, Collections: opts.Collections, MovieCollections: opts.MovieCollections,
	}
	if opts.Images != nil {
		libraryDeps.LibraryImage = imageRoutes.LibraryImage
		libraryDeps.CandidatePoster = imageRoutes.CandidatePoster
	}
	libraryapi.New(libraryDeps).Register(humaAPI)
	fillerDeps := fillerapi.Deps{
		Log: log, Store: opts.Store, LiveSettings: opts.LiveSettings, Settings: opts.Settings, Filler: opts.Filler,
		FillerLayout: opts.FillerLayout, FillerResearch: opts.FillerResearch, FillerDefects: opts.FillerDefects,
		Channels: opts.Channels, Pods: opts.Pods, IntakeLanguages: opts.IntakeLanguages,
		SourceProviders: opts.SourceProviders, PlaysCounted: opts.PlayoutResolver != nil, Now: opts.Now,
		Taxonomy: opts.Taxonomy, Origin: srv.origin,
	}
	if opts.Images != nil {
		fillerDeps.ImageRecords = imageRoutes.ImageDTOsByHash
	}
	if opts.Settings != nil {
		fillerDeps.PatchSettings = func(ctx context.Context, edits map[string]string, updatedBy string) []string {
			results := opts.Settings.Patch(ctx, edits, updatedBy)
			statuses := make([]string, len(results))
			for i := range results {
				statuses[i] = results[i].Status
			}
			return statuses
		}
	}
	fillerapi.New(fillerDeps).Register(humaAPI)
	imageRoutes.Register(humaAPI)
	opsDeps := opsapi.Deps{
		Log: log, Store: opts.Store, Ready: ready, Metrics: recorder, MetricsToken: opts.MetricsToken,
		Pprof: opts.Pprof, Events: opts.Events, Shutdown: opts.Shutdown, StartupReports: opts.StartupReports,
		DiagnosticEvents: opts.DiagnosticEvents, DiagnosticProcesses: opts.DiagnosticProcesses,
		DiagnosticBundles: opts.DiagnosticBundles, DiagnosticCapture: opts.DiagnosticCapture,
		ClientDiagnostics: opts.ClientDiagnostics, ClientCrashes: opts.ClientCrashes,
		PlayoutObserver: opts.PlayoutObserver, ConnectionChecks: srv.runConnectionChecks, StoreFacts: storeFacts(opts.Store),
	}
	if opts.Devices != nil {
		opsDeps.Devices = opts.Devices // a typed nil must stay a nil interface
	}
	opsapi.New(opsDeps, false).Register(humaAPI)
	srv.registerSettings(humaAPI)
	srv.registerNotificationDestinations(humaAPI)
	srv.registerProvisioning(humaAPI)

	// ⚠ Everything that used to be listed here — /v1/backup, the backup download, the three clip
	// byte routes, and the channel-icon serve — is now registered with its own domain, as a rawOp
	// (rawop.go). They stream bytes and keep the (w, r) signature that http.ServeContent needs,
	// but they mount on the SAME Huma API as every other route, so they are covered by the one
	// authorization middleware and appear in api/openapi.yaml. Splitting one resource across two
	// registration mechanisms is what let /v1/system/backups be spec'd while its download was not.
	//
	// Internal playout (§9.1) mounts the same way — see registerPlayout.

	// The embedded SPA at / (§12): the catch-all, and now the ONLY raw mux registration —
	// it serves embedded static assets and is not an API route. Guard the prefix-based API
	// surfaces so an unmatched path under one 404s as an API error rather than silently
	// serving index.html. Exact routes already win by ServeMux specificity and never reach here.
	spa := web.Handler()
	// ⚠ Each entry earns its place by a failure it prevents, and "it moved under /v1" does not
	// remove the need for the OLD prefix — a stale bookmark, a scrape config or a docs link
	// still resolves against this server.
	//
	// `/openapi` and `/schemas/` stay even though both now live under /v1: without them a
	// request to the old /openapi.json falls through and returns index.html with a 200, so a
	// client parsing it reports malformed YAML rather than "moved".
	// `/debug/` stays even though pprof is usually NOT mounted, and that is the point: with the
	// flag off, /debug/pprof/profile would otherwise return index.html with a 200 and
	// `go tool pprof` reports "unrecognized profile format" — reading as a broken profiler
	// rather than a disabled endpoint. That cost two failed capture attempts before a test
	// caught it.
	// `/metrics` likewise: a scrape hitting HTML should fail as a 404, not parse-error.
	//
	// ⚠ `/hooks/` is GONE. No /hooks route has existed since the inbound arr webhook was
	// deleted, so the guard was reserving a prefix against nothing.
	// `/playout/` is likewise absent: those routes have been under /v1/playout since V47, so
	// /v1/ already covers them.
	apiPrefixes := []string{"/v1/", "/openapi", "/schemas/", "/metrics", "/debug/", "/healthz", "/readyz", "/docs"}
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, p := range apiPrefixes {
			if strings.HasPrefix(r.URL.Path, p) {
				srv.writeProblem(w, r, http.StatusNotFound, "Not found", "There's no endpoint at that address.")
				return
			}
		}
		spa.ServeHTTP(w, r)
	}))

	// Outermost so it observes total handling time; it reads r.Pattern after the
	// mux has matched (§18 request metrics). withRequestID wraps everything so the
	// correlation id exists for every handler — typed Huma ops AND the plain-mux ones
	// (/v1/events, /v1/backup) — and is echoed on the response header.
	// FanoutMiddleware sits outside the Recorder middleware and every handler: it installs the
	// per-request outbound counter that the instrumented transport increments, so
	// `loomarr_http_outbound_fanout{route=…}` answers "how many downstream calls does this
	// endpoint make" without serialising traffic and diffing a global counter by hand.
	// It must be outside because it routes a request copy carrying that counter; the Recorder and
	// mux must see that same copy for the mux-populated Pattern to reach the HTTP route label.
	return httpcore.WithSecurityHeaders(httpcore.WithRequestID(recorder.FanoutMiddleware(recorder.Middleware(logRequests(log, mux)))))
}

// systemGPU is the GPU name and VRAM the playout status header reads from the language-model
// service, which already probes them for the model picker; nil without that service.
func systemGPU(svc system.SystemLLMService) func(context.Context) (string, float64, error) {
	if svc == nil {
		return nil
	}
	return func(ctx context.Context) (string, float64, error) {
		status, err := svc.Status(ctx)
		if err != nil {
			return "", 0, err
		}
		return status.GPUName, status.VRAMGiB, nil
	}
}

// storeFacts is the schema version and backend name GET /v1/system/version reports, read from st on
// each call; nil on a store-less boot, which leaves both rows absent.
func storeFacts(st records.Store) func() (int64, string) {
	if st == nil {
		return nil
	}
	return func() (int64, string) {
		return store.SchemaVersion(st), string(store.DialectOf(st))
	}
}

func logRequests(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		log.Debug("http", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr)
	})
}
