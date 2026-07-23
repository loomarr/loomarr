package library

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// Compile-time proof the client satisfies both ports.
var (
	_ Library        = (*Client)(nil)
	_ LibraryScanner = (*Client)(nil)
)

// This file adds the BULK reads that drive poll-based availability (design §4 + §18.1).
// Unlike Lookup (one HTTP call per title — fine for the deadline backstop's handful of due
// records), a scan confirms availability for EVERY in-flight title in a single call: the
// media server returns many recently-added items with their provider ids, and the scan job
// correlates them against the in-flight set in memory. Both methods hit the same /Items
// surface as Search/Lookup; the flavor changes only auth headers (library.go).

// scanFields is the Fields projection a scan needs: provider ids for correlation, plus the
// display metadata the catalog/UI already reads elsewhere. Kept identical to Search's so the
// two share the searchItem/searchResponse decode shape.
const scanFields = "ProviderIds,ProductionYear,Genres,Overview,OfficialRating"

// RecentlyAdded returns library items created/updated at or after `since` — the incremental
// 5-minute availability path (§18.1). It sorts by DateCreated so the newest land first and
// filters server-side with MinDateLastSaved, so a large library costs one bounded call:
//
//	GET /Items?Recursive=true&IncludeItemTypes=Movie,Series
//	    &SortBy=DateCreated&SortOrder=Descending&MinDateLastSaved=<RFC3339>&Fields=<scanFields>
//
// A zero `since` degenerates to the whole library (no MinDateLastSaved) — callers that want
// that should use AllItems for intent's sake.
func (c *Client) RecentlyAdded(ctx context.Context, since time.Time) ([]SearchResult, error) {
	q := scanQuery()
	if !since.IsZero() {
		// MinDateLastSaved catches both fresh imports and re-saves (metadata refresh that
		// can attach a provider id after the fact). UTC RFC3339 is what both flavors accept.
		q.Set("MinDateLastSaved", since.UTC().Format(time.RFC3339))
	}
	return c.scan(ctx, q)
}

// AllItems returns every movie/series in the library — the periodic full sweep (§18.1), a
// safety net for anything the incremental "recently added" window missed (e.g. Loomarr was
// down across a scan interval, or a provider id landed on an item older than `since`).
func (c *Client) AllItems(ctx context.Context) ([]SearchResult, error) {
	return c.scan(ctx, scanQuery())
}

// scanQuery builds the shared /Items query for both scan variants.
func scanQuery() url.Values {
	q := url.Values{}
	q.Set("Recursive", "true")
	q.Set("IncludeItemTypes", "Movie,Series")
	q.Set("SortBy", "DateCreated")
	q.Set("SortOrder", "Descending")
	q.Set("Fields", scanFields)
	return q
}

// scan executes an /Items query and maps the response to SearchResults, reusing the search
// decode shape + helpers (mediaTypeFromEmby/atoiSafe from search.go). Only items carrying a
// usable provider id are of interest to the caller, but we return all rows and let the scan
// job's key-building drop the id-less ones — keeps this method a plain transport concern.
func (c *Client) scan(ctx context.Context, q url.Values) ([]SearchResult, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/Items?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	c.flavor.applyTokenAuth(req, c.token(), c.deviceID)

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
