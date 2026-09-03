package filler

import (
	"strings"
	"testing"
	"time"
)

func TestSegmentScreeningAxisEvidenceBindsRawBytesAndExactSpan(t *testing.T) {
	source := structureSource(60_000)
	recorded, err := NewSegmentScreeningAxisEvidence(
		source, 10_000, 40_000, screeningProfileFixture(ScreenVisualSafety, "1"),
		ScreenHold, "manual_review", []byte(`{"detections":[]}`),
		time.Date(2026, time.September, 12, 6, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	result := recorded.Evidence.Result()
	if result.Axis != ScreenVisualSafety || result.Outcome != ScreenHold || result.AuthoritySHA256 != recorded.Evidence.SHA256 || ValidateRecordedSegmentScreeningAxisEvidence(recorded) != nil {
		t.Fatalf("recorded=%+v result=%+v", recorded, result)
	}

	tests := []struct {
		name   string
		mutate func(*RecordedSegmentScreeningAxisEvidence)
	}{
		{name: "raw bytes", mutate: func(item *RecordedSegmentScreeningAxisEvidence) { item.RawEvidence = []byte("replaced") }},
		{name: "source", mutate: func(item *RecordedSegmentScreeningAxisEvidence) {
			item.Evidence.Source.SHA256 = strings.Repeat("f", 64)
		}},
		{name: "span", mutate: func(item *RecordedSegmentScreeningAxisEvidence) {
			item.Evidence.EndMs = item.Evidence.Source.DurationMs + 1
		}},
		{name: "profile", mutate: func(item *RecordedSegmentScreeningAxisEvidence) { item.Evidence.Profile.PolicySHA256 = "" }},
		{name: "outcome", mutate: func(item *RecordedSegmentScreeningAxisEvidence) { item.Evidence.Outcome = "maybe" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := recorded
			candidate.RawEvidence = append([]byte(nil), recorded.RawEvidence...)
			test.mutate(&candidate)
			if err := ValidateRecordedSegmentScreeningAxisEvidence(candidate); err == nil {
				t.Fatal("drifted axis evidence was accepted")
			}
		})
	}
}
