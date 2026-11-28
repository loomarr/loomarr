package testkit

import (
	"testing"
	"time"
)

// eventuallyEvery is how often Eventually asks again: short enough that a passing test pays
// almost nothing, long enough not to spin a loaded CI machine.
const eventuallyEvery = 5 * time.Millisecond

// Eventually waits for cond to hold and fails the test, naming what was awaited, if it does not
// within the bound. It is for a state that another goroutine or process reaches and that offers
// no channel or hook to wait on. Give the bound generous room: a passing test returns as soon as
// cond holds, so the bound is only paid by a failure.
//
// It cannot prove that something does not happen. For that, wait for the event that comes after
// the point where it would have happened, then assert.
func Eventually(t testing.TB, within time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %s waiting for %s", within, what)
			return
		}
		time.Sleep(eventuallyEvery)
	}
}
