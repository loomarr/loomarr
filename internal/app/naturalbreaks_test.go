package app

import (
	"testing"

	"github.com/loomarr/loomarr/internal/inventory"
)

// Only measured scene fades may become mid-roll breaks. A container chapter mark is not one:
// measured on a 1940s crime film remux, the chapter at 904.862 s sits in a bright scene
// (YAVG 88.8 half a second before, 93.4 after), while a real fade reads 16 (black).
func TestFadeCandidatesDropUnmeasuredChapterMarks(t *testing.T) {
	got := fadeCandidates([]inventory.Break{
		{AtMs: 904_862, Source: "chapter", Confidence: 0.9},
		{AtMs: 961_794, Source: "fade", OverlapMs: 640, Confidence: 0.64},
	})
	if len(got) != 1 || got[0].AtMs != 961_794 || got[0].Confidence != 0.64 {
		t.Fatalf("candidates = %+v, want only the measured fade", got)
	}
}
