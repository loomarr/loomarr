package app

import (
	"context"
	"testing"

	"github.com/mantonx/loomarr/internal/events"
	"github.com/mantonx/loomarr/internal/provision"
)

// TestEventEmitterPublishesToBus is the #11 seam: the composition-root emitter
// must turn a provisioning domain event into a `title` SSE frame on the bus, so
// /v1/events actually delivers state changes (before this wire, nothing ever
// called Publish and every SSE client waited forever). It also asserts the
// nil-engine path is safe (#10): an event emitted before the scheduler is wired
// still reaches the bus and never panics.
func TestEventEmitterPublishesToBus(t *testing.T) {
	bus := events.NewBus()
	sub, unsubscribe := bus.Subscribe()
	defer unsubscribe()

	// engine deliberately left nil (unwired scheduler / pre-setEngine window).
	emit := &eventEmitter{bus: bus}

	ev := provision.DomainEvent{
		Key:   provision.Key("movie:tmdb:603"),
		State: provision.Available,
		Title: provision.Title{MediaType: provision.Movie, TMDBID: 603, Name: "The Matrix"},
	}
	emit.Emit(context.Background(), ev)

	select {
	case got := <-sub:
		if got.Type != "title" {
			t.Errorf("event type = %q, want title", got.Type)
		}
		payload, ok := got.Payload.(map[string]string)
		if !ok {
			t.Fatalf("payload type = %T, want map[string]string", got.Payload)
		}
		if payload["key"] != "movie:tmdb:603" || payload["state"] != "available" || payload["name"] != "The Matrix" {
			t.Errorf("payload = %v, want key/state/name for The Matrix available", payload)
		}
	default:
		t.Fatal("no event published to the bus — #11 seam still open (nothing reached the subscriber)")
	}
}
