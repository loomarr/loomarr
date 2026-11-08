package app

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/channels/ideas"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/provisionbackstop"
)

// ideaLibraryTTL is how long one library listing serves channel ideas (#1665). Listing is one
// media-server call over the whole library and Home asks on every visit; an arrival shows in ideas
// within this.
const ideaLibraryTTL = 5 * time.Minute

type libraryLister interface {
	AllItems(ctx context.Context) ([]library.SearchResult, error)
}

// libraryIdeaTitles lists what channel ideas draw from, asking the Library first (#2151 step 15e):
// its movies and series with a file to play, with their year and genres, then only the media
// server's titles the Library doesn't hold (by provider id), so no title counts twice in a facet.
// With no media server, the Library's are the answer.
type libraryIdeaTitles struct {
	titles store.LibraryTitleReader
	facts  store.LibraryItemFactsReader
	server *library.Client
}

func (l libraryIdeaTitles) AllItems(ctx context.Context) ([]library.SearchResult, error) {
	titles, err := l.titles.LibraryTitles(ctx, time.Time{})
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(titles))
	for _, t := range titles {
		ids = append(ids, t.ID)
	}
	facts, err := l.facts.LibraryItemFacts(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]library.SearchResult, 0, len(titles))
	held := map[provision.Key]bool{}
	for _, t := range titles {
		r := librarySearchResult(t)
		r.Year, r.Genres = facts[t.ID].Year, facts[t.ID].Genres
		out = append(out, r)
		for _, k := range provisionbackstop.ScanItemKeys(r) {
			held[k] = true
		}
	}
	if l.server == nil {
		return out, nil
	}
	served, err := l.server.AllItems(ctx)
	if err != nil {
		// What the Library holds is the answer; the media server only adds to it.
		if len(out) > 0 || errors.Is(err, library.ErrConnectionRequired) {
			return out, nil
		}
		return nil, err
	}
	for _, r := range served {
		if !slices.ContainsFunc(provisionbackstop.ScanItemKeys(r), func(k provision.Key) bool { return held[k] }) {
			out = append(out, r)
		}
	}
	return out, nil
}

// libraryIdeaItems adapts a title listing to api.IdeaLibrary, caching the listing.
type libraryIdeaItems struct {
	lib libraryLister
	now func() time.Time

	mu     sync.Mutex
	listed time.Time
	items  []ideas.Item
}

func (a *libraryIdeaItems) IdeaItems(ctx context.Context) ([]ideas.Item, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	if a.items != nil && now.Sub(a.listed) < ideaLibraryTTL {
		return a.items, nil
	}
	results, err := a.lib.AllItems(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]ideas.Item, 0, len(results))
	for _, r := range results {
		keys := provisionbackstop.ScanItemKeys(r)
		if len(keys) == 0 {
			continue // no provider id: nothing a lineup could name
		}
		mt := provision.Movie
		if r.MediaType == library.Series {
			mt = provision.Series
		}
		items = append(items, ideas.Item{Keys: keys, MediaType: mt, Name: r.Name, Year: r.Year, Genres: r.Genres})
	}
	a.items, a.listed = items, now
	return items, nil
}
