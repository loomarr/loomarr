package fillerscreening

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

// These tests pin filler.SegmentScreeningEvidence's invariants. They build the aggregate through
// this package's axis-evidence fixtures, which internal/filler's tests cannot import.

func segmentScreeningFixture(t *testing.T) filler.SegmentScreeningEvidence {
	t.Helper()
	subject := screeningSubjectFixture(t)
	records := passingAxisEvidence(t, subject)
	results := make([]filler.SegmentScreeningResult, 0, len(records))
	for _, record := range records {
		results = append(results, record.Evidence.Result())
	}
	evidence, err := filler.NewSegmentScreeningEvidence(subject, results, screeningAirworthinessDecision(t, subject, records), time.Date(2026, time.September, 5, 14, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return evidence
}

func TestSegmentScreeningRequiresAllFourIndependentPasses(t *testing.T) {
	evidence := segmentScreeningFixture(t)
	if !evidence.Passes() || len(evidence.Results) != 4 || evidence.Results[0].Axis != filler.ScreenPlayback || evidence.Results[1].Axis != filler.ScreenSpokenSafety || evidence.Results[2].Axis != filler.ScreenVisualSafety || evidence.Results[3].Axis != filler.ScreenWrittenSafety {
		t.Fatalf("screening = %+v", evidence)
	}
	for _, axis := range []filler.SegmentScreeningAxis{filler.ScreenVisualSafety, filler.ScreenSpokenSafety, filler.ScreenWrittenSafety, filler.ScreenPlayback} {
		t.Run(string(axis), func(t *testing.T) {
			candidate := evidence
			candidate.Results = append([]filler.SegmentScreeningResult(nil), evidence.Results...)
			for index := range candidate.Results {
				if candidate.Results[index].Axis == axis {
					candidate.Results[index].Outcome = filler.ScreenHold
				}
			}
			candidate.SHA256 = filler.SegmentScreeningEvidenceSHA256(candidate)
			if candidate.Passes() {
				t.Fatalf("%s hold passed screening", axis)
			}
		})
	}
}

func TestValidateSegmentScreeningRejectsCoverageAndIdentityDrift(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*filler.SegmentScreeningEvidence)
	}{
		{name: "missing axis", mutate: func(e *filler.SegmentScreeningEvidence) { e.Results = e.Results[:3] }},
		{name: "duplicate axis", mutate: func(e *filler.SegmentScreeningEvidence) { e.Results[1].Axis = e.Results[0].Axis }},
		{name: "unknown outcome", mutate: func(e *filler.SegmentScreeningEvidence) { e.Results[0].Outcome = "maybe" }},
		{name: "missing authority", mutate: func(e *filler.SegmentScreeningEvidence) { e.Results[0].AuthoritySHA256 = "" }},
		{name: "unsafe reason text", mutate: func(e *filler.SegmentScreeningEvidence) { e.Results[0].ReasonCode = "contains restricted phrase" }},
		{name: "subject", mutate: func(e *filler.SegmentScreeningEvidence) { e.SubjectSHA256 = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evidence := segmentScreeningFixture(t)
			test.mutate(&evidence)
			evidence.SHA256 = filler.SegmentScreeningEvidenceSHA256(evidence)
			if err := filler.ValidateSegmentScreeningEvidence(evidence); err == nil {
				t.Fatal("mutated screening was accepted")
			}
		})
	}
}
