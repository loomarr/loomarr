package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// registerSearch mounts GET /v1/search (§7.2). Any authenticated user may search
// (read-only); adding a missing result still routes through submit→approve, so
// search adds no new privilege surface (§7.2). This is the SAME catalog impl as
// the LLM grounding tool — humans and the model see identical results.
func (s *Server) registerSearch(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "search", Method: http.MethodGet, Path: "/v1/search",
		Summary: "Federated search (library + TMDB + clips)", Tags: []string{"search"},
	}, s.doSearch)
}

type searchInput struct {
	Q     string `query:"q" doc:"Search terms"`
	Scope string `query:"scope" enum:"library,tmdb,clips,all" doc:"Corpus to search (default all)"`
	Limit int    `query:"limit" doc:"Max results (default 20)"`
}
type searchOutput struct {
	Body struct {
		Candidates []SearchCandidate `json:"candidates"`
	}
}

func (s *Server) doSearch(ctx context.Context, in *searchInput) (*searchOutput, error) {
	if s.search == nil {
		return nil, huma.Error501NotImplemented("search not configured")
	}
	if in.Q == "" {
		return nil, huma.Error400BadRequest("q is required")
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	cands, err := s.search.Search(ctx, in.Q, in.Scope, limit)
	if err != nil {
		return nil, huma.Error502BadGateway("search failed", err)
	}
	out := &searchOutput{}
	out.Body.Candidates = cands
	return out, nil
}
