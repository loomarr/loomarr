package app

import (
	"sync"
	"time"
)

// frameThrottle publishes a latest-wins frame at most once per gap. A change inside the gap
// schedules one trailing frame at its end, so the last change is always published; publish reads
// the state when it runs, so that frame carries the latest. An urgent change publishes at once and
// cancels a trailing frame already scheduled, because the urgent frame carries everything it would.
type frameThrottle struct {
	gap     time.Duration
	publish func()
	now     func() time.Time
	// after runs f once d has passed; time.AfterFunc outside tests.
	after func(d time.Duration, f func())

	mu       sync.Mutex
	last     time.Time
	trailing bool
	// round counts urgent frames, so a trailing frame scheduled before one knows it is spent.
	round int
	// sending orders the frames: each takes its snapshot and posts it in turn, so the last frame
	// out carries the latest state even when a trailing frame races an urgent one.
	sending sync.Mutex
}

func newFrameThrottle(gap time.Duration, publish func()) *frameThrottle {
	return &frameThrottle{
		gap: gap, publish: publish, now: time.Now,
		after: func(d time.Duration, f func()) { time.AfterFunc(d, f) },
	}
}

func (t *frameThrottle) send() {
	t.sending.Lock()
	defer t.sending.Unlock()
	t.publish()
}

// poke notes a change. It never blocks on the frame's subscribers: the bus cuts off a slow one.
func (t *frameThrottle) poke(urgent bool) {
	t.mu.Lock()
	now := t.now()
	switch {
	case urgent:
		t.last, t.trailing = now, false
		t.round++
		t.mu.Unlock()
		t.send()
	case !t.trailing && now.Sub(t.last) >= t.gap:
		t.last = now
		t.mu.Unlock()
		t.send()
	case t.trailing:
		// The trailing frame already scheduled carries this change.
		t.mu.Unlock()
	default:
		t.trailing = true
		wait, round := t.gap-now.Sub(t.last), t.round
		t.mu.Unlock()
		t.after(wait, func() { t.fire(round) })
	}
}

// fire publishes the trailing frame scheduled in round, unless an urgent frame has gone out since.
func (t *frameThrottle) fire(round int) {
	t.mu.Lock()
	if !t.trailing || round != t.round {
		t.mu.Unlock()
		return
	}
	t.trailing, t.last = false, t.now()
	t.mu.Unlock()
	t.send()
}
