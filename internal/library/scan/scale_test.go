package scan

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// scaleFiles is the library the scale guard builds: big enough that memory held per file stands
// well clear of the store's and the runtime's own, small enough for every run of the suite.
const scaleFiles = 2000

// scalePasses is how many passes run while the header stage is held. On the household's library
// (33k files, 180 TB) step 2 ran for hours, and the passes made meanwhile each handed the whole
// library over again: 12 GB before the kernel killed it (#2151).
const scalePasses = 3

// The scanner's memory and step 2's count follow the library, not how long its reads take. While
// the header stage waits, passes over the same library each find every file still pending; once
// they are done the stage still counts each file once, and the memory they leave held grows with
// nothing they brought (a budget per file per pass, well under one pass's queue of them).
func TestALongHeaderStageHoldsTheLibraryOnce(t *testing.T) {
	if testing.Short() || raceEnabled {
		// Under -race it takes a minute; CI runs it in the package's plain run (RACE_AND_PLAIN).
		t.Skip("builds a library of thousands of files")
	}
	h := newHarness(t)
	var live atomic.Bool
	live.Store(true)
	h.im.Busy = live.Load
	for i := range scaleFiles {
		if i%2 == 0 {
			h.file(fmt.Sprintf("Movies/Tango %04d (2001)/Tango %04d (2001).mkv", i, i), 10)
		} else {
			h.file(fmt.Sprintf("TV/Uniform %04d/Season 01/Uniform %04d S01E01.mkv", i/20, i), 10)
		}
	}
	ctx := context.Background()
	full := make(chan error, 1)
	go func() { full <- h.im.Run(ctx) }()
	for deadline := time.Now().Add(time.Minute); !h.paused(); time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the header stage never waited for playback")
		}
	}

	before := heldBytes()
	for range scalePasses {
		if err := h.im.Run(ctx); err != nil {
			t.Fatal(err)
		}
	}
	grew := heldBytes() - before
	t.Logf("%d passes left %d KB more held (%d B a file a pass)", scalePasses, grew/1024, grew/(scaleFiles*scalePasses))

	if got := h.im.Tracker.Snapshot(time.Now()); got.Step != StepReading || got.Total != scaleFiles {
		t.Errorf("step %v counts %d files after %d passes, want reading %d", got.Step, got.Total, scalePasses, scaleFiles)
	}
	const budget = 64 // bytes held per file per pass
	if limit := int64(budget * scaleFiles * scalePasses); grew > limit {
		t.Errorf("%d passes left %d KB more held (%d B a file a pass), want under %d KB", scalePasses, grew/1024,
			grew/(scaleFiles*scalePasses), limit/1024)
	}

	live.Store(false)
	if err := <-full; err != nil {
		t.Fatal(err)
	}
	if n := h.reads.Load(); n != scaleFiles {
		t.Errorf("header reads = %d, want %d", n, scaleFiles)
	}
}

// heldBytes is the heap still reachable after a full collection.
func heldBytes() int64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return int64(m.HeapAlloc)
}
