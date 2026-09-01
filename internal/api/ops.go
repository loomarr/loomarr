package api

import (
	"context"
	"net/http"
	"net/http/pprof"

	"github.com/danielgtaylor/huma/v2"
)

// Operational endpoints (§7, §17, §18): liveness, readiness, Prometheus, the profiler, and the
// offline API reference.
//
// ⚠ **Every route lives under /v1, and every one keeps its bare-path alias.** These are the
// last routes that were not behind the versioned prefix, and moving them is the point of this
// change — but their consumers are Docker HEALTHCHECK, orchestrators and Prometheus, which are
// configured OUTSIDE this repo and cannot be migrated by editing it. So both paths are
// registered against the same handler: the canonical `/v1/...` and the historical bare one.
//
// ⚠ **The aliases are Hidden, and the canonical routes are not.** The spec should list each
// endpoint once; a visible alias would generate a duplicate orval hook for the same operation.
//
// ⚠ **This reverses a decision that was pinned on purpose, and the reasoning matters.**
// PROGRESS.md recorded "deliberately did NOT move /healthz + /readyz into huma", and
// help_test.go pinned it, because at the time registering them in huma meant putting them
// inside the authenticated /v1 API — "an auth requirement in front of a container health
// probe". That is no longer what registration implies: RolePublic marks a route reachable with
// no credential at all, and the ops probes say so out loud. The objection was answered, not
// overruled — and TestOpsProbesStayUnauthenticated still guards the property it cared about,
// now across both paths.

// healthOutput is the liveness answer: the process is up enough to serve.
type healthOutput struct {
	Body struct {
		Status string `json:"status" example:"ok"`
	}
}

// readyOutput is the readiness answer.
//
// ⚠ **`Status` is a huma-recognised field that sets the RESPONSE CODE**, and it is here so the
// 503 keeps carrying `{ready, detail}`. Returning an error instead would have been the idiomatic
// huma move and would have replaced that body with an RFC 7807 problem — a wire change for every
// orchestrator parsing this, to gain nothing. A probe's job is to explain itself in the shape
// its consumer already reads.
//
// Stated precisely, because "unchanged body" would be an overclaim: the `ready` and `detail`
// fields and both status codes are exactly as before, and huma additionally emits its standard
// `$schema` link property, as it does on every other typed response in this API. A consumer
// reading named fields is unaffected; one comparing whole documents is not.
type readyOutput struct {
	Status int
	Body   struct {
		Ready  bool   `json:"ready" example:"true"`
		Detail string `json:"detail" example:"ok" doc:"Why, when not ready — the operator's first clue"`
	}
}

// registerOps mounts the operational surface. `pprofOn` gates the profiler routes.
func (s *Server) registerOps(api huma.API, pprofOn bool) {
	metricsHandler := http.NotFoundHandler()
	if s.metrics != nil {
		metricsHandler = s.metrics.Handler()
	}
	huma.Register(api, withRole(huma.Operation{
		OperationID: "healthz", Method: http.MethodGet, Path: "/v1/healthz",
		Summary: "Liveness probe",
		Description: "Public, no credential — the consumer is a container runtime, not a person. " +
			"Answers 200 whenever the process is serving, regardless of whether its dependencies " +
			"are healthy; that is what `readyz` is for. Also served at `/healthz`.",
		Tags: []string{"ops"},
	}, RolePublic), s.healthz)
	opsAlias(api, "healthz", http.MethodGet, "/healthz", s.healthz)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "readyz", Method: http.MethodGet, Path: "/v1/readyz",
		Summary: "Readiness probe",
		Description: "Public, no credential. 200 when the store is up and migrations are applied, " +
			"503 with a reason otherwise — a container started without DATABASE_URL is a SUPPORTED " +
			"degraded mode, and this is how it explains itself. Also served at `/readyz`.",
		Tags: []string{"ops"},
	}, RolePublic), s.readyz)
	opsAlias(api, "readyz", http.MethodGet, "/readyz", s.readyz)

	// Prometheus exposition (§17, §18). A third-party http.Handler, so rawOp rather than a
	// typed op — there is no Go type for the text exposition format, and inventing one to
	// satisfy the spec would describe it worse than the media type does.
	rawOp[struct{}](api, bytesResponse(huma.Operation{
		OperationID: "metrics", Method: http.MethodGet, Path: "/v1/metrics",
		Summary: "Prometheus metrics",
		Description: "Public, no credential — a scrape job holds no session. Go runtime collectors " +
			"plus Loomarr's §17 series. Also served at `/metrics`.",
		Tags: []string{"ops"},
	}, "Prometheus text exposition.", "text/plain"), RolePublic, metricsHandler.ServeHTTP)
	rawOpAlias(api, "metrics", http.MethodGet, "/metrics", RolePublic, metricsHandler.ServeHTTP)

	// The offline API reference (§7.1). Huma's built-in docs page loads Stoplight from a CDN,
	// which breaks air-gapped LAN installs, so cfg.DocsPath is blanked and this serves a
	// self-contained page instead.
	//
	// ⚠ `/v1/reference`, NOT `/v1/docs` — that path is taken by the in-app help markdown
	// (help.go). `/v1/docs/api` would have worked by ServeMux specificity while silently
	// making a help doc slugged "api" unreachable, which is the kind of collision that is
	// found months later by someone writing documentation.
	rawOp[struct{}](api, bytesResponse(huma.Operation{
		OperationID: "api-reference", Method: http.MethodGet, Path: "/v1/reference",
		Summary:     "Interactive API reference",
		Description: "Public. A self-contained page linking to the machine-readable spec; no CDN assets.",
		Tags:        []string{"ops"},
	}, "An HTML page.", "text/html"), RolePublic, docsHandler)
	rawOpAlias(api, "api-reference", http.MethodGet, "/docs", RolePublic, docsHandler)

	if !pprofOn {
		// ⚠ Not registering IS the gate, same posture as /v1/auth/dev-login: an install without
		// LOOMARR_PPROF 404s because the routes genuinely do not exist, not because a handler
		// refused. These expose stack traces and memory contents, and a repeated CPU profile
		// degrades a running server.
		return
	}
	for _, p := range []struct {
		id, path string
		h        http.HandlerFunc
	}{
		{"pprof-index", "/debug/pprof/", pprof.Index},
		{"pprof-cmdline", "/debug/pprof/cmdline", pprof.Cmdline},
		{"pprof-profile", "/debug/pprof/profile", pprof.Profile},
		{"pprof-symbol", "/debug/pprof/symbol", pprof.Symbol},
		{"pprof-trace", "/debug/pprof/trace", pprof.Trace},
		{"pprof-goroutineleak", "/debug/pprof/goroutineleak", pprof.Handler("goroutineleak").ServeHTTP},
	} {
		// Hidden even on the canonical path: the profiler is a debugging tool that exists only
		// when a flag is set, so publishing it in the spec would advertise a surface most
		// installs do not have. Registered by explicit path rather than by prefix so the set is
		// exactly these six, and pprof.Index serves its own links off the trailing-slash route.
		rawOp[struct{}](api, hiddenOp(p.id, http.MethodGet, "/v1"+p.path, "Go profiler"), RolePublic, p.h)
		rawOpAlias(api, p.id, http.MethodGet, p.path, RolePublic, p.h)
	}
}

func (s *Server) healthz(_ context.Context, _ *struct{}) (*healthOutput, error) {
	out := &healthOutput{}
	out.Body.Status = "ok"
	return out, nil
}

func (s *Server) readyz(_ context.Context, _ *struct{}) (*readyOutput, error) {
	out := &readyOutput{Status: http.StatusOK}
	ready, detail := true, "ok"
	if s.ready != nil {
		ready, detail = s.ready()
	}
	if !ready {
		out.Status = http.StatusServiceUnavailable
	}
	out.Body.Ready, out.Body.Detail = ready, detail
	return out, nil
}

// hiddenOp builds an operation that serves traffic but stays out of the spec.
//
// ⚠ The id must be UNIQUE even for an alias: huma keys operations by OperationID, so reusing
// the canonical one would silently overwrite it rather than add a second route.
func hiddenOp(id, method, path, summary string) huma.Operation {
	return huma.Operation{
		OperationID: id, Method: method, Path: path,
		Summary: summary,
		Description: "The pre-/v1 path, kept because container runtimes, orchestrators and scrape " +
			"configs live outside this repo and cannot be migrated by editing it.",
		Tags: []string{"ops"}, Hidden: true,
	}
}

// opsAlias registers a typed ops handler at its second, historical path, hidden from the spec
// so the endpoint is listed once.
func opsAlias[O any](api huma.API, id, method, path string, h func(context.Context, *struct{}) (*O, error)) {
	huma.Register(api, withRole(hiddenOp(id+"-alias", method, path, "Alias of "+id), RolePublic), h)
}

// rawOpAlias is the raw-handler equivalent.
func rawOpAlias(api huma.API, id, method, path string, role Role, h func(http.ResponseWriter, *http.Request)) {
	rawOp[struct{}](api, hiddenOp(id+"-alias", method, path, "Alias of "+id), role, h)
}
