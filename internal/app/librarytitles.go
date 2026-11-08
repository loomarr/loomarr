package app

import (
	"context"
	"strconv"
	"time"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// libraryTitleScanner answers the availability scan (#2151 step 15): the Library's own titles
// first, so a title confirms with its library item id, then the media server's items for anything
// the Library doesn't hold yet. With no media server configured, the Library's are the answer.
type libraryTitleScanner struct {
	titles store.LibraryTitleReader
	server *library.Client
}

func (s libraryTitleScanner) RecentlyAdded(ctx context.Context, since time.Time) ([]library.SearchResult, error) {
	return s.scan(ctx, since, func(server *library.Client) ([]library.SearchResult, error) {
		return server.RecentlyAdded(ctx, since)
	})
}

func (s libraryTitleScanner) AllItems(ctx context.Context) ([]library.SearchResult, error) {
	return s.scan(ctx, time.Time{}, func(server *library.Client) ([]library.SearchResult, error) {
		return server.AllItems(ctx)
	})
}

// scan keeps LibraryScanner's partial-answer contract: what was read rides with the error of the
// part that failed, so a media server that is down never holds back what the Library confirms.
func (s libraryTitleScanner) scan(
	ctx context.Context, since time.Time, fromServer func(*library.Client) ([]library.SearchResult, error),
) ([]library.SearchResult, error) {
	titles, err := s.titles.LibraryTitles(ctx, since)
	if err != nil {
		return nil, err
	}
	out := make([]library.SearchResult, 0, len(titles))
	for _, t := range titles {
		out = append(out, librarySearchResult(t))
	}
	if s.server == nil {
		return out, nil
	}
	server := s.server.Snapshot()
	if server.Connection() == (library.Connection{}) {
		return out, nil
	}
	items, err := fromServer(server)
	return append(out, items...), err
}

// librarySearchResult reads a Library title as the media server's search result, so scan keys
// come from the one derivation the media server's items use.
func librarySearchResult(t store.LibraryTitle) library.SearchResult {
	r := library.SearchResult{LibraryItemID: t.ID, Name: t.Name, MediaType: library.Movie}
	if t.Kind == "series" {
		r.MediaType = library.Series
	}
	r.TMDBID, _ = strconv.Atoi(t.TMDBID)
	r.TVDBID, _ = strconv.Atoi(t.TVDBID)
	return r
}

// libraryTitle is the Library's movie or series a provider id names, when titles is wired.
func libraryTitle(
	ctx context.Context, titles store.LibraryTitleReader, kind library.ProviderKind, id string, mt library.MediaType,
) (store.LibraryTitle, bool, error) {
	if titles == nil {
		return store.LibraryTitle{}, false, nil
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
