package main

import (
	"fmt"
	"net/http"

	"github.com/loomarr/loomarr/internal/demolibrary"
	"github.com/loomarr/loomarr/internal/tmdb"
	"github.com/loomarr/loomarr/internal/tmdb/tmdbstandin"
)

// TMDBPrefix is where `serve` mounts the TMDB stand-in (#1941). A backend started with
// LOOMARR_TMDB_BASE_URL=http://<addr>/tmdb/3 grounds AI suggestions in the demo catalogue, so
// every title it searches, validates or suggests is invented.
const TMDBPrefix = "/tmdb/3"

// tmdbCatalog serves the demo library's titles as TMDB would, keyed on each title's provider id.
// That is the TMDB id a film is keyed on, so a suggested film matches its library item. A series
// is keyed on TVDB, so its stand-in TMDB id is only the same number.
func tmdbCatalog(lib *demolibrary.Library) *tmdbstandin.Catalog {
	c := tmdbstandin.New()
	for _, t := range lib.Titles {
		var genres []int
		for _, g := range t.Genres {
			if id, ok := tmdb.GenreID(g); ok {
				genres = append(genres, id)
			}
		}
		var names []string
		for _, g := range t.Genres {
			if _, ok := tmdb.GenreID(g); ok {
				names = append(names, g)
			}
		}
		title := tmdbstandin.Title{
			ID: t.ProviderID(), Name: t.Name, Year: t.Year, GenreIDs: genres, GenreNames: names, Overview: t.Overview,
			OriginalLanguage: "en", OriginCountries: []string{"US"}, USRating: t.Rating,
		}
		if t.Kind != demolibrary.Series {
			if f, ok := lib.Shape(t.ID()); ok {
				title.RuntimeMinutes = f.Duration / 60
			}
			c.PutMovie(title)
			continue
		}
		// A show's folder names both ids (demolibrary.Layout.EpisodeFile), so Loomarr's scan fetches
		// it by TMDB and keys it by TVDB, as the media server did.
		title.TVDBID = t.ProviderID()
		eps := lib.EpisodesOf(t)
		seasons := map[int][]tmdbstandin.Episode{}
		for _, e := range eps {
			ep := tmdbstandin.Episode{
				ID: t.ProviderID()*1000 + e.Season*100 + e.Number, Number: e.Number, Name: e.Name,
				Overview: e.Overview, AirDate: fmt.Sprintf("%04d-01-01", t.Year),
			}
			if ep.Overview == "" {
				ep.Overview = t.Overview
			}
			if e.Year != 0 {
				ep.AirDate = fmt.Sprintf("%04d-01-01", e.Year)
			}
			if f, ok := lib.Shape(e.ID); ok {
				ep.RuntimeMinutes = f.Duration / 60
				title.RuntimeMinutes = ep.RuntimeMinutes
			}
			seasons[e.Season] = append(seasons[e.Season], ep)
		}
		c.PutSeries(title)
		for n, list := range seasons {
			c.PutSeason(t.ProviderID(), n, list)
		}
	}
	return c
}

// withTMDB serves the TMDB stand-in under TMDBPrefix and everything else from media.
func withTMDB(media http.Handler, lib *demolibrary.Library) http.Handler {
	mux := http.NewServeMux()
	mux.Handle(TMDBPrefix+"/", http.StripPrefix(TMDBPrefix, tmdbCatalog(lib).Handler()))
	mux.Handle("/", media)
	return mux
}
