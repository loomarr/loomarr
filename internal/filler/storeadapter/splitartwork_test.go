package storeadapter_test

import (
	"bytes"
	"image"
	"image/jpeg"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/filler/storeadapter"
	imagestore "github.com/loomarr/loomarr/internal/media/images/storeadapter"
	"github.com/loomarr/loomarr/internal/testkit/imagetestkit"
)

func TestSplitArtworkIsIngestedUnderItsProposal(t *testing.T) {
	st := catalogStore(t)
	// The image reference's owner is a foreign key to the proposal, so the ingest fails if the
	// adapter names another owner.
	if err := st.UpsertSplitProposal(t.Context(), filler.SplitProposal{ID: "p", ClipHash: "ready", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	service := imagetestkit.NewService(t, t.TempDir(), "", imagestore.New(st))
	var frame bytes.Buffer
	if err := jpeg.Encode(&frame, image.NewRGBA(image.Rect(0, 0, 64, 36)), nil); err != nil {
		t.Fatal(err)
	}
	hash, err := storeadapter.NewSplitArtwork(service).IngestSplitArtwork(
		t.Context(), "p", split.Segment{StartMs: 0, EndMs: 30_000}, frame.Bytes())
	if err != nil || hash == "" {
		t.Fatalf("ingest: hash=%q err=%v", hash, err)
	}
}
