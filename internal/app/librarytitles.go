package app

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/library/titlesearch"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// libraryTitleScanner answers the availability scan from the Library's own titles, so a title
// confirms with its library item id (#2151 step 16). The media server is never asked.
type libraryTitleScanner struct {
	titles store.LibraryTitleReader
}

func (s libraryTitleScanner) RecentlyAdded(ctx context.Context, since time.Time) ([]library.SearchResult, error) {
	return s.scan(ctx, since)
}

func (s libraryTitleScanner) AllItems(ctx context.Context) ([]library.SearchResult, error) {
	return s.scan(ctx, time.Time{})
}

func (s libraryTitleScanner) scan(ctx context.Context, since time.Time) ([]library.SearchResult, error) {
	titles, err := s.titles.LibraryTitles(ctx, since)
	if err != nil {
		return nil, err
	}
	out := make([]library.SearchResult, 0, len(titles))
	for _, t := range titles {
		out = append(out, titlesearch.LibrarySearchResult(t))
	}
	return out, nil
}

// libraryLookup is the provisioning backstop's last look before it gives a title up
// (provisionbackstop.LibraryLookup): a title the Library holds is confirmed with its library item
// id (#2151 step 16).
type libraryLookup struct {
	titles store.LibraryTitleReader
}

func (l libraryLookup) Lookup(
	ctx context.Context, kind library.ProviderKind, id string, mt library.MediaType,
) (string, bool, error) {
	t, ok, err := titlesearch.OwnedTitle(ctx, l.titles, kind, id, mt)
	return t.ID, ok, err
}
