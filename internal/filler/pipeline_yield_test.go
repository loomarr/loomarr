package filler_test

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

// scriptedHeadroom reports busy for its first `busyFor` questions, then clear.
type scriptedHeadroom struct {
	busyFor int
	asked   int
}

func (h *scriptedHeadroom) PlaybackBusy() (bool, string) {
	h.asked++
	if h.asked <= h.busyFor {
		return true, "2 channels streaming"
	}
	return false, ""
}

type sleepLog struct{ slept []time.Duration }

func (s *sleepLog) sleep(_ context.Context, d time.Duration) error {
	s.slept = append(s.slept, d)
	return nil
}

func yieldPipe(st *pipeMemStore, stages []filler.Stage, h filler.PlaybackHeadroom, sl *sleepLog) *filler.Pipeline {
	return newPipe(st, stages, filler.DefaultBudget()).WithPlaybackHeadroom(h, filler.PlaybackYield{
		Poll: 5 * time.Second, MaxWait: 20 * time.Second, Sleep: sl.sleep,
	})
}

// #1512 G5: a transcode that starts while a channel is playing competes with the stream for CPU.
// It waits, and runs once playback clears.
func TestPipelineYield_TranscodeWaitsForPlaybackToClearThenRuns(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "c1")
	stages := yieldStages()
	head, sl := &scriptedHeadroom{busyFor: 2}, &sleepLog{}

	if _, err := yieldPipe(st, asSlice(stages), head, sl).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := stages[filler.StageTranscode].runs; got != 1 {
		t.Fatalf("transcode ran %d times, want 1 after playback cleared", got)
	}
	if len(sl.slept) != 2 {
		t.Fatalf("waited %d polls, want 2 (busy twice)", len(sl.slept))
	}
}

// The wait is BOUNDED. Under continuous playback the clip is left queued for a later pass — it is
// not failed, and it does not hold the pass hostage.
func TestPipelineYield_ContinuousPlaybackLeavesTheClipQueuedAfterABoundedWait(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "c1")
	stages := yieldStages()
	sl := &sleepLog{}

	if _, err := yieldPipe(st, asSlice(stages), &scriptedHeadroom{busyFor: 1 << 30}, sl).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := stages[filler.StageTranscode].runs; got != 0 {
		t.Fatalf("transcode ran %d times while playback never cleared", got)
	}
	var total time.Duration
	for _, d := range sl.slept {
		total += d
	}
	if total > 20*time.Second {
		t.Fatalf("waited %v, the bound is 20s", total)
	}
	row := st.rows["c1"]
	if row.Stage != filler.StageTranscode || row.Status != filler.StatusQueued || row.Attempts != 0 {
		t.Fatalf("clip = %q/%q attempts=%d, want queued at transcode with no attempt spent", row.Stage, row.Status, row.Attempts)
	}
}

// One timed-out wait tells the rest of the pass playback is busy: N queued clips must not each
// wait out the bound.
func TestPipelineYield_OneTimeoutSpeaksForTheWholePass(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "c1")
	seedEnrolled(st, "c2")
	sl := &sleepLog{}

	if _, err := yieldPipe(st, asSlice(yieldStages()), &scriptedHeadroom{busyFor: 1 << 30}, sl).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sl.slept) != 4 { // 20s bound / 5s poll, once
		t.Fatalf("slept %d polls across two clips, want 4 (one bounded wait)", len(sl.slept))
	}
}

func TestPipelineYield_IdleBoxNeverWaits(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "c1")
	stages := yieldStages()
	sl := &sleepLog{}

	if _, err := yieldPipe(st, asSlice(stages), &scriptedHeadroom{}, sl).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sl.slept) != 0 || stages[filler.StageTranscode].runs != 1 {
		t.Fatalf("idle box: slept %v, transcode runs %d", sl.slept, stages[filler.StageTranscode].runs)
	}
}

// Only media-heavy rungs yield. A metadata read costs nothing worth protecting playback from.
func TestPipelineYield_CheapRungsAreNeverGated(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "c1")
	stages := yieldStages()
	sl := &sleepLog{}
	head := &scriptedHeadroom{busyFor: 1 << 30}

	if _, err := yieldPipe(st, asSlice(stages), head, sl).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, id := range filler.StageOrder {
		if id == filler.StageProbe && stages[id].runs != 1 {
			t.Errorf("cheap rung %s ran %d times under continuous playback, want 1", id, stages[id].runs)
		}
	}
}

// yieldStages is allStages with the transcode rung declaring the cost the real one does.
func yieldStages() map[filler.StageID]*fakeStage {
	stages := allStages()
	stages[filler.StageTranscode].cost = filler.CostTranscode
	return stages
}
