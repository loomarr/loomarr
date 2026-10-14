package images

import (
	"context"
	"sync"
)

type workerClass string

const (
	workerInteractive workerClass = "interactive"
	workerBackground  workerClass = "background"
)

// workerCapacity owns the complete admission policy for Rust processes. Callers identify only the
// workload class; reservation, single-slot priority, cancellation, and balanced release stay here.
type workerCapacity struct {
	mu                 sync.Mutex
	total              int
	backgroundLimit    int
	inUse              int
	backgroundInUse    int
	backgroundWaiters  int
	interactiveWaiters int
	changed            chan struct{}
}

func newWorkerCapacity(total int) *workerCapacity {
	total = max(1, total)
	return &workerCapacity{
		total: total, backgroundLimit: max(1, total-1), changed: make(chan struct{}),
	}
}

func (c *workerCapacity) acquire(ctx context.Context, class workerClass) (func(), error) {
	c.mu.Lock()
	if class == workerInteractive {
		c.interactiveWaiters++
		c.signalLocked()
	} else {
		c.backgroundWaiters++
	}
	for !c.canAcquireLocked(class) {
		changed := c.changed
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			c.mu.Lock()
			if class == workerInteractive {
				c.interactiveWaiters--
				c.signalLocked()
			} else {
				c.backgroundWaiters--
			}
			c.mu.Unlock()
			return nil, ctx.Err()
		case <-changed:
			c.mu.Lock()
		}
	}
	if class == workerInteractive {
		c.interactiveWaiters--
	} else {
		c.backgroundWaiters--
	}
	c.inUse++
	if class == workerBackground {
		c.backgroundInUse++
	}
	c.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			c.inUse--
			if class == workerBackground {
				c.backgroundInUse--
			}
			c.signalLocked()
			c.mu.Unlock()
		})
	}, nil
}

func (c *workerCapacity) canAcquireLocked(class workerClass) bool {
	if c.inUse >= c.total {
		return false
	}
	if class != workerBackground {
		return true
	}
	return c.backgroundInUse < c.backgroundLimit && c.interactiveWaiters == 0
}

func (c *workerCapacity) signalLocked() {
	close(c.changed)
	c.changed = make(chan struct{})
}
