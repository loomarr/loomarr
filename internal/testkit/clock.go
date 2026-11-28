package testkit

import (
	"sync"
	"time"
)

// Clock is a hand-driven wall clock for code that takes a `now func() time.Time`: pass its Now
// method and move time with Advance. It is safe for concurrent use.
//
// It moves only what reads Now. Timers and sleeps in the code under test still run on real time;
// a test about those uses testing/synctest.
type Clock struct {
	mu  sync.Mutex
	now time.Time
}

// NewClock returns a Clock reading start.
func NewClock(start time.Time) *Clock { return &Clock{now: start} }

// Now is the clock's current reading.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock by d.
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// Set moves the clock to at.
func (c *Clock) Set(at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = at
}
