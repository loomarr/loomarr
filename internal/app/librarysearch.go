package app

import (
	"context"
	"errors"
	"slices"
	"strconv"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// librarySearch is title search's library half (catalog.LibrarySearcher), asking the Library first
// (#2151 step 15e): the movies and series it holds with a file to play, by name. The media server
// answers only what the Library didn't, while the Library is short of the limit, so a title the
// Library holds is never answered under a media-server id.
type librarySearch struct {
	titles store.LibraryTitleSearcher
	facts  store.LibraryItemFactsReader
	lib    *library.Client
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
	held := map[string]bool{}
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
		for _, k := range titleKeys(r) {
			held[k] = true
		}
	}
	if len(out) >= limit || s.lib == nil {
		return out, nil
	}
	served, err := s.lib.Search(ctx, term, limit)
	if err != nil {
		// What the Library holds is the answer and the media server only adds to it, so neither
		// its failure beside Library hits nor its absence is an error.
		if len(out) > 0 || errors.Is(err, library.ErrConnectionRequired) {
			return out, nil
		}
		return nil, err
	}
	for _, r := range served {
		if len(out) == limit {
			break
		}
		if slices.ContainsFunc(titleKeys(r), func(k string) bool { return held[k] }) {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// titleKeys names a title by each provider id it has.
func titleKeys(r library.SearchResult) []string {
	var keys []string
	if r.TMDBID > 0 {
		keys = append(keys, string(r.MediaType)+":tmdb:"+strconv.Itoa(r.TMDBID))
	}
	if r.TVDBID > 0 {
		keys = append(keys, string(r.MediaType)+":tvdb:"+strconv.Itoa(r.TVDBID))
	}
	return keys
}
