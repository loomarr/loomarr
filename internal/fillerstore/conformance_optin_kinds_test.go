package fillerstore

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillerenrichment"
)

// testOptInKindProjection pins how an accepted kind reaches the catalog row (#2037). A source files
// campaign ads and weather reports as commercials, so political or news replaces a generic
// commercial or interstitial. Any other classified kind is kept, as before: a PSA stays a PSA, and a
// commercial is not turned into a bumper.
func testOptInKindProjection(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()
	at := time.Unix(1_700_000_000, 0).UTC()
	for hash, kind := range map[string]filler.Kind{
		"campaign": filler.Commercial, "forecast": filler.Interstitial, "safety": filler.PSA, "spot": filler.Commercial,
	} {
		clip := sampleClip(hash, hash, kind, 0, "", "")
		clip.CreatedAt, clip.UpdatedAt = at, at
		if err := s.UpsertClip(ctx, clip); err != nil {
			t.Fatal(err)
		}
	}
	for hash, kind := range map[string]string{"campaign": "political", "forecast": "news", "safety": "news", "spot": "bumper"} {
		if _, _, err := s.ApplyFillerEnrichment(ctx, fillerenrichment.State{
			ClipHash: hash, Axis: fillerenrichment.AxisKind, Status: fillerenrichment.StatusComplete,
			Value: fillerenrichment.Value{Text: kind},
			Evidence: fillerenrichment.Evidence{Kind: fillerenrichment.EvidenceItem, Reference: "item.title", Confidence: 100,
				Producer: fillerenrichment.DeterministicProducer, ProducerVersion: fillerenrichment.DeterministicProducerVersion,
				ObservedAt: at},
		}, at.Add(time.Minute)); err != nil {
			t.Fatalf("%s: %v", hash, err)
		}
	}
	for hash, want := range map[string]filler.Kind{
		"campaign": filler.Political, "forecast": filler.News, "safety": filler.PSA, "spot": filler.Commercial,
	} {
		clip, err := s.GetClip(ctx, hash)
		if err != nil {
			t.Fatal(err)
		}
		if clip.Kind != want {
			t.Errorf("%s kind = %q, want %q", hash, clip.Kind, want)
		}
	}

	// The real relabel path: the free pass's earlier version recorded the source's "commercial",
	// the new version reads the wording as political, and a later corrected rule takes it back.
	// Each answer replaces the same producer's older one, and the row follows.
	if err := s.UpsertClip(ctx, sampleClip("relabel", "relabel", filler.Commercial, 0, "", "")); err != nil {
		t.Fatal(err)
	}
	for step, answer := range []struct{ version, kind string }{
		{"5", "commercial"}, {"6", "political"}, {"7", "commercial"},
	} {
		observed := at.Add(time.Duration(step) * time.Minute)
		if _, _, err := s.ApplyFillerEnrichment(ctx, fillerenrichment.State{
			ClipHash: "relabel", Axis: fillerenrichment.AxisKind, Status: fillerenrichment.StatusComplete,
			Value: fillerenrichment.Value{Text: answer.kind},
			Evidence: fillerenrichment.Evidence{Kind: fillerenrichment.EvidenceItem, Reference: "item.title", Confidence: 100,
				Producer: fillerenrichment.DeterministicProducer, ProducerVersion: answer.version, ObservedAt: observed},
		}, observed); err != nil {
			t.Fatalf("v%s: %v", answer.version, err)
		}
		clip, err := s.GetClip(ctx, "relabel")
		if err != nil {
			t.Fatal(err)
		}
		if clip.Kind != filler.Kind(answer.kind) {
			t.Errorf("after v%s the row says %q, want %q", answer.version, clip.Kind, answer.kind)
		}
	}
}
