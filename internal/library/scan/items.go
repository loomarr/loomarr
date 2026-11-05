package scan

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/loomarr/loomarr/internal/inventory"
	"github.com/loomarr/loomarr/internal/store"
)

// Library → Items (#2151 step 10): the movies and series the scanner wrote, and one title's page.

// ItemDetail is one title as its page shows it.
type ItemDetail struct {
	store.LibraryItemSummary
	// Facts is what the scanner resolved for it: overview, genres, rating, runtime.
	Facts inventory.ItemFacts
	// Files is a movie's present files; a series lists its episodes in Seasons instead.
	Files []store.LibraryFile
	// Seasons is a series' episodes by season, in order; season 0 is the specials.
	Seasons []Season
}

// Season is one season's episodes in order.
type Season struct {
	Number   int
	Episodes []Episode
}

// Episode is one episode a series' files hold.
type Episode struct {
	ID     string
	Name   string
	Number int
	// End is the last episode a multi-episode file holds, 0 for one.
	End int
	// Ready is false while any of its files' headers is unread.
	Ready bool
}

// Items lists the movies and series with a present file.
func (l *Library) Items(ctx context.Context, q store.LibraryItemQuery) (store.LibraryItemPage, error) {
	return l.ItemStore.ListLibraryItems(ctx, q)
}

// Item is one title's page; store.ErrNotFound when it has no present file.
func (l *Library) Item(ctx context.Context, id string) (ItemDetail, error) {
	summary, err := l.ItemStore.LibraryItem(ctx, id)
	if err != nil {
		return ItemDetail{}, err
	}
	detail := ItemDetail{LibraryItemSummary: summary}
	if detail.Facts, err = l.scannedFacts(ctx, id); err != nil {
		return ItemDetail{}, err
	}
	files, err := l.ItemStore.LibraryItemFiles(ctx, id)
	if err != nil {
		return ItemDetail{}, err
	}
	if summary.Kind != string(kindSeries) {
		detail.Files = files
		return detail, nil
	}
	detail.Seasons, err = l.seasons(ctx, files)
	return detail, err
}

// seasons groups a series' episode files by the episode item each was written as.
func (l *Library) seasons(ctx context.Context, files []store.LibraryFile) ([]Season, error) {
	ready := map[string]bool{}
	var order []string
	for _, f := range files {
		if _, seen := ready[f.ItemID]; !seen {
			order = append(order, f.ItemID)
			ready[f.ItemID] = true
		}
		if f.Header == store.LibraryHeaderPending {
			ready[f.ItemID] = false
		}
	}
	bySeason := map[int][]Episode{}
	for _, id := range order {
		facts, err := l.scannedFacts(ctx, id)
		if err != nil {
			return nil, err
		}
		bySeason[facts.SeasonNumber] = append(bySeason[facts.SeasonNumber], Episode{
			ID: id, Name: facts.Name, Number: facts.EpisodeNumber, End: facts.EpisodeEnd, Ready: ready[id],
		})
	}
	seasons := make([]Season, 0, len(bySeason))
	for n, eps := range bySeason {
		slices.SortFunc(eps, func(a, b Episode) int {
			return cmp.Or(cmp.Compare(a.Number, b.Number), strings.Compare(a.Name, b.Name))
		})
		seasons = append(seasons, Season{Number: n, Episodes: eps})
	}
	slices.SortFunc(seasons, func(a, b Season) int { return cmp.Compare(a.Number, b.Number) })
	return seasons, nil
}

// scannedFacts is what the newest present scan origin of an item says about it.
func (l *Library) scannedFacts(ctx context.Context, id string) (inventory.ItemFacts, error) {
	item, ok, err := l.Inventory.Item(ctx, inventory.ItemRef{ID: inventory.ItemID(id)})
	if err != nil || !ok {
		return inventory.ItemFacts{}, err
	}
	var best *inventory.ItemOrigin
	for i, o := range item.Origins {
		if !strings.HasPrefix(string(o.Key.Authority), authorityPrefix) || !o.MissingAt.IsZero() {
			continue
		}
		if best == nil || o.Observation.ObservedAt.After(best.Observation.ObservedAt) {
			best = &item.Origins[i]
		}
	}
	if best == nil {
		return inventory.ItemFacts{}, nil
	}
	return best.Observation.Facts, nil
}
