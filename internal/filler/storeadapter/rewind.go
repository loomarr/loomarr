package storeadapter

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// SplitProposalArtworkOwner is the image owner kind of a split proposal's segment artwork.
const SplitProposalArtworkOwner = "filler_split_proposal"

// DeleteSplitProposalArtwork drops the image references a split proposal's segments hold.
func DeleteSplitProposalArtwork(ctx context.Context, st store.Store, proposalID string) {
	if st == nil || proposalID == "" {
		return
	}
	// Best effort, matching channel deletion: the image GC is intentionally eventual and a
	// temporary orphan must never turn a successful proposal transaction into a false failure.
	_ = st.DeleteImageRefs(ctx, SplitProposalArtworkOwner, proposalID)
}

// Rewind bridges the store → filler.RewindStore — the invalidation seam behind
// re-running a stage (§10 V51b).
//
// ⚠ Every method here is an EXISTING single writer, called with the "not yet" value it already
// defines. Rewind introduces no new writer of any clip column, which is what keeps the
// single-writer story the store conformance suite pins.
type Rewind struct{ st records.Store }

// NewRewind returns the rewind seam over st.
func NewRewind(st records.Store) Rewind { return Rewind{st: st} }

func (a Rewind) SetClipLanguage(ctx context.Context, path, language string, at time.Time) error {
	return a.st.SetClipLanguage(ctx, path, language, at)
}
func (a Rewind) SetClipTranscript(ctx context.Context, path, transcript string, at time.Time) error {
	return a.st.SetClipTranscript(ctx, path, transcript, at)
}
func (a Rewind) ClearClipVisibleText(ctx context.Context, path string, at time.Time) error {
	return a.st.ClearClipVisibleText(ctx, path, at)
}
func (a Rewind) ListSplitProposals(ctx context.Context) ([]filler.SplitProposal, error) {
	return a.st.ListSplitProposals(ctx)
}
func (a Rewind) DeleteSplitProposal(ctx context.Context, id string) error {
	err := a.st.DeleteSplitProposal(ctx, id)
	if err == nil {
		DeleteSplitProposalArtwork(ctx, a.st, id)
	}
	return err
}

// Sweep bridges the store → filler.SweepStore (§10 V54).
//
// ⚠ It exists for ONE translation: `records.SweepableProposal` → `filler.SweepableProposal`. The
// domain must not import the store (Tier 3), and the store's row type is a query result rather
// than a domain concept, so the two are deliberately separate structs with the same shape.
type Sweep struct{ st records.Store }

// NewSweep returns the split sweeper's store over st.
func NewSweep(st records.Store) Sweep { return Sweep{st: st} }

func (a Sweep) ListSweepableSplitProposals(ctx context.Context, before time.Time) ([]filler.SweepableProposal, error) {
	rows, err := a.st.ListSweepableSplitProposals(ctx, before)
	if err != nil {
		return nil, err
	}
	out := make([]filler.SweepableProposal, len(rows))
	for i, r := range rows {
		out[i] = filler.SweepableProposal{
			ProposalID: r.ProposalID, ClipHash: r.ClipHash, ClipPath: r.ClipPath, Segments: r.Segments,
		}
	}
	return out, nil
}
func (a Sweep) DeleteSplitProposal(ctx context.Context, id string) error {
	err := a.st.DeleteSplitProposal(ctx, id)
	if err == nil {
		DeleteSplitProposalArtwork(ctx, a.st, id)
	}
	return err
}
func (a Sweep) MarkClipReaped(ctx context.Context, hash string, at time.Time) error {
	return a.st.MarkClipReaped(ctx, hash, at)
}
func (a Sweep) MarkPipelineComplete(ctx context.Context, hash string, at time.Time) error {
	return a.st.MarkPipelineComplete(ctx, hash, at)
}
