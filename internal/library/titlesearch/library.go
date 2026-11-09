package titlesearch

import (
	"context"
	"strconv"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/provision"
)

// LibrarySearch is title search's library half (LibrarySearcher): the movies and series the
// Library holds with a file to play, by name (#2151 step 16). The media server is never asked.
type LibrarySearch struct {
	Titles librarystore.LibraryTitleSearcher
	Facts  librarystore.LibraryItemFactsReader
}

// Search answers with the Library's titles whose name matches term, best match first.
func (s LibrarySearch) Search(ctx context.Context, term string, limit int) ([]library.SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	titles, err := s.Titles.SearchLibraryTitles(ctx, term, limit)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(titles))
	for _, t := range titles {
		ids = append(ids, t.ID)
	}
	facts, err := s.Facts.LibraryItemFacts(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]library.SearchResult, 0, len(titles))
	for _, t := range titles {
		f := facts[t.ID]
		r := LibrarySearchResult(t)
		r.Year, r.Genres, r.Overview, r.RuntimeMinutes = f.Year, f.Genres, f.Overview, int(f.RuntimeMs/60_000)
		r.OfficialRating, r.SizeBytes = t.Rating, t.SizeBytes
		out = append(out, r)
	}
	return out, nil
}

// LibrarySearchResult reads a Library title as a search result: its library item id, name, kind
// and provider ids, so keys come from the one derivation every title uses.
func LibrarySearchResult(t librarystore.LibraryTitle) library.SearchResult {
	r := library.SearchResult{LibraryItemID: t.ID, Name: t.Name, MediaType: library.Movie}
	if t.Kind == "series" {
		r.MediaType = library.Series
	}
	// Provisioning keys speak TMDB and TVDB (the arrs' ids), so those are read by name.
	r.TMDBID, _ = strconv.Atoi(librarystore.IDIn(t.IDs, string(library.TMDB)))
	r.TVDBID, _ = strconv.Atoi(librarystore.IDIn(t.IDs, string(library.TVDB)))
	return r
}

// LibraryOwnership is LibraryPresence over the Library's titles: discovery marks a title the
// household already owns as in-library, with its library item id and rating (#2151 step 16). It
// prefers the TMDB id (the discovery id space) and falls back to TVDB for a series with no TMDB id.
type LibraryOwnership struct {
	Titles librarystore.LibraryTitleReader
}

// Present reports whether the Library holds the title, with its library item id and rating.
func (o LibraryOwnership) Present(ctx context.Context, mt provision.MediaType, tmdbID, tvdbID int) (Presence, bool, error) {
	kind, id := library.TMDB, strconv.Itoa(tmdbID)
	if tmdbID == 0 && tvdbID != 0 {
		kind, id = library.TVDB, strconv.Itoa(tvdbID)
	}
	lmt := library.Movie
	if mt == provision.Series {
		lmt = library.Series
	}
	t, ok, err := OwnedTitle(ctx, o.Titles, kind, id, lmt)
	if err != nil || !ok {
		return Presence{}, false, err
	}
	return Presence{LibraryItemID: t.ID, OfficialRating: t.Rating, Genres: t.Genres, SizeBytes: t.SizeBytes}, true, nil
}

// OwnedTitle is the Library's movie or series a provider id names. A nil reader owns nothing.
func OwnedTitle(
	ctx context.Context, titles librarystore.LibraryTitleReader, kind library.ProviderKind, id string, mt library.MediaType,
) (librarystore.LibraryTitle, bool, error) {
	if titles == nil {
		return librarystore.LibraryTitle{}, false, nil
	}
	namespace, libraryKind := "tmdb", "movie"
	if kind == library.TVDB {
		namespace = "tvdb"
	}
	if mt == library.Series {
		libraryKind = "series"
	}
	return titles.LibraryTitleByProviderID(ctx, libraryKind, namespace, id)
}
