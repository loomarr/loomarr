package filler_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/media/tools"
)

// A running row whose clip was removed can never publish: the finish rung refused it as stale and
// rested it a month at a time (2026-11-18: removed 10-31, parked to 12-18). It is retired on the
// first pass even though it is not due, and the clip keeps the time it was removed.
func TestPipeline_ARowWhoseClipWasRemovedRetiresEvenWhileResting(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "c1")
	removedAt := time.Unix(1_799_000_000, 0).UTC()
	clip := st.clips["c1"]
	clip.RemovedAt = removedAt
	st.clips["c1"] = clip
	row := st.rows["c1"]
	row.Stage, row.Status = pipelinestate.StageFinish, pipelinestate.StatusFailed
	row.NextRun = time.Unix(1_800_000_000, 0).Add(30 * 24 * time.Hour)
	st.rows["c1"] = row

	if _, err := newPipe(st, nil, filler.DefaultBudget()).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := st.rows["c1"]
	if got.Disposition != pipelinestate.DispositionRejected || !got.NextRun.IsZero() {
		t.Fatalf("row = %q next_run=%v, want rejected and unscheduled", got.Disposition, got.NextRun)
	}
	if !st.clips["c1"].RemovedAt.Equal(removedAt) {
		t.Errorf("removed_at = %v, want it kept at %v", st.clips["c1"].RemovedAt, removedAt)
	}
}

// Clips held for a person under the rule before "doubt means drop" are re-judged once from the
// quality report in their sidecar (2026-11-18: 13 fetched clips held since late October). A fetched
// single clip whose report reads as doubtful is rejected as undecided and leaves rotation. A clip an
// operator added, a reel waiting to be split, and a review the report does not explain stay put.
func TestPipeline_HeldDoubtsAreRejudgedUnderTheCurrentRule(t *testing.T) {
	blackSpan := tools.MediaQuality{DurationMs: 30_000, Black: []tools.Interval{{StartMs: 10_000, EndMs: 16_000}}}
	clean := tools.MediaQuality{DurationMs: 30_000}
	for _, tc := range []struct {
		name      string
		found     bool
		composite bool
		stage     pipelinestate.StageID
		quality   *tools.MediaQuality
		wantDisp  pipelinestate.Disposition
	}{
		{name: "fetched clip held for a black span", found: true, stage: pipelinestate.StageTranscode, quality: &blackSpan,
			wantDisp: pipelinestate.DispositionRejected},
		{name: "operator-added clip", stage: pipelinestate.StageTranscode, quality: &blackSpan,
			wantDisp: pipelinestate.DispositionReview},
		{name: "reel waiting to be split", found: true, composite: true, stage: pipelinestate.StageSplit, quality: &blackSpan,
			wantDisp: pipelinestate.DispositionReview},
		{name: "review the report does not explain", found: true, stage: pipelinestate.StageTranscode, quality: &clean,
			wantDisp: pipelinestate.DispositionReview},
		{name: "no report", found: true, stage: pipelinestate.StageTranscode,
			wantDisp: pipelinestate.DispositionReview},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			st := newPipeMemStore()
			seedEnrolled(st, "c1")
			clip := st.clips["c1"]
			clip.IsComposite = tc.composite
			st.clips["c1"] = clip
			row := st.rows["c1"]
			row.Stage, row.Status, row.Disposition = tc.stage, pipelinestate.StatusDone, pipelinestate.DispositionReview
			if tc.found {
				row.AcquisitionID = "acq-9000001"
			}
			st.rows["c1"] = row
			media := filepath.Join(dir, filepath.FromSlash(clip.Path))
			if err := os.MkdirAll(filepath.Dir(media), 0o755); err != nil {
				t.Fatal(err)
			}
			if tc.quality != nil {
				if err := asset.WriteSidecarTags(media, asset.SidecarTags{MediaQuality: tc.quality}, tc.found); err != nil {
					t.Fatal(err)
				}
			}

			pipe := newPipe(st, nil, filler.DefaultBudget()).WithRewind(nil, dir)
			if _, err := pipe.RunOnce(context.Background()); err != nil {
				t.Fatal(err)
			}
			got := st.rows["c1"]
			if got.Disposition != tc.wantDisp {
				t.Fatalf("disposition = %q, want %q", got.Disposition, tc.wantDisp)
			}
			rejected := tc.wantDisp == pipelinestate.DispositionRejected
			if rejected && (got.RejectReason != pipelinestate.ReasonUndecided || got.RejectDetail == "") {
				t.Errorf("reject = %q / %q, want undecided with the measured span", got.RejectReason, got.RejectDetail)
			}
			if out := st.clips["c1"].RemovedAt.IsZero(); out == rejected {
				t.Errorf("clip removed = %v, want %v", !out, rejected)
			}
		})
	}
}
