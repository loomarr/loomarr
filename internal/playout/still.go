package playout

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/diagnostics"
	"golang.org/x/sync/singleflight"
)

// Channel stills — the frame the switch overlay shows while a channel's video is still starting.
//
// A still is ONE frame decoded from the newest segment Loomarr already holds for the channel, so
// it is never older than one segment interval (hlsSegmentDuration) and never adds an encoder: the
// segment is already H.264/HEVC on disk, and decoding its first (independent) frame costs
// milliseconds. The decode happens at most once per segment — a request only ever reads the cache —
// so N viewers and N overlays cost one tiny decode per channel per segment, not one per request.
//
// Two places hold segments, tried in order (the same order Tune uses): the prepared publication
// covering "now", then the live HLS remux. A channel with neither has no still and the caller
// shows the card on the plain background.

// StillExtractor decodes the first video frame of an fMP4/MPEG-TS byte stream into a JPEG.
type StillExtractor func(ctx context.Context, media io.Reader) ([]byte, error)

// Still is one channel frame. At is when the segment it came from begins, so a caller can judge
// freshness without trusting the cache.
type Still struct {
	JPEG []byte
	At   time.Time
}

// stillSegment is the newest segment a source holds. key changes whenever a newer segment
// appears; parts are read in order (an fMP4 init segment, then the media segment) and are
// concatenated into one decodable stream.
type stillSegment struct {
	key   string
	at    time.Time
	parts []func() (io.ReadCloser, error)
}

type stillSource interface {
	newestStillSegment(ctx context.Context, channelID string, plan EncodePlan) (stillSegment, bool, error)
}

type stillEntry struct {
	key   string
	still Still
}

// stillCache memoises the last decoded frame per channel. One entry per channel bounds it by the
// channel count, and a new segment simply replaces the old frame.
type stillCache struct {
	mu      sync.Mutex
	entries map[string]stillEntry
	flight  singleflight.Group

	slotsOnce sync.Once
	slotsCh   chan struct{}
}

// slots is the decode semaphore, created on first use so a zero stillCache is ready.
func (c *stillCache) slots() chan struct{} {
	c.slotsOnce.Do(func() { c.slotsCh = make(chan struct{}, stillDecodeLimit) })
	return c.slotsCh
}

// Still returns the channel's latest frame. ok=false is a clean miss (no segment, no extractor
// wired, or the frame could not be decoded) — never a reason to fail the caller's page.
func (o *Origin) Still(ctx context.Context, channelID string, plan EncodePlan) (Still, bool, error) {
	if o.stillExtractor == nil {
		return Still{}, false, nil
	}
	var firstErr error
	for _, source := range o.stillSources {
		seg, ok, err := source.newestStillSegment(ctx, channelID, plan)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if !ok {
			continue
		}
		return o.stillFor(ctx, channelID, seg)
	}
	return Still{}, false, firstErr
}

func (o *Origin) stillFor(ctx context.Context, channelID string, seg stillSegment) (Still, bool, error) {
	// Keyed by channel AND segment: the cache never serves a frame from before the newest segment.
	o.stills.mu.Lock()
	entry, cached := o.stills.entries[channelID]
	o.stills.mu.Unlock()
	if cached && entry.key == seg.key {
		return entry.still, true, nil
	}

	// Single-flight per (channel, segment): N requests that miss together share ONE decode. The
	// decode outlives any single caller (WithoutCancel), so the first viewer navigating away does
	// not fail everyone waiting on it; each waiter still leaves as soon as its own ctx ends.
	flight := o.stills.flight.DoChan(channelID+"\x00"+seg.key, func() (any, error) {
		return o.decodeStill(context.WithoutCancel(ctx), channelID, seg)
	})
	select {
	case <-ctx.Done():
		return Still{}, false, ctx.Err()
	case result := <-flight:
		if result.Err != nil {
			return Still{}, false, result.Err
		}
		decoded := result.Val.(stillDecode)
		return decoded.still, decoded.ok, nil
	}
}

type stillDecode struct {
	still Still
	ok    bool
}

// decodeStill runs at most stillDecodeLimit decodes at once, process-wide. Past the bound it never
// queues ffmpeg: it serves the channel's previous frame while that is still recent (a slightly old
// picture behind a dimmed card beats none), else reports a miss and the overlay shows its card.
func (o *Origin) decodeStill(ctx context.Context, channelID string, seg stillSegment) (stillDecode, error) {
	select {
	case o.stills.slots() <- struct{}{}:
		defer func() { <-o.stills.slots() }()
	default:
		o.stills.mu.Lock()
		prev, ok := o.stills.entries[channelID]
		o.stills.mu.Unlock()
		if ok && o.stillNow().Sub(prev.still.At) <= stillStaleFallback {
			return stillDecode{still: prev.still, ok: true}, nil
		}
		return stillDecode{}, nil
	}

	readers := make([]io.Reader, 0, len(seg.parts))
	closers := make([]io.Closer, 0, len(seg.parts))
	defer func() {
		for _, c := range closers {
			_ = c.Close()
		}
	}()
	for _, open := range seg.parts {
		rc, err := open()
		if err != nil {
			return stillDecode{}, fmt.Errorf("playout: open still segment: %w", err)
		}
		closers = append(closers, rc)
		readers = append(readers, rc)
	}
	jpeg, err := o.stillExtractor(ctx, io.MultiReader(readers...))
	if err != nil {
		return stillDecode{}, fmt.Errorf("playout: decode still: %w", err)
	}
	still := Still{JPEG: jpeg, At: seg.at}
	o.stills.mu.Lock()
	if o.stills.entries == nil {
		o.stills.entries = map[string]stillEntry{}
	}
	o.stills.entries[channelID] = stillEntry{key: seg.key, still: still}
	o.stills.mu.Unlock()
	return stillDecode{still: still, ok: true}, nil
}

func (o *Origin) stillNow() time.Time {
	if o.stillClock != nil {
		return o.stillClock()
	}
	return time.Now()
}

// stillDecodeLimit bounds concurrent still decodes across all channels: a burst of channel surfing
// must not become a pile of ffmpeg processes.
const stillDecodeLimit = 2

// stillStaleFallback is how old a previous frame may be and still stand in when the decode bound
// is hit: two segment intervals.
const stillStaleFallback = 2 * hlsSegmentDuration * time.Second

// stillDecodeTimeout bounds one frame decode. The input is a single 4s segment, so this is
// generous; a decode that exceeds it is a miss, not a stuck request.
const stillDecodeTimeout = 3 * time.Second

// stillWidth caps the still's width: it is shown dimmed behind the card, so it never needs to be
// larger than a TV's 1080p surface, and a smaller JPEG is faster to land on a cold connection.
const stillWidth = 960

// FFmpegStill builds the production extractor: decode the first frame, downscale, JPEG. Input and
// output are pipes, so nothing touches disk.
func FFmpegStill(ffmpegPath string, observers ...*diagnostics.ProcessManager) StillExtractor {
	var manager *diagnostics.ProcessManager
	if len(observers) > 0 {
		manager = observers[0]
	}
	return func(ctx context.Context, media io.Reader) ([]byte, error) {
		ctx, cancel := context.WithTimeout(ctx, stillDecodeTimeout)
		defer cancel()
		args := []string{
			"-hide_banner", "-loglevel", "error",
			"-i", "pipe:0",
			"-map", "0:v:0", "-frames:v", "1",
			"-vf", fmt.Sprintf("scale='min(%d,iw)':-2", stillWidth),
			"-q:v", "5", "-f", "image2pipe", "-c:v", "mjpeg", "pipe:1",
		}
		cmd := exec.CommandContext(ctx, ffmpegPath, args...)
		cmd.Stdin = media
		var out, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &stderr
		spec, _ := diagnostics.ProcessSpecFromContext(ctx)
		spec.Purpose, spec.Executable, spec.Args, spec.Target = "channel_still", ffmpegPath, args, "first_frame"
		run := manager.Begin(spec)
		err := cmd.Run()
		if run != nil {
			run.Finish(diagnostics.ProcessResult{Err: err})
		}
		if err != nil {
			return nil, fmt.Errorf("ffmpeg: %w: %s", err, strings.TrimSpace(stderr.String()))
		}
		if out.Len() == 0 {
			return nil, fmt.Errorf("ffmpeg produced no frame")
		}
		return out.Bytes(), nil
	}
}

// newestStillSegment reads the live remux's playlist and returns its last COMPLETED segment (the
// playlist only lists segments ffmpeg has finished), plus the fMP4 init segment when there is one.
func (m *HLSManager) newestStillSegment(_ context.Context, channelID string, plan EncodePlan) (stillSegment, bool, error) {
	m.mu.Lock()
	r := m.remuxes[remuxKey{channel: channelID, plan: plan}]
	if r == nil {
		for key, candidate := range m.remuxes {
			if key.channel == channelID {
				r = candidate
				break
			}
		}
	}
	m.mu.Unlock()
	if r == nil {
		return stillSegment{}, false, nil
	}
	body, err := os.ReadFile(r.playlist)
	if err != nil {
		return stillSegment{}, false, nil // no playlist yet: the remux has not cut a segment
	}
	var init, last string
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "#EXT-X-MAP:URI=\""):
			init = strings.TrimSuffix(strings.TrimPrefix(line, "#EXT-X-MAP:URI=\""), "\"")
		case line != "" && !strings.HasPrefix(line, "#"):
			last = line
		}
	}
	if last == "" || filepath.Base(last) != last {
		return stillSegment{}, false, nil
	}
	info, err := os.Stat(filepath.Join(r.dir, last))
	if err != nil {
		return stillSegment{}, false, nil // pruned between the playlist read and now
	}
	open := func(name string) func() (io.ReadCloser, error) {
		return func() (io.ReadCloser, error) { return os.Open(filepath.Join(r.dir, name)) }
	}
	seg := stillSegment{key: r.dir + "/" + last, at: info.ModTime().Add(-hlsSegmentDuration * time.Second)}
	if init != "" && filepath.Base(init) == init {
		seg.parts = append(seg.parts, open(init))
	}
	seg.parts = append(seg.parts, open(last))
	return seg, true, nil
}

// segmentAtEdge is the media segment holding the airing's live edge — the same walk
// renderPreparedManifest does to place the manifest's newest entry, so the still is the frame a
// viewer tuning in now would be near.
func segmentAtEdge(media preparedMedia) (preparedSegment, bool) {
	offset := max(media.airing.Offset, 0)
	for _, segment := range media.segments {
		if offset < segment.duration {
			return segment, true
		}
		offset -= segment.duration
	}
	return preparedSegment{}, false
}

// newestStillSegment resolves the prepared publication covering now and points at the segment at
// its live edge. A miss (nothing prepared for the channel) lets Origin fall through to live.
func (o *PreparedOrigin) newestStillSegment(ctx context.Context, channelID string, plan EncodePlan) (stillSegment, bool, error) {
	if o == nil || o.library == nil || o.resolver == nil {
		return stillSegment{}, false, nil
	}
	window, ok, err := o.resolver.ResolvePrepared(ctx, TuneRequest{ChannelID: channelID, Plan: plan, Delivery: DeliveryHLS, PreparedOnly: true}, time.Time{})
	if err != nil || !ok {
		return stillSegment{}, false, err
	}
	media, ok, err := o.load(window.Current)
	if err != nil || !ok {
		return stillSegment{}, false, err
	}
	segment, ok := segmentAtEdge(media)
	if !ok {
		return stillSegment{}, false, nil
	}
	open := func(token string) func() (io.ReadCloser, error) {
		return func() (io.ReadCloser, error) {
			asset, found, err := o.OpenAsset("", plan, token)
			if err != nil {
				return nil, err
			}
			if !found {
				return nil, fmt.Errorf("prepared asset %q is gone", token)
			}
			return asset.Content, nil
		}
	}
	return stillSegment{
		key:   media.key + "/" + segment.uri,
		at:    segment.startsAt,
		parts: []func() (io.ReadCloser, error){open(media.init), open(segment.uri)},
	}, true, nil
}
