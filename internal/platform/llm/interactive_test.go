package llm

import "testing"

func TestInteractiveGateCountsOverlappingWork(t *testing.T) {
	var g InteractiveGate
	if g.Busy() {
		t.Fatal("a new gate is busy")
	}
	a, b := g.Begin(), g.Begin()
	a()
	a() // idempotent: a second end must not release b's hold
	if !g.Busy() {
		t.Fatal("the gate went idle with work still in flight")
	}
	b()
	if g.Busy() {
		t.Fatal("the gate stayed busy after all work ended")
	}
	var none *InteractiveGate
	none.Begin()()
	if none.Busy() {
		t.Fatal("a nil gate is busy")
	}
}
