package app

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/images"
	"github.com/loomarr/loomarr/internal/store"
)

const splitProposalArtworkOwner = "filler_split_proposal"

// fillerSplitArtworkAdapter is the composition seam between span extraction and the shared image
// service. The filler domain supplies exact source-bound JPEG bytes; the image service owns byte
// identity, inspection, responsive renditions and garbage collection.
type fillerSplitArtworkAdapter struct {
	images *images.Service
}

func (a fillerSplitArtworkAdapter) IngestSplitArtwork(
	ctx context.Context,
	proposalID string,
	segment filler.SplitSegment,
	jpeg []byte,
) (string, error) {
	meta, err := json.Marshal(map[string]any{
		"kind": "filler_split_segment", "proposalId": proposalID,
		"startMs": segment.StartMs, "endMs": segment.EndMs,
	})
	if err != nil {
		return "", err
	}
	image, err := a.images.Ingest(ctx, bytes.NewReader(jpeg), images.IngestRequest{
		Role: images.RoleThumb, Visibility: images.VisibilityMember, Origin: images.OriginExtracted,
		OwnerKind: splitProposalArtworkOwner, OwnerID: proposalID, Meta: string(meta),
	})
	if err != nil {
		return "", err
	}
	return image.Hash, nil
}

func deleteSplitProposalArtwork(ctx context.Context, st store.Store, proposalID string) {
	if st == nil || proposalID == "" {
		return
	}
	// Best effort, matching channel deletion: the image GC is intentionally eventual and a
	// temporary orphan must never turn a successful proposal transaction into a false failure.
	_ = st.DeleteImageRefs(ctx, splitProposalArtworkOwner, proposalID)
}
