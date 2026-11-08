package llm

import "sync"

// InteractiveGate counts the interactive LLM work in flight: a household member waiting on a
// suggestion (Add a channel, refine, find more). Background callers ask it before each model call
// and wait while it is busy, so interactive work gets the AI server's slots first (#1508). On a
// 2-slot server, one slot's long prefill (a vision call's keyframes) cut the other slot's decode
// from ~24 tok/s to 6-9 tok/s, and an Add a channel request took 52 s instead of ~25 s.
//
// A nil *InteractiveGate is never busy and counts nothing.
type InteractiveGate struct {
	mu sync.Mutex
	n  int
}

// Begin marks one piece of interactive work in flight until the returned end is called. end is
// idempotent.
func (g *InteractiveGate) Begin() (end func()) {
	if g == nil {
		return func() {}
	}
	g.mu.Lock()
	g.n++
	g.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			g.n--
			g.mu.Unlock()
		})
	}
}

// Busy reports whether any interactive work is in flight.
func (g *InteractiveGate) Busy() bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.n > 0
}
