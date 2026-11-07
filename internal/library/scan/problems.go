package scan

import (
	"context"

	"github.com/loomarr/loomarr/internal/store"
)

// Problem is a title that needs a look (#2151 decision 24): files that can't be played, or a
// movie's extra copies, which never play and take space.
type Problem struct {
	store.LibraryItemSummary
	// ExtraBytes is what a movie's extra copies take; 0 for none.
	ExtraBytes int64
}

// Problems lists the titles that need a look, by name. A movie's extra copies are the ones its page
// marks extra.
func (l *Library) Problems(ctx context.Context) ([]Problem, error) {
	page, err := l.ItemStore.ListLibraryItems(ctx, store.LibraryItemQuery{Problems: true})
	if err != nil {
		return nil, err
	}
	out := make([]Problem, 0, len(page.Items))
	for _, it := range page.Items {
		p := Problem{LibraryItemSummary: it}
		if it.Kind == string(kindMovie) && it.Files > 1 {
			d, err := l.Item(ctx, it.ID)
			if err != nil {
				return nil, err
			}
			for _, f := range d.Files {
				if f.Extra {
					p.ExtraBytes += f.Size
				}
			}
		}
		out = append(out, p)
	}
	return out, nil
}
