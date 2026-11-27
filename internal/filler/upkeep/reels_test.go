package upkeep

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/filler/quarantine"
	"github.com/loomarr/loomarr/internal/media/tools"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// reel adds a split reel off the belt, with a playback file and a sidecar naming nothing of
// Loomarr's, as a fetched reel is on disk. An empty acquisition is a reel added by hand.
func (f *fixture) reel(t *testing.T, hash, acquisition string) string {
	t.Helper()
	full := f.reject(t, hash, acquisition, "", t0)
	c := f.store.clips[hash]
	c.Held, c.IsComposite = false, true
	f.store.clips[hash] = c
	row := f.store.rows[hash]
	row.Disposition, row.RejectReason = pipelinestate.DispositionComplete, ""
	f.store.rows[hash] = row
	return full
}

// piece adds a clip cut from parent in the given state.
func (f *fixture) piece(hash, parent string, d pipelinestate.Disposition) {
	c := clipstore.Clip{}
	c.Hash, c.ParentHash = hash, parent
	f.store.clips[hash] = c
	f.store.rows[hash] = pipelinestate.ClipPipeline{ClipHash: hash, Disposition: d}
}

func TestASettledReelIsLetGoAndTheRestAreKept(t *testing.T) {
	f := newFixture(t)
	settled := f.reel(t, hashOf("settled"), "run-1")
	f.piece(hashOf("on air"), hashOf("settled"), pipelinestate.DispositionReady)
	f.piece(hashOf("dropped"), hashOf("settled"), pipelinestate.DispositionRejected)
	f.piece(hashOf("dismissed"), hashOf("settled"), pipelinestate.DispositionDismissed)

	busy := f.reel(t, hashOf("busy"), "run-2")
	f.piece(hashOf("busy on air"), hashOf("busy"), pipelinestate.DispositionReady)
	f.piece(hashOf("still running"), hashOf("busy"), pipelinestate.DispositionRunning)

	waiting := f.reel(t, hashOf("waiting"), "run-3")
	f.piece(hashOf("needs a person"), hashOf("waiting"), pipelinestate.DispositionReview)

	proposed := f.reel(t, hashOf("proposed"), "run-4")
	f.store.proposals[hashOf("proposed")] = true

	byHand := f.reel(t, hashOf("by hand"), "")

	now := func() time.Time { return t0 }
	res, err := New(f.store, quarantine.New(f.dir, now), nil, f.dir, now, nil).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reels != 1 || res.Quarantined != 1 {
		t.Fatalf("Run = %+v, want 1 reel let go and its files quarantined", res)
	}
	if exists(settled) || exists(tools.SidecarPathFor(settled)) {
		t.Error("the settled reel's files are still in place, want them in quarantine")
	}
	if !f.store.reaped[hashOf("settled")] || f.store.rows[hashOf("settled")].FilesDeletedAt.IsZero() {
		t.Error("the settled reel's row is not reaped with its files marked deleted")
	}
	if _, ok := f.store.clips[hashOf("settled")]; !ok {
		t.Error("the settled reel's row is gone, want it kept for its pieces' lineage")
	}
	for name, kept := range map[string]string{"busy": busy, "waiting": waiting, "proposed": proposed, "by hand": byHand} {
		if !exists(kept) {
			t.Errorf("the %s reel was moved, want it kept", name)
		}
	}
}

// A piece already taken off the catalog no longer holds its reel: it will never be worked again.
func TestARemovedPieceDoesNotHoldItsReel(t *testing.T) {
	f := newFixture(t)
	reel := f.reel(t, hashOf("reel"), "run-1")
	f.piece(hashOf("on air"), hashOf("reel"), pipelinestate.DispositionReady)
	f.piece(hashOf("removed"), hashOf("reel"), pipelinestate.DispositionRunning)
	c := f.store.clips[hashOf("removed")]
	c.RemovedAt = t0
	f.store.clips[hashOf("removed")] = c

	now := func() time.Time { return t0 }
	if res, err := New(f.store, quarantine.New(f.dir, now), nil, f.dir, now, nil).Run(context.Background()); err != nil {
		t.Fatal(err)
	} else if res.Reels != 1 || exists(reel) {
		t.Fatalf("Run = %+v, reel present=%v; want the reel let go", res, exists(reel))
	}
}

// A piece with no pipeline row has not been settled by anything, so it holds its reel.
func TestAPieceWithNoRowHoldsItsReel(t *testing.T) {
	f := newFixture(t)
	reel := f.reel(t, hashOf("reel"), "run-1")
	c := clipstore.Clip{}
	c.Hash, c.ParentHash = hashOf("unenrolled"), hashOf("reel")
	f.store.clips[c.Hash] = c

	now := func() time.Time { return t0 }
	if res, err := New(f.store, quarantine.New(f.dir, now), nil, f.dir, now, nil).Run(context.Background()); err != nil {
		t.Fatal(err)
	} else if res.Reels != 0 || !exists(reel) {
		t.Fatalf("Run = %+v, reel present=%v; want the reel kept", res, exists(reel))
	}
}

func TestAReelWhoseMoveFailedIsFinishedByTheNextPass(t *testing.T) {
	f := newFixture(t)
	reel := f.reel(t, hashOf("reel"), "run-1")
	f.piece(hashOf("on air"), hashOf("reel"), pipelinestate.DispositionReady)
	now := func() time.Time { return t0 }

	if _, err := New(f.store, failingBin{quarantine.New(f.dir, now)}, nil, f.dir, now, nil).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.store.retires != 1 || !exists(reel) {
		t.Fatalf("after the failed move: retires=%d file=%v; want retired once, file kept", f.store.retires, exists(reel))
	}
	res, err := New(f.store, quarantine.New(f.dir, now), nil, f.dir, now, nil).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if f.store.retires != 1 || res.Reels != 0 || res.Quarantined != 1 || exists(reel) || exists(tools.SidecarPathFor(reel)) {
		t.Fatalf("second pass = %+v, retires=%d; want the files moved without retiring again", res, f.store.retires)
	}
}
