package scan

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/loomarr/loomarr/internal/inventory"
	mp "github.com/loomarr/loomarr/internal/metadataprovider"
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

// Decider is what decides which title an item is, as "Wrong title?" needs it (decision 26).
type Decider string

const (
	// DecidedByApp is the arr that manages it: the title changes there.
	DecidedByApp Decider = "app"
	// DecidedByFile is the file itself: an id in its name, or an NFO beside it.
	DecidedByFile Decider = "file"
	// DecidedByYou is the maintainer's answer.
	DecidedByYou Decider = "you"
	// DecidedByNothing is nothing yet: the title needs the maintainer.
	DecidedByNothing Decider = "none"
)

// DecidedBy is what decides which title m says an item is.
func DecidedBy(m store.LibraryMatch) Decider {
	switch mp.Basis(m.Basis) {
	case mp.BasisManager:
		return DecidedByApp
	case mp.BasisEmbeddedID, mp.BasisSidecar:
		return DecidedByFile
	case mp.BasisLink:
		return DecidedByYou
	default:
		return DecidedByNothing
	}
}

// UpdateDetails is "Update details" (decision 20): the title's catalog record is fetched now, and
// its files are written again, so what its arr and its sidecars say now is read too. A title with
// no catalog record, or a catalog with no key, is written again from what the files and arrs say.
func (l *Library) UpdateDetails(ctx context.Context, id string) error {
	item, err := l.ItemStore.LibraryItem(ctx, id)
	if err != nil {
		return err
	}
	if item.Catalog.String() != "" && l.Enricher != nil {
		if err := l.Enricher.Update(ctx, item.Catalog); err != nil && !errors.Is(err, ErrNoCatalog) {
			return err
		}
	}
	files, err := l.NeedStore.RewriteLibraryItemFiles(ctx, id)
	if err != nil {
		return err
	}
	l.pass(files)
	return nil
}

// CheckFiles is "Check files again": the title's files are opened and their headers read again,
// for a file that was replaced in place or failed to read.
func (l *Library) CheckFiles(ctx context.Context, id string) error {
	if _, err := l.ItemStore.LibraryItem(ctx, id); err != nil {
		return err
	}
	files, err := l.ItemStore.RecheckLibraryItemFiles(ctx, id)
	if err != nil {
		return err
	}
	l.pass(files)
	return nil
}

// pass asks the scanner for a pass over the folders holding files.
func (l *Library) pass(files []store.LibraryFilePath) {
	if l.Scanner != nil {
		rewriteFolders(files, l.Scanner.PassFolders)
	}
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
