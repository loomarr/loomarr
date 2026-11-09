package scan

import (
	"context"
	"errors"

	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// OrderNeed is a show whose episodes keep their files' names and numbers until the household says
// which order its files follow (#2151 decision 53).
type OrderNeed struct {
	store.LibraryOrderNeed
	// Choices are the catalog's own seasons, then its other orders in the catalog's order. Empty
	// while the show's record isn't cached.
	Choices []OrderChoice
}

// OrderChoice is one order a show's files may follow.
type OrderChoice struct {
	// Choice is OrderByCatalog (the catalog's own seasons) or OrderByOrder (Order, by its id).
	Choice store.LibraryOrderChoice
	Order  mp.EpisodeOrder
	// Seasons is its layout: how many episodes each season has. Nil while an order's layout
	// hasn't been fetched yet.
	Seasons []mp.SeasonSize
	// Fits is whether every season on disk ends where its season does.
	Fits bool
}

// ErrUnknownOrder is an answer naming an order the show's catalog doesn't list.
var ErrUnknownOrder = errors.New("the show's catalog lists no such order")

// OrderNeeds lists the shows whose episodes wait on the household's answer, each with the orders
// its catalog knows and which of them fit the disk.
func (l *Library) OrderNeeds(ctx context.Context) ([]OrderNeed, error) {
	if l.Orders == nil {
		return []OrderNeed{}, nil
	}
	needs, err := l.Orders.LibraryOrderNeeds(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]OrderNeed, 0, len(needs))
	for _, n := range needs {
		need := OrderNeed{LibraryOrderNeed: n, Choices: []OrderChoice{}}
		series, ok, err := l.record(ctx, n.Catalog)
		if err != nil {
			return nil, err
		}
		if ok {
			d := series.Descriptive
			need.Choices = append(need.Choices, OrderChoice{
				Choice: store.OrderByCatalog, Seasons: d.Seasons, Fits: fitsDisk(d.Seasons, n.Last),
			})
			for _, o := range d.Orders {
				choice := OrderChoice{Choice: store.OrderByOrder, Order: o}
				layout, ok, err := l.record(ctx, orderKey(n.Catalog, o.ID))
				if err != nil {
					return nil, err
				}
				if ok {
					choice.Seasons = layout.Descriptive.Seasons
					choice.Fits = fitsDisk(choice.Seasons, n.Last)
				}
				need.Choices = append(need.Choices, choice)
			}
		}
		out = append(out, need)
	}
	return out, nil
}

// ChooseOrder answers which order a show's files follow: the catalog's own seasons, one of its
// other orders (orderID), or the files. Its episodes are written again with the answer.
func (l *Library) ChooseOrder(ctx context.Context, seriesID string, choice store.LibraryOrderChoice, orderID string) error {
	if l.Orders == nil {
		return ErrNotAnswerable
	}
	if (choice == store.OrderByOrder) != (orderID != "") {
		return store.ErrBadEpisodeOrder
	}
	kind, origin, err := l.origin(ctx, seriesID)
	if err != nil {
		return err
	}
	page, err := l.ItemStore.ListLibraryItems(ctx, store.LibraryItemQuery{Kind: kind, IDs: []string{seriesID}})
	if err != nil {
		return err
	}
	if kind != string(kindSeries) || len(page.Items) == 0 || page.Items[0].Catalog.String() == "" {
		return ErrNotAnswerable
	}
	series := page.Items[0].Catalog
	if choice == store.OrderByOrder {
		rec, ok, err := l.record(ctx, series)
		if err != nil {
			return err
		}
		known := false
		if ok {
			for _, o := range rec.Descriptive.Orders {
				known = known || o.ID == orderID
			}
		}
		if !known {
			return ErrUnknownOrder
		}
	}
	if err := l.Orders.SaveLibraryEpisodeOrder(ctx, store.LibraryEpisodeOrder{
		Series: series, Choice: choice, OrderID: orderID, AnsweredAt: l.now(),
	}); err != nil {
		return err
	}
	return l.rewrite(ctx, seriesID, origin)
}

// orderKey is the catalog row one of a show's other episode orders is fetched by, from the
// catalog the show's own row came from.
func orderKey(series store.LibraryCatalogKey, orderID string) store.LibraryCatalogKey {
	return store.LibraryCatalogKey{Provider: series.Provider, Kind: string(mp.KindEpisodeOrder), ExternalID: orderID}
}
