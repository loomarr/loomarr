package pod_test

import (
	"fmt"
	"testing"

	"github.com/loomarr/loomarr/internal/filler/pod"
)

// A break made only of station material holds one station segment, as any other break does.
func TestStationOnlyBreakHoldsOneStationSegment(t *testing.T) {
	var catalog []pod.Clip
	for i := 0; i < 4; i++ {
		catalog = append(catalog, pod.Clip{Hash: fmt.Sprintf("ident-%d", i), Path: fmt.Sprintf("ident-%d.mp4", i),
			Role: pod.StationID, Era: 1992, Audience: pod.Kids, DurationMs: 8_000,
			GeographicScope: pod.GeographicLocal, Country: "US", Station: "fcc:1"})
	}
	for seed := int64(1); seed <= 20; seed++ {
		w := pod.Window{ChannelID: "kids", Seed: seed, Era: pod.Year(1992), Audience: pod.Kids, GapMs: 60_000, PodMax: 4}
		brk := pod.Assemble(catalog, w, harborPolicy(), nil)
		var segments int
		for _, e := range brk.Entries {
			if !e.IsFallbackCard {
				segments++
			}
		}
		if segments != 1 {
			t.Fatalf("seed %d: %d station segments in one break, want exactly 1: %+v", seed, segments, brk.Entries)
		}
	}
}
