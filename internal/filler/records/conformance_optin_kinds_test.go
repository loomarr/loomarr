package records

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/enrichment"
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
	for hash, kind := range map[string]clips.Role{
		"campaign": clips.Commercial, "forecast": clips.Interstitial, "safety": clips.PSA, "spot": clips.Commercial,
	} {
		clip := sampleClip(hash, hash, kind, 0, "", "")
		clip.CreatedAt, clip.UpdatedAt = at, at
		if err := s.UpsertClip(ctx, clip); err != nil {
			t.Fatal(err)
		}
	}
	for hash, kind := range map[string]string{"campaign": "political", "forecast": "news", "safety": "news", "spot": "bumper"} {
		if _, _, err := s.ApplyFillerEnrichment(ctx, enrichment.State{
			ClipHash: hash, Axis: enrichment.AxisRole, Status: enrichment.StatusComplete,
			Value: enrichment.Value{Text: kind},
			Evidence: enrichment.Evidence{Kind: enrichment.EvidenceItem, Reference: "item.title", Confidence: 100,
				Producer: enrichment.DeterministicProducer, ProducerVersion: enrichment.DeterministicProducerVersion,
				ObservedAt: at},
		}, at.Add(time.Minute)); err != nil {
			t.Fatalf("%s: %v", hash, err)
		}
	}
	for hash, want := range map[string]clips.Role{
		"campaign": clips.Political, "forecast": clips.News, "safety": clips.PSA, "spot": clips.Commercial,
	} {
		clip, err := s.GetClip(ctx, hash)
		if err != nil {
			t.Fatal(err)
		}
		if clip.Role != want {
			t.Errorf("%s kind = %q, want %q", hash, clip.Role, want)
		}
	}

	// The real relabel path: the free pass's earlier version recorded the source's "commercial",
	// the new version reads the wording as political, and a later corrected rule takes it back.
	// Each answer replaces the same producer's older one, and the row follows.
	if err := s.UpsertClip(ctx, sampleClip("relabel", "relabel", clips.Commercial, 0, "", "")); err != nil {
		t.Fatal(err)
	}
	for step, answer := range []struct{ version, kind string }{
		{"5", "commercial"}, {"6", "political"}, {"7", "commercial"},
	} {
		observed := at.Add(time.Duration(step) * time.Minute)
		if _, _, err := s.ApplyFillerEnrichment(ctx, enrichment.State{
			ClipHash: "relabel", Axis: enrichment.AxisRole, Status: enrichment.StatusComplete,
			Value: enrichment.Value{Text: answer.kind},
			Evidence: enrichment.Evidence{Kind: enrichment.EvidenceItem, Reference: "item.title", Confidence: 100,
				Producer: enrichment.DeterministicProducer, ProducerVersion: answer.version, ObservedAt: observed},
		}, observed); err != nil {
			t.Fatalf("v%s: %v", answer.version, err)
		}
		clip, err := s.GetClip(ctx, "relabel")
		if err != nil {
			t.Fatal(err)
		}
		if clip.Role != clips.Role(answer.kind) {
			t.Errorf("after v%s the row says %q, want %q", answer.version, clip.Role, answer.kind)
		}
	}
}
