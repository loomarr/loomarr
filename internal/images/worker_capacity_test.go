package images

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"testing"
	"time"
)

func TestWorkerCapacityKeepsOneSlotForInteractiveRenditions(t *testing.T) {
	for _, total := range []int{2, 4} {
		t.Run(fmt.Sprintf("capacity-%d", total), func(t *testing.T) {
			capacity := newWorkerCapacity(total)
			var releases []func()
			for range total - 1 {
				release, err := capacity.acquire(context.Background(), workerBackground)
				if err != nil {
					t.Fatal(err)
				}
				releases = append(releases, release)
			}
			defer func() {
				for _, release := range releases {
					release()
				}
			}()

			backgroundCtx, cancelBackground := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancelBackground()
			if _, err := capacity.acquire(backgroundCtx, workerBackground); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("extra background acquire = %v, want deadline while the reserved slot stays free", err)
			}

			releaseInteractive, err := capacity.acquire(context.Background(), workerInteractive)
			if err != nil {
				t.Fatalf("interactive acquire behind saturated background work: %v", err)
			}
			releaseInteractive()
		})
	}
}

func TestWorkerCapacityPrioritizesInteractiveWaiterOnSingleSlotHost(t *testing.T) {
	capacity := newWorkerCapacity(1)
	releaseRunning, err := capacity.acquire(context.Background(), workerBackground)
	if err != nil {
		t.Fatal(err)
	}
	order := make(chan workerClass, 2)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	go func() {
		release, acquireErr := capacity.acquire(ctx, workerBackground)
		if acquireErr == nil {
			order <- workerBackground
			release()
		}
	}()
	go func() {
		release, acquireErr := capacity.acquire(ctx, workerInteractive)
		if acquireErr == nil {
			order <- workerInteractive
			release()
		}
	}()

	// Synchronization only: wait until the foreground call is inside the module before releasing
	// the running process. The assertion remains the externally visible admission order.
	for {
		capacity.mu.Lock()
		waiting := capacity.interactiveWaiters
		capacity.mu.Unlock()
		if waiting == 1 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("interactive call did not queue")
		default:
			runtime.Gosched()
		}
	}
	releaseRunning()

	if first := <-order; first != workerInteractive {
		t.Fatalf("first admitted waiter = %q, want interactive", first)
	}
	if second := <-order; second != workerBackground {
		t.Fatalf("second admitted waiter = %q, want background", second)
	}
}
