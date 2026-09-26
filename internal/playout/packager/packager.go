// Package packager is the channel packager (#1512 phase 2): one long-lived, in-process stitcher per
// (channel, output format) that turns a sequence of per-item fMP4 encodes into one gapless channel
// timeline. It forwards each encoder fragment, patching mfhd/tfdt onto its own counters, trims the
// encoder's over-production at each slot end, fills a slot with a pre-encoded slate when the item
// is not producing in time, applies run-ahead back-pressure against the wall clock, and serves a
// live HLS window with a listing gate.
//
// The packager knows nothing about schedules, media servers or ffmpeg arguments: the caller supplies
// a Schedule (what airs at an instant, and how to open its encoder) and a Slate.
package packager

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Item is what airs from an instant: Duration is the time left in that airing, Open starts its
// encoder for the given slot and returns the encoder's fMP4 stream. Open's context is cancelled
// when the packager no longer wants the item (slot filled, deadline missed, shutdown).
type Item struct {
	Label    string
	Duration time.Duration
	Open     func(ctx context.Context, slot Slot) (io.ReadCloser, error)
}

// Slot is one item's place on the channel timeline, fixed before its encoder starts.
type Slot struct {
	// AirAt is the wall-clock instant the slot's first frame airs.
	AirAt time.Time
	// Offset is the slot's channel media time: the encoder's -output_ts_offset.
	Offset time.Duration
	// Frames and AudioFrames are exact; the encoder should over-ask and the packager trims.
	Frames, AudioFrames int64
}

// Schedule resolves what airs at an instant. An error or a non-positive Duration fills
// Config.SlateRetry of slate before the packager asks again.
type Schedule func(ctx context.Context, at time.Time) (Item, error)

type Config struct {
	// FPS is the output frame rate; 90000 must divide by it (24, 25, 30, 50, 60).
	FPS int
	// Dir holds segment files. It must not be tmpfs: ~8 Mbit/s × the DVR window per channel.
	Dir string
	// RunAhead bounds how far the encoded timeline may lead the wall clock (default 12 s).
	RunAhead time.Duration
	// ListAhead is the listing gate and HOLD-BACK: segments are listed only up to now + ListAhead
	// (default 6 s).
	ListAhead time.Duration
	// FirstManifest holds the first playlist until this much media is listable (default 4 s).
	FirstManifest time.Duration
	// DVR is the playlist window (default 15 min).
	DVR time.Duration
	// SlateLead is how long before air time an item must be producing (default 2 s).
	SlateLead time.Duration
	// FirstItemWait is the tune-in item's deadline, which has no run-ahead to spend (default 3 s).
	FirstItemWait time.Duration
	// SlateRetry is how much slate fills a failed schedule lookup (default 10 s).
	SlateRetry time.Duration
	Log        *slog.Logger
	// Now is the clock; nil is time.Now.
	Now func() time.Time
}

func (c *Config) defaults() error {
	if c.FPS <= 0 || videoRate%c.FPS != 0 {
		return fmt.Errorf("packager: unsupported frame rate %d", c.FPS)
	}
	if c.Dir == "" {
		return errors.New("packager: no segment directory")
	}
	def := func(d *time.Duration, v time.Duration) {
		if *d <= 0 {
			*d = v
		}
	}
	def(&c.RunAhead, 12*time.Second)
	def(&c.ListAhead, 6*time.Second)
	def(&c.FirstManifest, 4*time.Second)
	def(&c.DVR, 15*time.Minute)
	def(&c.SlateLead, 2*time.Second)
	def(&c.FirstItemWait, 3*time.Second)
	def(&c.SlateRetry, 10*time.Second)
	if c.Log == nil {
		c.Log = slog.New(slog.DiscardHandler)
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	return nil
}

// Packager is one channel timeline at one output format.
type Packager struct {
	cfg      Config
	schedule Schedule
	slate    *Slate
	frameDur int64

	// Timeline counters, owned by Run: the next video tick (90 kHz) and audio sample (48 kHz).
	v, a int64
	seq  uint32

	mu       sync.Mutex
	epoch    time.Time // wall instant of media time 0
	init     []byte    // the channel init segment: the first producing encoder's ftyp+moov
	window   window
	changed  chan struct{} // closed and replaced on every change; the condition-wait primitive
	done     bool
	err      error
	stats    Stats
	listSkew time.Duration // test seam: none in production
}

// Stats counts what the packager did, for logs and tests.
type Stats struct {
	Items, Slates, DecoderMismatch, ZeroFrame, Late, Trimmed int
}

func New(cfg Config, schedule Schedule, slate *Slate) (*Packager, error) {
	if err := cfg.defaults(); err != nil {
		return nil, err
	}
	if schedule == nil || slate == nil {
		return nil, errors.New("packager: schedule and slate are required")
	}
	return &Packager{
		cfg: cfg, schedule: schedule, slate: slate, frameDur: int64(videoRate / cfg.FPS),
		changed: make(chan struct{}),
	}, nil
}

// Run packages until ctx ends. The timeline's media time 0 airs at the instant Run starts.
func (p *Packager) Run(ctx context.Context) error {
	p.mu.Lock()
	p.epoch = p.cfg.Now()
	p.mu.Unlock()
	err := p.run(ctx)
	if ctx.Err() != nil {
		err = nil
	}
	p.mu.Lock()
	p.done, p.err = true, err
	p.broadcastLocked()
	p.mu.Unlock()
	return err
}

func (p *Packager) run(ctx context.Context) error {
	for first := true; ctx.Err() == nil; first = false {
		airAt := p.airAt(p.v)
		item, err := p.schedule(ctx, airAt)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil || item.Duration <= 0 || item.Open == nil {
			// Nothing playable (an empty lineup, a card slot) slates for that slot's length, and a
			// failed lookup for SlateRetry; either way the schedule is asked again at most that late.
			d := p.cfg.SlateRetry
			if err == nil && item.Duration > 0 {
				d = min(item.Duration, d)
			} else {
				p.cfg.Log.Warn("packager: nothing to air; slate", "at", airAt, "err", err)
			}
			if err := p.fillSlate(ctx, p.slotFor(airAt, d)); err != nil {
				return err
			}
			continue
		}
		slot := p.slotFor(airAt, item.Duration)
		if slot.Frames <= 0 {
			// Less than a frame left in this airing: step one frame so the next lookup moves on.
			slot = p.slotFor(airAt, time.Second/time.Duration(p.cfg.FPS))
			if err := p.fillSlate(ctx, slot); err != nil {
				return err
			}
			continue
		}
		// An item must be producing SlateLead before it airs. When the timeline leads by less than
		// that (a short first airing at tune-in), it may still use the time until it airs; at
		// tune-in, which has no lead at all, it gets FirstItemWait.
		now := p.cfg.Now()
		deadline := airAt.Add(-p.cfg.SlateLead)
		if first {
			deadline = now.Add(p.cfg.FirstItemWait)
		} else if deadline.Before(now) {
			deadline = airAt
		}
		if err := p.airItem(ctx, item, slot, deadline); err != nil {
			return err
		}
	}
	return nil
}

func (p *Packager) airAt(v int64) time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.epoch.Add(ticks(v))
}

func ticks(v int64) time.Duration { return time.Duration(v) * time.Second / videoRate }

// slotFor fixes a slot at the current counters. Audio frames are owed by the video end, rounded to
// the nearest AAC frame against what the timeline already holds, so every boundary stays within
// ±512 samples and audio never gaps.
func (p *Packager) slotFor(airAt time.Time, d time.Duration) Slot {
	n := int64(math.Round(d.Seconds() * float64(p.cfg.FPS)))
	return Slot{AirAt: airAt, Offset: ticks(p.v), Frames: n, AudioFrames: p.audioOwed(p.v + n*p.frameDur)}
}

// audioOwed is the AAC frames still owed when the video timeline reaches vEnd.
func (p *Packager) audioOwed(vEnd int64) int64 {
	return max((vEnd*audioRate/videoRate+aacFrame/2-p.a)/aacFrame, 0)
}

type encoderStream struct {
	init  []byte
	frags chan []byte
	err   error // valid once frags is closed
}

// readStream splits an encoder's fMP4 output into its init segment and moof+mdat fragments.
func readStream(rc io.Reader) *encoderStream {
	s := &encoderStream{frags: make(chan []byte, 2)}
	go func() {
		defer close(s.frags)
		r := bufio.NewReaderSize(rc, 1<<16)
		var moof []byte
		for {
			typ, b, err := readBox(r)
			if err != nil {
				if !errors.Is(err, io.EOF) {
					s.err = err
				}
				return
			}
			switch typ {
			case "ftyp", "moov":
				s.init = append(s.init, b...)
			case "moof":
				moof = b
			case "mdat":
				if moof == nil {
					s.err = fmt.Errorf("%w: mdat without moof", errBadBox)
					return
				}
				s.frags <- append(moof, b...)
				moof = nil
			}
		}
	}()
	return s
}

// airItem runs one item's encoder into its slot, or fills the slot with slate when the encoder is
// not producing by the deadline, ends without a frame, or disagrees with the channel's decoder
// configuration.
func (p *Packager) airItem(ctx context.Context, item Item, slot Slot, deadline time.Time) error {
	ictx, cancel := context.WithCancel(ctx)
	defer cancel()
	rc, err := item.Open(ictx, slot)
	if err != nil {
		p.cfg.Log.Warn("packager: item did not open; slate", "item", item.Label, "err", err)
		return p.fillSlate(ctx, slot)
	}
	defer func() { _ = rc.Close() }()
	stream := readStream(rc)

	timer := time.NewTimer(max(deadline.Sub(p.cfg.Now()), 0))
	defer timer.Stop()
	var frag []byte
	select {
	case <-ctx.Done():
		return nil
	case <-timer.C:
		select {
		case f, ok := <-stream.frags: // a fragment that arrived with the deadline still counts
			if ok {
				frag = f
				break
			}
			p.count(func(s *Stats) { s.ZeroFrame++ })
			return p.fillSlate(ctx, slot)
		default:
		}
		if frag != nil {
			break
		}
		p.count(func(s *Stats) { s.Late++ })
		p.cfg.Log.Warn("packager: item not producing by its deadline; slate", "item", item.Label)
		cancel()
		return p.fillSlate(ctx, slot)
	case f, ok := <-stream.frags:
		if !ok {
			// A zero-frame EOF (a seek at or past the end, an encoder that exits 0 without a
			// frame) is "not ready", never an empty slot.
			p.count(func(s *Stats) { s.ZeroFrame++ })
			p.cfg.Log.Warn("packager: item ended without a frame; slate", "item", item.Label, "err", stream.err)
			_ = rc.Close() // reap the encoder (and surface why it failed) before the slot's slate
			return p.fillSlate(ctx, slot)
		}
		frag = f
	}
	if !p.acceptInit(stream.init) {
		p.count(func(s *Stats) { s.DecoderMismatch++ })
		p.cfg.Log.Error("packager: item's decoder configuration differs from the channel's; slate", "item", item.Label)
		cancel()
		return p.fillSlate(ctx, slot)
	}
	p.count(func(s *Stats) { s.Items++ })

	var nv, na int64
	first := true
	for ; frag != nil; first = false {
		keep := map[uint32]int64{videoTrack: slot.Frames - nv, audioTrack: slot.AudioFrames - na}
		c, err := p.forward(ctx, frag, first, keep)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			p.cfg.Log.Error("packager: bad fragment; ending item", "item", item.Label, "err", err)
			break
		}
		nv += c[videoTrack]
		na += c[audioTrack]
		if nv >= slot.Frames {
			break // slot full: stop the encoder's margin and start the next item now
		}
		f, ok := <-stream.frags
		if !ok {
			break
		}
		frag = f
	}
	cancel()
	if nv < slot.Frames && stream.err != nil {
		p.cfg.Log.Warn("packager: item ended early", "item", item.Label, "frames", nv, "want", slot.Frames, "err", stream.err)
	}
	return nil
}

// acceptInit makes the first producing encoder's init the channel's, and admits a later encoder
// only if its sample descriptions match byte for byte.
func (p *Packager) acceptInit(init []byte) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.init == nil {
		if len(sampleDescriptions(init)) == 0 {
			return false
		}
		p.init = init
		if err := os.WriteFile(filepath.Join(p.cfg.Dir, InitName), init, 0o600); err != nil {
			p.cfg.Log.Error("packager: write init", "err", err)
		}
		return true
	}
	return sameDecoderConfig(p.init, init)
}

// forward stamps one encoder fragment onto the timeline, trimming to keep, and appends it.
func (p *Packager) forward(ctx context.Context, frag []byte, first bool, keep map[uint32]int64) (map[uint32]int64, error) {
	base := map[uint32]uint64{videoTrack: uint64(p.v), audioTrack: uint64(p.a)}
	c, err := patchFragment(frag, p.seq, base)
	if err != nil {
		return nil, err
	}
	primed := int64(0)
	if first {
		primed = 1
	}
	out := frag
	if first || c[videoTrack] > keep[videoTrack] || c[audioTrack]-primed > keep[audioTrack] {
		if !first {
			p.count(func(s *Stats) { s.Trimmed++ })
		}
		if out, c, err = rewriteFragment(frag, p.seq, base, first, uint32(p.frameDur), keep); err != nil {
			return nil, err
		}
	}
	return c, p.appendSegment(ctx, out, c)
}

// appendSegment writes a stamped fragment as the next segment, advances the counters, publishes it
// and then holds the caller while the timeline leads the wall clock by more than RunAhead.
func (p *Packager) appendSegment(ctx context.Context, b []byte, c map[uint32]int64) error {
	if c[videoTrack] <= 0 {
		return nil // an all-trimmed fragment carries nothing to list
	}
	name := segmentName(p.seq)
	if err := os.WriteFile(filepath.Join(p.cfg.Dir, name), b, 0o600); err != nil {
		return fmt.Errorf("packager: write segment: %w", err)
	}
	seg := segment{seq: p.seq, name: name, start: p.v, dur: c[videoTrack] * p.frameDur}
	p.seq++
	p.v += seg.dur
	p.a += c[audioTrack] * aacFrame

	p.mu.Lock()
	for _, gone := range p.window.push(seg, p.v-int64(p.cfg.DVR.Seconds()*videoRate)) {
		_ = os.Remove(filepath.Join(p.cfg.Dir, gone.name))
	}
	p.broadcastLocked()
	p.mu.Unlock()

	lead := p.airAt(p.v).Sub(p.cfg.Now()) - p.cfg.RunAhead
	if lead <= 0 {
		return nil
	}
	t := time.NewTimer(lead)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
	return nil
}

func (p *Packager) count(f func(*Stats)) {
	p.mu.Lock()
	f(&p.stats)
	p.mu.Unlock()
}

func (p *Packager) broadcastLocked() {
	close(p.changed)
	p.changed = make(chan struct{})
}

// Stats returns a snapshot of the counters.
func (p *Packager) Stats() Stats {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stats
}

// ErrStopped means the packager ended before its first playlist was ready.
var ErrStopped = errors.New("packager: stopped")

// AwaitPlaylist waits until the first manifest condition holds: at least FirstManifest of media is
// listable under the listing gate. It is a condition wait on the packager, not a file poll.
func (p *Packager) AwaitPlaylist(ctx context.Context) error {
	for {
		p.mu.Lock()
		now := p.cfg.Now()
		ready := p.init != nil && p.window.listableTicks(p.listEdgeLocked(now)) >= int64(p.cfg.FirstManifest.Seconds()*videoRate)
		done, err, changed := p.done, p.err, p.changed
		p.mu.Unlock()
		switch {
		case ready:
			return nil
		case done && err != nil:
			return err
		case done:
			return ErrStopped
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}

// listEdgeLocked is the listing gate in media ticks: the channel time of now + ListAhead.
func (p *Packager) listEdgeLocked(now time.Time) int64 {
	return int64((now.Sub(p.epoch) + p.cfg.ListAhead + p.listSkew).Seconds() * videoRate)
}

// Playlist renders the live media playlist as of now.
func (p *Packager) Playlist() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.window.render(playlistView{
		edge: p.listEdgeLocked(p.cfg.Now()), epoch: p.epoch, holdBack: p.cfg.ListAhead,
	})
}

// Init returns the channel init segment, or nil before the first item produced.
func (p *Packager) Init() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.init
}
