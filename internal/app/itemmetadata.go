package app

import (
	"context"
	"errors"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// itemMetadata answers a guide's and a lineup's item metadata (#2151 step 15): a library item id
// from the Library's own tables, any other id from the media server, as before the cutover. lib
// may be nil when no media server is connected; its ids are then simply absent.
type itemMetadata struct {
	facts store.LibraryItemFactsReader
	lib   *library.Client
}

// ItemMetadataByID keeps library.Client's contract: missing ids are absent, and a partial answer
// rides with its error.
func (m itemMetadata) ItemMetadataByID(ctx context.Context, ids []string) (map[string]library.ItemMetadata, error) {
	out := make(map[string]library.ItemMetadata, len(ids))
	rest := ids
	var factsErr error
	if m.facts != nil {
		facts, err := m.facts.LibraryItemFacts(ctx, ids)
		factsErr = err
		rest = make([]string, 0, len(ids))
		for _, id := range ids {
			f, ok := facts[id]
			if !ok {
				rest = append(rest, id)
				continue
			}
			out[id] = library.ItemMetadata{Overview: f.Overview, Genres: f.Genres, Year: f.Year,
				OfficialRating: f.Rating, RuntimeMs: f.RuntimeMs}
		}
	}
	if len(rest) == 0 || m.lib == nil {
		return out, factsErr
	}
	served, err := m.lib.ItemMetadataByID(ctx, rest)
	for id, meta := range served {
		out[id] = meta
	}
	return out, errors.Join(factsErr, err)
}

// ItemDurationMs is one item's runtime, as the channel engine's availability asks it.
func (m itemMetadata) ItemDurationMs(ctx context.Context, id string) (int64, error) {
	meta, err := m.ItemMetadataByID(ctx, []string{id})
	if got, ok := meta[id]; ok {
		return got.RuntimeMs, nil
	}
	return 0, err
}
