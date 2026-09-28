package fillerstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
)

// testCoreTransactionSpansFillerTables proves the seam #1747 split the store along: one
// transaction begun through the core's Handle carries a core-table write and a filler-store write,
// and they commit or roll back together. A failure between the two halves must leave neither.
func testCoreTransactionSpansFillerTables(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := t.Context()
	fs := s.(extended).sqlStore
	at := time.Unix(1_700_000_000, 0).UTC()

	// writeBoth writes a core settings row, then a filler inference row through the filler
	// store's own tx-scoped helper, then fails or commits.
	writeBoth := func(key, evaluationID string, fail bool) error {
		tx, err := store.HandleOf(s).Begin(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		if _, err := tx.ExecContext(ctx, fs.ph(`INSERT INTO settings (key, value) VALUES (?, ?)`), key, "v"); err != nil {
			return err
		}
		evaluation := InferenceEvaluation{ID: evaluationID, ClipHash: "tx-clip", State: InferenceReserved, CreatedAt: at, UpdatedAt: at}
		if err := insertInferenceEvaluation(ctx, tx, fs.ph, evaluation); err != nil {
			return err
		}
		if fail {
			return errors.New("fail midway")
		}
		return tx.Commit()
	}

	if err := writeBoth("tx.rolled-back", "eval-rolled-back", true); err == nil {
		t.Fatal("the failing write reported success")
	}
	if _, err := s.GetSetting(ctx, "tx.rolled-back"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("core row after rollback: err = %v, want ErrNotFound", err)
	}
	if _, err := s.GetInferenceEvaluation(ctx, "eval-rolled-back"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("filler row after rollback: err = %v, want ErrNotFound", err)
	}

	// The control: the same writes committed are both visible, so the rollback above is the
	// transaction's doing and not a write that never happened.
	if err := writeBoth("tx.committed", "eval-committed", false); err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetSetting(ctx, "tx.committed"); err != nil || got != "v" {
		t.Errorf("core row after commit = %q (err %v), want v", got, err)
	}
	if _, err := s.GetInferenceEvaluation(ctx, "eval-committed"); err != nil {
		t.Errorf("filler row after commit: %v", err)
	}
}

// sampleChannel is a minimal saved-channel fixture: the filler tests only need a channel to exist
// so its filler selection can pin or exclude clips.
func sampleChannel(id string, number int, deadline time.Time) store.Channel {
	ch := store.Channel{}
	ch.ID = id
	ch.IntentRef = "intent-" + id
	ch.Name = "Channel " + id
	ch.Number = number
	ch.Group = "Loomarr"
	ch.Strategy = schedule.Sequential
	ch.Status = schedule.StatusLive
	ch.UpdatedAt = 1_700_000_000
	ch.Lineup = []schedule.LineupEntry{{Key: "movie:tmdb:1", Title: "A", DurationMs: 3600000}}
	ch.Desired = []schedule.Slot{
		{Kind: schedule.SlotProgram, Key: "movie:tmdb:1", LibraryItemID: "lib-1", Title: "A", DurationMs: 3600000},
	}
	ch.ReconcileDeadline = deadline
	return ch
}

func mustSaveChannel(t *testing.T, s Store, ch store.Channel) store.Channel {
	t.Helper()
	saved, err := s.SaveChannel(context.Background(), ch)
	if err != nil {
		t.Fatal(err)
	}
	return saved
}
