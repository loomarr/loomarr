package filler

import (
	"context"
	"errors"
	"time"

	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
)

// The library re-encode sweep (#2121, decisions 8 and 10). A deliberate exception to the rule
// against backfill: once, automatically, every airing clip encoded under an older profile is
// re-encoded onto the current one (TranscodeStage.ReplaceAiring). It takes only idle capacity:
// one clip, on a pass that found no due conveyor work, so a new arrival always goes first, and
// under the playback budget's background context, so a tune stops it.

// AiringClips lists the clips on air: unheld, not removed, not composite.
type AiringClips interface {
	ListClips(ctx context.Context, f ClipQuery) ([]cliplibrary.StoreClip, error)
}

// WithLibraryReencode lets an idle pass re-encode one airing clip onto the current profile.
func (p *Pipeline) WithLibraryReencode(stage *TranscodeStage, airing AiringClips) *Pipeline {
	p.reencode = &libraryReencode{stage: stage, airing: airing,
		attempts: map[string]int{}, retryAt: map[string]time.Time{}}
	return p
}

// libraryReencode is the sweep's state. A failed replacement is retried on the pipeline's own
// backoff, and after MaxAttempts not again until a restart: one that keeps failing must not take
// every idle pass. The state is per process because the clip it guards keeps airing either way.
type libraryReencode struct {
	stage    *TranscodeStage
	airing   AiringClips
	attempts map[string]int
	retryAt  map[string]time.Time
}

// next is the first airing clip due a re-encode, or false when none is.
func (r *libraryReencode) next(ctx context.Context, now time.Time) (cliplibrary.StoreClip, bool, error) {
	clips, err := r.airing.ListClips(ctx, ClipQuery{})
	if err != nil {
		return cliplibrary.StoreClip{}, false, err
	}
	for _, c := range clips {
		if r.attempts[c.Hash] >= MaxAttempts || now.Before(r.retryAt[c.Hash]) {
			continue
		}
		if r.stage.NeedsReencode(c) {
			return c, true, nil
		}
	}
	return cliplibrary.StoreClip{}, false, nil
}

// reencodeIdle re-encodes one airing clip when the pass found no conveyor work and playback
// leaves room. It reports whether a clip switched to its new bytes.
func (p *Pipeline) reencodeIdle(ctx context.Context, s *spend) bool {
	r := p.reencode
	if r == nil || s.playbackBusy {
		return false
	}
	if p.headroom != nil {
		if busy, _ := p.headroom.PlaybackBusy(); busy {
			return false
		}
	}
	c, ok, err := r.next(ctx, p.now())
	if err != nil {
		p.failures.Warn(p.log, "library-reencode", err.Error(), "filler pipeline: the library re-encode could not list airing clips", "err", err)
		return false
	}
	if !ok {
		return false
	}
	runCtx := ctx
	if p.headroom != nil {
		bctx, cancel := p.headroom.BackgroundContext(ctx)
		defer cancel()
		runCtx = bctx
	}
	replaced, err := r.stage.ReplaceAiring(runCtx, c)
	switch {
	case err == nil:
		delete(r.attempts, c.Hash)
		delete(r.retryAt, c.Hash)
		p.logger().Info("filler pipeline: re-encoded an airing clip onto the current profile",
			"clip", c.Hash, "new_clip", replaced.Hash)
		return true
	case ctx.Err() != nil:
		return false // the pass ended, not the clip
	case runCtx.Err() != nil:
		s.playbackBusy = true
		p.logger().Info("filler pipeline: playback started — the library re-encode yields and resumes on a later pass",
			"clip", c.Hash)
		return false
	}
	r.attempts[c.Hash]++
	r.retryAt[c.Hash] = p.now().Add(backoff(r.attempts[c.Hash]))
	msg := "filler pipeline: an airing clip's re-encode failed; it airs on as it was"
	if errors.Is(err, ErrReplacementRefused) {
		msg = "filler pipeline: an airing clip's re-encode did not pass; it airs on as it was"
	}
	p.failures.Warn(p.log, c.Hash, err.Error(), msg, "clip", c.Hash, "attempt", r.attempts[c.Hash], "err", err)
	return false
}
