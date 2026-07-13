package reconcile

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

type fakeSweeper struct {
	name   string
	purged int
	err    error
	calls  int
}

func (f *fakeSweeper) Name() string { return f.name }
func (f *fakeSweeper) Sweep(_ context.Context, _ time.Time) (int, error) {
	f.calls++
	return f.purged, f.err
}

// The janitor runs every registered sweeper once per Sweep.
func TestJanitorRunsAllSweepers(t *testing.T) {
	a := &fakeSweeper{name: "sessions", purged: 3}
	b := &fakeSweeper{name: "jobs", purged: 0}
	j := NewJanitor(slog.New(slog.DiscardHandler), func() time.Time { return now }, a, b)

	j.Sweep(context.Background())
	if a.calls != 1 || b.calls != 1 {
		t.Errorf("each sweeper should run once: a=%d b=%d", a.calls, b.calls)
	}
}

// A failing sweeper is skipped, not fatal — the others still run.
func TestJanitorFailingSweeperNonFatal(t *testing.T) {
	bad := &fakeSweeper{name: "bad", err: errors.New("db locked")}
	good := &fakeSweeper{name: "good", purged: 1}
	j := NewJanitor(slog.New(slog.DiscardHandler), func() time.Time { return now }, bad, good)

	j.Sweep(context.Background()) // must not panic
	if good.calls != 1 {
		t.Error("a failing sweeper must not prevent later sweepers from running")
	}
}
