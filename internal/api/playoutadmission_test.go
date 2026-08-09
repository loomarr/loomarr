package api

import (
	"sync"
	"testing"
)

// The gate is the proactive HW-encode bound (§9.1 V47): N slots, non-blocking acquire, software when
// full. These pin the contract streamProgram relies on — exhaust, release-frees, zero-denies,
// unbounded-admits — plus race-safety, since acquire/release run from many concurrent requests.

func TestHWEncodeGate_BoundsThenDenies(t *testing.T) {
	g := newHWEncodeGate(func() int { return 2 })

	r1, ok1 := g.tryAcquire()
	r2, ok2 := g.tryAcquire()
	if !ok1 || !ok2 {
		t.Fatalf("first two acquires should succeed, got %v %v", ok1, ok2)
	}
	if _, ok3 := g.tryAcquire(); ok3 {
		t.Fatal("third acquire should be denied — only 2 slots")
	}

	// Releasing one frees exactly one slot.
	r1()
	r3, ok3 := g.tryAcquire()
	if !ok3 {
		t.Fatal("after releasing a slot, an acquire should succeed")
	}
	if _, ok4 := g.tryAcquire(); ok4 {
		t.Fatal("still bounded to 2 — a fourth should be denied")
	}
	r2()
	r3()
}

func TestHWEncodeGate_ReleaseIsIdempotent(t *testing.T) {
	g := newHWEncodeGate(func() int { return 1 })
	r, ok := g.tryAcquire()
	if !ok {
		t.Fatal("first acquire should succeed")
	}
	r()
	r() // double release must not free a phantom second slot

	r2, ok2 := g.tryAcquire()
	if !ok2 {
		t.Fatal("one slot should be available after release")
	}
	if _, ok3 := g.tryAcquire(); ok3 {
		t.Fatal("double-release must NOT have created a second slot")
	}
	r2()
}

func TestHWEncodeGate_ZeroSlotsAlwaysSoftware(t *testing.T) {
	g := newHWEncodeGate(func() int { return 0 }) // software-only box
	if _, ok := g.tryAcquire(); ok {
		t.Fatal("a zero-slot gate must never admit hardware")
	}
}

func TestHWEncodeGate_NilCapacityIsUnbounded(t *testing.T) {
	g := newHWEncodeGate(nil) // no probe wired ⇒ pre-gate behaviour (admit unbounded)
	for i := 0; i < 100; i++ {
		if _, ok := g.tryAcquire(); !ok {
			t.Fatalf("unbounded gate denied acquire %d", i)
		}
	}
}

func TestHWEncodeGate_ConcurrentAcquireRelease(t *testing.T) {
	const slots = 4
	g := newHWEncodeGate(func() int { return slots })
	var wg sync.WaitGroup
	var mu sync.Mutex
	maxHeld, held := 0, 0
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if release, ok := g.tryAcquire(); ok {
				mu.Lock()
				held++
				if held > maxHeld {
					maxHeld = held
				}
				mu.Unlock()
				mu.Lock()
				held--
				mu.Unlock()
				release()
			}
		}()
	}
	wg.Wait()
	if maxHeld > slots {
		t.Fatalf("gate let %d hardware encodes run at once, cap is %d", maxHeld, slots)
	}
	// After everything releases, all slots are free again.
	for i := 0; i < slots; i++ {
		if _, ok := g.tryAcquire(); !ok {
			t.Fatalf("slot %d not free after all releases", i)
		}
	}
	if _, ok := g.tryAcquire(); ok {
		t.Fatal("gate exceeded capacity after drain")
	}
}
