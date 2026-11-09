package catalog

import (
	"context"

	"github.com/loomarr/loomarr/internal/api/httpcore"
)

// SearchService backs GET /v1/search (§7.2) — the SAME catalog impl as the LLM
// grounding tool. Returns candidates as generic maps so the API layer needn't
// import the catalog types (kept dependency-light).
type SearchService interface {
	Search(ctx context.Context, request SearchRequest) ([]httpcore.SearchCandidate, error)
}

// SearchRequest selects exactly one Catalog operation. Query uses federated title
// search; Discovery uses the provider-neutral structured discovery path. The API
// handler rejects requests that provide both or neither.
type SearchRequest struct {
	Query     string
	MediaType string
	Scope     string
	Limit     int
	Discovery *SearchDiscovery
}

// SearchDiscovery is the dependency-light API representation of Catalog's
// structured discovery request (§7.2). The composition-root adapter is the sole
// conversion seam into catalog.DiscoveryQuery.
type SearchDiscovery struct {
	MediaType        string
	Keywords         []string
	Genres           []string
	YearFrom         int
	YearTo           int
	OriginalLanguage string
	OriginCountry    string
	RuntimeMin       int
	RuntimeMax       int
	VoteAverageMin   float64
	VoteCountMin     int
	Network          string
	Cast             []string
	Creators         []string
}
