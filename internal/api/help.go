package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/mantonx/loomarr/docs"
	"github.com/mantonx/loomarr/internal/buildinfo"
)

// registerHelp mounts the in-app Help + system-info routes (§13). Both are readable by
// any authenticated user: Help is documentation, and the version is what a member quotes
// in a bug report.
func (s *Server) registerHelp(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "list-docs", Method: http.MethodGet, Path: "/v1/docs",
		Summary:     "List the embedded help pages",
		Description: "The Help section's table of contents (§13). Pages ship inside the binary, so Help works air-gapped.",
		Tags:        []string{"help"},
	}, s.listDocs)

	huma.Register(api, huma.Operation{
		OperationID: "get-doc", Method: http.MethodGet, Path: "/v1/docs/{slug}",
		Summary:     "Read one help page as markdown",
		Description: "Returns raw markdown, not HTML: the frontend renders it and searches it client-side (§7.2), which needs the source rather than a rendered blob.",
		Tags:        []string{"help"},
	}, s.getDoc)

	huma.Register(api, huma.Operation{
		OperationID: "system-version", Method: http.MethodGet, Path: "/v1/system/version",
		Summary:     "Version and readiness of this instance",
		Description: "What the operator quotes in a bug report, plus the readiness /readyz reports. /healthz and /readyz stay unauthenticated for orchestrators; this is their typed, authenticated twin for the UI.",
		Tags:        []string{"system"},
	}, s.systemVersion)
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
		return nil, huma.Error404NotFound("no such help page")
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
		Ready   bool   `json:"ready" doc:"Same readiness /readyz reports"`
		Detail  string `json:"detail,omitempty" doc:"Why it is not ready"`
	}
}

// systemVersion is the typed twin of /readyz.
//
// /healthz and /readyz deliberately stay OUTSIDE huma and unauthenticated: their consumers
// are Docker HEALTHCHECK and orchestrators, which have no session and must not need one.
// Registering them in huma to get them into the generated client would have put an auth
// requirement in front of a container health probe. So the UI gets this instead — same
// readiness, typed and authenticated, alongside the version the operator needs anyway.
func (s *Server) systemVersion(_ context.Context, _ *struct{}) (*systemVersionOutput, error) {
	info := buildinfo.Get()
	out := &systemVersionOutput{}
	out.Body.Version, out.Body.Commit = info.Version, info.Commit
	out.Body.BuiltAt, out.Body.Dirty = info.BuiltAt, info.Dirty
	out.Body.Ready, out.Body.Detail = true, ""
	if s.ready != nil {
		out.Body.Ready, out.Body.Detail = s.ready()
	}
	return out, nil
}
