package filler_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
)

// A check that cannot reach a verdict on a clip Loomarr found is a fail (#3838 decision 18): the
// clip is rejected with the reason recorded, so the upkeep pass deletes it and nobody is asked.
// A clip an operator added keeps the review it waits in.
func TestPipeline_ADoubtfulVerdictOnAFoundClipRejectsIt(t *testing.T) {
	for _, tc := range []struct {
		name       string
		found      bool
		stage      pipelinestate.StageID
		wantDisp   pipelinestate.Disposition
		wantReason pipelinestate.RejectReason
		wantDetail string
	}{
		{name: "found clip at a quality check", found: true, stage: pipelinestate.StageTranscode,
			wantDisp: pipelinestate.DispositionRejected, wantReason: pipelinestate.ReasonUndecided,
			wantDetail: "A 5.3s black span needs a look before this airs."},
		{name: "operator-added clip", found: false, stage: pipelinestate.StageTranscode,
			wantDisp: pipelinestate.DispositionReview},
		// Compilations wait for automatic splitting (#3838 step 6), which replaces the review.
		{name: "found compilation at the split", found: true, stage: pipelinestate.StageSplit,
			wantDisp: pipelinestate.DispositionReview},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := newPipeMemStore()
			seedEnrolled(st, "c1")
			row := st.rows["c1"]
			row.Stage = tc.stage
			if tc.found {
				row.AcquisitionID = "acq-9000001"
			}
			st.rows["c1"] = row
			doubtful := stage(tc.stage)
			doubtful.result = filler.StageResult{
				Verdict: pipelinestate.VerdictReview, Note: "A 5.3s black span needs a look before this airs.",
				// The split's review is a reel waiting for a cut, not a check that cannot decide.
				Doubt: tc.stage != pipelinestate.StageSplit,
			}
			if _, err := newPipe(st, []filler.Stage{doubtful}, filler.DefaultBudget()).RunOnce(context.Background()); err != nil {
				t.Fatal(err)
			}
			got := st.rows["c1"]
			if got.Disposition != tc.wantDisp {
				t.Fatalf("disposition = %q, want %q", got.Disposition, tc.wantDisp)
			}
			if got.RejectReason != tc.wantReason || got.RejectDetail != tc.wantDetail {
				t.Errorf("reject = %q / %q, want %q / %q", got.RejectReason, got.RejectDetail, tc.wantReason, tc.wantDetail)
			}
		})
	}
}

// A piece cut from a reel Loomarr found is found too (#4409): its row carries no acquisition, but
// its sidecar names the reel's, so a doubt drops it. A piece of a reel the operator added keeps
// its review.
func TestPipeline_ADoubtfulCutPieceFollowsItsReel(t *testing.T) {
	for _, tc := range []struct {
		name     string
		reelAcq  string
		wantDisp pipelinestate.Disposition
	}{
		{name: "piece of a found reel", reelAcq: "acq-9000001", wantDisp: pipelinestate.DispositionRejected},
		{name: "piece of an operator's reel", reelAcq: "", wantDisp: pipelinestate.DispositionReview},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			st := newPipeMemStore()
			seedEnrolled(st, "c1")
			clip := st.clips["c1"]
			clip.ParentHash = "reel"
			st.clips["c1"] = clip
			row := st.rows["c1"]
			row.Stage = pipelinestate.StageTranscode
			st.rows["c1"] = row
			media := filepath.Join(dir, filepath.FromSlash(clip.Path))
			if err := os.MkdirAll(filepath.Dir(media), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := asset.WriteSidecarTags(media, asset.SidecarTags{AcquisitionID: tc.reelAcq}, tc.reelAcq != ""); err != nil {
				t.Fatal(err)
			}
			doubtful := stage(pipelinestate.StageTranscode)
			doubtful.result = filler.StageResult{Verdict: pipelinestate.VerdictReview, Note: "unsure", Doubt: true}
			pipe := newPipe(st, []filler.Stage{doubtful}, filler.DefaultBudget()).WithRewind(nil, dir)
			if _, err := pipe.RunOnce(context.Background()); err != nil {
				t.Fatal(err)
			}
			if got := st.rows["c1"]; got.Disposition != tc.wantDisp {
				t.Fatalf("disposition = %q, want %q", got.Disposition, tc.wantDisp)
			}
		})
	}
}
