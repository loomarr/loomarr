package testkit

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
)

// stockedLibrary is a store whose library holds every item a saved channel plans to air.
type stockedLibrary struct {
	records.Store
	t testing.TB
}

// StockedLibrary wraps st so that saving a channel first stores a bare movie for each library item
// its schedule names that the library has no row for. A channel's planned items are foreign keys
// (#2793), so the real store refuses a schedule naming an unknown item; a test whose availability is
// a fake hands out ids no library has, and this keeps such a test about the channel rather than the
// library. An item the test already stored is left as it is.
func StockedLibrary(t testing.TB, st records.Store) records.Store {
	t.Helper()
	return stockedLibrary{Store: st, t: t}
}

func (s stockedLibrary) SaveChannel(ctx context.Context, ch channelstore.Channel) (channelstore.Channel, error) {
	var named []string
	for _, slot := range ch.Desired {
		if slot.LibraryItemID != "" {
			named = append(named, slot.LibraryItemID)
		}
	}
	if len(named) > 0 {
		known, err := s.LibraryItemFacts(ctx, named)
		if err != nil {
			return channelstore.Channel{}, err
		}
		var missing []string
		seen := map[string]bool{}
		for _, id := range named {
			if _, ok := known[id]; !ok && !seen[id] {
				seen[id] = true // listed once, however many slots name it
				missing = append(missing, id)
			}
		}
		if len(missing) > 0 {
			SaveLibraryItems(s.t, s.Store, missing...)
		}
	}
	return s.Store.SaveChannel(ctx, ch)
}
