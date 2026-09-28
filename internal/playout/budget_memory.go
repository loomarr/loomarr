package playout

import (
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/media"
)

// memoryRampWindow is how long a newly admitted encode is assumed not yet to show in the host's
// available-memory reading. A hardware encoder allocates its device context and pinned buffers over
// its first seconds; without this, encodes admitted together would each be checked against the same
// earlier reading. The estimate is deliberately conservative: an encode whose memory shows sooner is
// counted twice until the window passes, which can briefly under-admit but never over-admits into
// the reserve.
const memoryRampWindow = 30 * time.Second

// hostMemory is the ResourceBudget's host-memory gate on hardware transcodes (design §9.1). It is
// evaluated per admission rather than folded into capacity: available memory already excludes what
// running encodes hold, so a capacity derived from it would count them twice.
type hostMemory struct {
	gate media.MemoryGate
	now  func() time.Time

	mu sync.Mutex
	// ramping holds the admission time of each LIVE encode still inside memoryRampWindow, keyed by
	// a per-admission id. Release deletes the entry, so an encode that finished stops counting at
	// once instead of shadowing the next admission for the rest of the window.
	ramping map[uint64]time.Time
	next    uint64
}

// admit admits one hardware encode when available memory, less the reserve and the encodes admitted
// too recently to show in the reading, still covers one more. Unknown memory leaves the gate open.
// release is idempotent.
func (m *hostMemory) admit() (release func(), ok bool) {
	available, known := m.available() // read before locking: it touches the filesystem
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.admitsLocked(available, known) {
		return nil, false
	}
	if m.ramping == nil {
		m.ramping = make(map[uint64]time.Time)
	}
	m.next++
	id := m.next
	m.ramping[id] = m.clock()
	var once sync.Once
	return func() {
		once.Do(func() {
			m.mu.Lock()
			delete(m.ramping, id) // deleting an aged-out id is a no-op
			m.mu.Unlock()
		})
	}, true
}

func (m *hostMemory) admitsLocked(available int64, known bool) bool {
	if !known {
		return true
	}
	cutoff := m.clock().Add(-memoryRampWindow)
	for id, at := range m.ramping {
		if !at.After(cutoff) {
			delete(m.ramping, id) // old enough that the reading already reflects it
		}
	}
	cost := int64(0)
	if m.gate.PerEncode != nil {
		cost = m.gate.PerEncode()
	}
	if cost <= 0 {
		return true
	}
	reserve := int64(0)
	if m.gate.Reserve != nil {
		reserve = m.gate.Reserve()
	}
	return available-reserve-int64(len(m.ramping))*cost >= cost
}

func (m *hostMemory) available() (int64, bool) {
	if m.gate.Available == nil {
		return 0, false
	}
	return m.gate.Available()
}

func (m *hostMemory) clock() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now()
}
