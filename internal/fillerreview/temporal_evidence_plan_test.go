package fillerreview

import (
	"reflect"
	"testing"

	"github.com/loomarr/loomarr/internal/mediatools"
)

func TestBuildTemporalEvidencePlanPreservesBoundariesAndSamplesTransitions(t *testing.T) {
	plan, err := BuildTemporalEvidencePlan(10_000,
		[]mediatools.Interval{{StartMs: 0, EndMs: 1_000}, {StartMs: 8_500, EndMs: 10_000}},
		[]mediatools.Interval{{StartMs: 0, EndMs: 800}, {StartMs: 9_000, EndMs: 10_000}},
		[]int64{1_000, 2_000, 3_000, 4_000, 9_000})
	if err != nil {
		t.Fatal(err)
	}
	if plan.EvidenceStartMS != 550 || plan.EvidenceEndMS != 9_250 || plan.LeadingCleanup == nil || plan.TrailingCleanup == nil {
		t.Fatalf("cleanup plan = %+v", plan)
	}
	want := []int64{550, 850, 1_050, 1_150, 2_850, 3_150, 3_450, 6_350, 7_250, 8_250, 8_850}
	if !reflect.DeepEqual(plan.FrameTimesMS, want) {
		t.Fatalf("frame times = %v, want %v", plan.FrameTimesMS, want)
	}
}

func TestBuildTemporalEvidencePlanNeverErasesFullyBlankSpan(t *testing.T) {
	blank := []mediatools.Interval{{StartMs: 0, EndMs: 2_000}}
	plan, err := BuildTemporalEvidencePlan(2_000, blank, blank, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.EvidenceStartMS != 0 || plan.EvidenceEndMS != 2_000 || plan.LeadingCleanup != nil || plan.TrailingCleanup != nil {
		t.Fatalf("fully blank span was erased: %+v", plan)
	}
}
