// Package media owns host-wide resources shared by live and background media work.
package media

import (
	"context"
	"sync"
	"time"
)

const foregroundPreemptionWait = 500 * time.Millisecond

// memoryRampWindow is how long a newly admitted encode is assumed not yet to show in the host's
// available-memory reading. A hardware encoder allocates its device context and pinned buffers over
// its first seconds, and preparation admits leases in bursts; without this, every lease of a burst
// would be checked against the same pre-burst reading. The estimate is deliberately conservative: an
// encode whose memory shows sooner is counted twice until the window passes, which can briefly
// under-admit preparation but never over-admits into the reserve.
const memoryRampWindow = 30 * time.Second

// MemoryGate bounds hardware encode admission by host memory (design §9.1, prepared playout
// admission). It is evaluated per lease rather than folded into capacity: available memory already
// excludes what running encodes hold, so a capacity derived from it would let running preparation
// refuse live playback.
type MemoryGate struct {
	// Available is the host memory available for new work, in bytes; ok=false means unknown, which
	// leaves the gate open (the same stance as unmeasured capacity).
	Available func() (bytes int64, ok bool)
	// Reserve is the host memory, in bytes, that admission must leave available.
	Reserve func() int64
	// PerEncode is the host memory, in bytes, one hardware encode is expected to hold.
	PerEncode func() int64
}

// EncodePool is the single admission boundary for hardware video encodes. Foreground playback may
// use every measured slot. Background preparation may fill measured capacity minus one, leaving a
// live reserve; each lease receives a cancelled context when foreground demand needs its slot. An
// optional MemoryGate adds a host-memory bound to every lease decision without a second semaphore.
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

	memory *MemoryGate
	now    func() time.Time
	// ramping holds the admission time of each LIVE lease still inside memoryRampWindow, keyed by a
	// per-lease id. Release deletes the entry, so an encode that was preempted or finished stops
	// counting at once instead of shadowing the next admission for the rest of the window.
	ramping map[uint64]time.Time
	rampID  uint64
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

// WithMemoryGate adds the host-memory bound to every lease decision. It returns the pool for
// chaining and must be called before the pool is shared.
func (p *EncodePool) WithMemoryGate(gate MemoryGate) *EncodePool {
	p.memory = &gate
	return p
}

// memoryAdmits reports whether one more hardware encode fits in host memory: available memory, less
// the reserve and the cost of encodes admitted too recently to show in the reading, must still cover
// one encode. Callers hold p.mu; available is read before locking because it touches the filesystem.
func (p *EncodePool) memoryAdmitsLocked(available int64, known bool) bool {
	if p.memory == nil || !known {
		return true
	}
	cutoff := p.clock().Add(-memoryRampWindow)
	for id, at := range p.ramping {
		if !at.After(cutoff) {
			delete(p.ramping, id) // old enough that the reading already reflects it
		}
	}
	cost := p.memory.PerEncode()
	if cost <= 0 {
		return true
	}
	return available-p.memory.Reserve()-int64(len(p.ramping))*cost >= cost
}

func (p *EncodePool) readMemory() (int64, bool) {
	if p.memory == nil || p.memory.Available == nil {
		return 0, false
	}
	return p.memory.Available()
}

func (p *EncodePool) clock() time.Time {
	if p.now != nil {
		return p.now()
	}
	return time.Now()
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
		available, known := p.readMemory()
		p.mu.Lock()
		if capacity < 0 {
			p.mu.Unlock()
			return func() {}, true
		}
		// A memory shortfall is handled exactly like a slot shortfall: background work is preempted
		// below and the check repeats as it exits, so preparation can never cost live playback its
		// hardware encode. Only with no background work left does live fall back to software.
		if len(p.backgrounds) == 0 && p.held < capacity && p.memoryAdmitsLocked(available, known) {
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
	available, known := p.readMemory()
	p.mu.Lock()
	defer p.mu.Unlock()
	if capacity < 2 || p.foregrounds > 0 || p.waiters > 0 || p.held >= capacity-1 {
		return nil, nil, false
	}
	if !p.memoryAdmitsLocked(available, known) {
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
	var rampID uint64
	if p.memory != nil {
		if p.ramping == nil {
			p.ramping = make(map[uint64]time.Time)
		}
		p.rampID++
		rampID = p.rampID
		p.ramping[rampID] = p.clock()
	}
	if backgroundID == 0 {
		p.foregrounds++
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			p.mu.Lock()
			p.held--
			// The encode no longer exists; it must not keep counting as not-yet-visible memory.
			// once guards this path, and deleting an aged-out id is a no-op.
			delete(p.ramping, rampID)
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
