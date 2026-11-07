package enrichment_test

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/enrichment"
)

func TestProjectDetails_OwnsTheQuietUserFacingState(t *testing.T) {
	if got := enrichment.ProjectDetails(nil); got.State != enrichment.DetailAdding {
		t.Fatalf("empty projection = %+v", got)
	}

	now := time.Unix(900, 0).UTC()
	state := func(axis enrichment.Axis, value enrichment.Value, kind enrichment.EvidenceKind) enrichment.State {
		return enrichment.State{
			ClipHash: "clip", Axis: axis, Status: enrichment.StatusComplete, Value: value,
			Evidence: enrichment.Evidence{Kind: kind, Reference: "fixture", Confidence: 100,
				Producer: "fixture", ProducerVersion: "1", ObservedAt: now},
		}
	}
	limited := enrichment.ProjectDetails([]enrichment.State{
		state(enrichment.AxisKind, enrichment.Value{Text: "commercial"}, enrichment.EvidenceItem),
		state(enrichment.AxisBrand, enrichment.Value{}, enrichment.EvidenceInference),
	})
	if limited.State != enrichment.DetailLimited || len(limited.Facts) != 1 ||
		limited.Facts[0].Axis != enrichment.AxisKind || limited.Facts[0].Evidence != enrichment.EvidenceItem {
		t.Fatalf("limited projection = %+v", limited)
	}

	complete := enrichment.ProjectDetails([]enrichment.State{
		state(enrichment.AxisKind, enrichment.Value{Text: "commercial"}, enrichment.EvidenceItem),
		state(enrichment.AxisEra, enrichment.Value{Year: 1999}, enrichment.EvidenceItem),
		state(enrichment.AxisAudience, enrichment.Value{Text: "family"}, enrichment.EvidenceInference),
		state(enrichment.AxisBrand, enrichment.Value{Text: "Brambly Sauce"}, enrichment.EvidenceContent),
		state(enrichment.AxisGeography, enrichment.Value{Geography: enrichment.Geography{
			Scope: "national", Country: "GB",
		}}, enrichment.EvidenceTrustedMap),
		state(enrichment.AxisProduct, enrichment.Value{Tags: []string{"condiments"}}, enrichment.EvidenceItem),
	})
	if complete.State != enrichment.DetailComplete || len(complete.Facts) != 6 {
		t.Fatalf("complete projection = %+v", complete)
	}
}

func TestProjectDetails_StaleWorkIsAddingRatherThanAUserTask(t *testing.T) {
	now := time.Unix(901, 0).UTC()
	projection := enrichment.ProjectDetails([]enrichment.State{
		{ClipHash: "clip", Axis: enrichment.AxisAudience, Status: enrichment.StatusStale},
		{ClipHash: "clip", Axis: enrichment.AxisKind, Status: enrichment.StatusComplete,
			Value: enrichment.Value{Text: "commercial"}, Evidence: enrichment.Evidence{
				Kind: enrichment.EvidenceItem, Producer: "fixture", ProducerVersion: "1", ObservedAt: now,
			}},
	})
	if projection.State != enrichment.DetailAdding || len(projection.Facts) != 1 {
		t.Fatalf("stale projection = %+v", projection)
	}
}
