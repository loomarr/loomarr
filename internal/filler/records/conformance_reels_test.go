package records

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/filler/split"
)

// testClipPipelineRetireSettledReel pins #4421: a split reel Loomarr fetched is let go once every
// piece cut from it is settled, in one transaction that reaps the row and marks its files deleted.
// A reel added by hand, one with a proposal left, one with a piece still running or waiting on a
// person, and one already let go all stay. The reaped row survives a sync that no longer finds its
// file, so every piece's parent_hash still resolves.
func testClipPipelineRetireSettledReel(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_900_000_000, 0).UTC()
	add := func(name, parent string, composite bool, acquisition string, d pipelinestate.Disposition) string {
		t.Helper()
		c := clipAt("reels/"+name+".mp4", name, clips.Commercial, 30_000)
		c.IsComposite, c.ParentHash = composite, parent
		if err := s.UpsertClip(ctx, Clip{Clip: c, UpdatedAt: now.Add(-time.Hour)}); err != nil {
			t.Fatal(err)
		}
		if d != "" {
			if err := s.UpsertClipPipeline(ctx, pipelinestate.ClipPipeline{
				ClipHash: c.Hash, AcquisitionID: acquisition, Stage: pipelinestate.StageFinish,
				Status: pipelinestate.StatusDone, Disposition: d,
				EnrolledAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour),
			}); err != nil {
				t.Fatal(err)
			}
		}
		return c.Hash
	}
	reel := func(name, acquisition string) string {
		return add(name, "", true, acquisition, pipelinestate.DispositionComplete)
	}

	for _, run := range []string{"run-1", "run-2", "run-3", "run-4", "run-5", "run-6", "run-7"} {
		saveAcquisitionRun(t, s, run)
	}
	settled := reel("settled", "run-1")
	onAir := add("settled-on-air", settled, false, "run-1", pipelinestate.DispositionReady)
	add("settled-dropped", settled, false, "run-1", pipelinestate.DispositionRejected)
	add("settled-dismissed", settled, false, "run-1", pipelinestate.DispositionDismissed)

	running := reel("running", "run-2")
	add("running-piece", running, false, "run-2", pipelinestate.DispositionRunning)
	review := reel("review", "run-3")
	add("review-piece", review, false, "run-3", pipelinestate.DispositionReview)
	unenrolled := reel("unenrolled", "run-4")
	add("unenrolled-piece", unenrolled, false, "", "")
	byHand := reel("by-hand", "")
	proposed := reel("proposed", "run-5")
	if err := s.UpsertSplitProposal(ctx, filler.SplitProposal{
		ID: "left-over", ClipHash: proposed, CreatedAt: now,
		Segments: []split.Segment{{StartMs: 0, EndMs: 10_000, Name: "leftover"}},
	}); err != nil {
		t.Fatal(err)
	}
	// A piece already taken off the catalog will never be worked again, so it holds nothing.
	removedPiece := reel("removed-piece", "run-6")
	gone := add("removed-piece-running", removedPiece, false, "run-6", pipelinestate.DispositionRunning)
	goneClip, err := s.GetClip(ctx, gone)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetClipsRemoved(ctx, []string{goneClip.Path}, now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	// A composite still on the belt is not settled however its pieces stand.
	onBelt := add("on-belt", "", true, "run-7", pipelinestate.DispositionReview)

	listed, err := s.ListSettledReels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(listed)
	want := []string{removedPiece, settled}
	slices.Sort(want)
	if !slices.Equal(listed, want) {
		t.Fatalf("ListSettledReels = %v, want %v", listed, want)
	}

	for _, tc := range []struct {
		name, hash string
		want       bool
	}{
		{"settled", settled, true}, {"running", running, false}, {"review", review, false},
		{"unenrolled", unenrolled, false}, {"by hand", byHand, false}, {"proposed", proposed, false},
		{"on the belt", onBelt, false}, {"a piece", onAir, false}, {"settled again", settled, false},
	} {
		got, err := s.RetireSettledReel(ctx, tc.hash, now)
		if err != nil || got != tc.want {
			t.Fatalf("RetireSettledReel(%s) = %v, %v; want %v", tc.name, got, err, tc.want)
		}
	}
	if got, err := s.RetireSettledReel(ctx, "no-such-clip", now); err != nil || got {
		t.Fatalf("RetireSettledReel(unknown) = %v, %v; want false", got, err)
	}

	row, _, err := s.GetClipPipeline(ctx, settled)
	if err != nil {
		t.Fatal(err)
	}
	if !row.FilesDeletedAt.Equal(now) || row.Disposition != pipelinestate.DispositionComplete {
		t.Fatalf("let-go row = %+v, want complete with files deleted at %v", row, now)
	}
	live, err := s.LiveClipPaths(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, stillLive := live[settled]; stillLive {
		t.Error("the let-go reel is still live, want it reaped")
	}
	if listed, err = s.ListSettledReels(ctx); err != nil || slices.Contains(listed, settled) {
		t.Fatalf("after the retire, ListSettledReels = %v (%v); want the let-go reel gone from it", listed, err)
	}

	// A sync that no longer finds the reel's file keeps its row, and the pieces keep theirs.
	var keep []string
	for hash := range live {
		keep = append(keep, hash)
	}
	if _, err := s.DeleteClipsNotIn(ctx, keep); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetClip(ctx, settled)
	if err != nil {
		t.Fatalf("the let-go reel's row was pruned: %v", err)
	}
	if !got.RemovedAt.IsZero() || !got.IsComposite {
		t.Fatalf("let-go reel = removed %v composite %v, want present and composite", got.RemovedAt, got.IsComposite)
	}
	piece, err := s.GetClip(ctx, onAir)
	if err != nil || piece.ParentHash != settled || !piece.RemovedAt.IsZero() {
		t.Fatalf("the reel's piece = %+v, %v; want it on air with its parent", piece, err)
	}
}
