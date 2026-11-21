package filler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// PlaybackHeadroom answers whether live playback needs the machine right now (#1512 G5).
//
// Filler media work is background batch work in the same container as the app and the live
// streams. A transcode that starts while a channel is playing competes with that stream for CPU,
// so the pipeline asks before starting one and yields. The playout ResourceBudget implements it,
// busy while live playout is near its measured capacity.
type PlaybackHeadroom interface {
	// PlaybackBusy reports whether media work should wait, with a short operator-facing reason
	// for the log line.
	PlaybackBusy() (busy bool, reason string)
	// MediaWorkContext derives the context a preemptible rung runs under. It ends when a live
	// transcode is admitted and leaves playout tight, so a viewer's tune never waits on filler work
	// (#2121). Overdue work starts even while playout is tight, and still yields to that admission.
	MediaWorkContext(ctx context.Context, overdue bool) (context.Context, context.CancelFunc)
}

// PlaybackYield bounds how long a media rung waits for playback to clear.
type PlaybackYield struct {
	// Poll is the wait between headroom questions.
	Poll time.Duration
	// MaxWait is the most one rung waits. Past it the clip is left queued for a later pass and the
	// rest of THIS pass is told playback is busy, so a long broadcast day defers work rather than
	// stalling the scheduler's single media worker.
	MaxWait time.Duration
	// Overdue is how long a clip may wait at one media rung before it stops waiting for playback:
	// it then starts even beside a busy playout, and still yields to a tune that leaves no room.
	// Without a bound a household that always watches starves filler indefinitely.
	Overdue time.Duration
	// Sleep is the wait itself; nil uses a context-aware timer. A seam for tests.
	Sleep func(context.Context, time.Duration) error
}

// DefaultPlaybackYield polls every 5 s for up to 2 minutes, and stops waiting after a day.
func DefaultPlaybackYield() PlaybackYield {
	return PlaybackYield{Poll: 5 * time.Second, MaxWait: 2 * time.Minute, Overdue: 24 * time.Hour}
}

// overdue reports whether a clip queued at its rung since `queued` has waited past the bound.
func (y PlaybackYield) overdue(queued, now time.Time) bool {
	return y.Overdue > 0 && !queued.IsZero() && now.Sub(queued) >= y.Overdue
}

func (y PlaybackYield) sleep(ctx context.Context, d time.Duration) error {
	if y.Sleep != nil {
		return y.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// WithPlaybackHeadroom makes media-heavy rungs wait while playback is busy. Without it (unit
// tests, installs with no live playout) the pipeline never waits.
func (p *Pipeline) WithPlaybackHeadroom(h PlaybackHeadroom, y PlaybackYield) *Pipeline {
	p.headroom, p.yield = h, y
	return p
}

// yieldsToPlayback reports whether a rung of this cost is media-heavy enough to wait for playback.
func yieldsToPlayback(c StageCost) bool {
	return c == CostTranscode || c == CostWhisper || c == CostSplit
}

// preemptedByPlayback reports whether a rung of this cost is stopped mid-run when playback starts:
// the encodes (#2121). Transcription and splitting only wait before they start: a long recording's
// transcription interrupted at every tune would never finish.
func preemptedByPlayback(c StageCost) bool {
	return c == CostTranscode
}

// runYielding runs a preemptible rung under the playback budget's background context. When that
// context ends while the pass's own has not, playback took the machine: the rung yields as a
// deferral (no attempt spent, due next pass) and the rest of this pass starts no media work.
func (p *Pipeline) runYielding(ctx context.Context, s *spend, stage Stage, clip clipstore.Clip, overdue bool) (StageResult, error) {
	if p.headroom == nil || !preemptedByPlayback(stage.Cost()) {
		return p.runStage(ctx, stage, clip)
	}
	bctx, cancel := p.headroom.MediaWorkContext(ctx, overdue)
	defer cancel()
	out, err := p.runStage(bctx, stage, clip)
	if err != nil && bctx.Err() != nil && ctx.Err() == nil {
		s.playbackBusy = true
		p.logger().Info("filler pipeline: playback started — the stage yields and resumes on a later pass",
			"stage", string(stage.ID()), "clip", clip.Hash)
		return out, fmt.Errorf("%w: yielded to playback: %w", ErrDeferred, err)
	}
	return out, err
}

// waitForHeadroom blocks (bounded) until playback leaves room for a media rung. It returns false
// when the rung should not start now: playback stayed busy for the whole bound, or the pass ended.
// An overdue rung does not wait.
func (p *Pipeline) waitForHeadroom(ctx context.Context, s *spend, id pipelinestate.StageID, c StageCost, overdue bool) bool {
	if p.headroom == nil || !yieldsToPlayback(c) {
		return true
	}
	busy, reason := p.headroom.PlaybackBusy()
	if !busy {
		return true
	}
	if overdue {
		p.logger().Info("filler pipeline: media work has waited past its bound — starting beside playback",
			"stage", string(id), "reason", reason, "bound", p.yield.Overdue.String())
		return true
	}
	if s.playbackBusy {
		return false
	}
	poll := p.yield.Poll
	if poll <= 0 {
		poll = DefaultPlaybackYield().Poll
	}
	p.logger().Info("filler pipeline: yielding to playback before a media stage",
		"stage", string(id), "reason", reason, "max_wait", p.yield.MaxWait.String())
	for waited := time.Duration(0); waited < p.yield.MaxWait; waited += poll {
		if err := p.yield.sleep(ctx, poll); err != nil {
			return false
		}
		if busy, _ = p.headroom.PlaybackBusy(); !busy {
			return true
		}
	}
	s.playbackBusy = true
	p.logger().Info("filler pipeline: playback stayed busy — deferring media work to a later pass",
		"stage", string(id), "waited", p.yield.MaxWait.String())
	return false
}

func (p *Pipeline) logger() *slog.Logger {
	if p.log != nil {
		return p.log
	}
	return slog.Default()
}
