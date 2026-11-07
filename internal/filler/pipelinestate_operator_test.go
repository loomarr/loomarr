package filler_test

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

func TestClipPipeline_RestoreRefusalReturnsOnlyRefusalsToReview(t *testing.T) {
	at := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	for _, from := range []filler.Disposition{filler.DispositionRejected, filler.DispositionDismissed} {
		row := filler.ClipPipeline{Disposition: from, RejectReason: "silent-content", RejectDetail: "99% silent"}
		if !row.RestoreRefusal(at) || row.Disposition != filler.DispositionReview ||
			row.RejectReason != "" || row.RejectDetail != "" || !row.UpdatedAt.Equal(at) {
			t.Errorf("restore from %s = %+v, want review with the reason cleared", from, row)
		}
	}
	for _, from := range []filler.Disposition{filler.DispositionRunning, filler.DispositionReady, filler.DispositionReview} {
		row := filler.ClipPipeline{Disposition: from}
		if row.RestoreRefusal(at) || row.Disposition != from {
			t.Errorf("restore moved a %s row: %+v", from, row)
		}
	}
}

func TestClipPipeline_SettleByOperatorMovesOnlyFromListedOriginsAndUnschedules(t *testing.T) {
	at := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	row := filler.ClipPipeline{Disposition: filler.DispositionReady, NextRun: at.Add(time.Hour)}
	if !row.SettleByOperator(filler.DispositionDismissed, at, filler.DispositionReview, filler.DispositionReady) ||
		row.Disposition != filler.DispositionDismissed || !row.NextRun.IsZero() || !row.UpdatedAt.Equal(at) {
		t.Fatalf("settle = %+v, want dismissed and unscheduled", row)
	}
	running := filler.ClipPipeline{Disposition: filler.DispositionRunning, NextRun: at}
	if running.SettleByOperator(filler.DispositionDismissed, at, filler.DispositionReview, filler.DispositionReady) ||
		running.Disposition != filler.DispositionRunning || !running.NextRun.Equal(at) {
		t.Fatalf("a running row was settled by an operator verb: %+v", running)
	}
}
