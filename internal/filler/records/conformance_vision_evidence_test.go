package records

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/enrichment"
)

// A frame read never overwrites an era or brand that item-metadata or operator evidence holds, so
// the column and its evidence row agree after every vision pass.
func testVisionRespectsStrongerEvidence(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	at := time.Unix(1_700_000_000, 0).UTC()

	t.Run("OperatorEraAndBrandSurvive", func(t *testing.T) {
		s := newStore(t)
		c := sampleClip("v-op", "Operator dated", clips.Commercial, 0, clips.Family, "")
		putClip(t, s, c)
		for _, state := range []enrichment.State{
			enrichment.Operator(c.Hash, enrichment.AxisEra, enrichment.Value{Year: 1985}, "operator.clip_edit", "", at),
			enrichment.Operator(c.Hash, enrichment.AxisBrand, enrichment.Value{Text: "Operator Brand"}, "operator.clip_edit", "", at),
		} {
			if _, _, err := s.ApplyFillerEnrichment(ctx, state, at); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.ApplyClipVision(ctx, c.Hash, c.Path, "Frame Brand", "FRAME BRAND 1999", 1999, nil, at.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
		got, err := s.GetClip(ctx, c.Hash)
		if err != nil {
			t.Fatal(err)
		}
		if got.Era != 1985 || got.Brand != "Operator Brand" {
			t.Errorf("era/brand = %d/%q after a vision pass, want the operator's 1985/Operator Brand", got.Era, got.Brand)
		}
		if got.VisibleText != "FRAME BRAND 1999" {
			t.Errorf("visible text = %q, want the pass still recorded", got.VisibleText)
		}
	})

	t.Run("ItemMetadataEraSurvives", func(t *testing.T) {
		s := newStore(t)
		c := sampleClip("v-item", "Listed", clips.Commercial, 0, clips.Family, "")
		putClip(t, s, c)
		state := enrichment.State{ClipHash: c.Hash, Axis: enrichment.AxisEra, Status: enrichment.StatusComplete,
			Value: enrichment.Value{Year: 1991},
			Evidence: enrichment.Evidence{Kind: enrichment.EvidenceItem, Reference: "listing.upload_date",
				Confidence: 100, Producer: "deterministic", ProducerVersion: "1", ObservedAt: at}}
		if _, _, err := s.ApplyFillerEnrichment(ctx, state, at); err != nil {
			t.Fatal(err)
		}
		if err := s.ApplyClipVision(ctx, c.Hash, c.Path, "", "TEXT 1999", 1999, nil, at.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
		got, _ := s.GetClip(ctx, c.Hash)
		if got.Era != 1991 {
			t.Errorf("era = %d after a vision pass, want the listing's 1991", got.Era)
		}
	})

	t.Run("NoEvidenceAcceptsTheFrameRead", func(t *testing.T) {
		s := newStore(t)
		c := sampleClip("v-free", "Undated", clips.Commercial, 0, clips.Family, "")
		putClip(t, s, c)
		if err := s.ApplyClipVision(ctx, c.Hash, c.Path, "Frame Brand", "FRAME BRAND 1999", 1999, nil, at); err != nil {
			t.Fatal(err)
		}
		got, err := s.GetClip(ctx, c.Hash)
		if err != nil {
			t.Fatal(err)
		}
		if got.Era != 1999 || got.Brand != "Frame Brand" {
			t.Errorf("era/brand = %d/%q, want the frame read 1999/Frame Brand when nothing stronger holds them", got.Era, got.Brand)
		}
	})
}
