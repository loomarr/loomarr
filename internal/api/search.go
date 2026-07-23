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
		Summary: "Federated search (library + TMDB)", Tags: []string{"search"},
	}, s.doSearch)
}

type searchInput struct {
	Q string `query:"q" doc:"Search terms"`
	// Clips are NOT a scope (§7.2): Candidate models a provisionable title, and a clip
	// is not one (§10). Clip search is GET /v1/filler?q=, which returns ClipDTOs.
	Scope string `query:"scope" enum:"library,tmdb,all" doc:"Corpus to search (default all). Clips are not searchable here — use /v1/filler?q="`
	Limit int    `query:"limit" doc:"Max results (default 20)"`
}
type searchOutput struct {
	Body struct {
		Candidates []SearchCandidate `json:"candidates"`
	}
}

func (s *Server) doSearch(ctx context.Context, in *searchInput) (*searchOutput, error) {
	if s.search == nil || s.unconfigured("library.url") {
		return nil, errNotImplemented("Search isn't set up", "Connect your media library in Settings to search for titles.")
	}
	if in.Q == "" {
		return nil, errBadRequest("Search term required", "Enter something to search for.")
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	cands, err := s.search.Search(ctx, in.Q, in.Scope, limit)
	if err != nil {
		return nil, apiErrWithCause(http.StatusBadGateway, "Search failed",
			"The search couldn't be completed. Check your media library connection and try again.", err)
	}
	out := &searchOutput{}
	out.Body.Candidates = cands
	return out, nil
}
