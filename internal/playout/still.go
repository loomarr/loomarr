package playout

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/diagnostics"
	"golang.org/x/sync/singleflight"
)

// Channel stills — the frame the switch overlay shows while a channel's video is still starting.
//
// A still is ONE decoded frame and never adds an encoder. Two places supply it, tried in order:
//
//  1. A WARM channel's newest live segment. It is never older than one segment interval, and
//     decoding its first (independent) frame costs milliseconds.
//  2. A COLD channel's source file: one frame at the current airing's offset, decoded on demand
//     from the file the airing plays (direct via library.path_map, else the media server's
//     stream). Nothing is encoded ahead of time (#1512); the frame is cached for the airing.
//
// Decodes are cached, so a request normally only reads the cache: N viewers and N overlays cost one
// decode per channel per segment (warm) or per airing (cold), not one per request. With neither
// source the caller shows the card on the plain background.

// StillExtractor decodes the first video frame of an fMP4/MPEG-TS byte stream into a JPEG.
type StillExtractor func(ctx context.Context, media io.Reader) ([]byte, error)

// StillAiring is what a cold channel's still needs from the schedule: the airing on now.
type StillAiring struct {
	// Key identifies the airing; the decoded frame is cached under it until the next airing.
	Key string
	// Offset is how far into the source "now" is.
	Offset time.Duration
	// At is when the airing began.
	At time.Time
	// Source resolves what ffmpeg reads. It runs only on a cache miss, because resolving a library
	// item can cost a media-server round trip. ok=false means no readable source.
	Source func(context.Context) (StillSource, bool, error)
}

// StillSource is the media a source still is decoded from.
type StillSource struct {
	// Input is the file path (library.path_map), or the media server's stream URL.
	Input string
	// HDR marks a PQ/HLG source, whose frame must be tone-mapped to look right as a JPEG.
	HDR bool
	// Keyframe is the source's last keyframe at or before the airing's offset, from Loomarr's
	// measured keyframe index; Indexed says it is known. The still seeks straight to it. Without an
	// index, ffmpeg's container seek finds the same keyframe where the container has an index.
	Keyframe time.Duration
	Indexed  bool
}

// StillAiringResolver reports the channel's current airing, cheaply and without side effects: it
// runs on every still request. ok=false means nothing with a source airs now (an empty lineup, a
// break).
type StillAiringResolver func(ctx context.Context, channelID string) (StillAiring, bool, error)

// SourceStillExtractor decodes one frame of a source at an offset into a JPEG.
type SourceStillExtractor func(ctx context.Context, source StillSource, offset time.Duration) ([]byte, error)

// Still is one channel frame. At is when the segment it came from begins, so a caller can judge
// freshness without trusting the cache.
type Still struct {
	JPEG []byte
	At   time.Time
}

// stillSegment is what one source can decode a frame from now. key changes whenever a newer frame
// is due. A segment source sets parts, read in order (an fMP4 init segment, then the media segment)
// and concatenated into one stream for the StillExtractor; the source-file source sets decode.
type stillSegment struct {
	key   string
	at    time.Time
	parts []func() (io.ReadCloser, error)
	// decode, when set, produces the JPEG itself instead of streaming parts.
	decode func(context.Context) ([]byte, error)
	// wait makes the decode queue for a slot (bounded by stillSlotWait) instead of missing.
	wait bool
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
	var firstErr error
	for _, source := range o.stillSources {
		seg, ok, err := source.newestStillSegment(ctx, channelID, plan)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if !ok || (seg.decode == nil && o.stillExtractor == nil) {
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

// decodeStill runs at most stillDecodeLimit decodes at once, process-wide. Past the bound a segment
// decode never queues ffmpeg: it serves the channel's previous frame while that is still recent (a
// slightly old picture behind a dimmed card beats none), else reports a miss and the overlay shows
// its card. A source decode (seg.wait) has no older frame to stand in, so it waits for a slot up to
// stillSlotWait: a surf's tune plus two neighbour prefetches is three stills at once.
func (o *Origin) decodeStill(ctx context.Context, channelID string, seg stillSegment) (stillDecode, error) {
	acquired := false
	select {
	case o.stills.slots() <- struct{}{}:
		acquired = true
	default:
	}
	if !acquired && seg.wait {
		timer := time.NewTimer(stillSlotWait)
		select {
		case o.stills.slots() <- struct{}{}:
			acquired = true
		case <-timer.C:
		case <-ctx.Done():
		}
		timer.Stop()
	}
	if !acquired {
		o.stills.mu.Lock()
		prev, ok := o.stills.entries[channelID]
		o.stills.mu.Unlock()
		if ok && o.stillNow().Sub(prev.still.At) <= stillStaleFallback {
			return stillDecode{still: prev.still, ok: true}, nil
		}
		return stillDecode{}, nil
	}
	defer func() { <-o.stills.slots() }()

	jpeg, err := o.extractStill(ctx, seg)
	if err != nil {
		return stillDecode{}, err
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

func (o *Origin) extractStill(ctx context.Context, seg stillSegment) ([]byte, error) {
	if seg.decode != nil {
		jpeg, err := seg.decode(ctx)
		if err != nil {
			return nil, fmt.Errorf("playout: decode source still: %w", err)
		}
		return jpeg, nil
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
			return nil, fmt.Errorf("playout: open still segment: %w", err)
		}
		closers = append(closers, rc)
		readers = append(readers, rc)
	}
	jpeg, err := o.stillExtractor(ctx, io.MultiReader(readers...))
	if err != nil {
		return nil, fmt.Errorf("playout: decode still: %w", err)
	}
	return jpeg, nil
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
// is hit. It is a surf burst's slack, not a segment count: the packager's 1 s segments would make
// "two segments" too tight to cover a burst of two queued source decodes.
const stillStaleFallback = 8 * time.Second

// stillSlotWait bounds how long a source still queues for a decode slot. Two source decodes take
// ~0.1-0.3 s each, so this covers a surf burst; past it the request is a miss.
const stillSlotWait = 2 * time.Second

// stillDecodeTimeout bounds one frame decode. The input is a single 4s segment, so this is
// generous; a decode that exceeds it is a miss, not a stuck request.
const stillDecodeTimeout = 3 * time.Second

// sourceStillTimeout bounds one source-file decode: open, seek and one frame. A remote media server
// stream pays a few HTTP round trips on top of a local file's ~0.1 s.
const sourceStillTimeout = 5 * time.Second

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
		return runStillFFmpeg(ctx, manager, ffmpegPath, args, media, "first_frame")
	}
}

// FFmpegSourceStill builds the cold-channel extractor: seek the source to the airing's offset and
// decode ONE frame on the CPU. Measured on 2026-09-26 (RTX 3080 Ti host): 1080p H.264 ~95 ms and 4K
// HEVC 10-bit ~105 ms end to end. -hwaccel cuda took ~330 ms for the same frame, because initialising
// the GPU context costs more than the one decode it saves. Two choices make that speed possible:
//   - -noaccurate_seek + -skip_frame nokey: the frame is the keyframe at or before "now" (at most one
//     GOP early, invisible behind the overlay's dimmed card), so nothing is decoded forward to the
//     exact offset (650 ms for 4K HEVC without them).
//   - -thread_type slice: frame threading buffers several frames before the first one comes out.
//
// tonemap reports whether this build can tone-map on the CPU (TonemapperFor). An HDR frame is
// tone-mapped after the downscale, so it costs a 960-px frame. Nil or false leaves it untouched.
func FFmpegSourceStill(ffmpegPath string, tonemap func() bool, observers ...*diagnostics.ProcessManager) SourceStillExtractor {
	var manager *diagnostics.ProcessManager
	if len(observers) > 0 {
		manager = observers[0]
	}
	return func(ctx context.Context, source StillSource, offset time.Duration) ([]byte, error) {
		ctx, cancel := context.WithTimeout(ctx, sourceStillTimeout)
		defer cancel()
		filter := fmt.Sprintf("scale='min(%d,iw)':-2", stillWidth)
		if source.HDR && tonemap != nil && tonemap() {
			filter += "," + hdrToSDR(ToneCurveHable)
		}
		args := []string{
			"-hide_banner", "-loglevel", "error",
			"-skip_frame", "nokey", "-thread_type", "slice",
			"-ss", strconv.FormatFloat(max(offset, 0).Seconds(), 'f', 3, 64), "-noaccurate_seek",
			"-i", source.Input,
			"-map", "0:v:0", "-frames:v", "1",
			"-vf", filter,
			"-q:v", "5", "-f", "image2pipe", "-c:v", "mjpeg", "pipe:1",
		}
		return runStillFFmpeg(ctx, manager, ffmpegPath, args, nil, "source_frame")
	}
}

func runStillFFmpeg(ctx context.Context, manager *diagnostics.ProcessManager, ffmpegPath string, args []string, stdin io.Reader, target string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	cmd.Stdin = stdin
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	spec, _ := diagnostics.ProcessSpecFromContext(ctx)
	spec.Purpose, spec.Executable, spec.Args, spec.Target = "channel_still", ffmpegPath, args, target
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

// airingStillSource is the cold-channel still: the airing on now, decoded from its source file. Its
// key is the airing, so the frame is decoded once per airing, however far "now" moves within it.
type airingStillSource struct {
	resolve StillAiringResolver
	extract SourceStillExtractor
}

func (s airingStillSource) newestStillSegment(ctx context.Context, channelID string, _ EncodePlan) (stillSegment, bool, error) {
	airing, ok, err := s.resolve(ctx, channelID)
	if err != nil || !ok || airing.Source == nil {
		return stillSegment{}, false, err
	}
	return stillSegment{
		key: "airing:" + airing.Key,
		at:  airing.At,
		decode: func(ctx context.Context) ([]byte, error) {
			source, ok, err := airing.Source(ctx)
			if err != nil {
				return nil, err
			}
			if !ok || source.Input == "" {
				return nil, errors.New("the airing has no readable source")
			}
			seek := airing.Offset
			if source.Indexed {
				seek = source.Keyframe
			}
			return s.extract(ctx, source, seek)
		},
		wait: true,
	}, true, nil
}
