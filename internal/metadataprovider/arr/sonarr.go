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

// Sonarr is the Sonarr metadata provider: series and their episodes.
type Sonarr struct {
	c client
}

// NewSonarr returns the Sonarr provider. httpClient may be nil, which uses a plain client with a
// request timeout.
func NewSonarr(conn Conn, httpClient *http.Client) *Sonarr {
	return &Sonarr{c: newClient("sonarr", conn, httpClient)}
}

var sonarrSource = mp.Source{Provider: mp.IDSonarr, Basis: mp.BasisManager, Catalog: mp.NamespaceTVDB}

func (*Sonarr) ID() mp.ID { return mp.IDSonarr }

// Capabilities: Sonarr is a manager whose text comes from TVDB. It fetches a series by its TVDB
// id and returns the episodes with it; it has no lookup by an episode's own id.
func (*Sonarr) Capabilities() mp.Capabilities {
	return mp.Capabilities{
		Basis:    sonarrSource.Basis,
		Catalog:  sonarrSource.Catalog,
		Kinds:    []mp.Kind{mp.KindSeries, mp.KindEpisode},
		Resolves: []mp.Resolvable{{Kind: mp.KindSeries, Namespace: mp.NamespaceTVDB}},
		Families: []mp.Family{mp.FamilyIdentity, mp.FamilyDescriptive, mp.FamilyCertification, mp.FamilyTechnical, mp.FamilyArtwork},
		Rate:     mp.RateHint{MaxConcurrent: 1, PreferList: true},
	}
}

// Fetch returns one series, by TVDB id, with its episodes as Children.
func (s *Sonarr) Fetch(ctx context.Context, ref mp.Ref) (mp.Record, error) {
	tvdb, err := numericRef(s.c.app, s.Capabilities(), ref)
	if err != nil {
		return mp.Record{}, err
	}
	ep, err := s.c.endpoint()
	if err != nil {
		return mp.Record{}, err
	}
	var found []sonarrSeries
	if err := s.c.get(ctx, ep, "/api/v3/series", url.Values{"tvdbId": {strconv.Itoa(tvdb)}}, &found); err != nil {
		return mp.Record{}, err
	}
	for _, series := range found {
		if series.TVDBID == tvdb {
			return s.seriesRecord(ctx, ep, series)
		}
	}
	return mp.Record{}, fmt.Errorf("sonarr: series tvdb %d: %w", tvdb, mp.ErrNotFound)
}

// List returns every series Sonarr holds, each with its episodes.
func (s *Sonarr) List(ctx context.Context) ([]mp.Record, error) {
	ep, err := s.c.endpoint()
	if err != nil {
		return nil, err
	}
	var all []sonarrSeries
	if err := s.c.get(ctx, ep, "/api/v3/series", nil, &all); err != nil {
		return nil, err
	}
	out := make([]mp.Record, 0, len(all))
	for _, series := range all {
		rec, err := s.seriesRecord(ctx, ep, series)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}

func (s *Sonarr) seriesRecord(ctx context.Context, ep endpoint, series sonarrSeries) (mp.Record, error) {
	q := url.Values{"seriesId": {strconv.Itoa(series.ID)}}
	var episodes []sonarrEpisode
	if err := s.c.get(ctx, ep, "/api/v3/episode", q, &episodes); err != nil {
		return mp.Record{}, err
	}
	var files []sonarrEpisodeFile
	if err := s.c.get(ctx, ep, "/api/v3/episodefile", q, &files); err != nil {
		return mp.Record{}, err
	}
	byFile := make(map[int]sonarrEpisodeFile, len(files))
	for _, f := range files {
		byFile[f.ID] = f
	}

	rec := mp.Record{
		Source: sonarrSource,
		Kind:   mp.KindSeries,
		IDs: ids(intID(mp.NamespaceTVDB, series.TVDBID), intID(mp.NamespaceTMDB, series.TMDBID),
			mp.ExternalID{Namespace: mp.NamespaceIMDB, Value: strings.TrimSpace(series.IMDBID)}),
		Descriptive: &mp.Descriptive{
			Title:        strings.TrimSpace(series.Title),
			SortTitle:    strings.TrimSpace(series.SortTitle),
			Overview:     strings.TrimSpace(series.Overview),
			PremiereDate: date(series.FirstAired),
			Year:         series.Year,
			Genres:       series.Genres,
			Network:      strings.TrimSpace(series.Network),
			Status:       strings.TrimSpace(series.Status),
			Runtime:      minutes(series.Runtime),
		},
		Artwork: artwork(series.Images),
	}
	if cert := strings.TrimSpace(series.Certification); cert != "" {
		// Sonarr's certification is TVDB's US content rating.
		rec.Certification = &mp.Certification{Rating: cert, Country: "US"}
	}
	for _, e := range episodes {
		rec.Children = append(rec.Children, episodeRecord(rec.IDs, e, byFile))
	}
	return rec, nil
}

func episodeRecord(parent []mp.ExternalID, e sonarrEpisode, byFile map[int]sonarrEpisodeFile) mp.Record {
	rec := mp.Record{
		Source: sonarrSource,
		Kind:   mp.KindEpisode,
		IDs:    ids(intID(mp.NamespaceTVDB, e.TVDBID)),
		Parent: parent,
		Numbering: &mp.Numbering{
			Season:            e.SeasonNumber,
			Episode:           e.EpisodeNumber,
			Absolute:          e.AbsoluteEpisodeNumber,
			AirsAfterSeason:   e.AirsAfterSeasonNumber,
			AirsBeforeSeason:  e.AirsBeforeSeasonNumber,
			AirsBeforeEpisode: e.AirsBeforeEpisodeNumber,
		},
		Descriptive: &mp.Descriptive{
			Title:        strings.TrimSpace(e.Title),
			Overview:     strings.TrimSpace(e.Overview),
			PremiereDate: date(e.AirDate),
			Runtime:      minutes(e.Runtime),
		},
	}
	if f, ok := byFile[e.EpisodeFileID]; ok && e.HasFile && strings.TrimSpace(f.Path) != "" {
		rec.Files = []mp.FileRef{{Path: f.Path, SizeBytes: f.Size}}
		rec.Technical = technical(f.MediaInfo, f.Size)
	}
	return rec
}

// sonarrSeries is the part of Sonarr's /api/v3/series resource Loomarr reads.
type sonarrSeries struct {
	ID            int      `json:"id"`
	Title         string   `json:"title"`
	SortTitle     string   `json:"sortTitle"`
	Status        string   `json:"status"`
	Overview      string   `json:"overview"`
	Network       string   `json:"network"`
	Images        []image  `json:"images"`
	Year          int      `json:"year"`
	Runtime       int      `json:"runtime"`
	TVDBID        int      `json:"tvdbId"`
	TMDBID        int      `json:"tmdbId"`
	IMDBID        string   `json:"imdbId"`
	FirstAired    string   `json:"firstAired"`
	Certification string   `json:"certification"`
	Genres        []string `json:"genres"`
}

// sonarrEpisode is the part of Sonarr's /api/v3/episode resource Loomarr reads.
type sonarrEpisode struct {
	ID                      int    `json:"id"`
	SeriesID                int    `json:"seriesId"`
	TVDBID                  int    `json:"tvdbId"`
	EpisodeFileID           int    `json:"episodeFileId"`
	SeasonNumber            int    `json:"seasonNumber"`
	EpisodeNumber           int    `json:"episodeNumber"`
	Title                   string `json:"title"`
	AirDate                 string `json:"airDate"`
	Overview                string `json:"overview"`
	HasFile                 bool   `json:"hasFile"`
	AbsoluteEpisodeNumber   int    `json:"absoluteEpisodeNumber"`
	AirsAfterSeasonNumber   int    `json:"airsAfterSeasonNumber"`
	AirsBeforeSeasonNumber  int    `json:"airsBeforeSeasonNumber"`
	AirsBeforeEpisodeNumber int    `json:"airsBeforeEpisodeNumber"`
	Runtime                 int    `json:"runtime"`
}

// sonarrEpisodeFile is the part of Sonarr's /api/v3/episodefile resource Loomarr reads.
type sonarrEpisodeFile struct {
	ID        int        `json:"id"`
	Path      string     `json:"path"`
	Size      int64      `json:"size"`
	MediaInfo *mediaInfo `json:"mediaInfo"`
}
