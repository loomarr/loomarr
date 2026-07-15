package library

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// SearchResult is one library item from a term search (§7.2), carrying the
// external ids the catalog/grounding layer needs. Pinned to the real Emby 4.10
// /Items?SearchTerm shape (fixture emby/search_matrix.json): Items[] with
// ProviderIds{Tmdb,Tvdb,Imdb}, Type, ProductionYear, and the library item Id.
type SearchResult struct {
	LibraryItemID string
	Name          string
	Year          int
	MediaType     MediaType // Movie | Series (from Emby "Type")
	TMDBID        int
	TVDBID        int
	IMDBID        string
	// Genres + Overview enrich the catalog candidate for theme reasoning/scoring
	// (§8); read from Emby when the Fields param requests them. Display only.
	Genres   []string
	Overview string
	// OfficialRating is the media server's content rating ("TV-Y7", "PG-13", …),
	// the input to ChannelPolicy audience enforcement (programming-design §4).
	// Display/enforcement metadata only — never identity.
	OfficialRating string
}

// searchItem mirrors the slice of an /Items entry we read for search.
type searchItem struct {
	ID             string   `json:"Id"`
	Name           string   `json:"Name"`
	ProductionYear int      `json:"ProductionYear"`
	Type           string   `json:"Type"`
	Genres         []string `json:"Genres"`
	Overview       string   `json:"Overview"`
	OfficialRating string   `json:"OfficialRating"`
	ProviderIds    struct {
		Tmdb string `json:"Tmdb"`
		Tvdb string `json:"Tvdb"`
		Imdb string `json:"Imdb"`
	} `json:"ProviderIds"`
}

type searchResponse struct {
	Items []searchItem `json:"Items"`
}

// Search runs a library term search (§7.2 federated search, library scope):
//
//	GET /Items?Recursive=true&IncludeItemTypes=Movie,Series&SearchTerm=<q>
//	           &Limit=<n>&Fields=ProviderIds,ProductionYear
//
// It returns library items with their external ids so the catalog can dedupe/
// merge against TMDB and set in_library. Header auth per flavor (§6). This is the
// same /Items surface as Lookup — one media-server client.
func (c *Client) Search(ctx context.Context, term string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	q := url.Values{}
	q.Set("Recursive", "true")
	q.Set("IncludeItemTypes", "Movie,Series")
	q.Set("SearchTerm", term)
	q.Set("Limit", strconv.Itoa(limit))
	q.Set("Fields", "ProviderIds,ProductionYear,Genres,Overview,OfficialRating")

	req, err := c.newRequest(ctx, http.MethodGet, "/Items?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	c.flavor.applyTokenAuth(req, c.token, c.deviceID)

	var out searchResponse
	if err := c.do(req, &out); err != nil {
		return nil, err
	}
	results := make([]SearchResult, 0, len(out.Items))
	for _, it := range out.Items {
		results = append(results, SearchResult{
			LibraryItemID:  it.ID,
			Name:           it.Name,
			Year:           it.ProductionYear,
			MediaType:      mediaTypeFromEmby(it.Type),
			TMDBID:         atoiSafe(it.ProviderIds.Tmdb),
			TVDBID:         atoiSafe(it.ProviderIds.Tvdb),
			IMDBID:         it.ProviderIds.Imdb,
			Genres:         it.Genres,
			Overview:       it.Overview,
			OfficialRating: it.OfficialRating,
		})
	}
	return results, nil
}

// mediaTypeFromEmby maps Emby's "Type" to our MediaType (Movie/Series).
func mediaTypeFromEmby(t string) MediaType {
	switch t {
	case "Series":
		return Series
	default:
		return Movie
	}
}

// atoiSafe parses an int, returning 0 on any error (a missing/blank provider id).
func atoiSafe(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}
