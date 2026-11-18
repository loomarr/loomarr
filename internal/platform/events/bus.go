// Package events is the in-memory event bus behind SSE (§7 /v1/events, §8). It
// fans provisioning + channel + job state changes out to connected subscribers.
// Per §8, SSE progress is a LATENCY optimization, never load-bearing: the store
// (GET endpoints) is always the source of truth on reconnect, which is why a
// subscriber that falls behind is disconnected rather than left with a silent
// gap (see Bus). The bus is deliberately simple and
// single-process; cross-replica delivery is future work (§20) and unnecessary
// because the periodic sweeps make the system correct without it.
package events

import "sync"

// Event is one state change broadcast to subscribers. Type + JSON payload keep
// the bus domain-neutral (the API renders it as an SSE `event:`/`data:` frame).
type Event struct {
	Type    string `json:"type"`    // "title", "channel", "job"
	Payload any    `json:"payload"` // marshaled to JSON by the SSE writer
}

// Bus is a fan-out publisher. Subscribers get a buffered channel, and a slow
// subscriber never blocks the publisher (§8). A subscriber whose buffer is full
// is cut off instead of quietly missing the event: its channel closes, so its
// stream ends and the client reconnects and reads every GET again. A gap a
// client knows about costs one refetch; a gap it never learns of leaves a
// screen stale until the next reload.
type Bus struct {
	mu   sync.RWMutex
	subs map[int]chan Event
	next int
}

// NewBus builds an empty bus.
func NewBus() *Bus {
	return &Bus{subs: map[int]chan Event{}}
}

// Subscribe registers a subscriber and returns its channel plus an unsubscribe
// func. The channel is buffered; when Publish finds it full, it closes it and
// forgets the subscriber.
func (b *Bus) Subscribe() (<-chan Event, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.next
	b.next++
	ch := make(chan Event, 32)
	b.subs[id] = ch
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if c, ok := b.subs[id]; ok {
			delete(b.subs, id)
			close(c)
		}
	}
}

// Publish fans an event to all subscribers without blocking (a stuck client
// must never stall the publisher, §8), and cuts off any whose buffer is full.
func (b *Bus) Publish(ev Event) {
	b.mu.RLock()
	var lagging []int
	for id, ch := range b.subs {
		select {
		case ch <- ev:
		default:
			lagging = append(lagging, id)
		}
	}
	b.mu.RUnlock()
	if len(lagging) == 0 {
		return
	}
	// Sends hold the read lock, so closing under the write lock never races one.
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, id := range lagging {
		if ch, ok := b.subs[id]; ok {
			delete(b.subs, id)
			close(ch)
		}
	}
}
