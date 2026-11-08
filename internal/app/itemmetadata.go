package app

import (
	"context"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// itemMetadata answers a guide's and a lineup's item metadata from the Library's own tables
// (#2151 step 16). An id that is no library item is absent: the media server is never asked.
type itemMetadata struct {
	facts store.LibraryItemFactsReader
}

// ItemMetadataByID keeps library.Client's contract: missing ids are absent, and a partial answer
// rides with its error.
func (m itemMetadata) ItemMetadataByID(ctx context.Context, ids []string) (map[string]library.ItemMetadata, error) {
	facts, err := m.facts.LibraryItemFacts(ctx, ids)
	out := make(map[string]library.ItemMetadata, len(facts))
	for id, f := range facts {
		out[id] = library.ItemMetadata{Overview: f.Overview, Genres: f.Genres, Year: f.Year,
			OfficialRating: f.Rating, RuntimeMs: f.RuntimeMs}
	}
	return out, err
}

// ItemDurationMs is one item's runtime, as the channel engine's availability asks it.
func (m itemMetadata) ItemDurationMs(ctx context.Context, id string) (int64, error) {
	meta, err := m.ItemMetadataByID(ctx, []string{id})
	if got, ok := meta[id]; ok {
		return got.RuntimeMs, nil
	}
	return 0, err
}
