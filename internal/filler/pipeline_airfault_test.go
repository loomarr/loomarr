package filler_test

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
)

// A clip that keeps failing to encode at air time is fed back to the catalog: its row is rejected as
// unplayable with the cause, and the clip is taken out of rotation so no break picks it again
// (#3855).
func TestPipeline_AirFaultRejectsAReadyClipAndTakesItOffAir(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "90002001")
	p := newPipe(st, asSlice(allStages()), filler.DefaultBudget())
	if _, err := p.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := st.rows["90002001"].Disposition; got != pipelinestate.DispositionReady {
		t.Fatalf("setup: clip is %q, want ready", got)
	}

	if err := p.RejectAirFault(context.Background(), "90002001", "ended without a frame three times"); err != nil {
		t.Fatal(err)
	}
	row := st.rows["90002001"]
	if row.Disposition != pipelinestate.DispositionRejected || row.RejectReason != pipelinestate.ReasonUnplayable ||
		row.RejectDetail != "ended without a frame three times" {
		t.Fatalf("row = %s/%s/%q, want rejected as unplayable with the cause", row.Disposition, row.RejectReason, row.RejectDetail)
	}
	if clip := st.clips["90002001"]; clip.RemovedAt.IsZero() {
		t.Fatal("the clip is still in rotation")
	}
}

// A clip that is not Ready is the pipeline's own business, and an unknown clip is nobody's: neither
// is touched.
func TestPipeline_AirFaultLeavesOtherClipsAlone(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "90002002") // still running
	p := newPipe(st, asSlice(allStages()), filler.DefaultBudget())

	if err := p.RejectAirFault(context.Background(), "90002002", "x"); err != nil {
		t.Fatal(err)
	}
	if got := st.rows["90002002"].Disposition; got != pipelinestate.DispositionRunning {
		t.Fatalf("a clip still on the conveyor became %q", got)
	}
	if err := p.RejectAirFault(context.Background(), "90002999", "x"); err != nil {
		t.Fatalf("an unknown clip is an error: %v", err)
	}
}
