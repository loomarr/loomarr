package storeadapter

import (
	"context"
	"errors"

	"github.com/loomarr/loomarr/internal/filler/identity"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// Identity bridges the store → identity.Store (#2033). It translates the
// catalog listing, which carries admission time, and the merge record; the rest passes through.
type Identity struct{ records.Store }

// NewIdentity returns the identity job's store over st.
func NewIdentity(st records.Store) Identity { return Identity{Store: st} }

func (a Identity) ListIdentityClips(ctx context.Context) ([]identity.IdentityClip, error) {
	clips, err := a.ListClips(ctx, clipstore.ClipFilter{IncludeHeld: true, IncludeComposites: true, IncludeRemoved: true})
	if err != nil {
		return nil, err
	}
	// A corrupt defect row is omitted and that copy ranks as unmeasured; the rest still rank.
	records, err := a.ListClipDefects(ctx)
	if err != nil && !errors.Is(err, clipstore.ErrCorruptClipDefects) {
		return nil, err
	}
	out := make([]identity.IdentityClip, len(clips))
	for i, c := range clips {
		out[i] = identity.IdentityClip{Clip: c}
		if r, ok := records[c.Hash]; ok {
			out[i].Defects = &r.Assessment
		}
	}
	return out, nil
}

func (a Identity) ListWantedCopies(ctx context.Context) ([]identity.WantedCopy, error) {
	wanted, err := a.Store.ListWantedCopies(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]identity.WantedCopy, len(wanted))
	for i, w := range wanted {
		out[i] = identity.WantedCopy{ClipHash: w.ClipHash, Path: w.Path}
	}
	return out, nil
}
func (a Identity) MergeInferiorCopy(ctx context.Context, c identity.InferiorCopy) error {
	return a.Store.MergeInferiorCopy(ctx, clipstore.InferiorCopy{
		ClipHash: c.ClipHash, SourceHash: c.SourceHash, SurvivorHash: c.SurvivorHash, DecidedAt: c.DecidedAt,
	})
}
