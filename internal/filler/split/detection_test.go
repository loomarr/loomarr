package split

import (
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/media/tools"
)

func TestAssessCurrentSplitStructureRetainsIndependentSignalsAndDiscardedTime(t *testing.T) {
	source := structureSource(65_000)
	progress := DetectionProgress{
		ScannedThroughMs: source.DurationMs,
		Black: []tools.Interval{
			{StartMs: 29_800, EndMs: 30_200},
			{StartMs: 32_800, EndMs: 33_200},
		},
		Silence: []tools.Interval{
			{StartMs: 29_900, EndMs: 30_100},
			{StartMs: 32_900, EndMs: 33_100},
		},
		Discarded: []tools.Interval{{StartMs: 30_000, EndMs: 33_000}},
	}
	segments := []Segment{{StartMs: 0, EndMs: 30_000}, {StartMs: 33_000, EndMs: 65_000}}
	assessment, err := AssessDetection(source, progress, segments, time.Date(2026, time.September, 3, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Source != source || assessment.Kind != KindAmbiguous {
		t.Fatalf("assessment identity/verdict = %+v", assessment)
	}
	if len(assessment.Observations) != 4 {
		t.Fatalf("observations = %+v, want black and silence retained separately at both edges", assessment.Observations)
	}
	if len(assessment.Plan) != 3 {
		t.Fatalf("plan = %+v, want kept-shape/discard/kept-shape coverage", assessment.Plan)
	}
	middle := assessment.Plan[1]
	if middle.StartMs != 30_000 || middle.EndMs != 33_000 || middle.Disposition != DispositionDiscard || middle.DiscardReason != DiscardBelowClipFloor {
		t.Fatalf("middle = %+v, want exact explained discarded interval", middle)
	}
	if assessment.Plan[0].StartMs != 0 || assessment.Plan[2].EndMs != source.DurationMs || assessment.Plan[0].EndMs != middle.StartMs || middle.EndMs != assessment.Plan[2].StartMs {
		t.Fatalf("plan does not account for every millisecond: %+v", assessment.Plan)
	}
}

func TestAssessCurrentSplitStructureTranscriptOnlyBoundaryRemainsUnresolved(t *testing.T) {
	source := structureSource(149_000)
	segments := []Segment{
		{StartMs: 0, EndMs: 54_000, EndEvidence: "transcript", EndSrc: SrcTranscript, Transcript: "first product"},
		{StartMs: 54_000, EndMs: 149_000, StartEvidence: "transcript", StartSrc: SrcTranscript, Transcript: "second product"},
	}
	assessment, err := AssessDetection(source, DetectionProgress{ScannedThroughMs: source.DurationMs}, segments, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(assessment.Boundaries) != 1 || assessment.Boundaries[0].Status != BoundaryUnresolved || assessment.Kind != KindAmbiguous {
		t.Fatalf("transcript-only assessment = %+v", assessment)
	}
	if !strings.HasPrefix(assessment.Observations[0].Producer, "v34-split-shadow") {
		t.Fatalf("legacy model observation hid its non-certifying producer: %+v", assessment.Observations[0])
	}
}

func TestChapterEdgesRetainBoundsOfDroppedChapter(t *testing.T) {
	edges := ChapterEdges([]tools.Chapter{
		{StartMs: 0, EndMs: 30_000},
		{StartMs: 30_000, EndMs: 33_000},
		{StartMs: 33_000, EndMs: 65_000},
	})
	want := []int64{0, 30_000, 33_000, 65_000}
	if len(edges) != len(want) {
		t.Fatalf("edges = %v, want %v", edges, want)
	}
	for i := range want {
		if edges[i] != want[i] {
			t.Fatalf("edges = %v, want %v", edges, want)
		}
	}
}
