package fillerstore

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/defects"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/store"
)

// #2036: a copy that fails the floor is retired. The row outlives its file, so it is the "wanted:
// better copy" entry; it cannot be restored onto air, and a better copy folding it in removes it
// from the list.
func testDefectiveCopyRetirement(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	at := time.Unix(1_800_000_000, 0).UTC()
	bad := sampleClip("retire-bad", "Soap", filler.Commercial, 1988, filler.General, "")
	passing := sampleClip("retire-ok", "Cola", filler.Commercial, 1988, filler.General, "")
	for _, c := range []Clip{bad, passing} {
		if err := s.UpsertClip(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	verdict := func(hash string, v defects.Verdict, reason string) store.ClipDefectRecord {
		return store.ClipDefectRecord{ClipHash: hash, Readings: defects.Readings{Version: defects.MeasurementVersion},
			Assessment: defects.Assessment{Version: defects.FloorVersion}, Era: 1988, Verdict: v, VerdictReason: reason, MeasuredAt: at}
	}
	for _, r := range []store.ClipDefectRecord{verdict(bad.Hash, defects.VerdictFail, "detectors"),
		verdict(passing.Hash, defects.VerdictPass, "detectors")} {
		if err := s.UpsertClipDefects(ctx, r); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.RetireDefectiveCopy(ctx, bad.Hash, at); err != nil {
		t.Fatal(err)
	}
	if err := s.RetireDefectiveCopy(ctx, "no-such-clip", at); err == nil {
		t.Fatal("retiring an unknown clip reported success")
	}
	wanted, err := s.ListWantedCopies(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(wanted) != 1 || wanted[0].ClipHash != bad.Hash || wanted[0].Name != "Soap" || wanted[0].Reason != "detectors" ||
		!wanted[0].RetiredAt.Equal(at) {
		t.Fatalf("wanted = %+v, want only the retired copy", wanted)
	}
	if listed, err := s.ListClips(ctx, store.ClipFilter{IncludeHeld: true}); err != nil {
		t.Fatal(err)
	} else if len(listed) != 1 || listed[0].Hash != passing.Hash {
		t.Fatalf("catalog = %+v, want only the passing copy", listed)
	}

	// The file is gone, so the next sync does not find it; the entry must survive that.
	if _, err := s.DeleteClipsNotIn(ctx, []string{passing.Hash}); err != nil {
		t.Fatal(err)
	}
	// Restoring would air a missing file.
	if _, err := s.SetClipsRemoved(ctx, []string{bad.Path}, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetClip(ctx, bad.Hash); err != nil || got.RemovedAt.IsZero() {
		t.Fatalf("retired copy after sync and restore = %+v (err %v), want still removed", got, err)
	}

	// A better copy arrives and identity folds the entry into it: no longer wanted.
	better := sampleClip("retire-better", "Soap", filler.Commercial, 1988, filler.General, "")
	if err := s.UpsertClip(ctx, better); err != nil {
		t.Fatal(err)
	}
	if err := s.MergeInferiorCopy(ctx, store.InferiorCopy{ClipHash: bad.Hash, SourceHash: bad.Hash,
		SurvivorHash: better.Hash, DecidedAt: at.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if wanted, err := s.ListWantedCopies(ctx); err != nil || len(wanted) != 0 {
		t.Fatalf("wanted after the upgrade = %+v (err %v), want none", wanted, err)
	}
}
