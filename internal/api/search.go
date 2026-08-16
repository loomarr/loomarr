package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// registerSearch mounts GET /v1/search (§7.2). Any authenticated user may search
// (read-only); adding a missing result still routes through submit→approve, so
// search adds no new privilege surface (§7.2). This is the SAME catalog impl as
// the LLM grounding tool — humans and the model see identical results.
func (s *Server) registerSearch(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "search", Method: http.MethodGet, Path: "/v1/search",
		Summary: "Federated search (library + TMDB)", Tags: []string{"search"},
	}, RoleMember), s.doSearch)
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
	if strings.TrimSpace(in.Q) == "" {
		return nil, errBadRequest("Search term required", "Enter something to search for.")
	}
	if s.search == nil {
		return nil, errNotImplemented("Search isn't set up", "Connect your media library or add a TMDB API key in Settings to search for titles.")
	}
	scope, configured := s.configuredSearchScope(in.Scope)
	if !configured {
		switch normalizeSearchScope(in.Scope) {
		case "library":
			return nil, errNotImplemented("Library search isn't set up", "Connect your media library in Settings to search it.")
		case "tmdb":
			return nil, errNotImplemented("TMDB search isn't set up", "Add a TMDB API key in Settings to search TMDB.")
		default:
			return nil, errNotImplemented("Search isn't set up", "Connect your media library or add a TMDB API key in Settings to search for titles.")
		}
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	cands, err := s.search.Search(ctx, in.Q, scope, limit)
	if err != nil {
		return nil, apiErrWithCause(http.StatusBadGateway, "Search failed",
			"The search couldn't be completed. Check the configured title sources and try again.", err)
	}
	out := &searchOutput{}
	out.Body.Candidates = cands
	return out, nil
}

func normalizeSearchScope(scope string) string {
	switch scope {
	case "library", "tmdb":
		return scope
	default:
		return "all"
	}
}

// configuredSearchScope narrows all/default to the corpora configured for this
// request. The composition root keeps both adapters alive, so a saved setting
// changes the next call without rebuilding the server. A nil liveConfig is the
// established unit-test convention: an explicitly wired fake is usable.
func (s *Server) configuredSearchScope(requested string) (string, bool) {
	scope := normalizeSearchScope(requested)
	libraryConfigured := !s.libraryUnconfigured()
	// A nil liveConfig is the established unit-test convention: an explicitly wired
	// adapter is usable. Production always supplies it, so the current key gates TMDB.
	tmdbConfigured := s.liveConfig == nil || strings.TrimSpace(s.liveConfig("tmdb.api_key")) != ""
	switch scope {
	case "library":
		return scope, libraryConfigured
	case "tmdb":
		return scope, tmdbConfigured
	default:
		switch {
		case libraryConfigured && tmdbConfigured:
			return "all", true
		case libraryConfigured:
			return "library", true
		case tmdbConfigured:
			return "tmdb", true
		default:
			return "all", false
		}
	}
}
