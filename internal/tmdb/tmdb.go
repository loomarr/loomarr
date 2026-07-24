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
		http:    httpx.NewNamed("tmdb", httpx.TimeoutTMDB),
	}
}

// NewWithBase is for tests: point at a mock TMDB server.
func NewWithBase(baseURL, apiKey string) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, http: httpx.NewNamed("tmdb", httpx.TimeoutTMDB)}
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
	// Enrichment (§8): TMDB already returns these on /search/multi + /discover; we
	// now parse them so the model reasons about theme (genre/overview), not titles.
	GenreIDs []int  `json:"genre_ids"`
	Overview string `json:"overview"`
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
			Genres:    genreNames(r.GenreIDs),
			Overview:  r.Overview,
			Source:    catalog.ScopeTMDB,
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// discoverResult is one /discover row. Same shape as multiResult minus
// media_type (discover is per-endpoint: /discover/movie vs /discover/tv).
type discoverResult struct {
	ID           int    `json:"id"`
	Title        string `json:"title"`
	Name         string `json:"name"`
	ReleaseDate  string `json:"release_date"`
	FirstAirDate string `json:"first_air_date"`
	GenreIDs     []int  `json:"genre_ids"`
	Overview     string `json:"overview"`
}

type discoverResponse struct {
	Results []discoverResult `json:"results"`
}

// Discover finds titles by GENRE + ERA rather than title text (§8 discovery path)
// — the fix for abstract intents ("high-energy 90s action") that keyword search
// can't surface. genres are human names (mapped to TMDB ids); era bounds the
// primary release date (yearFrom/yearTo, 0 = unbounded). Returns enriched, grounded
// Candidates (real ids + genres/overview). Movies + series unless a media type is
// pinned via mt ("" = both). sort_by=popularity.desc so the strongest fits lead.
func (c *Client) Discover(ctx context.Context, mt provision.MediaType, genres []string, yearFrom, yearTo, limit int) ([]catalog.Candidate, error) {
	ids := genreIDsFor(genres)
	var out []catalog.Candidate
	if mt == "" || mt == provision.Movie {
		rows, err := c.discover(ctx, "/discover/movie", ids, yearFrom, yearTo, "primary_release_date")
		if err != nil {
			return nil, err
		}
		out = appendDiscover(out, rows, provision.Movie)
	}
	if mt == "" || mt == provision.Series {
		rows, err := c.discover(ctx, "/discover/tv", ids, yearFrom, yearTo, "first_air_date")
		if err != nil {
			return nil, err
		}
		out = appendDiscover(out, rows, provision.Series)
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (c *Client) discover(ctx context.Context, path string, genreIDs []int, yearFrom, yearTo int, dateField string) ([]discoverResult, error) {
	q := url.Values{}
	q.Set("include_adult", "false")
	q.Set("sort_by", "popularity.desc")
	if len(genreIDs) > 0 {
		parts := make([]string, len(genreIDs))
		for i, id := range genreIDs {
			parts[i] = strconv.Itoa(id)
		}
		q.Set("with_genres", strings.Join(parts, ",")) // comma = OR
	}
	if yearFrom > 0 {
		q.Set(dateField+".gte", fmt.Sprintf("%04d-01-01", yearFrom))
	}
	if yearTo > 0 {
		q.Set(dateField+".lte", fmt.Sprintf("%04d-12-31", yearTo))
	}
	var resp discoverResponse
	if err := c.get(ctx, path+"?"+q.Encode(), &resp); err != nil {
		return nil, err
	}
	return resp.Results, nil
}

func appendDiscover(out []catalog.Candidate, rows []discoverResult, mt provision.MediaType) []catalog.Candidate {
	for _, r := range rows {
		name, date := r.Title, r.ReleaseDate
		if mt == provision.Series {
			name, date = r.Name, r.FirstAirDate
		}
		out = append(out, catalog.Candidate{
			MediaType: mt, TMDBID: r.ID, Name: name, Year: yearFromDate(date),
			InLibrary: false, Genres: genreNames(r.GenreIDs), Overview: r.Overview,
			Source: catalog.ScopeTMDB,
		})
	}
	return out
}

// genreIDsFor maps human genre names (case-insensitive) to TMDB ids, dropping
// unknowns. Used by Discover to translate an intent's genre terms.
func genreIDsFor(names []string) []int {
	var out []int
	for _, n := range names {
		if id, ok := genreIDByName[canonGenre(n)]; ok {
			out = append(out, id)
		}
	}
	return out
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

// CollectionID returns the TMDB collection (franchise) a MOVIE belongs to — the id of
// belongs_to_collection on GET /movie/{id} — so the scheduler can keep a franchise's films
// together, in release order (§5). Returns 0 (no error) for a standalone movie or a series
// (TV has no belongs_to_collection). This is the authoritative franchise signal: it groups
// "Raiders of the Lost Ark" with the "Indiana Jones and the…" films even though they share
// no title base, which a title heuristic can't do. Fetched at reconcile-heal time and
// stamped onto the lineup entry (like the rating heal), so the pure scheduler stays I/O-free.
func (c *Client) CollectionID(ctx context.Context, mt provision.MediaType, tmdbID int) (int, error) {
	if tmdbID <= 0 || mt == provision.Series {
		return 0, nil
	}
	var body struct {
		BelongsToCollection *struct {
			ID int `json:"id"`
		} `json:"belongs_to_collection"`
	}
	if err := c.get(ctx, "/movie/"+strconv.Itoa(tmdbID), &body); err != nil {
		return 0, err
	}
	if body.BelongsToCollection != nil {
		return body.BelongsToCollection.ID, nil
	}
	return 0, nil
}

// ContentRating returns the US content rating for a title — TV series via
// /tv/{id}/content_ratings (the TV-* certifications), movies via
// /movie/{id}/release_dates (the MPAA certification on a US release). It exists so a
// not-yet-owned acquisition can still carry a rating: the library cannot rate a title
// it does not have, and under an audience ceiling an unrated entry is dropped
// (dead air, §9). Returns "" (no error) when TMDB has no US rating — sparse coverage
// is normal, so an empty answer is a legitimate result, not a failure.
func (c *Client) ContentRating(ctx context.Context, mt provision.MediaType, tmdbID int) (string, error) {
	if tmdbID <= 0 {
		return "", nil
	}
	if mt == provision.Series {
		var body struct {
			Results []struct {
				ISO    string `json:"iso_3166_1"`
				Rating string `json:"rating"`
			} `json:"results"`
		}
		if err := c.get(ctx, "/tv/"+strconv.Itoa(tmdbID)+"/content_ratings", &body); err != nil {
			return "", err
		}
		for _, r := range body.Results {
			if r.ISO == "US" {
				return strings.TrimSpace(r.Rating), nil
			}
		}
		return "", nil
	}
	var body struct {
		Results []struct {
			ISO          string `json:"iso_3166_1"`
			ReleaseDates []struct {
				Certification string `json:"certification"`
			} `json:"release_dates"`
		} `json:"results"`
	}
	if err := c.get(ctx, "/movie/"+strconv.Itoa(tmdbID)+"/release_dates", &body); err != nil {
		return "", err
	}
	for _, r := range body.Results {
		if r.ISO != "US" {
			continue
		}
		for _, d := range r.ReleaseDates {
			if cert := strings.TrimSpace(d.Certification); cert != "" {
				return cert, nil
			}
		}
	}
	return "", nil
}

// imageBase is TMDB's image CDN + a mid-size width good for a channel icon (a poster at
// w500 is crisp on a guide tile without being a full-res download). TMDB's /configuration
// endpoint returns the canonical base, but it's stable and documented, so we hardcode it
// (one fewer round-trip) — same pragmatism as the other pinned TMDB shapes.
const imageBase = "https://image.tmdb.org/t/p/w500"

// PosterURL returns a full, directly-fetchable poster image URL for a title (§icon), or ""
// (no error) when TMDB has no poster — sparse coverage is normal, so an empty answer is a
// legitimate result the caller falls back on (no icon), not a failure. Used to give a
// channel a themed icon from its primary series/movie. Tunarr fetches this URL directly, so
// no proxying is needed. A poster (portrait) reads better as a channel tile than a backdrop.
func (c *Client) PosterURL(ctx context.Context, mt provision.MediaType, tmdbID int) (string, error) {
	if tmdbID <= 0 {
		return "", nil
	}
	path := "/movie/" + strconv.Itoa(tmdbID)
	if mt == provision.Series {
		path = "/tv/" + strconv.Itoa(tmdbID)
	}
	var body struct {
		PosterPath string `json:"poster_path"`
	}
	if err := c.get(ctx, path, &body); err != nil {
		return "", err
	}
	if p := strings.TrimSpace(body.PosterPath); p != "" {
		return imageBase + p, nil // poster_path already has a leading "/"
	}
	return "", nil
}

// PosterURLByTVDB resolves a TVDB series id to a TMDB poster via the /find bridge
// (/find/{tvdb_id}?external_source=tvdb_id → the matching tv result's poster_path). Our
// series are often TVDB-keyed (Seerr's canonical id for series), but posters live on TMDB;
// this is the one-hop bridge. Returns "" (no error) when no match / no poster.
func (c *Client) PosterURLByTVDB(ctx context.Context, tvdbID int) (string, error) {
	if tvdbID <= 0 {
		return "", nil
	}
	var body struct {
		TVResults []struct {
			PosterPath string `json:"poster_path"`
		} `json:"tv_results"`
	}
	if err := c.get(ctx, "/find/"+strconv.Itoa(tvdbID)+"?external_source=tvdb_id", &body); err != nil {
		return "", err
	}
	for _, r := range body.TVResults {
		if p := strings.TrimSpace(r.PosterPath); p != "" {
			return imageBase + p, nil
		}
	}
	return "", nil
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
