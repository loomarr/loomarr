package storeadapter

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/media/images"
)

// SplitArtwork is the seam between span extraction and the shared image
// service. The filler domain supplies exact source-bound JPEG bytes; the image service owns byte
// identity, inspection, responsive renditions and garbage collection.
type SplitArtwork struct {
	images *images.Service
}

// NewSplitArtwork returns the split artwork ingester over the image service.
func NewSplitArtwork(images *images.Service) SplitArtwork { return SplitArtwork{images: images} }

func (a SplitArtwork) IngestSplitArtwork(
	ctx context.Context,
	proposalID string,
	segment split.Segment,
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
		OwnerKind: SplitProposalArtworkOwner, OwnerID: proposalID, Meta: string(meta),
	})
	if err != nil {
		return "", err
	}
	return image.Hash, nil
}
