// Package api wires Loomarr's inbound HTTP surface (§7). The router is the
// stdlib net/http ServeMux (Go 1.22 patterns) with Huma v2 mounted via humago
// for the versioned /v1 API — code-first OpenAPI 3.1, one source of truth for
// spec + validation + docs (§7.1). No third-party router; the embedded
// same-origin SPA means no CORS layer.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/mantonx/loomarr/internal/metrics"
	"github.com/mantonx/loomarr/internal/web"
)

// ReadyFunc reports readiness (DB + migrations; soft Tunarr) — §17.
type ReadyFunc func() (ready bool, detail string)

// Router builds the top-level handler from the given options.
func Router(log *slog.Logger, opts Options) http.Handler {
	mux := http.NewServeMux()

	// Ops endpoints, unauthenticated on the LAN (§7).
	ready := opts.Ready
	if ready == nil {
		ready = func() (bool, string) { return true, "ok" }
	}
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ok, detail := ready()
		code := http.StatusOK
		if !ok {
			code = http.StatusServiceUnavailable
		}
		writeJSON(w, code, map[string]any{"ready": ok, "detail": detail})
	})

	// Prometheus exposition (§7 /metrics, §18). Unauthenticated on the LAN like
	// the other ops probes; the metrics.Middleware below records every request.
	mux.Handle("GET /metrics", metrics.Handler())

	// Go's profiler (§7), mounted ONLY when the server was started with LOOMARR_PPROF=1.
	// Not registering is the gate, the same posture as /v1/auth/dev-login: an install without
	// the flag 404s because the routes genuinely do not exist, not because a handler refused.
	//
	// Unauthenticated by nature — a profiler is not a browser and holds no session — which is
	// precisely why it is off by default: these handlers expose stack traces and memory
	// contents, and a repeated CPU profile degrades a running server.
	//
	// Registered by explicit path rather than by prefix so the surface is exactly the four
	// handlers listed, and `pprof.Index` serves its own links off /debug/pprof/.
	if opts.Pprof {
		mux.HandleFunc("GET /debug/pprof/", pprof.Index)
		mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)
	}

	// The Huma API (§7.1): /v1 operations, /openapi.{json,yaml}. Auth is applied
	// as Huma middleware so every /v1 op resolves a role (§7 authorization model).
	cfg := humaConfig()
	// Stamp every error response with the request's correlation id + log its full cause
	// (§7). A response Transformer is the one seam that sees EVERY error body — returned
	// StatusErrors AND huma's own validation failures — with the request context.
	cfg.Transformers = append(cfg.Transformers, errorTransformer(log))
	humaAPI := humago.New(mux, cfg)
	srv := &Server{
		// Stamped per handler build, so a restart (§9.2) resets the uptime About reports
		// — a package-level var would survive the rebuild and keep counting from the
		// original boot.
		startedAt: time.Now(),
		store:     opts.Store, auth: opts.Auth, log: log, backupSQLite: opts.BackupSQLite,
		login: opts.Login, sessions: opts.Sessions, passwords: opts.Passwords, userSync: opts.UserSync, cookieSecure: opts.CookieSecure, devLogin: opts.DevLogin,
		channels: opts.Channels, livetv: opts.LiveTV, tunarrConnect: opts.TunarrConnect,
		suggest: opts.Suggest, search: opts.Search, collections: opts.Collections, icons: opts.Icons, events: opts.Events, filler: opts.Filler, pods: opts.Pods,
		jobs:      opts.Jobs,
		systemLLM: opts.SystemLLM, database: opts.Database, backups: opts.Backups, restart: opts.Restart, activity: opts.Activity, sso: opts.SSO,
		bootstrapDrift: opts.BootstrapDrift,
		settings:       opts.Settings, provision: opts.Provision, guide: opts.Guide,
		liveConfig: opts.LiveConfig, liveConfigInt: opts.LiveConfigInt, ready: ready,
		binder:          opts.Binder,
		playoutSessions: opts.PlayoutSessions, playoutSecret: opts.PlayoutSecret,
		playoutResolver: opts.PlayoutResolver, playoutEncoder: opts.PlayoutEncoder,
		playoutGuide: opts.PlayoutGuide, playoutFont: opts.PlayoutFont,
	}
	srv.registerMiddleware(humaAPI)
	srv.registerTitles(humaAPI)
	srv.registerAuth(humaAPI)
	srv.registerUsers(humaAPI)
	srv.registerPasswords(humaAPI)
	srv.registerChannels(humaAPI)
	srv.registerGuide(humaAPI)
	srv.registerProgramming(humaAPI)
	srv.registerSetup(humaAPI)
	srv.registerSuggestions(humaAPI)
	srv.registerSearch(humaAPI)
	srv.registerCollections(humaAPI)
	srv.registerFiller(humaAPI)
	srv.registerFillerSources(humaAPI)
	srv.registerJobs(humaAPI)
	srv.registerDashboard(humaAPI)
	srv.registerSystemLLM(humaAPI)
	srv.registerSystemDatabase(humaAPI)
	srv.registerSystemBackups(humaAPI)
	srv.registerSystemRestart(humaAPI)
	srv.registerDashboardPanels(humaAPI)
	srv.registerSettings(humaAPI)
	srv.registerHelp(humaAPI)
	srv.registerProvisioning(humaAPI)

	// GET /v1/backup streams a binary snapshot, so it's a plain mux handler
	// (not a typed Huma op — §16). Auth checked inline.
	mux.HandleFunc("GET /v1/backup", srv.backupHandler)

	// Downloading an ALREADY-WRITTEN backup (§16, V12) — same reason it isn't a Huma op:
	// it streams a file. The list half of /v1/system/backups IS typed and lives with the
	// other system routes.
	mux.HandleFunc("GET /v1/system/backups/{name}", srv.downloadBackupHandler)

	// SSO's two routes are browser REDIRECTS (§11, V8), so they are plain mux handlers like
	// /v1/backup — Huma models typed JSON, and a 302 carrying Set-Cookie is neither. Not
	// mounted at all when no provider is wired.
	srv.registerSSO(mux)

	// GET /v1/events streams SSE (§7/§8) — a plain mux handler (Huma returns typed
	// bodies). Auth checked inline via the same authorizer.
	mux.HandleFunc("GET /v1/events", srv.eventsHandler)

	// Channel icon upload (multipart) + serve (raw image bytes) are plain mux handlers —
	// neither fits Huma's typed-JSON model. Upload is admin-only (checked inline); serve is
	// public so Tunarr can fetch the icon machine-to-machine, like a TMDB poster URL.
	mux.HandleFunc("POST /v1/channels/{id}/icon", srv.uploadChannelIcon)
	mux.HandleFunc("GET /v1/channels/{id}/icon", srv.serveChannelIcon)

	// Internal playout (§9.1): the tuner M3U, the ffconcat playlist, and the continuous
	// MPEG-TS stream. Plain mux handlers (they stream bytes, two of them forever) with
	// DEVICE auth by `playout_token` rather than session auth — a television cannot hold a
	// cookie (§11).
	srv.registerPlayout(mux)

	// Self-hosted offline docs (§7.1) — override Huma's CDN default.
	mux.HandleFunc("GET /docs", docsHandler)

	// The embedded SPA at / (§12): the catch-all. Guard the prefix-based API
	// surfaces so an unknown /v1 or /hooks path 404s as an API error rather than
	// silently serving index.html. Exact ops routes (/healthz, /docs, …) already
	// win by ServeMux specificity and never reach here.
	spa := web.Handler()
	// /playout/ is in this list for a real reason, not tidiness: without it an unmatched
	// playout path (a typo'd channel id, a route we have not built yet) falls through to the
	// SPA and returns index.html with a 200. ffmpeg would then read HTML as a transport
	// stream, and the failure surfaces as a corrupt-stream error naming neither the URL nor
	// the typo.
	// `/debug/` is here even though pprof is usually NOT mounted, and that is the point: with
	// the flag off, a request to /debug/pprof/profile would otherwise fall through to the SPA
	// and return index.html with a 200. `go tool pprof` then reports "unrecognized profile
	// format" — which reads as a broken profiler rather than as a disabled endpoint, and cost
	// two failed capture attempts before a test caught it.
	apiPrefixes := []string{"/v1/", "/hooks/", "/openapi", "/schemas/", "/metrics", "/playout/", "/debug/"}
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
	// FanoutMiddleware sits INSIDE metrics.Middleware but outside every handler: it installs the
	// per-request outbound counter that the instrumented transport increments, so
	// `loomarr_http_outbound_fanout{route=…}` answers "how many downstream calls does this
	// endpoint make" without serialising traffic and diffing a global counter by hand.
	return withRequestID(metrics.Middleware(metrics.FanoutMiddleware(logRequests(log, mux))))
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func logRequests(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		log.Debug("http", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr)
	})
}
