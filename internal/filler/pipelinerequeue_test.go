package filler_test

import (
	"context"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/filler"
)

// seedParkedReel puts a compilation in the state that was unreachable before V54a: parked at the
// split rung, holding a proposal, claimed by nothing.
//
// ⚠ `Attempts` is deliberately non-zero. The retries these rows spent were spent losing to a gate
// that structurally could not pass, and a requeue that carried them forward would exhaust the
// budget on the way to succeeding — the same reason migration 00050 reset the column.
func seedParkedReel(st *pipeMemStore, hash string) {
	st.put(filler.StoreClip{Clip: filler.Clip{Hash: hash, Path: "reels/" + hash + ".mp4", Name: hash}})
	st.rows[hash] = filler.ClipPipeline{
		ClipHash: hash, Stage: filler.StageSplit, Status: filler.StatusQueued,
		Disposition: filler.DispositionReview, Attempts: 3,
		NextRun: time.Unix(1_900_000_000, 0).UTC(), // far future: parked rows are not due
	}
}

// claimable reports whether the pipeline can actually SEE the row — the property that was broken.
//
// ⚠ Asserted through `ListPipelineWork` rather than by reading `Disposition` back. The bug was
// never "the field says review"; it was "nothing claims this row, ever". A test that reads the
// field it just wrote would pass against a requeue that set the disposition but left `next_run` in
// the future, which is the same dead end wearing a different value.
func claimable(t *testing.T, st *pipeMemStore, hash string) bool {
	t.Helper()
	at := time.Unix(1_800_000_000, 0).UTC()
	work, err := st.ListPipelineWork(context.Background(), at, 0)
	if err != nil {
		t.Fatalf("list pipeline work: %v", err)
	}
	for _, r := range work {
		if r.ClipHash == hash {
			return true
		}
	}
	return false
}

// The whole point: a reel parked at split goes back on the belt.
func TestRequeue_ReturnsAParkedReelToTheBelt(t *testing.T) {
	st := newPipeMemStore()
	seedParkedReel(st, "reel1")
	p := newPipe(st, nil, filler.Budget{})

	if claimable(t, st, "reel1") {
		t.Fatal("precondition: a parked reel must not be claimable before the requeue")
	}

	moved, err := p.Requeue(context.Background(), "reel1")
	if err != nil {
		t.Fatalf("requeue: %v", err)
	}
	if !moved {
		t.Fatal("requeue reported it moved nothing, but the reel was parked")
	}
	if !claimable(t, st, "reel1") {
		t.Fatal("the reel is still invisible to ListPipelineWork — the requeue did not un-park it")
	}
	if got := st.rows["reel1"].Stage; got != filler.StageSplit {
		t.Errorf("stage moved to %q; a requeue must leave the reel at the split rung", got)
	}
	if got := st.rows["reel1"].Attempts; got != 0 {
		t.Errorf("attempts = %d, want 0 — retries spent losing to the old gate are given back", got)
	}
}

// A refusal is not overturned by re-detecting. `rejected` has its own restore path (`Soft()`),
// where the operator can see the reason and argue with it.
func TestRequeue_LeavesARejectedReelRefused(t *testing.T) {
	st := newPipeMemStore()
	seedParkedReel(st, "reel1")
	row := st.rows["reel1"]
	row.Disposition = filler.DispositionRejected
	row.RejectReason, row.RejectDetail = filler.ReasonTooShort, "8.2s; floor is 10s"
	st.rows["reel1"] = row

	moved, err := p2(st).Requeue(context.Background(), "reel1")
	if err != nil {
		t.Fatalf("requeue: %v", err)
	}
	if moved {
		t.Fatal("a re-detect quietly overturned a rejection")
	}
	if got := st.rows["reel1"].RejectReason; got != filler.ReasonTooShort {
		t.Errorf("reject reason = %q, want it left intact", got)
	}
	if claimable(t, st, "reel1") {
		t.Fatal("a rejected reel became claimable")
	}
}

// Scope is `stage='split'`. A clip parked further up the ladder is a different problem, and
// un-parking it here would resume a rung whose inputs nobody re-derived.
func TestRequeue_LeavesAReelParkedAtALaterStageAlone(t *testing.T) {
	st := newPipeMemStore()
	seedParkedReel(st, "reel1")
	row := st.rows["reel1"]
	row.Stage = filler.StageScore
	st.rows["reel1"] = row

	moved, err := p2(st).Requeue(context.Background(), "reel1")
	if err != nil {
		t.Fatalf("requeue: %v", err)
	}
	if moved {
		t.Fatal("requeued a row parked at a stage other than split")
	}
	if claimable(t, st, "reel1") {
		t.Fatal("a row parked at score became claimable")
	}
}

// Idempotent: a reel already on the belt is left exactly as it is, so a second re-detect cannot
// reset the attempts of a run that is legitimately retrying.
func TestRequeue_LeavesARunningRowAlone(t *testing.T) {
	st := newPipeMemStore()
	seedParkedReel(st, "reel1")
	row := st.rows["reel1"]
	row.Disposition, row.Attempts, row.NextRun = filler.DispositionRunning, 2, time.Time{}
	st.rows["reel1"] = row

	moved, err := p2(st).Requeue(context.Background(), "reel1")
	if err != nil {
		t.Fatalf("requeue: %v", err)
	}
	if moved {
		t.Fatal("requeue reported a change on a row that was already running")
	}
	if got := st.rows["reel1"].Attempts; got != 2 {
		t.Errorf("attempts = %d, want 2 — a running row's retry budget must not be reset", got)
	}
}

// A clip with no pipeline row is not enrolled; enrolment owns that, and inventing a row here would
// create one at a stage the enroller never chose.
func TestRequeue_IsANoOpWithNoPipelineRow(t *testing.T) {
	st := newPipeMemStore()
	st.put(filler.StoreClip{Clip: filler.Clip{Hash: "reel1", Path: "reels/reel1.mp4", Name: "reel1"}})

	moved, err := p2(st).Requeue(context.Background(), "reel1")
	if err != nil {
		t.Fatalf("requeue: %v", err)
	}
	if moved {
		t.Fatal("requeue reported a change for a clip with no pipeline row")
	}
	if _, ok := st.rows["reel1"]; ok {
		t.Fatal("requeue invented a pipeline row; enrolment owns that decision")
	}
}

func p2(st *pipeMemStore) *filler.Pipeline { return newPipe(st, nil, filler.Budget{}) }
