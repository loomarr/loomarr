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
	"strings"

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

	// The Huma API (§7.1): /v1 operations, /openapi.{json,yaml}. Auth is applied
	// as Huma middleware so every /v1 op resolves a role (§7 authorization model).
	cfg := humaConfig()
	// Stamp every error response with the request's correlation id + log its full cause
	// (§7). A response Transformer is the one seam that sees EVERY error body — returned
	// StatusErrors AND huma's own validation failures — with the request context.
	cfg.Transformers = append(cfg.Transformers, errorTransformer(log))
	humaAPI := humago.New(mux, cfg)
	srv := &Server{
		store: opts.Store, auth: opts.Auth, log: log, backupSQLite: opts.BackupSQLite,
		login: opts.Login, sessions: opts.Sessions, passwords: opts.Passwords, userSync: opts.UserSync, cookieSecure: opts.CookieSecure,
		channels: opts.Channels, livetv: opts.LiveTV, tunarrConnect: opts.TunarrConnect,
		suggest: opts.Suggest, search: opts.Search, icons: opts.Icons, events: opts.Events, filler: opts.Filler, pods: opts.Pods,
		jobs:      opts.Jobs,
		systemLLM: opts.SystemLLM, settings: opts.Settings, provision: opts.Provision, guide: opts.Guide,
		liveConfig: opts.LiveConfig, liveConfigInt: opts.LiveConfigInt, ready: ready,
		binder: opts.Binder,
	}
	srv.registerMiddleware(humaAPI)
	srv.registerTitles(humaAPI)
	srv.registerAuth(humaAPI)
	srv.registerUsers(humaAPI)
	srv.registerPasswords(humaAPI)
	srv.registerChannels(humaAPI)
	srv.registerProgramming(humaAPI)
	srv.registerSetup(humaAPI)
	srv.registerSuggestions(humaAPI)
	srv.registerSearch(humaAPI)
	srv.registerFiller(humaAPI)
	srv.registerJobs(humaAPI)
	srv.registerSystemLLM(humaAPI)
	srv.registerSettings(humaAPI)
	srv.registerHelp(humaAPI)
	srv.registerProvisioning(humaAPI)

	// GET /v1/backup streams a binary snapshot, so it's a plain mux handler
	// (not a typed Huma op — §16). Auth checked inline.
	mux.HandleFunc("GET /v1/backup", srv.backupHandler)

	// GET /v1/events streams SSE (§7/§8) — a plain mux handler (Huma returns typed
	// bodies). Auth checked inline via the same authorizer.
	mux.HandleFunc("GET /v1/events", srv.eventsHandler)

	// Channel icon upload (multipart) + serve (raw image bytes) are plain mux handlers —
	// neither fits Huma's typed-JSON model. Upload is admin-only (checked inline); serve is
	// public so Tunarr can fetch the icon machine-to-machine, like a TMDB poster URL.
	mux.HandleFunc("POST /v1/channels/{id}/icon", srv.uploadChannelIcon)
	mux.HandleFunc("GET /v1/channels/{id}/icon", srv.serveChannelIcon)

	// Self-hosted offline docs (§7.1) — override Huma's CDN default.
	mux.HandleFunc("GET /docs", docsHandler)

	// The embedded SPA at / (§12): the catch-all. Guard the prefix-based API
	// surfaces so an unknown /v1 or /hooks path 404s as an API error rather than
	// silently serving index.html. Exact ops routes (/healthz, /docs, …) already
	// win by ServeMux specificity and never reach here.
	spa := web.Handler()
	apiPrefixes := []string{"/v1/", "/hooks/", "/openapi", "/schemas/", "/metrics"}
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
	return withRequestID(metrics.Middleware(logRequests(log, mux)))
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
