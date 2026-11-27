package packager

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// faultPlan is one scheduled item for the fault tests: how it opens and whether it reports.
type faultPlan struct {
	label   string
	dur     time.Duration
	enc     synth
	openErr error
}

// runFaultPlans airs the plans in order and returns the faults each item reported, by label.
func runFaultPlans(t *testing.T, plans []faultPlan) map[string][]error {
	t.Helper()
	var mu sync.Mutex
	faults := map[string][]error{}
	var i atomic.Int32
	finished := make(chan struct{})
	sched := func(ctx context.Context, _ time.Time) (Item, error) {
		n := int(i.Add(1)) - 1
		if n >= len(plans) {
			close(finished)
			<-ctx.Done()
			return Item{}, ctx.Err()
		}
		pl := plans[n]
		return Item{Label: pl.label, Duration: pl.dur,
			Open: func(ictx context.Context, slot Slot) (io.ReadCloser, error) {
				if pl.openErr != nil {
					return nil, pl.openErr
				}
				return pl.enc.encode(t, ictx, slot), nil
			},
			Faulted: func(err error) {
				mu.Lock()
				defer mu.Unlock()
				faults[pl.label] = append(faults[pl.label], err)
			},
		}, nil
	}
	p, err := New(Config{FPS: testFPS, Dir: t.TempDir(), RunAhead: time.Hour, SlateLead: 1500 * time.Millisecond}, sched, readySlate(testSlate(t)))
	if err != nil {
		t.Fatal(err)
	}
	start(t, p)
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("plans did not finish airing")
	}
	mu.Lock()
	defer mu.Unlock()
	out := map[string][]error{}
	for k, v := range faults {
		out[k] = append([]error(nil), v...)
	}
	return out
}

// An item whose encoder ends without a frame, or that cannot be opened, reports a fault so its
// clip can be fed back to the catalog; an item that airs reports none (#3855).
func TestAirItemReportsAnEncodeFault(t *testing.T) {
	boom := errors.New("source unreadable")
	faults := runFaultPlans(t, []faultPlan{
		{label: "good", dur: time.Second, enc: synth{label: "good"}},
		{label: "empty", dur: 1500 * time.Millisecond, enc: synth{label: "empty", zeroFrames: true}},
		{label: "unopenable", dur: time.Second, openErr: boom},
		{label: "after", dur: time.Second, enc: synth{label: "after"}},
	})
	if len(faults["good"]) != 0 || len(faults["after"]) != 0 {
		t.Fatalf("healthy items reported faults: %v", faults)
	}
	if len(faults["empty"]) != 1 {
		t.Fatalf("a zero-frame item reported %d faults, want 1", len(faults["empty"]))
	}
	if len(faults["unopenable"]) != 1 || !errors.Is(faults["unopenable"][0], boom) {
		t.Fatalf("an item that did not open reported %v, want its open error", faults["unopenable"])
	}
}

// An item that is merely late, or whose decoder configuration differs, says nothing about its
// clip's bytes and reports no fault.
func TestAirItemLateOrMismatchedIsNotAFault(t *testing.T) {
	never := make(chan struct{})
	other := append([]byte(nil), testSPS...)
	other[3] = 0x1e
	faults := runFaultPlans(t, []faultPlan{
		{label: "prog", dur: time.Second, enc: synth{label: "prog"}},
		{label: "late", dur: time.Second, enc: synth{label: "late", blockBefore: never}},
		{label: "odd", dur: time.Second, enc: synth{label: "odd", sps: other}},
	})
	if len(faults) != 0 {
		t.Fatalf("late and mismatched items reported faults: %v", faults)
	}
}
