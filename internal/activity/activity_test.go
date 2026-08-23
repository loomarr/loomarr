package activity

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/loomarr/loomarr/internal/store"
)

type spySink struct {
	rows []store.Activity
	err  error
}

func (s *spySink) RecordActivity(_ context.Context, a store.Activity) error {
	if s.err != nil {
		return s.err
	}
	s.rows = append(s.rows, a)
	return nil
}

type spyNotifier struct{ n int }

func (s *spyNotifier) ActivityRecorded() { s.n++ }

// The three levels reach the store as written — they drive the UI's dot, and a level the
// frontend has no colour for renders as nothing at all.
func TestRecorder_LevelsReachTheStore(t *testing.T) {
	sink := &spySink{}
	r := New(sink, slog.New(slog.DiscardHandler))
	ctx := context.Background()

	r.Info(ctx, store.ActivityKindTitle, "t1", "landed")
	r.Warn(ctx, store.ActivityKindSystem, "", "seerr timed out")
	r.Error(ctx, store.ActivityKindChannel, "ch1", "reconcile failed")

	if len(sink.rows) != 3 {
		t.Fatalf("recorded %d rows, want 3", len(sink.rows))
	}
	for i, want := range []string{store.ActivityInfo, store.ActivityWarn, store.ActivityError} {
		if sink.rows[i].Level != want {
			t.Errorf("row %d level = %q, want %q", i, sink.rows[i].Level, want)
		}
	}
}

// ⚠ Best-effort by CONTRACT: Record returns nothing, so a caller cannot treat telemetry as
// load-bearing. A failing sink must not panic or block the operation that was being recorded.
func TestRecorder_StoreFailureIsSwallowed(t *testing.T) {
	r := New(&spySink{err: errors.New("disk full")}, slog.New(slog.DiscardHandler))
	r.Info(context.Background(), store.ActivityKindTitle, "t1", "landed") // must not panic
}

// A nil recorder and a nil sink are both no-ops, so subsystems built without one (unit tests,
// a store-less boot) need no guard at every call site.
func TestRecorder_NilIsSafe(t *testing.T) {
	var nilRec *Recorder
	nilRec.Info(context.Background(), store.ActivityKindTitle, "t1", "landed")
	New(nil, nil).Info(context.Background(), store.ActivityKindTitle, "t1", "landed")
}

// The SSE frame fires on a successful write — that is what lets the Dashboard feed skip
// polling entirely (§7, §12).
func TestRecorder_NotifiesOnWrite(t *testing.T) {
	spy := &spyNotifier{}
	r := New(&spySink{}, slog.New(slog.DiscardHandler)).WithNotifier(spy)

	r.Info(context.Background(), store.ActivityKindTitle, "t1", "landed")

	if spy.n != 1 {
		t.Errorf("notified %d times, want 1", spy.n)
	}
}

// ⚠ …and NOT on a failed one. A frame announcing a row that was never written would send the
// Dashboard to refetch a list that has not changed, and on a persistent failure it would do so
// on every attempt.
func TestRecorder_DoesNotNotifyWhenTheWriteFailed(t *testing.T) {
	spy := &spyNotifier{}
	r := New(&spySink{err: errors.New("disk full")}, slog.New(slog.DiscardHandler)).WithNotifier(spy)

	r.Info(context.Background(), store.ActivityKindTitle, "t1", "landed")

	if spy.n != 0 {
		t.Errorf("notified %d times after a failed write, want 0", spy.n)
	}
}
