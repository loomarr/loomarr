// Package tmdb provides TMDB as a metadata provider (#2151 decisions 7, 13, 15, 18, 20, 21). It is
// a reference catalog: it returns a movie or a series by TMDB id with the guide text, genres,
// keywords, community rating, collection, US certification, artwork references and external ids
// that one detail call carries (append_to_response), and an episode by its own TVDB episode id
// through TMDB's /find.
//
// An episode fetched by its TVDB id carries no numbering: it is the TMDB view of the episode the id
// names. TMDB and TVDB lay out 81 of 527 reference shows differently, so a number from one can be
// the wrong episode in the other. A season (by TMDB series id and season number) lists its
// episodes numbered, and the scanner uses it only for a show TMDB knows one ordering of whose
// seasons on disk fit TMDB's (#2151 decision 53).
//
// It reuses the shared TMDB client for the credential, transport and metrics, so the key is
// resolved once per operation and never reaches a URL. One installation-wide language and country
// shape the text and the certification; a country other than the US is refused because only the US
// rating ladder exists.
package tmdb

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/platform/httpx"
	"github.com/loomarr/loomarr/internal/platform/rating"
	tmdbclient "github.com/loomarr/loomarr/internal/tmdb"
)

// Locale returns the installation's metadata language (such as "en-US") and certification country
// (ISO 3166-1 alpha-2). It is read once per operation. Empty values mean "en-US" and "US".
type Locale func() (language, country string)

// Window is how far back TMDB's change feeds answer. TMDB rejects a range longer than 14 days.
const Window = 14 * 24 * time.Hour

const (
	// maxKeywords bounds the keywords kept per item. TMDB returns every tag, and a long tail of
	// them says little.
	maxKeywords = 40
	// maxChangePages is TMDB's own page ceiling for a list endpoint.
	maxChangePages = 500
	// usCountry is the one certification country with a ladder.
	usCountry = "US"
)

var source = mp.Source{Provider: mp.IDTMDB, Basis: mp.BasisCatalog, Catalog: mp.NamespaceTMDB}

// Provider is the TMDB metadata provider.
type Provider struct {
	client *tmdbclient.Client
	locale Locale
	now    func() time.Time
}

// New returns the TMDB provider over client. locale may be nil, which reads as en-US and US.
func New(client *tmdbclient.Client, locale Locale) *Provider {
	return &Provider{client: client, locale: locale, now: time.Now}
}

var (
	_ mp.Provider   = (*Provider)(nil)
	_ mp.ChangeFeed = (*Provider)(nil)
)

// ID is the provider's id.
func (*Provider) ID() mp.ID { return mp.IDTMDB }

// Capabilities: TMDB is a catalog whose text comes from TMDB. It fetches movies and series by TMDB
// id and episodes by TVDB episode id. It supplies ids, guide text, certification and artwork; it
// supplies no technical facts because it has not seen the file.
func (*Provider) Capabilities() mp.Capabilities {
	return mp.Capabilities{
		Basis:   source.Basis,
		Catalog: source.Catalog,
		Kinds:   []mp.Kind{mp.KindMovie, mp.KindSeries, mp.KindEpisode, mp.KindCollection, mp.KindSeason},
		Resolves: []mp.Resolvable{
			{Kind: mp.KindMovie, Namespace: mp.NamespaceTMDB},
			{Kind: mp.KindSeries, Namespace: mp.NamespaceTMDB},
			{Kind: mp.KindEpisode, Namespace: mp.NamespaceTVDB},
			{Kind: mp.KindCollection, Namespace: mp.NamespaceTMDBCollection},
			{Kind: mp.KindSeason, Namespace: mp.NamespaceTMDB},
		},
		Families: []mp.Family{mp.FamilyIdentity, mp.FamilyDescriptive, mp.FamilyCertification, mp.FamilyArtwork},
		// TMDB documents roughly 50 requests per second per address and 20 simultaneous
		// connections. 8 in flight at 25 ms apart stays well under both.
		Rate: mp.RateHint{MaxConcurrent: 8, MinInterval: 25 * time.Millisecond},
	}
}

// Fetch returns a movie or series by TMDB id, an episode by TVDB episode id, or a TMDB collection
// by its collection id.
func (p *Provider) Fetch(ctx context.Context, ref mp.Ref) (mp.Record, error) {
	if !p.Capabilities().CanResolve(ref) {
		return mp.Record{}, fmt.Errorf("tmdb: %s by %s: %w", ref.Kind, ref.ID.Namespace, mp.ErrUnsupported)
	}
	if ref.Kind == mp.KindSeason {
		series, number, ok := SeasonRef(ref.ID.Value)
		if !ok {
			return mp.Record{}, fmt.Errorf("tmdb: season %q is not <series id>/<season>: %w", ref.ID.Value, mp.ErrUnsupported)
		}
		loc, err := p.resolveLocale()
		if err != nil {
			return mp.Record{}, err
		}
		return p.season(ctx, loc, series, number)
	}
	id, err := strconv.Atoi(strings.TrimSpace(ref.ID.Value))
	if err != nil || id <= 0 {
		return mp.Record{}, fmt.Errorf("tmdb: %s id %q is not a positive integer: %w", ref.ID.Namespace, ref.ID.Value, mp.ErrUnsupported)
	}
	loc, err := p.resolveLocale()
	if err != nil {
		return mp.Record{}, err
	}
	switch ref.Kind {
	case mp.KindMovie:
		return p.movie(ctx, loc, id)
	case mp.KindSeries:
		return p.series(ctx, loc, id)
	case mp.KindCollection:
		return p.collection(ctx, loc, id)
	default:
		return p.episode(ctx, loc, id)
	}
}

// locale is one operation's language settings.
type locale struct {
	language string // as TMDB takes it, such as "en-US"
	base     string // the ISO 639-1 part, such as "en"
}

func (p *Provider) resolveLocale() (locale, error) {
	var language, country string
	if p.locale != nil {
		language, country = p.locale()
	}
	language = strings.TrimSpace(language)
	if language == "" {
		language = "en-US"
	}
	country = strings.ToUpper(strings.TrimSpace(country))
	if country == "" {
		country = usCountry
	}
	if country != usCountry {
		return locale{}, fmt.Errorf("tmdb: no rating ladder for country %q: %w", country, mp.ErrUnsupported)
	}
	base, _, _ := strings.Cut(language, "-")
	return locale{language: language, base: strings.ToLower(base)}, nil
}

// detailQuery is the query shared by the movie and series detail calls. images are limited to the
// installation's language and textless ones, so a logo or poster in a language nobody reads is not
// returned.
func (l locale) detailQuery(appendTo string) string {
	q := url.Values{}
	q.Set("language", l.language)
	q.Set("append_to_response", appendTo)
	q.Set("include_image_language", l.base+",null")
	return q.Encode()
}

// get decodes path into out and maps the client's errors onto the contract's.
func (p *Provider) get(ctx context.Context, path string, out any) error {
	err := p.client.GetJSON(ctx, path, out)
	var status *httpx.StatusError
	switch {
	case err == nil:
		return nil
	case errors.Is(err, tmdbclient.ErrAPIKeyRequired):
		return fmt.Errorf("tmdb: %w", mp.ErrNotConfigured)
	case errors.As(err, &status) && status.Status == http.StatusNotFound:
		return fmt.Errorf("tmdb %s: %w", pathOnly(path), mp.ErrNotFound)
	}
	return err
}

func pathOnly(path string) string {
	p, _, _ := strings.Cut(path, "?")
	return p
}

// ---- wire shapes ----

type idName struct {
	Name string `json:"name"`
}

type wireImage struct {
	FilePath string `json:"file_path"`
	Language string `json:"iso_639_1"`
}

type wireImages struct {
	Posters   []wireImage `json:"posters"`
	Backdrops []wireImage `json:"backdrops"`
	Logos     []wireImage `json:"logos"`
}

// wireVideos are a title's videos (append_to_response=videos).
type wireVideos struct {
	Results []struct {
		Key      string `json:"key"`
		Site     string `json:"site"`
		Type     string `json:"type"`
		Official bool   `json:"official"`
	} `json:"results"`
}

type wireExternalIDs struct {
	IMDB string `json:"imdb_id"`
	TVDB int    `json:"tvdb_id"`
}

// ---- movie ----

type wireMovie struct {
	ID            int             `json:"id"`
	IMDB          string          `json:"imdb_id"`
	Title         string          `json:"title"`
	OriginalTitle string          `json:"original_title"`
	Overview      string          `json:"overview"`
	ReleaseDate   string          `json:"release_date"`
	Runtime       int             `json:"runtime"`
	Tagline       string          `json:"tagline"`
	VoteAverage   float64         `json:"vote_average"`
	VoteCount     int             `json:"vote_count"`
	Genres        []idName        `json:"genres"`
	Companies     []idName        `json:"production_companies"`
	Countries     []idName        `json:"production_countries"`
	Videos        wireVideos      `json:"videos"`
	PosterPath    string          `json:"poster_path"`
	BackdropPath  string          `json:"backdrop_path"`
	Collection    *wireCollection `json:"belongs_to_collection"`
	ExternalIDs   wireExternalIDs `json:"external_ids"`
	Images        wireImages      `json:"images"`
	Keywords      struct {
		Keywords []idName `json:"keywords"`
	} `json:"keywords"`
	ReleaseDatesWrap struct {
		Results []struct {
			Country string `json:"iso_3166_1"`
			Dates   []struct {
				Certification string `json:"certification"`
			} `json:"release_dates"`
		} `json:"results"`
	} `json:"release_dates"`
}

type wireCollection struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func (p *Provider) movie(ctx context.Context, loc locale, id int) (mp.Record, error) {
	var m wireMovie
	path := "/movie/" + strconv.Itoa(id) + "?" + loc.detailQuery("external_ids,release_dates,keywords,images,videos")
	if err := p.get(ctx, path, &m); err != nil {
		return mp.Record{}, err
	}
	if m.ID != id {
		return mp.Record{}, fmt.Errorf("tmdb movie %d: answer names movie %d: %w", id, m.ID, mp.ErrNotFound)
	}
	rec := mp.Record{
		Source: source,
		Kind:   mp.KindMovie,
		IDs: ids(
			mp.ExternalID{Namespace: mp.NamespaceTMDB, Value: strconv.Itoa(m.ID)},
			mp.ExternalID{Namespace: mp.NamespaceIMDB, Value: firstNonEmpty(m.ExternalIDs.IMDB, m.IMDB)},
		),
		Descriptive: &mp.Descriptive{
			Title:           m.Title,
			OriginalTitle:   m.OriginalTitle,
			Overview:        m.Overview,
			PremiereDate:    m.ReleaseDate,
			Year:            yearOf(m.ReleaseDate),
			Genres:          names(m.Genres),
			Keywords:        keywords(m.Keywords.Keywords),
			CommunityRating: communityRating(m.VoteAverage, m.VoteCount),
			Studios:         names(m.Companies),
			Runtime:         time.Duration(m.Runtime) * time.Minute,
			Tagline:         strings.TrimSpace(m.Tagline),
			Countries:       names(m.Countries),
			Trailer:         trailer(m.Videos),
		},
		Artwork: artwork(loc, m.Images, m.PosterPath, m.BackdropPath),
	}
	if m.Collection != nil && m.Collection.ID > 0 {
		rec.Descriptive.Collection = &mp.CollectionRef{
			ID:   mp.ExternalID{Namespace: mp.NamespaceTMDBCollection, Value: strconv.Itoa(m.Collection.ID)},
			Name: m.Collection.Name,
		}
	}
	var ratings []string
	for _, r := range m.ReleaseDatesWrap.Results {
		if strings.EqualFold(r.Country, usCountry) {
			for _, d := range r.Dates {
				ratings = append(ratings, d.Certification)
			}
		}
	}
	rec.Certification = strictest(ratings)
	return rec, nil
}

// ---- collection ----

type wireCollectionDetail struct {
	ID           int        `json:"id"`
	Name         string     `json:"name"`
	Overview     string     `json:"overview"`
	PosterPath   string     `json:"poster_path"`
	BackdropPath string     `json:"backdrop_path"`
	Images       wireImages `json:"images"`
	Parts        []struct {
		ID          int    `json:"id"`
		MediaType   string `json:"media_type"`
		Title       string `json:"title"`
		ReleaseDate string `json:"release_date"`
	} `json:"parts"`
}

// collection is a TMDB collection: its name, its artwork, and its movies in release order (an
// unreleased one last), each as a record with its TMDB id, title and year.
func (p *Provider) collection(ctx context.Context, loc locale, id int) (mp.Record, error) {
	var c wireCollectionDetail
	path := "/collection/" + strconv.Itoa(id) + "?" + loc.detailQuery("images")
	if err := p.get(ctx, path, &c); err != nil {
		return mp.Record{}, err
	}
	if c.ID != id {
		return mp.Record{}, fmt.Errorf("tmdb collection %d: answer names collection %d: %w", id, c.ID, mp.ErrNotFound)
	}
	parts := c.Parts[:0:0]
	for _, part := range c.Parts {
		if part.ID > 0 && strings.TrimSpace(part.Title) != "" && (part.MediaType == "" || part.MediaType == "movie") {
			parts = append(parts, part)
		}
	}
	sort.SliceStable(parts, func(i, j int) bool {
		a, b := parts[i].ReleaseDate, parts[j].ReleaseDate
		if (a == "") != (b == "") {
			return b == ""
		}
		return a < b
	})
	rec := mp.Record{
		Source:      source,
		Kind:        mp.KindCollection,
		IDs:         []mp.ExternalID{{Namespace: mp.NamespaceTMDBCollection, Value: strconv.Itoa(c.ID)}},
		Descriptive: &mp.Descriptive{Title: c.Name, Overview: c.Overview},
		Artwork:     artwork(loc, c.Images, c.PosterPath, c.BackdropPath),
	}
	for _, part := range parts {
		rec.Children = append(rec.Children, mp.Record{
			Source: source,
			Kind:   mp.KindMovie,
			IDs:    []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: strconv.Itoa(part.ID)}},
			Descriptive: &mp.Descriptive{
				Title: strings.TrimSpace(part.Title), PremiereDate: part.ReleaseDate, Year: yearOf(part.ReleaseDate),
			},
		})
	}
	return rec, nil
}

// ---- series ----

type wireSeries struct {
	ID             int             `json:"id"`
	Name           string          `json:"name"`
	OriginalName   string          `json:"original_name"`
	Overview       string          `json:"overview"`
	FirstAirDate   string          `json:"first_air_date"`
	Status         string          `json:"status"`
	EpisodeRuntime []int           `json:"episode_run_time"`
	VoteAverage    float64         `json:"vote_average"`
	VoteCount      int             `json:"vote_count"`
	Tagline        string          `json:"tagline"`
	Genres         []idName        `json:"genres"`
	Networks       []idName        `json:"networks"`
	Countries      []idName        `json:"production_countries"`
	Videos         wireVideos      `json:"videos"`
	PosterPath     string          `json:"poster_path"`
	BackdropPath   string          `json:"backdrop_path"`
	ExternalIDs    wireExternalIDs `json:"external_ids"`
	Images         wireImages      `json:"images"`
	Seasons        []struct {
		SeasonNumber int    `json:"season_number"`
		EpisodeCount int    `json:"episode_count"`
		PosterPath   string `json:"poster_path"`
	} `json:"seasons"`
	// EpisodeGroups are the other orderings TMDB knows; nil when the answer did not carry them.
	EpisodeGroups *struct {
		Results []struct {
			ID string `json:"id"`
		} `json:"results"`
	} `json:"episode_groups"`
	Keywords struct {
		Results []idName `json:"results"`
	} `json:"keywords"`
	ContentRatings struct {
		Results []struct {
			Country string `json:"iso_3166_1"`
			Rating  string `json:"rating"`
		} `json:"results"`
	} `json:"content_ratings"`
}

func (p *Provider) series(ctx context.Context, loc locale, id int) (mp.Record, error) {
	var s wireSeries
	path := "/tv/" + strconv.Itoa(id) + "?" + loc.detailQuery("external_ids,content_ratings,keywords,images,episode_groups,videos")
	if err := p.get(ctx, path, &s); err != nil {
		return mp.Record{}, err
	}
	if s.ID != id {
		return mp.Record{}, fmt.Errorf("tmdb series %d: answer names series %d: %w", id, s.ID, mp.ErrNotFound)
	}
	var tvdb string
	if s.ExternalIDs.TVDB > 0 {
		tvdb = strconv.Itoa(s.ExternalIDs.TVDB)
	}
	rec := mp.Record{
		Source: source,
		Kind:   mp.KindSeries,
		IDs: ids(
			mp.ExternalID{Namespace: mp.NamespaceTMDB, Value: strconv.Itoa(s.ID)},
			mp.ExternalID{Namespace: mp.NamespaceTVDB, Value: tvdb},
			mp.ExternalID{Namespace: mp.NamespaceIMDB, Value: s.ExternalIDs.IMDB},
		),
		Descriptive: &mp.Descriptive{
			Title:           s.Name,
			OriginalTitle:   s.OriginalName,
			Overview:        s.Overview,
			PremiereDate:    s.FirstAirDate,
			Year:            yearOf(s.FirstAirDate),
			Genres:          names(s.Genres),
			Keywords:        keywords(s.Keywords.Results),
			CommunityRating: communityRating(s.VoteAverage, s.VoteCount),
			Studios:         names(s.Networks),
			Status:          seriesStatus(s.Status),
			Tagline:         strings.TrimSpace(s.Tagline),
			Countries:       names(s.Countries),
			Trailer:         trailer(s.Videos),
		},
		Artwork: artwork(loc, s.Images, s.PosterPath, s.BackdropPath),
	}
	for _, season := range s.Seasons {
		if season.SeasonNumber >= 0 && season.EpisodeCount > 0 {
			rec.Descriptive.Seasons = append(rec.Descriptive.Seasons,
				mp.SeasonSize{Number: season.SeasonNumber, Episodes: season.EpisodeCount})
		}
		if ref := tmdbclient.ImageURL(season.PosterPath); ref != "" && season.SeasonNumber >= 0 {
			rec.Artwork = append(rec.Artwork, mp.ArtworkRef{Kind: mp.KindSeasonPoster, Season: season.SeasonNumber, Ref: ref})
		}
	}
	if s.EpisodeGroups != nil {
		n := len(s.EpisodeGroups.Results)
		rec.Descriptive.Orderings = &n
	}
	if len(s.EpisodeRuntime) > 0 && s.EpisodeRuntime[0] > 0 {
		rec.Descriptive.Runtime = time.Duration(s.EpisodeRuntime[0]) * time.Minute
	}
	var ratings []string
	for _, r := range s.ContentRatings.Results {
		if strings.EqualFold(r.Country, usCountry) {
			ratings = append(ratings, r.Rating)
		}
	}
	rec.Certification = strictest(ratings)
	return rec, nil
}

// seriesStatus maps TMDB's status onto the vocabulary Sonarr uses, so one resolved field never
// mixes two spellings of the same state. A status it does not know is kept lower-cased.
func seriesStatus(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "returning series":
		return "continuing"
	case "ended", "canceled", "cancelled":
		return "ended"
	case "planned", "in production", "pilot":
		return "upcoming"
	default:
		return strings.ToLower(strings.TrimSpace(s))
	}
}

// ---- season ----

// SeasonRef reads a season's id, "<TMDB series id>/<season number>"; false for anything else.
func SeasonRef(value string) (series, number int, ok bool) {
	s, n, found := strings.Cut(strings.TrimSpace(value), "/")
	if !found {
		return 0, 0, false
	}
	series, err := strconv.Atoi(s)
	if err != nil || series <= 0 {
		return 0, 0, false
	}
	number, err = strconv.Atoi(n)
	if err != nil || number < 0 {
		return 0, 0, false
	}
	return series, number, true
}

type wireSeason struct {
	SeasonNumber int `json:"season_number"`
	Episodes     []struct {
		ID            int     `json:"id"`
		EpisodeNumber int     `json:"episode_number"`
		SeasonNumber  int     `json:"season_number"`
		Name          string  `json:"name"`
		Overview      string  `json:"overview"`
		AirDate       string  `json:"air_date"`
		Runtime       int     `json:"runtime"`
		VoteAverage   float64 `json:"vote_average"`
		VoteCount     int     `json:"vote_count"`
	} `json:"episodes"`
}

// season is one season of a series: its episodes in TMDB's numbering, as Children.
func (p *Provider) season(ctx context.Context, loc locale, series, number int) (mp.Record, error) {
	var s wireSeason
	q := url.Values{}
	q.Set("language", loc.language)
	path := "/tv/" + strconv.Itoa(series) + "/season/" + strconv.Itoa(number) + "?" + q.Encode()
	if err := p.get(ctx, path, &s); err != nil {
		return mp.Record{}, err
	}
	if s.SeasonNumber != number || len(s.Episodes) == 0 {
		return mp.Record{}, fmt.Errorf("tmdb series %d season %d: %w", series, number, mp.ErrNotFound)
	}
	parent := []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: strconv.Itoa(series)}}
	rec := mp.Record{Source: source, Kind: mp.KindSeason, Parent: parent}
	for _, e := range s.Episodes {
		if e.ID <= 0 || e.EpisodeNumber <= 0 {
			continue
		}
		rec.Children = append(rec.Children, mp.Record{
			Source: source, Kind: mp.KindEpisode, Parent: parent,
			IDs:       []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: strconv.Itoa(e.ID)}},
			Numbering: &mp.Numbering{Season: number, Episode: e.EpisodeNumber},
			Descriptive: &mp.Descriptive{
				Title: e.Name, Overview: e.Overview, PremiereDate: e.AirDate, Year: yearOf(e.AirDate),
				CommunityRating: communityRating(e.VoteAverage, e.VoteCount),
				Runtime:         time.Duration(e.Runtime) * time.Minute,
			},
		})
	}
	return rec, nil
}

// ---- episode ----

type wireFind struct {
	Episodes []struct {
		ID          int     `json:"id"`
		ShowID      int     `json:"show_id"`
		Name        string  `json:"name"`
		Overview    string  `json:"overview"`
		AirDate     string  `json:"air_date"`
		Runtime     int     `json:"runtime"`
		StillPath   string  `json:"still_path"`
		VoteAverage float64 `json:"vote_average"`
		VoteCount   int     `json:"vote_count"`
	} `json:"tv_episode_results"`
}

// episode resolves a TVDB episode id through /find (decision 15). The record's own ids are the
// TVDB id it was asked by and TMDB's episode id; its parent is TMDB's series id.
func (p *Provider) episode(ctx context.Context, loc locale, tvdb int) (mp.Record, error) {
	var found wireFind
	q := url.Values{}
	q.Set("external_source", "tvdb_id")
	q.Set("language", loc.language)
	path := "/find/" + strconv.Itoa(tvdb) + "?" + q.Encode()
	if err := p.get(ctx, path, &found); err != nil {
		return mp.Record{}, err
	}
	if len(found.Episodes) == 0 || found.Episodes[0].ID <= 0 {
		return mp.Record{}, fmt.Errorf("tmdb episode tvdb %d: %w", tvdb, mp.ErrNotFound)
	}
	e := found.Episodes[0]
	rec := mp.Record{
		Source: source,
		Kind:   mp.KindEpisode,
		IDs: ids(
			mp.ExternalID{Namespace: mp.NamespaceTVDB, Value: strconv.Itoa(tvdb)},
			mp.ExternalID{Namespace: mp.NamespaceTMDB, Value: strconv.Itoa(e.ID)},
		),
		Descriptive: &mp.Descriptive{
			Title:           e.Name,
			Overview:        e.Overview,
			PremiereDate:    e.AirDate,
			Year:            yearOf(e.AirDate),
			CommunityRating: communityRating(e.VoteAverage, e.VoteCount),
			Runtime:         time.Duration(e.Runtime) * time.Minute,
		},
	}
	if e.ShowID > 0 {
		rec.Parent = []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: strconv.Itoa(e.ShowID)}}
	}
	if ref := tmdbclient.ImageURL(e.StillPath); ref != "" {
		rec.Artwork = []mp.ArtworkRef{{Kind: "thumb", Ref: ref}}
	}
	return rec, nil
}

// ---- change feed ----

// Window is how far back the change feeds answer.
func (*Provider) Window() time.Duration { return Window }

type wireChanges struct {
	Results []struct {
		ID int `json:"id"`
	} `json:"results"`
	TotalPages int `json:"total_pages"`
}

// Changes lists the movies and series TMDB changed since a time, as refs. It reports every change
// on TMDB, not only the ones an installation owns; the caller filters. A time older than Window
// cannot be answered, and is ErrUnsupported so the caller refetches instead.
func (p *Provider) Changes(ctx context.Context, since time.Time) ([]mp.Ref, error) {
	now := p.now().UTC()
	if since.Before(now.Add(-Window)) {
		return nil, fmt.Errorf("tmdb: changes since %s are older than the %s feed window: %w",
			since.UTC().Format(time.DateOnly), Window, mp.ErrUnsupported)
	}
	if since.After(now) {
		since = now
	}
	var out []mp.Ref
	for _, feed := range []struct {
		kind mp.Kind
		path string
	}{{mp.KindMovie, "/movie/changes"}, {mp.KindSeries, "/tv/changes"}} {
		refs, err := p.changePages(ctx, feed.kind, feed.path, since.UTC(), now)
		if err != nil {
			return nil, err
		}
		out = append(out, refs...)
	}
	return out, nil
}

func (p *Provider) changePages(ctx context.Context, kind mp.Kind, path string, from, to time.Time) ([]mp.Ref, error) {
	var out []mp.Ref
	seen := map[int]bool{}
	for page := 1; page <= maxChangePages; page++ {
		q := url.Values{}
		q.Set("start_date", from.Format(time.DateOnly))
		q.Set("end_date", to.Format(time.DateOnly))
		q.Set("page", strconv.Itoa(page))
		var body wireChanges
		if err := p.get(ctx, path+"?"+q.Encode(), &body); err != nil {
			return nil, err
		}
		for _, r := range body.Results {
			if r.ID <= 0 || seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			out = append(out, mp.Ref{Kind: kind, ID: mp.ExternalID{Namespace: mp.NamespaceTMDB, Value: strconv.Itoa(r.ID)}})
		}
		if page >= body.TotalPages {
			return out, nil
		}
	}
	return nil, fmt.Errorf("tmdb %s: more than %d pages of changes", path, maxChangePages)
}

// ---- field helpers ----

// ids keeps the ids that have a value.
func ids(all ...mp.ExternalID) []mp.ExternalID {
	var out []mp.ExternalID
	for _, id := range all {
		if v := strings.TrimSpace(id.Value); v != "" {
			id.Value = v
			out = append(out, id)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

func names(list []idName) []string {
	var out []string
	for _, n := range list {
		if v := strings.TrimSpace(n.Name); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// trailer is the web page of a title's trailer: an official YouTube trailer first, then any
// YouTube trailer; "" when it has none.
func trailer(videos wireVideos) string {
	best := -1
	for i, v := range videos.Results {
		if v.Site != "YouTube" || v.Type != "Trailer" || strings.TrimSpace(v.Key) == "" {
			continue
		}
		if best < 0 || (v.Official && !videos.Results[best].Official) {
			best = i
		}
	}
	if best < 0 {
		return ""
	}
	return "https://www.youtube.com/watch?v=" + url.QueryEscape(videos.Results[best].Key)
}

func keywords(list []idName) []string {
	out := names(list)
	if len(out) > maxKeywords {
		out = out[:maxKeywords]
	}
	return out
}

func yearOf(date string) int {
	if len(date) < 4 {
		return 0
	}
	y, err := strconv.Atoi(date[:4])
	if err != nil {
		return 0
	}
	return y
}

// communityRating is TMDB's vote average, or zero when nobody voted: TMDB reports 0.0 for an
// unrated title, which is not a score.
func communityRating(average float64, votes int) float64 {
	if votes <= 0 || average <= 0 || average > 10 {
		return 0
	}
	return average
}

// strictest returns the highest US rating among values that the ladder knows, or nil. A title has
// one rating per release (theatrical, digital, physical), and the strictest is the one enforcement
// must honour (decision 7).
func strictest(values []string) *mp.Certification {
	best, bestRank := "", -1
	for _, v := range values {
		v = strings.TrimSpace(v)
		if rank, ok := rating.Normalize(v).Rank(); ok && rank > bestRank {
			best, bestRank = v, rank
		}
	}
	if best == "" {
		return nil
	}
	return &mp.Certification{Rating: best, Country: usCountry}
}

// artwork picks one image per kind: a poster and a logo in the installation's language (a logo is
// text), fanart without text first. Detail-level paths fill a kind the image list left empty.
func artwork(loc locale, images wireImages, poster, backdrop string) []mp.ArtworkRef {
	var out []mp.ArtworkRef
	add := func(kind, path string) {
		if ref := tmdbclient.ImageURL(path); ref != "" {
			out = append(out, mp.ArtworkRef{Kind: kind, Ref: ref})
		}
	}
	add("poster", firstNonEmpty(pick(images.Posters, loc.base, ""), poster))
	add("fanart", firstNonEmpty(pick(images.Backdrops, "", loc.base), backdrop))
	add("clearlogo", pick(images.Logos, loc.base, ""))
	return out
}

// pick returns the first image in the preferred language, else the first in the fallback
// language, else the first image. Empty preferences match textless images (TMDB's null language).
func pick(images []wireImage, prefer, fallback string) string {
	for _, lang := range []string{prefer, fallback} {
		for _, img := range images {
			if img.FilePath != "" && img.Language == lang {
				return img.FilePath
			}
		}
	}
	for _, img := range images {
		if img.FilePath != "" {
			return img.FilePath
		}
	}
	return ""
}
