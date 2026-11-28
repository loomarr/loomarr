package storeadapter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// Wake is the post-commit latency path that announces a filler catalog change to the channels
// whose pods it affects.
type Wake interface {
	Run(ctx context.Context, snapshots []clips.Clip)
}

// splitRows are the clip and pipeline rows the splitter and the operator read or write exactly as
// the store does them.
type splitRows interface {
	UpsertClip(ctx context.Context, c clipstore.Clip) error
	SetClipComposite(ctx context.Context, hash string, composite bool, at time.Time) error
	// GetClipPipeline is the operator's check that a reel's files still exist.
	GetClipPipeline(ctx context.Context, hash string) (pipelinestate.ClipPipeline, bool, error)
}

// Split adapts the store to the splitter's catalog and proposal ports (V34). Its methods are only
// the ones that differ from the store's: a changed shape, a channel wake, an artwork cleanup or an
// error translation. Everything else is the store's own method, promoted through the embedded
// interfaces.
type Split struct {
	splitRows
	filler.SplitProposals
	st   records.Store
	wake Wake
}

// NewSplit returns the splitter's store over st. wake may be nil.
func NewSplit(st records.Store, wake Wake) Split {
	return Split{splitRows: st, SplitProposals: st, st: st, wake: wake}
}

// Ports is the splitter's storage: the adapter where it adds behaviour, the store itself where it
// satisfies a port as it is.
func (a Split) Ports() filler.SplitPorts {
	return filler.SplitPorts{SplitCatalog: a, SplitFingerprints: a.st, SplitChildMeta: a.st, SplitProposals: a}
}

// GetClip reports an absent clip as not found rather than as an error.
func (a Split) GetClip(ctx context.Context, id string) (clipstore.Clip, bool, error) {
	return Sync{st: a.st}.GetClip(ctx, id)
}

// ListClips is the unfiltered catalog.
func (a Split) ListClips(ctx context.Context) ([]clipstore.Clip, error) {
	return a.st.ListClips(ctx, clipstore.ClipFilter{})
}

// ReleaseCompositeHolds wakes the channels whose pods the released clips affect.
func (a Split) ReleaseCompositeHolds(ctx context.Context, paths []string, at time.Time) (int, error) {
	snapshots := clipsByPath(ctx, a.st, paths)
	updated, err := a.st.ReleaseCompositeHolds(ctx, paths, at)
	if err == nil && updated > 0 && a.wake != nil {
		a.wake.Run(ctx, snapshots)
	}
	return updated, err
}

func clipsByPath(ctx context.Context, st clipPathReader, paths []string) []clips.Clip {
	out := make([]clips.Clip, 0, len(paths))
	for _, path := range paths {
		if clip, err := st.GetClipByPath(ctx, path); err == nil {
			out = append(out, clip.Clip)
		}
	}
	return out
}

// UpsertSplitProposal drops the artwork of the proposal this one replaces.
func (a Split) UpsertSplitProposal(ctx context.Context, p filler.SplitProposal) error {
	var replaced string
	if proposals, err := a.st.ListSplitProposals(ctx); err == nil {
		for _, existing := range proposals {
			if existing.ClipHash == p.ClipHash && existing.ID != p.ID {
				replaced = existing.ID
				break
			}
		}
	}
	err := a.st.UpsertSplitProposal(ctx, p)
	if err == nil && replaced != "" {
		DeleteSplitProposalArtwork(ctx, a.st, replaced)
	}
	return err
}

// DeleteSplitProposal drops the proposal's artwork with it.
func (a Split) DeleteSplitProposal(ctx context.Context, id string) error {
	err := a.st.DeleteSplitProposal(ctx, id)
	if err == nil {
		DeleteSplitProposalArtwork(ctx, a.st, id)
	}
	return err
}

// CompleteSplitConfirmation drops the consumed proposal's artwork.
func (a Split) CompleteSplitConfirmation(ctx context.Context, completion filler.SplitCompletion) (int, error) {
	n, err := a.st.CompleteSplitConfirmation(ctx, completion)
	if err == nil {
		DeleteSplitProposalArtwork(ctx, a.st, completion.ProposalID)
	}
	return n, err
}

// ⚠ Translates the store's ErrNotFound into the DOMAIN's ErrProposalGone. `internal/filler` does
// not import `internal/platform/store` (Tier 3), and the distinction is load-bearing rather than cosmetic:
// the split rung must tell "the proposal was confirmed under me" apart from a real write failure,
// because the first is a normal outcome and the second must fail the pass.
func (a Split) UpdateSplitProposal(ctx context.Context, p filler.SplitProposal) error {
	err := a.st.UpdateSplitProposal(ctx, p)
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("%w: %s", filler.ErrProposalGone, p.ID)
	}
	return err
}
