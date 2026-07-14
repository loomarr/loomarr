// Package tmdb is the TMDB adapter (design §8 grounding): the TMDB-scope corpus
// for the catalog and the exists-check for acquisition validation. TMDB grounds
// the suggester — the LLM selects from real TMDB ids, and every acquisition is
// re-validated against TMDB (exists) before it's actionable (§8).
//
// Built against TMDB's documented v3 API (api.themoviedb.org/3); the live fixture
// capture is deferred (no TMDB_API_KEY this session) — see
// testkit/fixtures/llm/FINDINGS.md. When a key is supplied, pin real fixtures and
// reconcile any shape difference doc-first.
package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/mantonx/loomarr/internal/catalog"
	"github.com/mantonx/loomarr/internal/httpx"
	"github.com/mantonx/loomarr/internal/provision"
)

// Client is the TMDB v3 client. The api key rides as a bearer token (v4 auth) or
// the api_key query param (v3) — we use the Authorization bearer form so the key
// never lands in a URL/log (§6 anti-leak discipline, applied to TMDB too).
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// New builds a TMDB client. baseURL defaults to the public API host.
func New(apiKey string) *Client {
	return &Client{
		baseURL: "https://api.themoviedb.org/3",
		apiKey:  apiKey,
		http:    httpx.New(httpx.TimeoutTMDB),
	}
}

// NewWithBase is for tests: point at a mock TMDB server.
func NewWithBase(baseURL, apiKey string) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, http: httpx.New(httpx.TimeoutTMDB)}
}

// multiResult is one /search/multi row. media_type distinguishes movie/tv/person;
// we keep only movie+tv. Movies carry `title`/`release_date`, tv `name`/
// `first_air_date`.
type multiResult struct {
	ID            int    `json:"id"`
	MediaType     string `json:"media_type"`
	Title         string `json:"title"`
	Name          string `json:"name"`
	ReleaseDate   string `json:"release_date"`
	FirstAirDate  string `json:"first_air_date"`
	OriginalTitle string `json:"original_title"`
}

type multiResponse struct {
	Results []multiResult `json:"results"`
}

// Search implements catalog.TMDBSearcher: GET /search/multi?query=<q>, mapping
// movie/tv results to Candidates with real TMDB ids (§8 grounding). Person
// results are dropped. in_library is false here — the catalog sets it by merging
// with library results.
func (c *Client) Search(ctx context.Context, term string, limit int) ([]catalog.Candidate, error) {
	q := url.Values{}
	q.Set("query", term)
	q.Set("include_adult", "false")

	var resp multiResponse
	if err := c.get(ctx, "/search/multi?"+q.Encode(), &resp); err != nil {
		return nil, err
	}
	out := make([]catalog.Candidate, 0, len(resp.Results))
	for _, r := range resp.Results {
		mt, ok := mediaType(r.MediaType)
		if !ok {
			continue // person / unknown
		}
		name := r.Title
		date := r.ReleaseDate
		if mt == provision.Series {
			name = r.Name
			date = r.FirstAirDate
		}
		out = append(out, catalog.Candidate{
			MediaType: mt,
			TMDBID:    r.ID,
			Name:      name,
			Year:      yearFromDate(date),
			InLibrary: false,
			Source:    catalog.ScopeTMDB,
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// Exists re-validates an acquisition against TMDB (§8): GET /movie/{id} or
// /tv/{id}; a 200 means the id is real. A 404 means the LLM proposed a
// non-existent id — the acquisition must be dropped, never actioned.
func (c *Client) Exists(ctx context.Context, mt provision.MediaType, tmdbID int) (bool, error) {
	if tmdbID <= 0 {
		return false, nil
	}
	path := "/movie/" + strconv.Itoa(tmdbID)
	if mt == provision.Series {
		path = "/tv/" + strconv.Itoa(tmdbID)
	}
	status, err := c.getStatus(ctx, path, nil)
	if err != nil {
		return false, err
	}
	switch status {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("tmdb exists %s: status %d", path, status)
	}
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	status, err := c.getStatus(ctx, path, out)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("tmdb GET %s: status %d", path, status)
	}
	return nil
}

func (c *Client) getStatus(ctx context.Context, path string, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return 0, err
	}
	// Bearer auth (v4 token style) keeps the key out of the URL (§6 anti-leak).
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("tmdb GET %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if out != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, fmt.Errorf("tmdb decode %s: %w", path, err)
		}
	}
	return resp.StatusCode, nil
}

// mediaType maps TMDB's media_type to provision's, reporting whether it's a kind
// we keep (movie/tv → true; person/other → false).
func mediaType(t string) (provision.MediaType, bool) {
	switch t {
	case "movie":
		return provision.Movie, true
	case "tv":
		return provision.Series, true
	default:
		return "", false
	}
}

// yearFromDate extracts the year from a TMDB date ("1994-06-10" → 1994).
func yearFromDate(date string) int {
	if len(date) < 4 {
		return 0
	}
	y, err := strconv.Atoi(date[:4])
	if err != nil {
		return 0
	}
	return y
}

var _ catalog.TMDBSearcher = (*Client)(nil)
