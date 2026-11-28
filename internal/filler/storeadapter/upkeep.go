package storeadapter

import (
	"context"

	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// Upkeep bridges the store to the upkeep job (#3855), which lets go of fetched rejects.
type Upkeep struct {
	upkeepRows
	st records.Store
}

// NewUpkeep returns the upkeep job's store over st.
func NewUpkeep(st records.Store) Upkeep { return Upkeep{upkeepRows: st, st: st} }

// GetClip reports an absent clip as not found rather than as an error.
func (a Upkeep) GetClip(ctx context.Context, hash string) (clipstore.Clip, bool, error) {
	return findClip(ctx, a.st, hash)
}

// ListReadyClips is the playable catalog.
func (a Upkeep) ListReadyClips(ctx context.Context) ([]clipstore.Clip, error) {
	return a.st.ListClips(ctx, clipstore.ClipFilter{})
}

// ListClipPaths is every catalogued path, held, composite and removed clips included.
func (a Upkeep) ListClipPaths(ctx context.Context) ([]string, error) {
	return NewDefects(a.st).ListClipPaths(ctx)
}
