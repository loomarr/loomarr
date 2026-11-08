package app

import (
	"context"
	"strconv"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// librarySearch is title search's library half (catalog.LibrarySearcher): the movies and series
// the Library holds with a file to play, by name (#2151 step 16). The media server is never asked.
type librarySearch struct {
	titles store.LibraryTitleSearcher
	facts  store.LibraryItemFactsReader
}

func (s librarySearch) Search(ctx context.Context, term string, limit int) ([]library.SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	titles, err := s.titles.SearchLibraryTitles(ctx, term, limit)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(titles))
	for _, t := range titles {
		ids = append(ids, t.ID)
	}
	facts, err := s.facts.LibraryItemFacts(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]library.SearchResult, 0, len(titles))
	for _, t := range titles {
		f := facts[t.ID]
		r := library.SearchResult{
			LibraryItemID: t.ID, Name: t.Name, Year: f.Year, MediaType: library.Movie,
			Genres: f.Genres, Overview: f.Overview, RuntimeMinutes: int(f.RuntimeMs / 60_000),
			OfficialRating: t.Rating, SizeBytes: t.SizeBytes,
		}
		if t.Kind == "series" {
			r.MediaType = library.Series
		}
		r.TMDBID, _ = strconv.Atoi(t.TMDBID)
		r.TVDBID, _ = strconv.Atoi(t.TVDBID)
		out = append(out, r)
	}
	return out, nil
}
