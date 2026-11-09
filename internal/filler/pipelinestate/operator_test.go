package pipelinestate_test

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
)

func TestClipPipeline_RestoreRefusalReturnsOnlyRefusalsToReview(t *testing.T) {
	at := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	for _, from := range []pipelinestate.Disposition{pipelinestate.DispositionRejected, pipelinestate.DispositionDismissed} {
		row := pipelinestate.ClipPipeline{Disposition: from, RejectReason: "silent-content", RejectDetail: "99% silent"}
		if !row.RestoreRefusal(at) || row.Disposition != pipelinestate.DispositionReview ||
			row.RejectReason != "" || row.RejectDetail != "" || !row.UpdatedAt.Equal(at) {
			t.Errorf("restore from %s = %+v, want review with the reason cleared", from, row)
		}
	}
	for _, from := range []pipelinestate.Disposition{pipelinestate.DispositionRunning, pipelinestate.DispositionReady, pipelinestate.DispositionReview} {
		row := pipelinestate.ClipPipeline{Disposition: from}
		if row.RestoreRefusal(at) || row.Disposition != from {
			t.Errorf("restore moved a %s row: %+v", from, row)
		}
	}
}

func TestClipPipeline_SettleByOperatorMovesOnlyFromListedOriginsAndUnschedules(t *testing.T) {
	at := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	row := pipelinestate.ClipPipeline{Disposition: pipelinestate.DispositionReady, NextRun: at.Add(time.Hour)}
	if !row.SettleByOperator(pipelinestate.DispositionDismissed, at, pipelinestate.DispositionReview, pipelinestate.DispositionReady) ||
		row.Disposition != pipelinestate.DispositionDismissed || !row.NextRun.IsZero() || !row.UpdatedAt.Equal(at) {
		t.Fatalf("settle = %+v, want dismissed and unscheduled", row)
	}
	running := pipelinestate.ClipPipeline{Disposition: pipelinestate.DispositionRunning, NextRun: at}
	if running.SettleByOperator(pipelinestate.DispositionDismissed, at, pipelinestate.DispositionReview, pipelinestate.DispositionReady) ||
		running.Disposition != pipelinestate.DispositionRunning || !running.NextRun.Equal(at) {
		t.Fatalf("a running row was settled by an operator verb: %+v", running)
	}
}
