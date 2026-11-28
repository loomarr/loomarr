package testkit_test

import (
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"github.com/loomarr/loomarr/internal/testkit"
)

// fatalRecorder stands in for the test Eventually would fail.
type fatalRecorder struct {
	testing.TB
	message string
}

func (r *fatalRecorder) Helper() {}

func (r *fatalRecorder) Fatalf(format string, args ...any) { r.message = fmt.Sprintf(format, args...) }

func TestEventuallyReturnsOnceTheConditionHolds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		recorder := &fatalRecorder{TB: t}
		testkit.Eventually(recorder, time.Minute, "a second to pass", func() bool {
			return time.Since(start) >= time.Second
		})
		if recorder.message != "" {
			t.Fatalf("failed: %s", recorder.message)
		}
		if took := time.Since(start); took != time.Second {
			t.Fatalf("returned after %s, want 1s", took)
		}
	})
}

func TestEventuallyFailsNamingWhatNeverHappened(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		recorder := &fatalRecorder{TB: t}
		testkit.Eventually(recorder, time.Second, "the pigs to fly", func() bool { return false })
		if want := "timed out after 1s waiting for the pigs to fly"; recorder.message != want {
			t.Fatalf("message %q, want %q", recorder.message, want)
		}
		if took := time.Since(start); took < time.Second || took > 2*time.Second {
			t.Fatalf("gave up after %s, want about 1s", took)
		}
	})
}

func TestClockMovesOnlyWhenTold(t *testing.T) {
	start := time.Unix(1000, 0)
	clock := testkit.NewClock(start)
	if got := clock.Now(); !got.Equal(start) {
		t.Fatalf("Now() = %s, want %s", got, start)
	}
	clock.Advance(time.Hour)
	if got := clock.Now(); !got.Equal(start.Add(time.Hour)) {
		t.Fatalf("after Advance, Now() = %s", got)
	}
	clock.Set(start)
	if got := clock.Now(); !got.Equal(start) {
		t.Fatalf("after Set, Now() = %s", got)
	}
}
