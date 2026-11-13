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
	setupapi "github.com/loomarr/loomarr/internal/api/setup"
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

	// ⚠ The ops surface (liveness, readiness, Prometheus, the profiler, the API reference)
	// registers with everything else, through opsapi.Handlers — under /v1, with its bare paths
	// kept as hidden aliases. See api/ops/ops.go for why the aliases are not optional.

	// The Huma API (§7.1): /v1 operations, /v1/openapi.{json,yaml}. Auth is applied
	// as Huma middleware so every /v1 op resolves a role (§7 authorization model).
	cfg := humaConfig()
	// Stamp every error response with the request's correlation id + log its full cause
	// (§7). A response Transformer is the one seam that sees EVERY error body — returned
	// StatusErrors AND huma's own validation failures — with the request context.
	cfg.Transformers = append(cfg.Transformers, httpcore.ErrorTransformer(log))
	humaAPI := humago.New(mux, cfg)
	srv := &Server{
		auth: opts.Auth, log: log,
		origin:   httpcore.Origin{CookieSecure: opts.CookieSecure, TrustedProxies: opts.TrustedProxies},
		channels: opts.Channels.Service,
		viewing:  opts.Playout.Viewing,
	}
	if srv.viewing == nil {
		srv.viewing = viewing.New()
	}
	srv.registerMiddleware(humaAPI)
	setupRoutes := setupapi.New(setupapi.Deps{
		Log: log, Store: opts.Store, LiveSettings: opts.LiveSettings, Settings: opts.Settings,
		NotificationDestinations: opts.Setup.NotificationDestinations, WebPushPublicKey: opts.Setup.WebPushPublicKey,
		BackendTransition: opts.Setup.BackendTransition, LiveTV: opts.Setup.LiveTV, TunarrConnect: opts.Setup.TunarrConnect,
		ArrDiscovery: opts.Setup.ArrDiscovery, LibraryConfigured: opts.Library.Configured,
	})
	catalogapi.New(catalogapi.Deps{
		Store: opts.Store, LiveSettings: opts.LiveSettings, Search: opts.Library.Search, Now: opts.Now,
	}).Register(humaAPI)
	people.New(people.Deps{
		Store: opts.Store, Log: log, Metrics: recorder, Origin: srv.origin,
		Login: opts.People.Login, Sessions: opts.People.Sessions, Passwords: opts.People.Passwords,
		UserSync: opts.People.UserSync, Invitations: opts.People.Invitations,
		InvitationDelivery:   opts.People.InvitationDelivery,
		InvitationRedemption: opts.People.InvitationRedemption, PasswordRecovery: opts.People.PasswordRecovery,
		AccessPublicURL: opts.People.AccessPublicURL, Devices: opts.People.Devices,
		DeviceLimiter: opts.People.DeviceLimiter, DevLogin: opts.People.DevLogin, SSO: opts.People.SSO,
		Provision: opts.People.Provision, Demo: opts.People.Demo,
		ConfigInt: setupRoutes.ConfigInt, LibraryUnconfigured: setupRoutes.LibraryUnconfigured,
	}, false).Register(humaAPI)
	system.New(system.Deps{
		Log: log,
		// Stamped per handler build, so a restart (§9.2) resets the uptime About reports — a
		// package-level var would survive the rebuild and keep counting from the original boot.
		StartedAt: startedAt(opts.Now), Ready: ready,
		StoreFacts:   storeFacts(opts.Store),
		Operations:   opts.Store,
		BackupSQLite: opts.System.BackupSQLite,
		Backups:      opts.System.Backups, Database: opts.System.Database, Encryption: opts.System.Encryption, Jobs: opts.System.Jobs,
		SystemLLM: opts.System.LLM, Restart: opts.System.Restart, RestartDrift: opts.System.RestartDrift,
		PlayoutObserver: opts.Playout.Observer, ConnectionChecks: setupRoutes.ConnectionChecks,
	}, false).Register(humaAPI)
	imageRoutes := imagesapi.New(imagesapi.Deps{Log: log, Images: opts.Images})
	channelDeps := channelsapi.Deps{
		Log: log, Store: opts.Store, LiveSettings: opts.LiveSettings, BackendCheckpoint: opts.Setup.BackendCheckpoint,
		Settings: opts.Settings, Suggest: opts.Proposals.Suggest, Channels: opts.Channels.Service, Binder: opts.Channels.Binder,
		TunerRescanner: opts.Channels.TunerRescanner, Icons: opts.Channels.Icons, Playout: opts.Playout.Service, Activity: opts.Activity,
		Images: opts.Images, WatermarkPreview: opts.Channels.WatermarkPreview, Now: opts.Now,
		IdeaLibrary: opts.Channels.IdeaLibrary, Approver: opts.Proposals.Approver, Viewing: srv.viewing,
		Calendar: func() schedule.Calendar { return srv.holidayCalendar() },
	}
	if opts.Images != nil {
		channelDeps.ImageRecords = imageRoutes.ImageDTOsByHash
	}
	channelsapi.New(channelDeps).Register(humaAPI)
	// §9.1 streaming routes (V47), the play-url op and the status projection: Huma-mounted, shared auth.
	playoutapi.New(playoutapi.Deps{
		Log: log, Store: opts.Store, LiveSettings: opts.LiveSettings,
		BackendCheckpoint: opts.Setup.BackendCheckpoint,
		Playout:           opts.Playout.Service, PlayoutObserver: opts.Playout.Observer, PlayoutGuide: opts.Playout.Guide,
		PlayoutResolver: opts.Playout.Resolver, PlayoutCapability: opts.Playout.Capability,
		PlayoutSecret: opts.Playout.Secret, PlayoutSecretCurrent: opts.Playout.SecretCurrent,
		Viewing: srv.viewing, Now: opts.Now, GPU: systemGPU(opts.System.LLM), ResidentLLMVRAM: opts.Playout.ResidentLLMVRAM,
	}, false).Register(humaAPI)
	guideDeps := guideapi.Deps{
		Log: log, Store: opts.Store, LiveSettings: opts.LiveSettings, Now: opts.Now, Channels: opts.Channels.Service,
		Pods: opts.Guide.Pods, Guide: opts.Guide.Reader, PlayoutGuide: opts.Playout.Guide, TimelineThumbs: opts.Guide.TimelineThumbs,
	}
	if opts.Images != nil {
		guideDeps.Images = opts.Images
		guideDeps.ImageRecords = imageRoutes.ImageDTOsByHash
		guideDeps.LibraryImage = imageRoutes.LibraryImage
	}
	guideapi.New(guideDeps).Register(humaAPI)
	proposalsapi.New(proposalsapi.Deps{
		Store: opts.Store, Suggest: opts.Proposals.Suggest, Approver: opts.Proposals.Approver, ProposalOutlook: opts.Proposals.Outlook,
		ProposalWorkflow: opts.Proposals.Workflow, ProposalProgress: opts.Proposals.Progress, Settings: opts.Settings,
		Activity: opts.Activity, DecisionQuality: opts.Proposals.DecisionQuality, ProposalNotifications: opts.Proposals.Notifications,
		Holidays: func() holidayvocab.Vocabulary { return srv.holidayCalendar().Vocab() },
	}).Register(humaAPI)
	libraryDeps := libraryapi.Deps{
		Store: opts.Store, LiveSettings: opts.LiveSettings, LibraryRoots: opts.Library.Roots,
		LibraryHooks: opts.Library.Hooks, Collections: opts.Library.Collections, MovieCollections: opts.Library.MovieCollections,
	}
	if opts.Images != nil {
		libraryDeps.LibraryImage = imageRoutes.LibraryImage
		libraryDeps.CandidatePoster = imageRoutes.CandidatePoster
	}
	libraryapi.New(libraryDeps).Register(humaAPI)
	fillerDeps := fillerapi.Deps{
		Log: log, Store: opts.Store, LiveSettings: opts.LiveSettings, Settings: opts.Settings, Filler: opts.Filler.Service,
		FillerLayout: opts.Filler.Layout, FillerResearch: opts.Filler.Research, FillerDefects: opts.Filler.Defects,
		Channels: opts.Channels.Service, Pods: opts.Guide.Pods, IntakeLanguages: opts.Filler.IntakeLanguages,
		SourceProviders: opts.Filler.SourceProviders, PlaysCounted: opts.Playout.Resolver != nil, Now: opts.Now,
		Taxonomy: opts.Filler.Taxonomy, Origin: srv.origin,
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
		Log: log, Store: opts.Store, Ready: ready, Metrics: recorder, MetricsToken: opts.Ops.MetricsToken,
		Pprof: opts.Ops.Pprof, Events: opts.Ops.Events, Shutdown: opts.Shutdown, StartupReports: opts.Ops.StartupReports,
		DiagnosticEvents: opts.Ops.DiagnosticEvents, DiagnosticProcesses: opts.Ops.DiagnosticProcesses,
		DiagnosticBundles: opts.Ops.DiagnosticBundles, DiagnosticCapture: opts.Ops.DiagnosticCapture,
		ClientDiagnostics: opts.Ops.ClientDiagnostics, ClientCrashes: opts.Ops.ClientCrashes,
		PlayoutObserver: opts.Playout.Observer, ConnectionChecks: setupRoutes.ConnectionChecks, StoreFacts: storeFacts(opts.Store),
	}
	if opts.People.Devices != nil {
		opsDeps.Devices = opts.People.Devices // a typed nil must stay a nil interface
	}
	opsapi.New(opsDeps, false).Register(humaAPI)
	setupRoutes.Register(humaAPI)

	// ⚠ The byte-streaming routes (/v1/backup, the backup download, the clip byte routes, the
	// channel-icon serve) are registered by their own domain's Handlers as httpcore.RawOp
	// (api/httpcore/rawop.go). They keep the (w, r) signature that http.ServeContent needs, but
	// they mount on the SAME Huma API as every other route, so they are covered by the one
	// authorization middleware and appear in api/openapi.yaml. Splitting one resource across two
	// registration mechanisms is what let /v1/system/backups be spec'd while its download was not.
	//
	// Internal playout (§9.1) mounts the same way — see playout.Handlers.Register.

	// The embedded SPA at / (§12): the catch-all, and the ONLY raw mux registration —
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
	// correlation id exists for every handler — typed Huma ops, RawOps (/v1/backup),
	// SSE (/v1/events) AND the SPA catch-all — and is echoed on the response header.
	// FanoutMiddleware sits outside the Recorder middleware and every handler: it installs the
	// per-request outbound counter that the instrumented transport increments, so
	// `loomarr_http_outbound_fanout{route=…}` answers "how many downstream calls does this
	// endpoint make" without serialising traffic and diffing a global counter by hand.
	// It must be outside because it routes a request copy carrying that counter; the Recorder and
	// mux must see that same copy for the mux-populated Pattern to reach the HTTP route label.
	// WithServerTiming stamps `Server-Timing` (#3399): total time plus any phases a handler names
	// with httpcore.Phase, readable in the browser's network panel.
	return httpcore.WithSecurityHeaders(httpcore.WithRequestID(httpcore.WithServerTiming(recorder.FanoutMiddleware(recorder.Middleware(logRequests(log, mux))))))
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

// startedAt is the instant a handler generation reports as its start: the request clock when one is
// injected, else the wall clock.
func startedAt(now func() time.Time) time.Time {
	if now != nil {
		return now()
	}
	return time.Now()
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
