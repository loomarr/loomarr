package bench

import (
	"math"
	"testing"
	"time"
)

func TestThroughputLeavesOutStartUp(t *testing.T) {
	// Measured on an RTX 3080 Ti, 2026-11-08: a 10 s 1080p clip took 983 ms, one frame 455 ms.
	got := throughput(10, 983*time.Millisecond, 455*time.Millisecond)

	if math.Abs(got-18.94) > 0.01 {
		t.Fatalf("throughput = %.2fx, want 18.94x (10 s over 528 ms of encoding)", got)
	}
}

func TestThroughputNeverExceedsTheWholeRunWhenStartUpIsNoise(t *testing.T) {
	got := throughput(10, 900*time.Millisecond, 950*time.Millisecond)

	if math.Abs(got-11.11) > 0.01 {
		t.Fatalf("throughput = %.2fx, want the whole run's 11.11x", got)
	}
}
