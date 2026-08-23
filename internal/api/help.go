package api

import (
	"context"
	"net/http"
	"runtime"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/docs"
	"github.com/loomarr/loomarr/internal/buildinfo"
	"github.com/loomarr/loomarr/internal/store"
)

// registerHelp mounts the in-app Help + system-info routes (§13). Both are readable by
// any authenticated user: Help is documentation, and the version is what a member quotes
// in a bug report.
func (s *Server) registerHelp(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "list-docs", Method: http.MethodGet, Path: "/v1/docs",
		Summary:     "List the embedded help pages",
		Description: "The Help section's table of contents (§13). Pages ship inside the binary, so Help works air-gapped.",
		Tags:        []string{"help"},
	}, RoleMember), s.listDocs)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "get-doc", Method: http.MethodGet, Path: "/v1/docs/{slug}",
		Summary:     "Read one help page as markdown",
		Description: "Returns raw markdown, not HTML: the frontend renders it and searches it client-side (§7.2), which needs the source rather than a rendered blob.",
		Tags:        []string{"help"},
	}, RoleMember), s.getDoc)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "system-version", Method: http.MethodGet, Path: "/v1/system/version",
		Summary:     "Version and readiness of this instance",
		Description: "What the operator quotes in a bug report, plus the readiness /v1/readyz reports. The probes stay unauthenticated for orchestrators (RolePublic); this is their authenticated twin, carrying the build and schema detail a probe has no business returning.",
		Tags:        []string{"system"},
	}, RolePublic), s.systemVersion)
}

// DocSummary is one entry in the Help table of contents.
type DocSummary struct {
	Slug  string `json:"slug" example:"troubleshooting"`
	Title string `json:"title" example:"Troubleshooting"`
}

type listDocsOutput struct {
	Body struct {
		Docs []DocSummary `json:"docs"`
	}
}

func (s *Server) listDocs(_ context.Context, _ *struct{}) (*listDocsOutput, error) {
	out := &listDocsOutput{}
	pages := docs.Pages()
	out.Body.Docs = make([]DocSummary, 0, len(pages))
	for _, p := range pages {
		out.Body.Docs = append(out.Body.Docs, DocSummary{Slug: p.Slug, Title: p.Title})
	}
	return out, nil
}

type getDocInput struct {
	Slug string `path:"slug" example:"troubleshooting"`
}
type getDocOutput struct {
	Body struct {
		Slug     string `json:"slug"`
		Title    string `json:"title"`
		Markdown string `json:"markdown"`
	}
}

func (s *Server) getDoc(_ context.Context, in *getDocInput) (*getDocOutput, error) {
	page, ok := docs.Get(in.Slug)
	if !ok {
		return nil, errNotFound("Help page not found", "That help page doesn't exist — it may have been moved or renamed.")
	}
	out := &getDocOutput{}
	out.Body.Slug, out.Body.Title, out.Body.Markdown = page.Slug, page.Title, page.Markdown
	return out, nil
}

type systemVersionOutput struct {
	Body struct {
		Version string `json:"version" example:"v0.3.1" doc:"Release tag, or 'dev' for an unstamped build"`
		Commit  string `json:"commit,omitempty"`
		BuiltAt string `json:"builtAt,omitempty"`
		Dirty   bool   `json:"dirty,omitempty" doc:"Built from a working tree with uncommitted changes"`
		Ready   bool   `json:"ready" doc:"Same readiness /v1/readyz reports"`
		Detail  string `json:"detail,omitempty" doc:"Why it is not ready"`

		// The About page's remaining rows (§16, V12) — what an operator quotes in a bug
		// report alongside the version.
		GoVersion string `json:"goVersion,omitempty" example:"go1.23.4" doc:"Go runtime this binary was built with"`
		Platform  string `json:"platform,omitempty" example:"linux/amd64" doc:"os/arch this process is running on"`
		// StartedAt is the process start as RFC3339 — NOT a pre-computed uptime. A
		// duration is stale the moment it is serialized; the client renders elapsed time
		// from this instant and can keep it current without re-fetching.
		StartedAt string `json:"startedAt,omitempty" doc:"When this process started (RFC3339); the UI derives uptime from it"`
		// SchemaVersion is the applied migration version. Omitted when unknown (0) rather
		// than sent as a real schema 0 — a wrong number in a bug report is worse than an
		// absent one.
		SchemaVersion int64  `json:"schemaVersion,omitempty" example:"20" doc:"Applied database migration version"`
		Backend       string `json:"backend,omitempty" example:"sqlite" doc:"Active database backend"`
	}
}

// systemVersion is the typed twin of /v1/readyz.
//
// ⚠ The old note here said the probes "deliberately stay OUTSIDE huma and unauthenticated…
// registering them would have put an auth requirement in front of a container health probe".
// The concern was right and its premise has since stopped holding: RolePublic marks an
// operation reachable with no credential, so /v1/healthz and /v1/readyz are Huma operations,
// still anonymous, with their bare paths kept as permanent aliases (ops.go).
//
// This endpoint is NOT redundant with them. It carries what an operator quotes in a bug
// report — version, commit, build time, Go runtime, os/arch, process start, applied schema
// version and backend — which is authenticated information a liveness probe has no business
// returning to an unauthenticated caller.

func (s *Server) systemVersion(_ context.Context, _ *struct{}) (*systemVersionOutput, error) {
	info := buildinfo.Get()
	out := &systemVersionOutput{}
	out.Body.Version, out.Body.Commit = info.Version, info.Commit
	out.Body.BuiltAt, out.Body.Dirty = info.BuiltAt, info.Dirty
	out.Body.GoVersion = runtime.Version()
	out.Body.Platform = runtime.GOOS + "/" + runtime.GOARCH
	out.Body.StartedAt = s.startedAt.UTC().Format(time.RFC3339)
	// Schema version and backend need the store. A store-less boot (§7 readiness) still
	// serves this endpoint — it is what an operator reads to find out WHY the install is
	// unhealthy — so both rows are simply absent rather than the handler failing.
	if s.store != nil {
		out.Body.SchemaVersion = store.SchemaVersion(s.store)
		out.Body.Backend = string(store.DialectOf(s.store))
	}
	out.Body.Ready, out.Body.Detail = true, ""
	if s.ready != nil {
		out.Body.Ready, out.Body.Detail = s.ready()
	}
	return out, nil
}
