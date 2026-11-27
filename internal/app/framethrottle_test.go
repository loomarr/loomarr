package app

import (
	"testing"
	"time"
)

// fakeThrottle is a frameThrottle on a settable clock whose trailing frame fires when the test
// says.
func fakeThrottle(gap time.Duration) (*frameThrottle, *int, *time.Time, *[]func()) {
	published := 0
	at := time.Unix(1_900_000_000, 0)
	var timers []func()
	t := newFrameThrottle(gap, func() { published++ })
	t.now = func() time.Time { return at }
	t.after = func(_ time.Duration, f func()) { timers = append(timers, f) }
	return t, &published, &at, &timers
}

// A burst of changes publishes one frame at once and one trailing frame at the end of the gap,
// which carries the last change.
func TestFrameThrottleBurstGivesOneFrameNowAndOneTrailing(t *testing.T) {
	th, published, at, timers := fakeThrottle(2 * time.Second)
	for range 50 {
		th.poke(false)
		*at = at.Add(10 * time.Millisecond)
	}
	if *published != 1 || len(*timers) != 1 {
		t.Fatalf("published %d with %d trailing frames scheduled, want 1 and 1", *published, len(*timers))
	}
	*at = at.Add(2 * time.Second)
	(*timers)[0]()
	if *published != 2 {
		t.Fatalf("published %d after the trailing frame, want 2", *published)
	}
	// Quiet since: the next change goes out at once.
	*at = at.Add(3 * time.Second)
	th.poke(false)
	if *published != 3 || len(*timers) != 1 {
		t.Fatalf("published %d, %d timers; want the next change at once", *published, len(*timers))
	}
}

// An urgent change, a step beginning or ending or a pause, is never held back.
func TestFrameThrottleUrgentPublishesAtOnce(t *testing.T) {
	th, published, _, _ := fakeThrottle(time.Second)
	th.poke(false)
	th.poke(true)
	th.poke(true)
	if *published != 3 {
		t.Fatalf("published %d, want every urgent change at once", *published)
	}
}

// An urgent frame carries the change a trailing frame was waiting to send, so the trailing frame
// is spent: the last state goes out once, not twice.
func TestFrameThrottleUrgentSpendsTheTrailingFrame(t *testing.T) {
	th, published, at, timers := fakeThrottle(time.Second)
	th.poke(false)
	*at = at.Add(100 * time.Millisecond)
	th.poke(false)
	if *published != 1 || len(*timers) != 1 {
		t.Fatalf("published %d with %d trailing frames scheduled, want 1 and 1", *published, len(*timers))
	}
	th.poke(true)
	*at = at.Add(time.Second)
	(*timers)[0]()
	if *published != 2 {
		t.Fatalf("published %d, want the first frame and the urgent one only", *published)
	}
	// The spent trailing frame left the throttle free: a change after the gap goes out at once.
	*at = at.Add(time.Second)
	th.poke(false)
	if *published != 3 {
		t.Fatalf("published %d, want the next change at once", *published)
	}
}
