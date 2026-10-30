package filler_test

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

// scriptedHeadroom reports busy for its first `busyFor` questions, then clear. startPlayback is a
// live transcode's admission: it ends every background context handed out, and playback stays
// busy from then on.
type scriptedHeadroom struct {
	busyFor     int
	asked       int
	playing     bool
	backgrounds []context.CancelFunc
}

func (h *scriptedHeadroom) BackgroundContext(ctx context.Context) (context.Context, context.CancelFunc) {
	bctx, cancel := context.WithCancel(ctx)
	h.backgrounds = append(h.backgrounds, cancel)
	return bctx, cancel
}

func (h *scriptedHeadroom) startPlayback() {
	h.playing = true
	for _, cancel := range h.backgrounds {
		cancel()
	}
}

func (h *scriptedHeadroom) PlaybackBusy() (bool, string) {
	h.asked++
	if h.playing || h.asked <= h.busyFor {
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

// #2121: a viewer tuning in mid-encode stops the encode at once. That is a deferral, not a failure:
// the clip stays queued at transcode with no attempt spent, and the rest of the pass starts no
// other encode while playback runs.
func TestPipelineYield_PlaybackStartingMidEncodeDefersIt(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "c1")
	seedEnrolled(st, "c2")
	stages := yieldStages()
	head := &scriptedHeadroom{}
	stages[filler.StageTranscode].killCtx = head.startPlayback

	res, err := yieldPipe(st, asSlice(stages), head, &sleepLog{}).RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := stages[filler.StageTranscode].runs; got != 1 {
		t.Fatalf("transcode ran %d times, want 1: the yield speaks for the rest of the pass", got)
	}
	if res.Failed != 0 || res.Deferred == 0 {
		t.Fatalf("pass = failed %d deferred %d, want the yield counted as a deferral", res.Failed, res.Deferred)
	}
	for _, id := range []string{"c1", "c2"} {
		row := st.rows[id]
		if row.Stage != filler.StageTranscode || row.Status != filler.StatusQueued || row.Attempts != 0 {
			t.Errorf("%s = %q/%q attempts=%d, want queued at transcode with no attempt spent", id, row.Stage, row.Status, row.Attempts)
		}
	}
}

// Only the encodes are stopped mid-run. Transcription waits before it starts but is never cut off
// by a tune, or a long recording's transcription would restart forever.
func TestPipelineYield_TranscriptionIsNotPreempted(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "c1")
	stages := yieldStages()
	head := &scriptedHeadroom{}
	stages[filler.StageTranscribe].cost = filler.CostWhisper
	stages[filler.StageTranscribe].killCtx = head.startPlayback

	if _, err := yieldPipe(st, asSlice(stages), head, &sleepLog{}).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if row := st.rows["c1"]; filler.StageIndex(row.Stage) <= filler.StageIndex(filler.StageTranscribe) {
		t.Fatalf("clip = %q/%q, want it past transcription", row.Stage, row.Status)
	}
}

// yieldStages is allStages with the transcode rung declaring the cost the real one does.
func yieldStages() map[filler.StageID]*fakeStage {
	stages := allStages()
	stages[filler.StageTranscode].cost = filler.CostTranscode
	return stages
}
