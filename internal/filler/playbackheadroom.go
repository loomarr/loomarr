package filler

import (
	"context"
	"log/slog"
	"time"
)

// PlaybackHeadroom answers whether live playback needs the machine right now (#1512 G5).
//
// Filler media work is background batch work in the same container as the app and the live
// streams. A transcode that starts while a channel is playing competes with that stream for CPU,
// so the pipeline asks before starting one and yields. The interface is deliberately one method:
// the playout ResourceBudget implements it, busy while a live transcode holds a lease.
type PlaybackHeadroom interface {
	// PlaybackBusy reports whether media work should wait, with a short operator-facing reason
	// for the log line.
	PlaybackBusy() (busy bool, reason string)
}

// PlaybackYield bounds how long a media rung waits for playback to clear.
type PlaybackYield struct {
	// Poll is the wait between headroom questions.
	Poll time.Duration
	// MaxWait is the most one rung waits. Past it the clip is left queued for a later pass and the
	// rest of THIS pass is told playback is busy, so a long broadcast day defers work rather than
	// stalling the scheduler's single media worker.
	MaxWait time.Duration
	// Sleep is the wait itself; nil uses a context-aware timer. A seam for tests.
	Sleep func(context.Context, time.Duration) error
}

// DefaultPlaybackYield polls every 5 s for up to 2 minutes.
func DefaultPlaybackYield() PlaybackYield {
	return PlaybackYield{Poll: 5 * time.Second, MaxWait: 2 * time.Minute}
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

// waitForHeadroom blocks (bounded) until playback leaves room for a media rung. It returns false
// when the rung should not start now: playback stayed busy for the whole bound, or the pass ended.
func (p *Pipeline) waitForHeadroom(ctx context.Context, s *spend, id StageID, c StageCost) bool {
	if p.headroom == nil || !yieldsToPlayback(c) {
		return true
	}
	busy, reason := p.headroom.PlaybackBusy()
	if !busy {
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
