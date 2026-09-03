package filler

import (
	"strings"
	"testing"
	"time"
)

func screeningProfileFixture(axis SegmentScreeningAxis, digit string) SegmentScreeningAxisProfile {
	return SegmentScreeningAxisProfile{
		Axis: axis, EvidenceContract: "axis-evidence-v1",
		PolicySHA256: strings.Repeat(digit, 64), CertificationSHA256: strings.Repeat("a", 64), ImplementationSHA256: strings.Repeat("b", 64),
	}
}

func passingAxisEvidence(t *testing.T, source SplitSourceAsset, startMs, endMs int64) []RecordedSegmentScreeningAxisEvidence {
	t.Helper()
	records := make([]RecordedSegmentScreeningAxisEvidence, 0, len(segmentScreeningAxisOrder))
	for index, axis := range segmentScreeningAxisOrder {
		reason := "policy_clear"
		if axis == ScreenRights {
			reason = "rights_verified"
		} else if axis == ScreenPlayback {
			reason = "playback_verified"
		}
		recorded, err := NewSegmentScreeningAxisEvidence(
			source, startMs, endMs, screeningProfileFixture(axis, string(rune('1'+index))), ScreenPass, reason,
			[]byte("recorded-"+string(axis)), time.Date(2026, time.September, 12, 4, 0, 0, 0, time.UTC),
		)
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, recorded)
	}
	return records
}
