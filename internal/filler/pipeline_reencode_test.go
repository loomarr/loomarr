package filler_test

import (
	"context"
	"errors"
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// airingList is the catalog's clips on air.
type airingList []clipstore.Clip

func (l airingList) ListClips(context.Context, filler.ClipQuery) ([]clipstore.Clip, error) {
	return l, nil
}

// #2121 decision 10: the library re-encode takes only idle capacity. A pass with no conveyor work
// re-encodes one airing clip; a pass with work, or one that finds playback busy, re-encodes none.
func TestPipelineReencode_TakesOnlyIdlePasses(t *testing.T) {
	pass := func(context.Context) error { return nil }

	t.Run("idle", func(t *testing.T) {
		stage, airing, switches := filler.AiringReencodeFixture(t, pass)
		res, err := newPipe(newPipeMemStore(), nil, filler.DefaultBudget()).
			WithLibraryReencode(stage, airingList{airing}).RunOnce(context.Background())
		if err != nil || res.Reencoded != 1 || switches() != 1 {
			t.Fatalf("idle pass: reencoded %d, switches %d, %v", res.Reencoded, switches(), err)
		}
	})
	t.Run("a new arrival is due", func(t *testing.T) {
		stage, airing, switches := filler.AiringReencodeFixture(t, pass)
		st := newPipeMemStore()
		seedEnrolled(st, "c1")
		res, err := newPipe(st, asSlice(allStages()), filler.DefaultBudget()).
			WithLibraryReencode(stage, airingList{airing}).RunOnce(context.Background())
		if err != nil || res.Reencoded != 0 || switches() != 0 {
			t.Fatalf("busy pass: reencoded %d, switches %d, %v", res.Reencoded, switches(), err)
		}
	})
	t.Run("playback is busy", func(t *testing.T) {
		stage, airing, switches := filler.AiringReencodeFixture(t, pass)
		res, err := newPipe(newPipeMemStore(), nil, filler.DefaultBudget()).
			WithLibraryReencode(stage, airingList{airing}).
			WithPlaybackHeadroom(&scriptedHeadroom{playing: true}, filler.DefaultPlaybackYield()).
			RunOnce(context.Background())
		if err != nil || res.Reencoded != 0 || switches() != 0 {
			t.Fatalf("pass during playback: reencoded %d, switches %d, %v", res.Reencoded, switches(), err)
		}
	})
}

// #2121 decisions 4 and 11: a tune stops the re-encode, and that is a deferral, so the next idle
// pass goes again. A replacement that fails waits out the pipeline's backoff instead.
func TestPipelineReencode_YieldsToPlaybackButBacksOffAFailure(t *testing.T) {
	t.Run("a tune stops it", func(t *testing.T) {
		head := &scriptedHeadroom{}
		tuned := false
		stage, airing, switches := filler.AiringReencodeFixture(t, func(ctx context.Context) error {
			if !tuned {
				tuned = true
				head.startPlayback()
				head.playing = false // the tune ends before the next pass
				return ctx.Err()
			}
			return nil
		})
		pipe := newPipe(newPipeMemStore(), nil, filler.DefaultBudget()).
			WithLibraryReencode(stage, airingList{airing}).
			WithPlaybackHeadroom(head, filler.DefaultPlaybackYield())
		for range 2 {
			if _, err := pipe.RunOnce(context.Background()); err != nil {
				t.Fatal(err)
			}
		}
		if switches() != 1 {
			t.Fatalf("switches = %d, want the yielded re-encode to finish on the next pass", switches())
		}
	})
	t.Run("a failure backs off", func(t *testing.T) {
		checks := 0
		stage, airing, switches := filler.AiringReencodeFixture(t, func(context.Context) error {
			checks++
			return errors.New("decoder rejected the stream")
		})
		pipe := newPipe(newPipeMemStore(), nil, filler.DefaultBudget()).WithLibraryReencode(stage, airingList{airing})
		for range 2 {
			if _, err := pipe.RunOnce(context.Background()); err != nil {
				t.Fatal(err)
			}
		}
		if checks != 1 || switches() != 0 {
			t.Fatalf("checks = %d, switches = %d; want one attempt inside the backoff", checks, switches())
		}
	})
}
