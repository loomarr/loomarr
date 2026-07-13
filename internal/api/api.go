// Package api wires Loomarr's inbound HTTP surface. Per design §14 the router is
// the stdlib net/http ServeMux (Go 1.22 method+path patterns) — no third-party
// router; the embedded same-origin SPA means no CORS layer. Phase 8 mounts the
// versioned /v1 API here via Huma on the humago adapter; Phase 1 provides only
// liveness (/healthz) and a readiness stub.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// ReadyFunc reports whether the process is ready to serve (DB connectivity +
// migrations, and soft Tunarr reachability — §17). Phase 1 has no DB, so main
// supplies a stub that always reports ready; Phase 3 replaces it.
type ReadyFunc func() (ready bool, detail string)

// Router builds the top-level handler. Later phases extend the returned mux.
func Router(log *slog.Logger, ready ReadyFunc) http.Handler {
	mux := http.NewServeMux()

	// Liveness: the process is up. Cheap, dependency-free (§17).
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Readiness: true only once dependencies are satisfied (§17). 503 until then.
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ok, detail := ready()
		code := http.StatusOK
		if !ok {
			code = http.StatusServiceUnavailable
		}
		writeJSON(w, code, map[string]any{"ready": ok, "detail": detail})
	})

	return logRequests(log, mux)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// logRequests emits one structured line per request (§17 observability baseline).
func logRequests(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		log.Debug("http", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr)
	})
}
