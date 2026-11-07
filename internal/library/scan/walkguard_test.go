package scan

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// A walk that keeps making progress is waited for however long it takes; one that stops is given
// up on, and done closes when it finally ends.
func TestWatchedWalkGivesUpOnlyOnAStall(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	slow := func(steps *atomic.Int64) (Walk, error) {
		for range 10 {
			time.Sleep(5 * time.Millisecond)
			steps.Add(1)
		}
		return Walk{Complete: true}, nil
	}
	if w, _, err := watchedWalk(ctx, 20*time.Millisecond, slow); err != nil || !w.Complete {
		t.Fatalf("slow but moving walk = %+v, %v", w, err)
	}

	release := make(chan struct{})
	stuck := func(*atomic.Int64) (Walk, error) {
		<-release // a call blocked on a dead mount
		return Walk{}, nil
	}
	_, done, err := watchedWalk(ctx, 20*time.Millisecond, stuck)
	if !errors.Is(err, errWalkStalled) {
		t.Fatalf("stuck walk = %v, want a stall", err)
	}
	select {
	case <-done:
		t.Fatal("done closed while the walk is still blocked")
	default:
	}
	close(release)
	<-done
}

// While a root's given-up walk is still blocked, its passes read it as offline without walking
// again; once the walk ends, the next pass walks.
func TestStalledRootWaitsForItsWalk(t *testing.T) {
	h := newHarness(t)
	h.file("Alpha/Alpha.mkv", 10)
	blocked := make(chan struct{})
	h.im.markStalled(h.root, blocked)
	if !h.im.stalled(h.root) {
		t.Fatal("a blocked walk does not read as stalled")
	}
	h.run()
	if p := h.im.Progress(); len(p.Roots) != 1 || p.Roots[0].State != RootUnreadable {
		t.Fatalf("roots = %+v, want the stalled root offline", p.Roots)
	}
	close(blocked)
	h.run()
	if p := h.im.Progress(); p.Roots[0].State != RootScanned {
		t.Fatalf("after the walk ended = %+v", p.Roots)
	}
}
