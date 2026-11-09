package acquisition_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/acquisition"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
)

// #749: a run's clips are attributed to the coverage gap their download was for, including the
// clips a compilation download was split into, so yield reads per gap rather than per run.
func TestAcquisitionGapYieldFrom_AttributesClipsAndTheirSplitsToTheDownloadsGap(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_900_000_000, 0).UTC()
	artifacts := []acquisition.Artifact{
		{ID: "a", ClipHash: "single", Gap: "era:1990-1999"},
		{ID: "b", ClipHash: "reel", Gap: "era:1990-1999"},
		{ID: "c", ClipHash: "modern"},                 // unsteered: no gap row
		{ID: "d", ClipHash: "", Gap: "era:1970-1979"}, // downloaded, not yet catalogued
	}
	row := func(hash string, d pipelinestate.Disposition) pipelinestate.ClipPipeline {
		return pipelinestate.ClipPipeline{ClipHash: hash, Status: pipelinestate.StatusDone, Disposition: d}
	}
	rows := []pipelinestate.ClipPipeline{
		row("single", pipelinestate.DispositionReady),
		row("reel", pipelinestate.DispositionDismissed),
		row("reel-spot-1", pipelinestate.DispositionReady),
		row("reel-spot-2", pipelinestate.DispositionRejected),
		row("modern", pipelinestate.DispositionReady),
		row("dropped-by-hand", pipelinestate.DispositionReady),
	}
	parents := map[string]string{"reel-spot-1": "reel", "reel-spot-2": "reel"}

	got := acquisition.GapYieldFrom(artifacts, rows, parents, now)
	want := []acquisition.GapYield{
		{Gap: "era:1970-1979", Downloads: 1},
		{Gap: "era:1990-1999", Downloads: 2, Outcome: acquisition.Outcome{
			Enrolled: 4, Ready: 2, Rejected: 1, Dismissed: 1,
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("gap yield = %+v\nwant %+v", got, want)
	}
	if got := acquisition.GapYieldFrom(artifacts[2:3], rows, parents, now); got != nil {
		t.Fatalf("unsteered run gap yield = %+v, want none", got)
	}
}
