// Package media owns host-wide resources shared by live and background media work.
package media

import (
	"context"
	"sync"
	"time"
)

const foregroundPreemptionWait = 500 * time.Millisecond

// EncodePool is the single admission boundary for hardware video encodes. Foreground playback may
// use every measured slot. Background preparation may fill measured capacity minus one, leaving a
// live reserve; each lease receives a cancelled context when foreground demand needs its slot.
type EncodePool struct {
	capacityFn func() int
	dynamic    bool

	once     sync.Once
	capacity int // -1 means unmeasured: foreground is unbounded, background is disabled.

	mu          sync.Mutex
	held        int
	foregrounds int
	waiters     int
	nextID      uint64
	backgrounds map[uint64]*backgroundLease
	changed     chan struct{}
}

type backgroundLease struct {
	id         uint64
	neededAt   time.Time
	cancel     context.CancelFunc
	preempting bool
}

// NewEncodePool creates a host-wide hardware encode pool. capacity is resolved once because the
// underlying encoder probe is a property of the running process and can be expensive.
func NewEncodePool(capacity func() int) *EncodePool {
	return &EncodePool{
		capacityFn: capacity, backgrounds: make(map[uint64]*backgroundLease), changed: make(chan struct{}),
	}
}

// NewDynamicEncodePool creates a pool whose inexpensive effective-capacity
// callback is evaluated for every lease attempt. Use it when operator limits or
// shared-resource pressure can change during the process lifetime.
func NewDynamicEncodePool(capacity func() int) *EncodePool {
	return &EncodePool{
		capacityFn: capacity, dynamic: true,
		backgrounds: make(map[uint64]*backgroundLease), changed: make(chan struct{}),
	}
}

func (p *EncodePool) init() {
	p.once.Do(func() {
		p.capacity = -1
		if p.capacityFn != nil {
			p.capacity = p.capacityFn()
		}
	})
}

func (p *EncodePool) capacityForAdmission() int {
	if p.dynamic {
		if p.capacityFn == nil {
			return -1
		}
		return p.capacityFn()
	}
	p.init()
	return p.capacity
}

// AcquireForeground takes a hardware slot for live playback. The first foreground caller cancels
// every background lease and waits briefly for preparation to drain. Accelerated preparation is
// deliberately excluded while any foreground lease is held: the synthetic single-encoder capacity
// measurement does not prove that unpaced real-file preparation can share decode/filter throughput
// with latency-critical playback.
func (p *EncodePool) AcquireForeground(ctx context.Context) (release func(), ok bool) {
	if p == nil {
		return func() {}, true
	}
	timer := time.NewTimer(foregroundPreemptionWait)
	defer timer.Stop()
	waiting := false
	for {
		capacity := p.capacityForAdmission()
		p.mu.Lock()
		if capacity < 0 {
			p.mu.Unlock()
			return func() {}, true
		}
		if len(p.backgrounds) == 0 && p.held < capacity {
			if waiting {
				p.waiters--
			}
			release = p.acquireLocked(0, nil)
			p.mu.Unlock()
			return release, true
		}
		if len(p.backgrounds) == 0 {
			if waiting {
				p.waiters--
			}
			p.mu.Unlock()
			return nil, false
		}
		if !waiting {
			p.waiters++
			waiting = true
		}
		for _, background := range p.backgrounds {
			if !background.preempting {
				background.preempting = true
				background.cancel()
			}
		}
		changed := p.changed
		p.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			p.removeWaiter()
			return nil, false
		case <-timer.C:
			p.removeWaiter()
			return nil, false
		}
	}
}

// AcquireBackground takes one preparation slot only when measured hardware capacity leaves a
// separate foreground reserve. neededAt lets foreground preempt the least urgent work. The returned
// context, not the caller's original context, must be passed to the encoder.
func (p *EncodePool) AcquireBackground(ctx context.Context, neededAt time.Time) (
	workCtx context.Context, release func(), ok bool,
) {
	if p == nil {
		return nil, nil, false
	}
	capacity := p.capacityForAdmission()
	p.mu.Lock()
	defer p.mu.Unlock()
	if capacity < 2 || p.foregrounds > 0 || p.waiters > 0 || p.held >= capacity-1 {
		return nil, nil, false
	}
	workCtx, cancel := context.WithCancel(ctx)
	p.nextID++
	id := p.nextID
	p.backgrounds[id] = &backgroundLease{id: id, neededAt: neededAt, cancel: cancel}
	return workCtx, p.acquireLocked(id, cancel), true
}

func (p *EncodePool) acquireLocked(backgroundID uint64, cancel context.CancelFunc) func() {
	p.held++
	if backgroundID == 0 {
		p.foregrounds++
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			p.mu.Lock()
			p.held--
			if backgroundID == 0 {
				p.foregrounds--
			} else {
				delete(p.backgrounds, backgroundID)
				cancel()
			}
			p.signalLocked()
			p.mu.Unlock()
		})
	}
}

func (p *EncodePool) removeWaiter() {
	p.mu.Lock()
	p.waiters--
	p.signalLocked()
	p.mu.Unlock()
}

func (p *EncodePool) signalLocked() {
	close(p.changed)
	p.changed = make(chan struct{})
}
