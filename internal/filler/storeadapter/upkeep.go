package storeadapter

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// Upkeep bridges the store to the upkeep job (#3855), which lets go of fetched rejects.
type Upkeep struct{ st records.Store }

// NewUpkeep returns the upkeep job's store over st.
func NewUpkeep(st records.Store) Upkeep { return Upkeep{st: st} }

func (a Upkeep) ListClipPipelines(ctx context.Context, f pipelinestate.PipelineFilter) ([]pipelinestate.ClipPipeline, error) {
	return a.st.ListClipPipelines(ctx, f)
}

func (a Upkeep) RetireRejectedClip(ctx context.Context, hash string, rejectedBy, at time.Time) (bool, error) {
	return a.st.RetireRejectedClip(ctx, hash, rejectedBy, at)
}

func (a Upkeep) GetClip(ctx context.Context, hash string) (clipstore.Clip, bool, error) {
	return NewSync(a.st).GetClip(ctx, hash)
}

func (a Upkeep) ListClipPaths(ctx context.Context) ([]string, error) {
	return NewDefects(a.st).ListClipPaths(ctx)
}
