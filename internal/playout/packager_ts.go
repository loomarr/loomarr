package playout

import (
	"bytes"
	"context"
	"sync"

	"github.com/loomarr/loomarr/internal/playout/packager"
)

// Attach serves a media-server tuner the channel's packaged timeline as MPEG-TS (#1512 phase 2b).
// It reads the same channel packager as browser HLS, so a channel watched in the browser and on a
// TV is one encode; each tuner gets its own TSWriter (its own continuity counters), joins at the
// segment airing now, and is paced by the listing gate like a player.
func (m *PackagerHLS) Attach(ctx context.Context, channelID string, _ EncodePlan) (Stream, func(), error) {
	c, release, err := m.acquire(channelID, FormatBaseline)
	if err != nil {
		return nil, nil, err
	}
	if err := c.p.AwaitPlaylist(ctx); err != nil {
		release()
		return nil, nil, err
	}
	s := &tsStream{p: c.p, seq: c.p.LiveSeq()}
	if s.w, err = packager.NewTSWriter(&s.buf, c.p.Init()); err != nil {
		release()
		return nil, nil, err
	}
	return s, release, nil
}

// tsStream is one tuner's transport stream: the packaged segments from its join point on, muxed
// one segment per Next.
type tsStream struct {
	p   *packager.Packager
	w   *packager.TSWriter
	buf bytes.Buffer
	seq uint32
}

func (s *tsStream) Next(ctx context.Context) ([]byte, error) {
	seg, err := s.p.WaitSegment(ctx, s.seq)
	if err != nil {
		return nil, err // ErrSegmentGone: a tuner that stalled past the DVR window reconnects
	}
	s.seq++
	s.buf.Reset()
	if err := s.w.WriteSegment(seg); err != nil {
		return nil, err
	}
	return bytes.Clone(s.buf.Bytes()), nil
}

// switchedSessions picks the channel packager or the session manager for each new tuner, by the
// same live setting as switchedHLS. Stops go to both, so a tuner keeps its source until it ends.
type switchedSessions struct {
	live        sessionAttacher
	packaged    *PackagerHLS
	usePackager func() bool
}

func (s switchedSessions) Attach(ctx context.Context, channelID string, plan EncodePlan) (Stream, func(), error) {
	if s.live == nil || s.usePackager() {
		return s.packaged.Attach(ctx, channelID, plan)
	}
	return s.live.Attach(ctx, channelID, plan)
}

func (s switchedSessions) StopChannel(channelID string) {
	s.packaged.StopChannel(channelID)
	if s.live != nil {
		s.live.StopChannel(channelID)
	}
}

func (s switchedSessions) Stop() {
	s.packaged.StopAll()
	if s.live != nil {
		s.live.Stop()
	}
}

var _ sessionAttacher = switchedSessions{}

// onceRelease makes a lease's release idempotent.
func onceRelease(f func()) func() {
	var once sync.Once
	return func() { once.Do(f) }
}
