package main

import (
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
		title := tmdbstandin.Title{
			ID: t.ProviderID(), Name: t.Name, Year: t.Year, GenreIDs: genres, Overview: t.Overview,
			OriginalLanguage: "en", OriginCountries: []string{"US"}, USRating: t.Rating,
		}
		if t.Kind == demolibrary.Series {
			c.PutSeries(title)
		} else {
			c.PutMovie(title)
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
