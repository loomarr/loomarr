package clipstore

import (
	"testing"

	"github.com/loomarr/loomarr/internal/filler/enrichment"
)

// A frame read yields to item-metadata evidence and everything stronger, and to nothing weaker: the
// store's rank is the enrichment model's item rank, so renumbering the ranks cannot move it.
func TestVisionYieldsFromTheItemRank(t *testing.T) {
	if visionYieldsFromRank != int(enrichment.RankItem) {
		t.Fatalf("vision yields from rank %d, but item metadata is rank %d", visionYieldsFromRank, enrichment.RankItem)
	}
	if int(enrichment.RankContent) >= visionYieldsFromRank {
		t.Fatalf("content observation (rank %d) would block a frame read", enrichment.RankContent)
	}
}
