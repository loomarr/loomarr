// Package events is the in-memory event bus behind SSE (§7 /v1/events, §8). It
// fans provisioning + channel + job state changes out to connected subscribers.
// Per §8, SSE progress is a LATENCY optimization, never load-bearing: a dropped
// event is a latency bug, not a correctness bug — the store (GET endpoints) is
// always the source of truth on reconnect. The bus is deliberately simple and
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

// Bus is a fan-out publisher. Subscribers get a buffered channel; a slow
// subscriber drops events (bounded buffer) rather than blocking the publisher —
// correctness never depends on delivery (§8).
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
// func. The channel is buffered; when full, Publish drops for that subscriber.
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

// Publish fans an event to all subscribers, dropping for any whose buffer is
// full (non-blocking — a stuck client must never stall the publisher, §8).
func (b *Bus) Publish(ev Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subs {
		select {
		case ch <- ev:
		default: // buffer full → drop (latency bug, not correctness bug)
		}
	}
}
