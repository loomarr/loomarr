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
	c, release, err := m.acquire(channelID, FormatBaseline, false)
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

// onceRelease makes a lease's release idempotent.
func onceRelease(f func()) func() {
	var once sync.Once
	return func() { once.Do(f) }
}

// packagedTuners is the packager as Origin's tuner seam. Origin's fail-closed stop ends every
// channel but keeps the packager's scratch root, which only the application's shutdown removes.
type packagedTuners struct{ *PackagerHLS }

func (t packagedTuners) Stop() { t.StopAll() }

var _ sessionAttacher = packagedTuners{}
