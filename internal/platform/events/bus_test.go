package events

import "testing"

// A subscriber that keeps up gets every event, in order.
func TestBusDeliversToEverySubscriber(t *testing.T) {
	bus := NewBus()
	first, unsubscribeFirst := bus.Subscribe()
	defer unsubscribeFirst()
	second, unsubscribeSecond := bus.Subscribe()
	defer unsubscribeSecond()

	bus.Publish(Event{Type: "title"})
	bus.Publish(Event{Type: "channel"})

	for _, ch := range []<-chan Event{first, second} {
		if a, b := <-ch, <-ch; a.Type != "title" || b.Type != "channel" {
			t.Fatalf("got %q then %q, want title then channel", a.Type, b.Type)
		}
	}
}

// A subscriber that falls behind is cut off rather than left with a gap it can't see: its channel
// closes after the events it buffered, so its stream ends and its client reconnects and reads
// every GET again. The subscribers that keep up are untouched.
func TestBusCutsOffASubscriberThatFallsBehind(t *testing.T) {
	bus := NewBus()
	slow, unsubscribeSlow := bus.Subscribe()
	defer unsubscribeSlow()
	fast, unsubscribeFast := bus.Subscribe()
	defer unsubscribeFast()

	buffered := cap(slow)
	for range buffered + 1 {
		bus.Publish(Event{Type: "title"})
		<-fast
	}

	for range buffered {
		if _, ok := <-slow; !ok {
			t.Fatal("the slow subscriber's buffered events were lost")
		}
	}
	if _, ok := <-slow; ok {
		t.Fatal("the slow subscriber's channel is still open, want it closed")
	}

	bus.Publish(Event{Type: "channel"})
	if ev := <-fast; ev.Type != "channel" {
		t.Fatalf("fast subscriber got %q, want channel", ev.Type)
	}
	// Unsubscribing after the cut-off is harmless.
	unsubscribeSlow()
}
