// Package media owns host-wide resources shared by live and background media work.
package media

import (
	"context"
	"sync"
	"time"
)

const foregroundPreemptionWait = 500 * time.Millisecond

// EncodePool is the single admission boundary for hardware video encodes. Foreground playback may
// use every measured slot. Background preparation is limited to one slot, must leave one slot idle
// for playback, and receives a cancelled context when foreground demand needs its slot.
type EncodePool struct {
	capacityFn func() int

	once     sync.Once
	capacity int // -1 means unmeasured: foreground is unbounded, background is disabled.

	mu               sync.Mutex
	held             int
	background       bool
	backgroundCancel context.CancelFunc
	changed          chan struct{}
}

// NewEncodePool creates a host-wide hardware encode pool. capacity is resolved once because the
// underlying encoder probe is a property of the running process and can be expensive.
func NewEncodePool(capacity func() int) *EncodePool {
	return &EncodePool{capacityFn: capacity, changed: make(chan struct{})}
}

func (p *EncodePool) init() {
	p.once.Do(func() {
		p.capacity = -1
		if p.capacityFn != nil {
			p.capacity = p.capacityFn()
		}
	})
}

// AcquireForeground takes a hardware slot for live playback. When preparation owns the last slot,
// its context is cancelled and playback waits briefly for the process to release it; a stuck
// background process therefore degrades this caller to software rather than delaying tune-in.
func (p *EncodePool) AcquireForeground(ctx context.Context) (release func(), ok bool) {
	if p == nil {
		return func() {}, true
	}
	p.init()

	p.mu.Lock()
	if p.capacity < 0 {
		p.mu.Unlock()
		return func() {}, true
	}
	if p.held < p.capacity {
		release = p.acquireLocked(false, nil)
		p.mu.Unlock()
		return release, true
	}
	if !p.background {
		p.mu.Unlock()
		return nil, false
	}
	cancel := p.backgroundCancel
	changed := p.changed
	p.mu.Unlock()
	cancel()

	timer := time.NewTimer(foregroundPreemptionWait)
	defer timer.Stop()
	select {
	case <-changed:
	case <-ctx.Done():
		return nil, false
	case <-timer.C:
		return nil, false
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.held >= p.capacity {
		return nil, false
	}
	return p.acquireLocked(false, nil), true
}

// AcquireBackground takes the one preparation slot only when measured hardware capacity leaves a
// separate foreground reserve. The returned context, not the caller's original context, must be
// passed to the encoder so live playback can preempt the work.
func (p *EncodePool) AcquireBackground(ctx context.Context) (
	workCtx context.Context, release func(), ok bool,
) {
	if p == nil {
		return nil, nil, false
	}
	p.init()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.capacity < 2 || p.background || p.held >= p.capacity-1 {
		return nil, nil, false
	}
	workCtx, cancel := context.WithCancel(ctx)
	return workCtx, p.acquireLocked(true, cancel), true
}

func (p *EncodePool) acquireLocked(background bool, cancel context.CancelFunc) func() {
	p.held++
	if background {
		p.background = true
		p.backgroundCancel = cancel
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			p.mu.Lock()
			p.held--
			if background {
				p.background = false
				p.backgroundCancel = nil
				cancel()
			}
			close(p.changed)
			p.changed = make(chan struct{})
			p.mu.Unlock()
		})
	}
}
