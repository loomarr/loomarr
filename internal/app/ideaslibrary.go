package app

import (
	"context"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/ideas"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/reconcile"
)

// ideaLibraryTTL is how long one library listing serves channel ideas (#1665). Listing is one
// media-server call over the whole library and Home asks on every visit; an arrival shows in ideas
// within this.
const ideaLibraryTTL = 5 * time.Minute

type libraryLister interface {
	AllItems(ctx context.Context) ([]library.SearchResult, error)
}

// libraryIdeaItems adapts the library client to api.IdeaLibrary, caching the listing.
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
		keys := reconcile.ScanItemKeys(r)
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
