// Package tmdbstandin is the one TMDB v3 stand-in: an in-memory catalogue served over the
// endpoints internal/tmdb calls. Tests serve it through internal/testkit (testkit.NewTMDB), and
// the demo library serves it over its invented catalogue so a lane or demo backend grounds
// suggestions offline (#1941, LOOMARR_TMDB_BASE_URL). One implementation, so the demo cannot
// drift from what the tests pin.
package tmdbstandin

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/loomarr/loomarr/internal/provision"
)

// Title is one movie or series the stand-in knows. Ids absent from the catalogue 404 on the
// details endpoints, which is how grounding drops a fabricated id (§8).
type Title struct {
	ID               int
	CollectionID     int
	CollectionName   string
	Name             string
	Year             int
	Date             string
	GenreIDs         []int // endpoint-specific TMDB genre ids (movie 878 vs TV 10765 for Sci-Fi)
	KeywordIDs       []int
	Overview         string
	OriginalLanguage string
	OriginCountries  []string
	VoteAverage      float64
	VoteCount        int
	RuntimeMinutes   int
	CastIDs          []int
	CreatorIDs       []int
	NetworkID        int
	// USRating is the US content rating /content_ratings (tv) or /release_dates (movie)
	// reports. Empty ⇒ no US rating, the common sparse-coverage case.
	USRating string
	// GenreNames name GenreIDs in order, for the details endpoints' genres; absent, details
	// carry none.
	GenreNames []string
	// TVDBID is a series' TVDB id in its details' external_ids; zero carries none.
	TVDBID int
}

// Episode is one episode a season lists (/tv/{id}/season/{n}).
type Episode struct {
	ID             int
	Number         int
	Name           string
	Overview       string
	AirDate        string
	RuntimeMinutes int
}

type network struct {
	ID            int
	Name          string
	OriginCountry string
}

type collection struct {
	ID       int
	Name     string
	MovieIDs []int
}

// Catalog is the stand-in's corpus. It is safe for concurrent use: setters may run while the
// handler serves.
type Catalog struct {
	mu            sync.RWMutex
	movies        map[int]Title
	series        map[int]Title
	keywords      map[int]string
	nextKeywordID int
	// recommends is the adjacency graph /{movie,tv}/{id}/recommendations serves (§8.3). A seed
	// absent from it returns an empty result, the real API's answer for an obscure title.
	recommends  map[int][]int
	people      map[int]string
	networks    map[int]network
	collections map[int]collection
	// seasons are each series' episodes by season number.
	seasons     map[int]map[int][]Episode
	personDecoy bool
}

// New returns an empty catalogue.
func New() *Catalog {
	return &Catalog{
		movies: map[int]Title{}, series: map[int]Title{}, keywords: map[int]string{}, nextKeywordID: 5000,
		recommends: map[int][]int{}, people: map[int]string{}, networks: map[int]network{}, collections: map[int]collection{},
		seasons: map[int]map[int][]Episode{},
	}
}

// PutSeason adds or replaces one season of a series.
func (c *Catalog) PutSeason(seriesID, number int, episodes []Episode) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.seasons[seriesID] == nil {
		c.seasons[seriesID] = map[int][]Episode{}
	}
	c.seasons[seriesID][number] = append([]Episode(nil), episodes...)
}

// WithPersonDecoy makes /search/multi also return a person row, which the client must filter out.
func (c *Catalog) WithPersonDecoy() *Catalog {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.personDecoy = true
	return c
}

// SetRecommendations replaces the adjacency graph /recommendations serves.
func (c *Catalog) SetRecommendations(graph map[int][]int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recommends = graph
}

// PutMovie and PutSeries add or replace one title whole.
func (c *Catalog) PutMovie(t Title) { c.put(c.movies, t) }

// PutSeries adds or replaces one series whole.
func (c *Catalog) PutSeries(t Title) { c.put(c.series, t) }

func (c *Catalog) put(into map[int]Title, t Title) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t.Date == "" && t.Year > 0 {
		t.Date = fmt.Sprintf("%04d-01-01", t.Year)
	}
	into[t.ID] = t
}

// AddMovie adds one movie.
func (c *Catalog) AddMovie(id int, name string, year int, genreIDs []int, overview string) {
	c.PutMovie(Title{ID: id, Name: name, Year: year, GenreIDs: append([]int(nil), genreIDs...), Overview: overview})
}

// AddSeries adds one series.
func (c *Catalog) AddSeries(id int, name string, year int, genreIDs []int, overview string) {
	c.PutSeries(Title{ID: id, Name: name, Year: year, GenreIDs: append([]int(nil), genreIDs...), Overview: overview})
}

// SetDiscoveryEvidence sets the source metadata and runtime TMDB's remote discovery filters on.
func (c *Catalog) SetDiscoveryEvidence(mt provision.MediaType, id int, language string, countries []string, runtimeMinutes int, voteAverage float64, voteCount int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	catalog := c.movies
	if mt == provision.Series {
		catalog = c.series
	}
	title := catalog[id]
	title.ID = id
	title.OriginalLanguage = language
	title.OriginCountries = append([]string(nil), countries...)
	title.RuntimeMinutes = runtimeMinutes
	title.VoteAverage = voteAverage
	title.VoteCount = voteCount
	catalog[id] = title
}

// SetCollectionID sets belongs_to_collection on a movie's details.
func (c *Catalog) SetCollectionID(movieID, collectionID int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := c.movies[movieID]
	t.ID, t.CollectionID = movieID, collectionID
	c.movies[movieID] = t
}

// SetMovieCollection sets a /collection/{id} roster and each member's belongs_to_collection.
func (c *Catalog) SetMovieCollection(collectionID int, name string, movieIDs ...int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.collections[collectionID] = collection{ID: collectionID, Name: strings.TrimSpace(name), MovieIDs: append([]int(nil), movieIDs...)}
	for _, movieID := range movieIDs {
		title := c.movies[movieID]
		title.ID, title.CollectionID, title.CollectionName = movieID, collectionID, strings.TrimSpace(name)
		c.movies[movieID] = title
	}
}

// SetRating sets a title's US content rating, adding the id if absent.
func (c *Catalog) SetRating(mt provision.MediaType, tmdbID int, rating string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cat := c.movies
	if mt == provision.Series {
		cat = c.series
	}
	t := cat[tmdbID]
	t.ID, t.USRating = tmdbID, rating
	cat[tmdbID] = t
}

// AddPerson adds one identity to /search/person.
func (c *Catalog) AddPerson(id int, name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.people[id] = strings.TrimSpace(name)
}

// SetMoviePeople binds cast and creator identities to a movie.
func (c *Catalog) SetMoviePeople(movieID int, castIDs, creatorIDs []int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	title := c.movies[movieID]
	title.CastIDs = append([]int(nil), castIDs...)
	title.CreatorIDs = append([]int(nil), creatorIDs...)
	c.movies[movieID] = title
}

// AddNetwork adds one TV network to the daily export and the details endpoint.
func (c *Catalog) AddNetwork(id int, name, originCountry string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.networks[id] = network{ID: id, Name: strings.TrimSpace(name), OriginCountry: strings.ToUpper(strings.TrimSpace(originCountry))}
}

// SetSeriesNetwork binds a series to the network TV discovery filters on.
func (c *Catalog) SetSeriesNetwork(seriesID, networkID int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	title := c.series[seriesID]
	title.NetworkID = networkID
	c.series[seriesID] = title
}

// AddKeywordMovie adds a movie whose thematic match lives in TMDB's keyword metadata rather than
// its title, such as "Christmas" finding "Snowbound Reunion".
func (c *Catalog) AddKeywordMovie(id int, name string, year int, genreIDs []int, overview string, keywords ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	keywordIDs := make([]int, 0, len(keywords))
	for _, keyword := range keywords {
		keyword = strings.TrimSpace(keyword)
		if keyword == "" {
			continue
		}
		var keywordID int
		for id, existing := range c.keywords {
			if strings.EqualFold(existing, keyword) {
				keywordID = id
				break
			}
		}
		if keywordID == 0 {
			keywordID = c.nextKeywordID
			c.nextKeywordID++
			c.keywords[keywordID] = keyword
		}
		keywordIDs = append(keywordIDs, keywordID)
	}
	c.movies[id] = Title{
		ID: id, Name: name, Year: year, Date: fmt.Sprintf("%04d-01-01", year),
		GenreIDs: append([]int(nil), genreIDs...), KeywordIDs: keywordIDs, Overview: overview,
	}
}

// Handler serves the TMDB v3 endpoints internal/tmdb calls, rooted at the base URL the client
// is given (no /3 prefix of its own).
func (c *Catalog) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /search/multi", func(w http.ResponseWriter, r *http.Request) {
		q := strings.ToLower(r.URL.Query().Get("query"))
		var results []map[string]any
		for _, mv := range sortedTitles(c.movies) {
			if strings.Contains(strings.ToLower(mv.Name), q) {
				results = append(results, movieRow(mv))
			}
		}
		for _, s := range sortedTitles(c.series) {
			if strings.Contains(strings.ToLower(s.Name), q) {
				results = append(results, tvRow(s))
			}
		}
		if c.personDecoy {
			results = append(results, map[string]any{"id": 90009999, "media_type": "person", "name": "Some Actor"})
		}
		writeJSON(w, map[string]any{"results": results})
	})
	mux.HandleFunc("GET /search/keyword", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"page": 1, "total_pages": 1, "results": matchingNames(c.keywords, r.URL.Query().Get("query"))})
	})
	mux.HandleFunc("GET /search/person", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"page": 1, "total_pages": 1, "results": matchingNames(c.people, r.URL.Query().Get("query"))})
	})
	mux.HandleFunc("GET /p/exports/{file}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		zipped := gzip.NewWriter(w)
		ids := make([]int, 0, len(c.networks))
		for id := range c.networks {
			ids = append(ids, id)
		}
		sort.Ints(ids)
		for _, id := range ids {
			n := c.networks[id]
			_ = json.NewEncoder(zipped).Encode(map[string]any{"id": n.ID, "name": n.Name})
		}
		_ = zipped.Close()
	})
	mux.HandleFunc("GET /network/{id}", func(w http.ResponseWriter, r *http.Request) {
		n, ok := c.networks[atoiPath(r.PathValue("id"))]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]any{"id": n.ID, "name": n.Name, "origin_country": n.OriginCountry})
	})
	// /discover/{movie,tv} filters the catalogue: comma-separated ids are AND, pipe-separated
	// keywords are OR, as TMDB documents them.
	mux.HandleFunc("GET /discover/movie", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		var results []map[string]any
		for _, mv := range sortedTitles(c.movies) {
			if allIDsMatch(mv.GenreIDs, parseIDs(q.Get("with_genres"))) && anyIDMatches(mv.KeywordIDs, parseIDs(q.Get("with_keywords"))) &&
				allIDsMatch(mv.CastIDs, parseIDs(q.Get("with_cast"))) && allIDsMatch(mv.CreatorIDs, parseIDs(q.Get("with_crew"))) &&
				matchesDiscoveryQualifiers(mv, q) {
				results = append(results, movieRow(mv))
			}
		}
		results, page, totalPages := tmdbPage(results, q.Get("page"))
		writeJSON(w, map[string]any{"page": page, "total_pages": totalPages, "results": results})
	})
	mux.HandleFunc("GET /discover/tv", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		var results []map[string]any
		for _, s := range sortedTitles(c.series) {
			if allIDsMatch(s.GenreIDs, parseIDs(q.Get("with_genres"))) && anyIDMatches(s.KeywordIDs, parseIDs(q.Get("with_keywords"))) &&
				matchesNetwork(s.NetworkID, parseIDs(q.Get("with_networks"))) && matchesDiscoveryQualifiers(s, q) {
				results = append(results, tvRow(s))
			}
		}
		results, page, totalPages := tmdbPage(results, q.Get("page"))
		writeJSON(w, map[string]any{"page": page, "total_pages": totalPages, "results": results})
	})
	mux.HandleFunc("GET /movie/{id}/recommendations", func(w http.ResponseWriter, r *http.Request) { c.recommend(w, r, c.movies) })
	mux.HandleFunc("GET /tv/{id}/recommendations", func(w http.ResponseWriter, r *http.Request) { c.recommend(w, r, c.series) })
	mux.HandleFunc("GET /movie/{id}", func(w http.ResponseWriter, r *http.Request) {
		t, ok := c.movies[atoiPath(r.PathValue("id"))]
		if !ok {
			w.WriteHeader(http.StatusNotFound) // fabricated id → 404 (grounding drops it)
			return
		}
		writeJSON(w, movieDetails(t))
	})
	mux.HandleFunc("GET /tv/{id}", func(w http.ResponseWriter, r *http.Request) {
		t, ok := c.series[atoiPath(r.PathValue("id"))]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeJSON(w, c.seriesDetails(t))
	})
	mux.HandleFunc("GET /tv/{id}/season/{n}", func(w http.ResponseWriter, r *http.Request) {
		number := atoiPath(r.PathValue("n"))
		eps, ok := c.seasons[atoiPath(r.PathValue("id"))][number]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		rows := make([]map[string]any, 0, len(eps))
		for _, e := range eps {
			rows = append(rows, map[string]any{
				"id": e.ID, "episode_number": e.Number, "season_number": number, "name": e.Name,
				"overview": e.Overview, "air_date": e.AirDate, "runtime": e.RuntimeMinutes,
			})
		}
		writeJSON(w, map[string]any{"season_number": number, "episodes": rows})
	})
	mux.HandleFunc("GET /collection/{id}", func(w http.ResponseWriter, r *http.Request) {
		col, ok := c.collections[atoiPath(r.PathValue("id"))]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		parts := make([]map[string]any, 0, len(col.MovieIDs))
		for _, movieID := range col.MovieIDs {
			if title, exists := c.movies[movieID]; exists {
				parts = append(parts, movieRow(title))
			}
		}
		writeJSON(w, map[string]any{"id": col.ID, "name": col.Name, "parts": parts})
	})
	// Content ratings: a US entry iff the title has a USRating; an empty results array is the
	// realistic sparse-coverage answer.
	mux.HandleFunc("GET /tv/{id}/content_ratings", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"results": contentRatings(c.series[atoiPath(r.PathValue("id"))])})
	})
	mux.HandleFunc("GET /movie/{id}/release_dates", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"results": releaseDates(c.movies[atoiPath(r.PathValue("id"))])})
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.RLock()
		defer c.mu.RUnlock()
		mux.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func matchingNames(names map[int]string, query string) []map[string]any {
	q := strings.ToLower(strings.TrimSpace(query))
	ids := make([]int, 0, len(names))
	for id := range names {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	results := make([]map[string]any, 0)
	for _, id := range ids {
		if strings.Contains(strings.ToLower(names[id]), q) {
			results = append(results, map[string]any{"id": id, "name": names[id]})
		}
	}
	return results
}

// movieDetails is a movie's /movie/{id} answer, with the release dates TMDB appends.
func movieDetails(t Title) map[string]any {
	row := map[string]any{
		"id": t.ID, "title": t.Name, "original_title": t.Name, "overview": t.Overview,
		"release_date": t.Date, "runtime": t.RuntimeMinutes, "genres": genres(t),
		"vote_average": t.VoteAverage, "vote_count": t.VoteCount, "belongs_to_collection": nil,
		"release_dates": map[string]any{"results": releaseDates(t)},
	}
	if t.CollectionID > 0 {
		row["belongs_to_collection"] = map[string]any{"id": t.CollectionID, "name": t.CollectionName}
	}
	return row
}

// seriesDetails is a series' /tv/{id} answer, with what TMDB appends: its content ratings, its
// TVDB id, and no other episode orderings, so its episodes are found by season and number.
func (c *Catalog) seriesDetails(t Title) map[string]any {
	numbers := make([]int, 0, len(c.seasons[t.ID]))
	for n := range c.seasons[t.ID] {
		numbers = append(numbers, n)
	}
	sort.Ints(numbers)
	seasons := make([]map[string]any, 0, len(numbers))
	for _, n := range numbers {
		seasons = append(seasons, map[string]any{"season_number": n, "episode_count": len(c.seasons[t.ID][n])})
	}
	runtimes := []int{}
	if t.RuntimeMinutes > 0 {
		runtimes = append(runtimes, t.RuntimeMinutes)
	}
	row := map[string]any{
		"id": t.ID, "name": t.Name, "original_name": t.Name, "overview": t.Overview,
		"first_air_date": t.Date, "episode_run_time": runtimes, "genres": genres(t),
		"vote_average": t.VoteAverage, "vote_count": t.VoteCount, "seasons": seasons,
		"episode_groups":  map[string]any{"results": []any{}},
		"content_ratings": map[string]any{"results": contentRatings(t)},
		"external_ids":    map[string]any{},
	}
	if t.TVDBID > 0 {
		row["external_ids"] = map[string]any{"tvdb_id": t.TVDBID}
	}
	return row
}

// genres pairs GenreIDs with GenreNames, as details carry them.
func genres(t Title) []map[string]any {
	out := make([]map[string]any, 0, len(t.GenreNames))
	for i, name := range t.GenreNames {
		row := map[string]any{"name": name}
		if i < len(t.GenreIDs) {
			row["id"] = t.GenreIDs[i]
		}
		out = append(out, row)
	}
	return out
}

// releaseDates and contentRatings carry a US entry iff the title has a USRating; an empty list is
// the realistic sparse-coverage answer.
func releaseDates(t Title) []map[string]any {
	if t.USRating == "" {
		return []map[string]any{}
	}
	return []map[string]any{{"iso_3166_1": "US", "release_dates": []map[string]any{{"certification": t.USRating}}}}
}

func contentRatings(t Title) []map[string]any {
	if t.USRating == "" {
		return []map[string]any{}
	}
	return []map[string]any{{"iso_3166_1": "US", "rating": t.USRating}}
}

// recommend serves the adjacency graph for one seed; an unwired seed answers an empty list (200).
func (c *Catalog) recommend(w http.ResponseWriter, r *http.Request, from map[int]Title) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	var results []map[string]any
	for _, recID := range c.recommends[id] {
		if t, ok := from[recID]; ok {
			if _, isSeries := c.series[recID]; isSeries {
				results = append(results, tvRow(t))
			} else {
				results = append(results, movieRow(t))
			}
		}
	}
	writeJSON(w, map[string]any{"results": results})
}

func atoiPath(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func movieRow(mv Title) map[string]any {
	return map[string]any{
		"id": mv.ID, "media_type": "movie", "title": mv.Name, "release_date": mv.Date,
		"genre_ids": mv.GenreIDs, "overview": mv.Overview,
		"original_language": mv.OriginalLanguage, "origin_country": mv.OriginCountries,
		"vote_average": mv.VoteAverage, "vote_count": mv.VoteCount,
	}
}

func tvRow(s Title) map[string]any {
	return map[string]any{
		"id": s.ID, "media_type": "tv", "name": s.Name, "first_air_date": s.Date,
		"genre_ids": s.GenreIDs, "overview": s.Overview,
		"original_language": s.OriginalLanguage, "origin_country": s.OriginCountries,
		"vote_average": s.VoteAverage, "vote_count": s.VoteCount,
	}
}

func sortedTitles(catalog map[int]Title) []Title {
	ids := make([]int, 0, len(catalog))
	for id := range catalog {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	out := make([]Title, 0, len(ids))
	for _, id := range ids {
		out = append(out, catalog[id])
	}
	return out
}

func matchesDiscoveryQualifiers(title Title, query url.Values) bool {
	for _, field := range []string{"primary_release_date", "first_air_date"} {
		if minimum := query.Get(field + ".gte"); minimum != "" && title.Date < minimum {
			return false
		}
		if maximum := query.Get(field + ".lte"); maximum != "" && title.Date > maximum {
			return false
		}
	}
	if want := query.Get("with_original_language"); want != "" && !strings.EqualFold(title.OriginalLanguage, want) {
		return false
	}
	if want := query.Get("with_origin_country"); want != "" {
		matched := false
		for _, country := range title.OriginCountries {
			matched = matched || strings.EqualFold(country, want)
		}
		if !matched {
			return false
		}
	}
	if minimum := atoiPath(query.Get("with_runtime.gte")); minimum > 0 && title.RuntimeMinutes < minimum {
		return false
	}
	if maximum := atoiPath(query.Get("with_runtime.lte")); maximum > 0 && title.RuntimeMinutes > maximum {
		return false
	}
	if minimum, _ := strconv.ParseFloat(query.Get("vote_average.gte"), 64); minimum > 0 && title.VoteAverage < minimum {
		return false
	}
	if minimum := atoiPath(query.Get("vote_count.gte")); minimum > 0 && title.VoteCount < minimum {
		return false
	}
	return true
}

// tmdbPage cuts results into TMDB's twenty-row discovery pages. Modelling that boundary is
// load-bearing: a client that never advances page=2 has only widened its local slice.
func tmdbPage(results []map[string]any, rawPage string) ([]map[string]any, int, int) {
	const pageSize = 20
	page := atoiPath(rawPage)
	if page <= 0 {
		page = 1
	}
	totalPages := max(1, (len(results)+pageSize-1)/pageSize)
	start := (page - 1) * pageSize
	if start >= len(results) {
		return nil, page, totalPages
	}
	return results[start:min(start+pageSize, len(results))], page, totalPages
}

// parseIDs splits a comma- or pipe-separated id list.
func parseIDs(v string) []int {
	var out []int
	for _, part := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == '|' }) {
		if id := atoiPath(strings.TrimSpace(part)); id != 0 {
			out = append(out, id)
		}
	}
	return out
}

// allIDsMatch models comma-separated AND semantics.
func allIDsMatch(have, want []int) bool {
	for _, w := range want {
		found := false
		for _, h := range have {
			found = found || h == w
		}
		if !found {
			return false
		}
	}
	return true
}

// anyIDMatches models pipe-separated OR semantics.
func anyIDMatches(have, want []int) bool {
	if len(want) == 0 {
		return true
	}
	for _, w := range want {
		for _, h := range have {
			if h == w {
				return true
			}
		}
	}
	return false
}

func matchesNetwork(have int, want []int) bool {
	return len(want) == 0 || len(want) == 1 && have == want[0]
}
