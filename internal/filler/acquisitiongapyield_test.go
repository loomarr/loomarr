package filler_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

// #749: a run's clips are attributed to the coverage gap their download was for, including the
// clips a compilation download was split into, so yield reads per gap rather than per run.
func TestAcquisitionGapYieldFrom_AttributesClipsAndTheirSplitsToTheDownloadsGap(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_900_000_000, 0).UTC()
	artifacts := []filler.AcquisitionArtifact{
		{ID: "a", ClipHash: "single", Gap: "era:1990-1999"},
		{ID: "b", ClipHash: "reel", Gap: "era:1990-1999"},
		{ID: "c", ClipHash: "modern"},                 // unsteered: no gap row
		{ID: "d", ClipHash: "", Gap: "era:1970-1979"}, // downloaded, not yet catalogued
	}
	row := func(hash string, d filler.Disposition) filler.ClipPipeline {
		return filler.ClipPipeline{ClipHash: hash, Status: filler.StatusDone, Disposition: d}
	}
	rows := []filler.ClipPipeline{
		row("single", filler.DispositionReady),
		row("reel", filler.DispositionDismissed), // the compilation, cut into its spots
		row("reel-spot-1", filler.DispositionReady),
		row("reel-spot-2", filler.DispositionRejected),
		row("modern", filler.DispositionReady),
		row("dropped-by-hand", filler.DispositionReady), // no artifact: not attributed
	}
	parents := map[string]string{"reel-spot-1": "reel", "reel-spot-2": "reel"}

	got := filler.AcquisitionGapYieldFrom(artifacts, rows, parents, now)
	want := []filler.AcquisitionGapYield{
		{Gap: "era:1970-1979", Downloads: 1},
		{Gap: "era:1990-1999", Downloads: 2, Outcome: filler.AcquisitionOutcome{
			Enrolled: 4, Ready: 2, Rejected: 1, Dismissed: 1,
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("gap yield = %+v\nwant %+v", got, want)
	}
	if got := filler.AcquisitionGapYieldFrom(artifacts[2:3], rows, parents, now); got != nil {
		t.Fatalf("unsteered run gap yield = %+v, want none", got)
	}
}
