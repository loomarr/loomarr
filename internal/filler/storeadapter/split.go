package storeadapter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/filler/research"
	"github.com/loomarr/loomarr/internal/filler/taxonomy"
	"github.com/loomarr/loomarr/internal/media/creative"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// Wake is the post-commit latency path that announces a filler catalog change to the channels
// whose pods it affects.
type Wake interface {
	Run(ctx context.Context, snapshots []clips.Clip)
}

// Split bridges the store → filler.SplitStore (V34). The
// proposal methods pass filler.SplitProposal straight through — the store
// persists exactly that type, so there is nothing to translate.
type Split struct {
	st   records.Store
	wake Wake
}

// NewSplit returns the splitter's store over st. wake may be nil.
func NewSplit(st records.Store, wake Wake) Split { return Split{st: st, wake: wake} }

func (a Split) GetClip(ctx context.Context, id string) (cliplibrary.StoreClip, bool, error) {
	return Sync{st: a.st}.GetClip(ctx, id)
}
func (a Split) ListClips(ctx context.Context) ([]cliplibrary.StoreClip, error) {
	clips, err := a.st.ListClips(ctx, clipstore.ClipFilter{})
	if err != nil {
		return nil, err
	}
	return storeClips(clips), nil
}
func (a Split) ListClipFingerprints(ctx context.Context, algorithm string) (map[string]creative.Fingerprint, error) {
	return a.st.ListClipFingerprints(ctx, algorithm)
}
func (a Split) UpsertClipFingerprint(ctx context.Context, clipHash, algorithm string, fp creative.Fingerprint) error {
	return a.st.UpsertClipFingerprint(ctx, clipHash, algorithm, fp)
}
func (a Split) UpsertClip(ctx context.Context, c cliplibrary.StoreClip) error {
	return a.st.UpsertClip(ctx, clipstore.Clip{Clip: c.Clip, UpdatedAt: c.UpdatedAt})
}
func (a Split) RecordFillerEraObservation(ctx context.Context, clipHash string, era int, source research.ObservationSource, at time.Time) error {
	return a.st.RecordFillerEraObservation(ctx, clipHash, era, source, at)
}
func (a Split) GetClipTags(ctx context.Context, clipHash string, leavesOnly bool) ([]string, error) {
	return a.st.GetClipTags(ctx, clipHash, leavesOnly)
}
func (a Split) SetClipTags(ctx context.Context, clipHash string, leaves []string) error {
	return a.st.SetClipTags(ctx, clipHash, leaves)
}
func (a Split) UpsertClipPipeline(ctx context.Context, row pipelinestate.ClipPipeline) error {
	return a.st.UpsertClipPipeline(ctx, row)
}
func (a Split) ReplaceSplitChildren(ctx context.Context, parentHash string, keepHashes []string, at time.Time) (int, error) {
	return a.st.ReplaceSplitChildren(ctx, parentHash, keepHashes, at)
}
func (a Split) DeleteClip(ctx context.Context, id string) error {
	return a.st.DeleteClip(ctx, id)
}
func (a Split) SetClipComposite(ctx context.Context, hash string, composite bool, at time.Time) error {
	return a.st.SetClipComposite(ctx, hash, composite, at)
}
func (a Split) ReleaseCompositeHolds(ctx context.Context, paths []string, at time.Time) (int, error) {
	snapshots := clipsByPath(ctx, a.st, paths)
	updated, err := a.st.ReleaseCompositeHolds(ctx, paths, at)
	if err == nil && updated > 0 && a.wake != nil {
		a.wake.Run(ctx, snapshots)
	}
	return updated, err
}

func clipsByPath(ctx context.Context, st store.Store, paths []string) []clips.Clip {
	out := make([]clips.Clip, 0, len(paths))
	for _, path := range paths {
		if clip, err := st.GetClipByPath(ctx, path); err == nil {
			out = append(out, clip.Clip)
		}
	}
	return out
}

// ListTaxa: split-segment classification serves + grounds against the taxonomy graph (§10 V45a).
func (a Split) ListTaxa(ctx context.Context) ([]taxonomy.Taxon, error) {
	return a.st.ListTaxa(ctx)
}
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
func (a Split) GetSplitProposal(ctx context.Context, id string) (filler.SplitProposal, error) {
	return a.st.GetSplitProposal(ctx, id)
}
func (a Split) AcquireSplitProposalClaim(ctx context.Context, id, token string, at, expiresAt time.Time) (filler.SplitProposal, error) {
	return a.st.AcquireSplitProposalClaim(ctx, id, token, at, expiresAt)
}
func (a Split) RenewSplitProposalClaim(ctx context.Context, id, token string, expiresAt time.Time) error {
	return a.st.RenewSplitProposalClaim(ctx, id, token, expiresAt)
}
func (a Split) ReleaseSplitProposalClaim(ctx context.Context, id, token string) error {
	return a.st.ReleaseSplitProposalClaim(ctx, id, token)
}
func (a Split) DeleteSplitProposal(ctx context.Context, id string) error {
	err := a.st.DeleteSplitProposal(ctx, id)
	if err == nil {
		DeleteSplitProposalArtwork(ctx, a.st, id)
	}
	return err
}
func (a Split) MarkPipelineComplete(ctx context.Context, hash string, at time.Time) error {
	return a.st.MarkPipelineComplete(ctx, hash, at)
}
func (a Split) CompleteSplitConfirmation(ctx context.Context, completion filler.SplitCompletion) (int, error) {
	n, err := a.st.CompleteSplitConfirmation(ctx, completion)
	if err == nil {
		DeleteSplitProposalArtwork(ctx, a.st, completion.ProposalID)
	}
	return n, err
}
func (a Split) CompletePartialSplitConfirmation(ctx context.Context, completion filler.SplitPartialCompletion) error {
	return a.st.CompletePartialSplitConfirmation(ctx, completion)
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

// ListSplitProposals is the split RUNG's "is one already waiting?" read (§10 V51b) — one read of
// the pending queue rather than an existence check per candidate. Re-detecting over a proposal an
// operator is halfway through editing is what it prevents.
func (a Split) ListSplitProposals(ctx context.Context) ([]filler.SplitProposal, error) {
	return a.st.ListSplitProposals(ctx)
}
