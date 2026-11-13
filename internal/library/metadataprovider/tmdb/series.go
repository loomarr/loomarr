package tmdb

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
	tmdbclient "github.com/loomarr/loomarr/internal/library/tmdb"
)

// ---- series ----

type wireSeries struct {
	ID             int              `json:"id"`
	Name           string           `json:"name"`
	OriginalName   string           `json:"original_name"`
	Overview       string           `json:"overview"`
	FirstAirDate   string           `json:"first_air_date"`
	Status         string           `json:"status"`
	EpisodeRuntime []int            `json:"episode_run_time"`
	VoteAverage    float64          `json:"vote_average"`
	VoteCount      int              `json:"vote_count"`
	Tagline        string           `json:"tagline"`
	Genres         []idName         `json:"genres"`
	Networks       []idName         `json:"networks"`
	Countries      []idName         `json:"production_countries"`
	Videos         wireVideos       `json:"videos"`
	Credits        wireCredits      `json:"credits"`
	CreatedBy      []wireCrew       `json:"created_by"`
	PosterPath     string           `json:"poster_path"`
	BackdropPath   string           `json:"backdrop_path"`
	ExternalIDs    wireExternalIDs  `json:"external_ids"`
	Images         wireImages       `json:"images"`
	Translations   wireTranslations `json:"translations"`
	Seasons        []struct {
		SeasonNumber int    `json:"season_number"`
		EpisodeCount int    `json:"episode_count"`
		PosterPath   string `json:"poster_path"`
	} `json:"seasons"`
	// EpisodeGroups are the other orderings TMDB knows; nil when the answer did not carry them.
	EpisodeGroups *struct {
		Results []struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			Type         int    `json:"type"`
			EpisodeCount int    `json:"episode_count"`
			GroupCount   int    `json:"group_count"`
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

// orderKinds are TMDB's episode group types; an unknown one leaves the kind empty.
var orderKinds = map[int]mp.OrderKind{
	1: mp.OrderAired, 2: mp.OrderAbsolute, 3: mp.OrderDVD, 4: mp.OrderDigital,
	5: mp.OrderStory, 6: mp.OrderProduction, 7: mp.OrderTV,
}

func (p *Provider) series(ctx context.Context, loc locale, id int) (mp.Record, error) {
	var s wireSeries
	path := "/tv/" + strconv.Itoa(id) + "?" + loc.detailQuery("external_ids,content_ratings,keywords,images,episode_groups,videos,credits")
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
			People:          people(s.Credits, s.CreatedBy),
		},
		Artwork: artwork(loc, s.Images, s.PosterPath, s.BackdropPath),
	}
	fillFromEnglish(rec.Descriptive, s.Translations)
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
		rec.Descriptive.Orders = []mp.EpisodeOrder{}
		for _, g := range s.EpisodeGroups.Results {
			rec.Descriptive.Orders = append(rec.Descriptive.Orders, mp.EpisodeOrder{
				ID: g.ID, Name: strings.TrimSpace(g.Name), Kind: orderKinds[g.Type],
				Seasons: g.GroupCount, Episodes: g.EpisodeCount,
			})
		}
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
	path := "/tv/" + strconv.Itoa(series) + "/season/" + strconv.Itoa(number) + "?"
	if err := p.get(ctx, path+languageQuery(loc.language).Encode(), &s); err != nil {
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
	// TMDB has no per-episode translations to append, so an episode with no overview in the
	// installation's language takes the season read once more in English.
	if loc.fallsBack() && anyOverviewMissing(rec.Children) {
		var en wireSeason
		if err := p.get(ctx, path+languageQuery(fallbackLanguage).Encode(), &en); err != nil {
			return mp.Record{}, err
		}
		english := map[string]string{}
		for _, e := range en.Episodes {
			english[strconv.Itoa(e.ID)] = strings.TrimSpace(e.Overview)
		}
		fillEpisodes(rec.Children, english)
	}
	return rec, nil
}

// ---- episode order ----

type wireEpisodeGroup struct {
	ID     string      `json:"id"`
	Groups []wireGroup `json:"groups"`
}

// wireGroup is one season of an episode order; Order is its number.
type wireGroup struct {
	Order    int                `json:"order"`
	Episodes []wireGroupEpisode `json:"episodes"`
}

// wireGroupEpisode is one episode of an order's season; Order is its 0-based place there.
type wireGroupEpisode struct {
	ID          int     `json:"id"`
	ShowID      int     `json:"show_id"`
	Order       int     `json:"order"`
	Name        string  `json:"name"`
	Overview    string  `json:"overview"`
	AirDate     string  `json:"air_date"`
	Runtime     int     `json:"runtime"`
	VoteAverage float64 `json:"vote_average"`
	VoteCount   int     `json:"vote_count"`
}

// episodeOrder is one of a series' other episode orders (a TMDB episode group): its episodes as
// Children, numbered as the order numbers them (the group's order is the season, an episode's
// place in it the episode), and each season's size. An episode TMDB has no id for is left out of
// Children but keeps its place.
func (p *Provider) episodeOrder(ctx context.Context, loc locale, id string) (mp.Record, error) {
	if id == "" || strings.ContainsAny(id, "/?#") {
		return mp.Record{}, fmt.Errorf("tmdb: episode order %q: %w", id, mp.ErrUnsupported)
	}
	var g wireEpisodeGroup
	path := "/tv/episode_group/" + url.PathEscape(id) + "?"
	if err := p.get(ctx, path+languageQuery(loc.language).Encode(), &g); err != nil {
		return mp.Record{}, err
	}
	if g.ID != id {
		return mp.Record{}, fmt.Errorf("tmdb episode order %s: answer names %q: %w", id, g.ID, mp.ErrNotFound)
	}
	rec := mp.Record{
		Source: source, Kind: mp.KindEpisodeOrder,
		IDs:         []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: id}},
		Descriptive: &mp.Descriptive{},
	}
	// TMDB lists seasons and episodes in no promised order; each says its place.
	groups := slices.Clone(g.Groups)
	slices.SortStableFunc(groups, func(a, b wireGroup) int { return a.Order - b.Order })
	for _, group := range groups {
		episodes := slices.Clone(group.Episodes)
		slices.SortStableFunc(episodes, func(a, b wireGroupEpisode) int { return a.Order - b.Order })
		for _, e := range episodes {
			if e.ID <= 0 || e.Order < 0 {
				continue
			}
			if rec.Parent == nil && e.ShowID > 0 {
				rec.Parent = []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: strconv.Itoa(e.ShowID)}}
			}
			rec.Children = append(rec.Children, mp.Record{
				Source: source, Kind: mp.KindEpisode,
				IDs:       []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: strconv.Itoa(e.ID)}},
				Numbering: &mp.Numbering{Season: group.Order, Episode: e.Order + 1},
				Descriptive: &mp.Descriptive{
					Title: e.Name, Overview: e.Overview, PremiereDate: e.AirDate, Year: yearOf(e.AirDate),
					CommunityRating: communityRating(e.VoteAverage, e.VoteCount),
					Runtime:         time.Duration(e.Runtime) * time.Minute,
				},
			})
		}
		// A season's size is its places, an episode TMDB has no id for included: what the last
		// episode on disk is compared with.
		if len(episodes) > 0 {
			rec.Descriptive.Seasons = append(rec.Descriptive.Seasons,
				mp.SeasonSize{Number: group.Order, Episodes: len(episodes)})
		}
	}
	if len(rec.Children) == 0 {
		return mp.Record{}, fmt.Errorf("tmdb episode order %s has no episodes: %w", id, mp.ErrNotFound)
	}
	for i := range rec.Children {
		rec.Children[i].Parent = rec.Parent
	}
	if loc.fallsBack() && anyOverviewMissing(rec.Children) {
		var en wireEpisodeGroup
		if err := p.get(ctx, path+languageQuery(fallbackLanguage).Encode(), &en); err != nil {
			return mp.Record{}, err
		}
		english := map[string]string{}
		for _, group := range en.Groups {
			for _, e := range group.Episodes {
				english[strconv.Itoa(e.ID)] = strings.TrimSpace(e.Overview)
			}
		}
		fillEpisodes(rec.Children, english)
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
	find := func(language string) (wireFind, error) {
		var found wireFind
		q := languageQuery(language)
		q.Set("external_source", "tvdb_id")
		err := p.get(ctx, "/find/"+strconv.Itoa(tvdb)+"?"+q.Encode(), &found)
		return found, err
	}
	found, err := find(loc.language)
	if err != nil {
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
		rec.Artwork = []mp.ArtworkRef{{Kind: mp.KindThumb, Ref: ref}}
	}
	if loc.fallsBack() && strings.TrimSpace(e.Overview) == "" {
		en, err := find(fallbackLanguage)
		if err != nil {
			return mp.Record{}, err
		}
		if len(en.Episodes) > 0 && en.Episodes[0].ID == e.ID {
			fillOverview(rec.Descriptive, strings.TrimSpace(en.Episodes[0].Overview))
		}
	}
	return rec, nil
}
