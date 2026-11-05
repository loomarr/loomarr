package scan

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
)

// walkStall is how long a walk may go without listing a folder or reading a file before the pass
// gives up on it. One folder of a few thousand entries lists in seconds even over SMB; a mount
// whose server went away can block a call for good.
const walkStall = 2 * time.Minute

// errWalkStalled is a walk that stopped making progress.
var errWalkStalled = errors.New("library scan: the walk stopped making progress")

// watchedWalk runs walk and gives up on it once it makes no progress for stall. A call blocked on a
// dead mount cannot be cancelled, so a walk given up on is left to end on its own: done closes
// when it does.
func watchedWalk(ctx context.Context, stall time.Duration, walk func(steps *atomic.Int64) (Walk, error)) (Walk, <-chan struct{}, error) {
	type result struct {
		walk Walk
		err  error
	}
	var steps atomic.Int64
	out := make(chan result, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		w, err := walk(&steps)
		out <- result{w, err}
	}()
	check := time.NewTicker(max(stall/4, time.Millisecond))
	defer check.Stop()
	last, lastAt := steps.Load(), time.Now()
	for {
		select {
		case r := <-out:
			return r.walk, done, r.err
		case <-ctx.Done():
			return Walk{}, done, ctx.Err()
		case now := <-check.C:
			if n := steps.Load(); n != last {
				last, lastAt = n, now
				continue
			}
			if now.Sub(lastAt) >= stall {
				return Walk{}, done, errWalkStalled
			}
		}
	}
}

// stalled reports whether root's last walk was given up on and has still not ended. Its next pass
// waits for it rather than starting another walk the same dead mount would block.
func (im *Importer) stalled(root string) bool {
	im.mu.Lock()
	defer im.mu.Unlock()
	done, ok := im.stalledWalks[root]
	if !ok {
		return false
	}
	select {
	case <-done:
		delete(im.stalledWalks, root)
		return false
	default:
		return true
	}
}

func (im *Importer) markStalled(root string, done <-chan struct{}) {
	im.mu.Lock()
	defer im.mu.Unlock()
	if im.stalledWalks == nil {
		im.stalledWalks = map[string]<-chan struct{}{}
	}
	im.stalledWalks[root] = done
}
