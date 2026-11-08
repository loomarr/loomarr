package arr

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	mp "github.com/loomarr/loomarr/internal/metadataprovider"
)

// Radarr is the Radarr metadata provider: movies.
type Radarr struct {
	c client
}

// NewRadarr returns the Radarr provider. httpClient may be nil, which uses a plain client with a
// request timeout.
func NewRadarr(conn Conn, httpClient *http.Client) *Radarr {
	return &Radarr{c: newClient("radarr", conn, httpClient)}
}

var radarrSource = mp.Source{Provider: mp.IDRadarr, Basis: mp.BasisManager, Catalog: mp.NamespaceTMDB}

func (*Radarr) ID() mp.ID { return mp.IDRadarr }

// Capabilities: Radarr is a manager whose text comes from TMDB. It fetches a movie by TMDB id.
func (*Radarr) Capabilities() mp.Capabilities {
	return mp.Capabilities{
		Basis:    radarrSource.Basis,
		Catalog:  radarrSource.Catalog,
		Kinds:    []mp.Kind{mp.KindMovie},
		Resolves: []mp.Resolvable{{Kind: mp.KindMovie, Namespace: mp.NamespaceTMDB}},
		Families: []mp.Family{mp.FamilyIdentity, mp.FamilyDescriptive, mp.FamilyCertification, mp.FamilyTechnical, mp.FamilyArtwork},
		Rate:     mp.RateHint{MaxConcurrent: 1, PreferList: true},
	}
}

// Fetch returns one movie by TMDB id.
func (r *Radarr) Fetch(ctx context.Context, ref mp.Ref) (mp.Record, error) {
	tmdb, err := numericRef(r.c.app, r.Capabilities(), ref)
	if err != nil {
		return mp.Record{}, err
	}
	ep, err := r.c.endpoint()
	if err != nil {
		return mp.Record{}, err
	}
	country, err := r.certificationCountry(ctx, ep)
	if err != nil {
		return mp.Record{}, err
	}
	var found []radarrMovie
	if err := r.c.get(ctx, ep, "/api/v3/movie", url.Values{"tmdbId": {strconv.Itoa(tmdb)}}, &found); err != nil {
		return mp.Record{}, err
	}
	for _, movie := range found {
		if movie.TMDBID == tmdb {
			return movieRecord(movie, country, ep.base), nil
		}
	}
	return mp.Record{}, fmt.Errorf("radarr: movie tmdb %d: %w", tmdb, mp.ErrNotFound)
}

// List returns every movie Radarr holds.
func (r *Radarr) List(ctx context.Context) ([]mp.Record, error) {
	ep, err := r.c.endpoint()
	if err != nil {
		return nil, err
	}
	country, err := r.certificationCountry(ctx, ep)
	if err != nil {
		return nil, err
	}
	var all []radarrMovie
	if err := r.c.get(ctx, ep, "/api/v3/movie", nil, &all); err != nil {
		return nil, err
	}
	out := make([]mp.Record, 0, len(all))
	for _, movie := range all {
		out = append(out, movieRecord(movie, country, ep.base))
	}
	return out, nil
}

// certificationCountry is the country whose ratings Radarr reports, upper case. A Radarr that
// does not say yields empty, and its ratings are then left out rather than guessed.
func (r *Radarr) certificationCountry(ctx context.Context, ep endpoint) (string, error) {
	var cfg struct {
		CertificationCountry string `json:"certificationCountry"`
	}
	if err := r.c.get(ctx, ep, "/api/v3/config/metadata", nil, &cfg); err != nil {
		if isStatus(err, http.StatusNotFound) {
			return "", nil
		}
		return "", err
	}
	return strings.ToUpper(strings.TrimSpace(cfg.CertificationCountry)), nil
}

func movieRecord(m radarrMovie, country, base string) mp.Record {
	rec := mp.Record{
		Source: radarrSource,
		Kind:   mp.KindMovie,
		Page:   page(base, "movie", m.TitleSlug),
		IDs: ids(intID(mp.NamespaceTMDB, m.TMDBID),
			mp.ExternalID{Namespace: mp.NamespaceIMDB, Value: strings.TrimSpace(m.IMDBID)}),
		Descriptive: &mp.Descriptive{
			Title:         strings.TrimSpace(m.Title),
			OriginalTitle: strings.TrimSpace(m.OriginalTitle),
			SortTitle:     strings.TrimSpace(m.SortTitle),
			Overview:      strings.TrimSpace(m.Overview),
			PremiereDate:  firstDate(m.InCinemas, m.DigitalRelease, m.PhysicalRelease),
			Year:          m.Year,
			Genres:        m.Genres,
			Studios:       named(m.Studio),
			Runtime:       minutes(m.Runtime),
		},
		Artwork: artwork(m.Images),
	}
	if cert := strings.TrimSpace(m.Certification); cert != "" && country != "" {
		rec.Certification = &mp.Certification{Rating: cert, Country: country}
	}
	if f := m.MovieFile; m.HasFile && f != nil && strings.TrimSpace(f.Path) != "" {
		rec.Files = []mp.FileRef{{Path: f.Path, SizeBytes: f.Size}}
		rec.Technical = technical(f.MediaInfo, f.Size)
		// The edition is the current file's, as Radarr read it from the release name.
		rec.Descriptive.Edition = strings.TrimSpace(f.Edition)
	}
	return rec
}

func firstDate(candidates ...string) string {
	for _, c := range candidates {
		if d := date(c); d != "" {
			return d
		}
	}
	return ""
}

// radarrMovie is the part of Radarr's /api/v3/movie resource Loomarr reads.
type radarrMovie struct {
	ID              int              `json:"id"`
	Title           string           `json:"title"`
	OriginalTitle   string           `json:"originalTitle"`
	SortTitle       string           `json:"sortTitle"`
	Overview        string           `json:"overview"`
	InCinemas       string           `json:"inCinemas"`
	PhysicalRelease string           `json:"physicalRelease"`
	DigitalRelease  string           `json:"digitalRelease"`
	Images          []image          `json:"images"`
	Year            int              `json:"year"`
	HasFile         bool             `json:"hasFile"`
	Studio          string           `json:"studio"`
	Runtime         int              `json:"runtime"`
	IMDBID          string           `json:"imdbId"`
	TMDBID          int              `json:"tmdbId"`
	TitleSlug       string           `json:"titleSlug"`
	Certification   string           `json:"certification"`
	Genres          []string         `json:"genres"`
	MovieFile       *radarrMovieFile `json:"movieFile"`
}

type radarrMovieFile struct {
	Path      string     `json:"path"`
	Size      int64      `json:"size"`
	Edition   string     `json:"edition"`
	MediaInfo *mediaInfo `json:"mediaInfo"`
}
