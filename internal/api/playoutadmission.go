package api

import "sync"

// Hardware-encode admission (§9.1 V47) — a proactive bound on how many programs transcode on the GPU
// AT ONCE, so a fifth channel does not pile onto a saturated encoder and stall.
//
// ⚠ **Proactive, not reactive.** The earlier model let every over-capacity channel try hardware,
// produce zero bytes (VRAM/encoder saturated), evict the LLM, and retry before finally falling to
// software — a failed encode + an eviction round-trip of latency per channel, and specific to an
// NVIDIA+Ollama box. This gate instead chooses the right path UP FRONT: a transcode acquires a slot
// and runs on hardware, or (no slot) runs on software immediately. The reactive evict-and-retry
// ladder stays in streamProgram as a SAFETY NET for the rarer case where a slot-holder's hardware
// encode still fails (a driver hiccup, not saturation).
//
// Hardware-agnostic: it is a plain slot count, sized from the capability probe's measured throughput
// (HWEncodeSlots). It works the same on NVIDIA/Intel/AMD, and a software-only box reports 0 slots —
// so nothing is ever admitted to hardware there, which is exactly right.
//
// The slot count is read LAZILY on first use, because the capability probe runs lazily on the first
// program (a ~20s trial-encode sweep we do not pay at boot). slots() memoises the first answer.
type hwEncodeGate struct {
	// capacity reports the number of hardware encode slots. Injected so the gate does not depend on
	// the resolver directly and is trivially testable. Called once, lazily.
	capacity func() int

	once   sync.Once
	tokens chan struct{} // buffered to the slot count; a token is one in-flight hardware encode
}

// newHWEncodeGate builds a gate whose slot count comes from capacity() on first use. A nil capacity
// (an install without the probe wired) means "unknown" → the gate admits hardware unbounded, i.e. it
// is a no-op, preserving the pre-gate behaviour rather than starving hardware.
func newHWEncodeGate(capacity func() int) *hwEncodeGate {
	return &hwEncodeGate{capacity: capacity}
}

func (g *hwEncodeGate) init() {
	g.once.Do(func() {
		n := -1 // sentinel: unbounded (no probe)
		if g.capacity != nil {
			n = g.capacity()
		}
		if n < 0 {
			g.tokens = nil // unbounded — tryAcquire always succeeds
			return
		}
		g.tokens = make(chan struct{}, n)
	})
}

// tryAcquire takes a hardware slot without blocking. Returns (release, true) when a slot was free —
// the caller MUST call release exactly once when its encode ends. Returns (nil, false) when every
// slot is in use (the caller should use software) or when there are zero slots (software-only box).
//
// Non-blocking on purpose: the product decision is "software now" over "wait for a slot", so a busy
// GPU never adds a black delay to a new channel.
func (g *hwEncodeGate) tryAcquire() (release func(), ok bool) {
	g.init()
	if g.tokens == nil {
		return func() {}, true // unbounded (unknown capacity) — admit, nothing to release
	}
	select {
	case g.tokens <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-g.tokens }) }, true
	default:
		return nil, false // saturated (or zero slots) — caller falls back to software
	}
}
