package fillerenrichment_test

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillerenrichment"
)

func TestProjectDetails_OwnsTheQuietUserFacingState(t *testing.T) {
	if got := fillerenrichment.ProjectDetails(nil); got.State != fillerenrichment.DetailAdding {
		t.Fatalf("empty projection = %+v", got)
	}

	now := time.Unix(900, 0).UTC()
	state := func(axis fillerenrichment.Axis, value fillerenrichment.Value, kind fillerenrichment.EvidenceKind) fillerenrichment.State {
		return fillerenrichment.State{
			ClipHash: "clip", Axis: axis, Status: fillerenrichment.StatusComplete, Value: value,
			Evidence: fillerenrichment.Evidence{Kind: kind, Reference: "fixture", Confidence: 100,
				Producer: "fixture", ProducerVersion: "1", ObservedAt: now},
		}
	}
	limited := fillerenrichment.ProjectDetails([]fillerenrichment.State{
		state(fillerenrichment.AxisKind, fillerenrichment.Value{Text: "commercial"}, fillerenrichment.EvidenceItem),
		state(fillerenrichment.AxisBrand, fillerenrichment.Value{}, fillerenrichment.EvidenceInference),
	})
	if limited.State != fillerenrichment.DetailLimited || len(limited.Facts) != 1 ||
		limited.Facts[0].Axis != fillerenrichment.AxisKind || limited.Facts[0].Evidence != fillerenrichment.EvidenceItem {
		t.Fatalf("limited projection = %+v", limited)
	}

	complete := fillerenrichment.ProjectDetails([]fillerenrichment.State{
		state(fillerenrichment.AxisKind, fillerenrichment.Value{Text: "commercial"}, fillerenrichment.EvidenceItem),
		state(fillerenrichment.AxisEra, fillerenrichment.Value{Year: 1999}, fillerenrichment.EvidenceItem),
		state(fillerenrichment.AxisAudience, fillerenrichment.Value{Text: "family"}, fillerenrichment.EvidenceInference),
		state(fillerenrichment.AxisBrand, fillerenrichment.Value{Text: "HP Sauce"}, fillerenrichment.EvidenceContent),
		state(fillerenrichment.AxisGeography, fillerenrichment.Value{Geography: fillerenrichment.Geography{
			Scope: "national", Country: "GB",
		}}, fillerenrichment.EvidenceTrustedMap),
		state(fillerenrichment.AxisProduct, fillerenrichment.Value{Tags: []string{"condiments"}}, fillerenrichment.EvidenceItem),
	})
	if complete.State != fillerenrichment.DetailComplete || len(complete.Facts) != 6 {
		t.Fatalf("complete projection = %+v", complete)
	}
}

func TestProjectDetails_StaleWorkIsAddingRatherThanAUserTask(t *testing.T) {
	now := time.Unix(901, 0).UTC()
	projection := fillerenrichment.ProjectDetails([]fillerenrichment.State{
		{ClipHash: "clip", Axis: fillerenrichment.AxisAudience, Status: fillerenrichment.StatusStale},
		{ClipHash: "clip", Axis: fillerenrichment.AxisKind, Status: fillerenrichment.StatusComplete,
			Value: fillerenrichment.Value{Text: "commercial"}, Evidence: fillerenrichment.Evidence{
				Kind: fillerenrichment.EvidenceItem, Producer: "fixture", ProducerVersion: "1", ObservedAt: now,
			}},
	})
	if projection.State != fillerenrichment.DetailAdding || len(projection.Facts) != 1 {
		t.Fatalf("stale projection = %+v", projection)
	}
}
