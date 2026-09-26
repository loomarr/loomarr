package playout

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/playout/packager"
)

// PackagerItem is what airs on a channel from an instant, resolved by the application: the source,
// how far into it, how long the airing has left, and the stream facts the builder needs. An empty
// Input is a card slot (nothing playable): the packager slates it.
type PackagerItem struct {
	Label      string
	Input      string
	Seek       time.Duration
	Remaining  time.Duration
	AudioTrack int
	Format     MediaFormat
	// GainDB is a filler clip's static loudness gain (FillerGain); 0 for a library title.
	GainDB float64
}

// PackagerSource is the application's side of the channel packager (#1512 phase 2): the schedule,
// and the host and output a (channel, plan) encodes with.
type PackagerSource interface {
	ItemAt(ctx context.Context, channelID string, plan EncodePlan, at time.Time) (PackagerItem, error)
	Output(ctx context.Context, channelID string, plan EncodePlan) (HostProfile, OutputProfile)
}

// PackagerHLS serves in-app HLS from one channel packager per (channel, plan): one encoder per
// scheduled item at a time, stitched in-process, no continuous transcode and no remux. It is an
// hlsOrigin, so Origin serves it exactly as it serves the remux.
type PackagerHLS struct {
	ffmpeg, root string
	unlock       func() // drops root's owner lock
	grace        time.Duration
	source       PackagerSource
	log          *slog.Logger

	slateMu sync.Mutex
	slates  map[string]*packager.Slate // keyed by the encode (host, output)

	mu       sync.Mutex
	channels map[remuxKey]*packagedChannel
}

type packagedChannel struct {
	p       *packager.Packager
	dir     string
	cancel  context.CancelFunc
	done    chan struct{}
	viewers int
	idle    *time.Timer
}

func NewPackagerHLS(source PackagerSource, ffmpeg, root string, grace time.Duration, log *slog.Logger) (*PackagerHLS, error) {
	// The same scratch rule as the remux (playout.hls_dir): a private, locked per-process root,
	// removed by Stop (scratch.go).
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	root, unlock, err := newScratchRoot(root, "loomarr-packager-", log)
	if err != nil {
		return nil, fmt.Errorf("packager hls: scratch root: %w", err)
	}
	return &PackagerHLS{ffmpeg: ffmpeg, root: root, unlock: unlock, grace: grace, source: source, log: log,
		slates: map[string]*packager.Slate{}, channels: map[remuxKey]*packagedChannel{}}, nil
}

func (m *PackagerHLS) acquirePlaylist(channelID string, plan EncodePlan, _ bool) (hlsPlaylistLease, error) {
	key := remuxKey{channel: channelID, plan: plan}
	m.mu.Lock()
	c := m.channels[key]
	if c != nil {
		select {
		case <-c.done: // the packager stopped (failed): start afresh
			m.removeLocked(key, c)
			c = nil
		default:
		}
	}
	m.mu.Unlock()
	if c == nil {
		var err error
		if c, err = m.start(key); err != nil {
			return hlsPlaylistLease{}, err
		}
	}
	m.mu.Lock()
	if cur := m.channels[key]; cur != c { // lost a start race: use the winner
		if cur != nil {
			c.cancel()
			c = cur
		} else {
			m.channels[key] = c
		}
	}
	c.viewers++
	if c.idle != nil {
		c.idle.Stop()
		c.idle = nil
	}
	m.mu.Unlock()
	var once sync.Once
	return hlsPlaylistLease{
		path:     filepath.Join(c.dir, "live.m3u8"),
		release:  func() { once.Do(func() { m.release(key, c) }) },
		await:    c.p.AwaitPlaylist,
		snapshot: func(context.Context) ([]byte, error) { return c.p.Playlist(), nil },
	}, nil
}

func (m *PackagerHLS) start(key remuxKey) (*packagedChannel, error) {
	ctx, cancel := context.WithCancel(context.Background())
	t0 := time.Now()
	host, out := m.source.Output(ctx, key.channel, key.plan)
	t1 := time.Now()
	slate, err := m.slate(ctx, host, out)
	// The tune-in (G2) split before the packager runs: the encode profile and the slate (encoded
	// once per host and output).
	m.log.Info("packager hls: channel start", "channel", key.channel, "output_ms", t1.Sub(t0).Milliseconds(),
		"slate_ms", time.Since(t1).Milliseconds())
	if err != nil {
		cancel()
		return nil, err
	}
	dir, err := os.MkdirTemp(m.root, "ch-")
	if err != nil {
		cancel()
		return nil, fmt.Errorf("packager hls: channel dir: %w", err)
	}
	log := m.log.With("channel", key.channel, "plan", key.plan.String())
	p, err := packager.New(packager.Config{FPS: out.FPS, Dir: dir, Log: log}, m.schedule(key, host, out, log), slate)
	if err != nil {
		cancel()
		_ = os.RemoveAll(dir)
		return nil, err
	}
	c := &packagedChannel{p: p, dir: dir, cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(c.done)
		if err := p.Run(ctx); err != nil {
			log.Error("packager hls: channel packager stopped", "err", err)
		}
		_ = os.RemoveAll(dir)
	}()
	log.Info("packager hls: started", "dir", dir, "output", fmt.Sprintf("%dx%d@%d", out.Width, out.Height, out.FPS))
	return c, nil
}

// schedule adapts the application's items to the packager: each Open builds the item's command
// with the phase-1a builder and starts its encoder.
func (m *PackagerHLS) schedule(key remuxKey, host HostProfile, out OutputProfile, log *slog.Logger) packager.Schedule {
	return func(ctx context.Context, at time.Time) (packager.Item, error) {
		it, err := m.source.ItemAt(ctx, key.channel, key.plan, at)
		if err != nil {
			return packager.Item{}, err
		}
		item := packager.Item{Label: it.Label, Duration: it.Remaining}
		if it.Input == "" {
			return item, nil
		}
		item.Open = func(ctx context.Context, slot packager.Slot) (io.ReadCloser, error) {
			pl, args, err := packagerItemArgs(host, out, it, slot)
			if err != nil {
				return nil, err
			}
			if len(pl.Fallbacks) > 0 {
				log.Info("packager hls: item leaves the GPU", "item", it.Label, "fallbacks", strings.Join(pl.Fallbacks, "; "))
			}
			return startFragmentEncoder(ctx, m.ffmpeg, args, log.With("item", it.Label))
		}
		return item, nil
	}
}

// packagerItemArgs is one item's encoder command for its slot: the phase-1a builder's pipeline,
// the filler gain in its audio stage, fMP4 out.
func packagerItemArgs(host HostProfile, out OutputProfile, it PackagerItem, slot packager.Slot) (Pipeline, []string, error) {
	pl, err := Build(host, it.Format, out)
	if err != nil {
		return Pipeline{}, nil, err
	}
	pl = pl.WithGain(it.GainDB)
	return pl, pl.FragmentArgs(it.Input, it.Seek, slot.Offset, slot.Frames, slot.AudioFrames, out.FPS, it.AudioTrack), nil
}

func (m *PackagerHLS) release(key remuxKey, c *packagedChannel) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c.viewers--
	if c.viewers > 0 || c.idle != nil {
		return
	}
	c.idle = time.AfterFunc(m.grace, func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if c.viewers == 0 && m.channels[key] == c {
			m.removeLocked(key, c)
		}
	})
}

func (m *PackagerHLS) removeLocked(key remuxKey, c *packagedChannel) {
	if m.channels[key] == c {
		delete(m.channels, key)
	}
	if c.idle != nil {
		c.idle.Stop()
	}
	c.cancel()
}

// AssetPath resolves the init segment or a media segment of a running channel packager.
func (m *PackagerHLS) AssetPath(channelID string, plan EncodePlan, rel string) (string, bool) {
	if rel != packager.InitName && (!strings.HasPrefix(rel, "seg") || !strings.HasSuffix(rel, ".m4s") || filepath.Base(rel) != rel) {
		return "", false
	}
	m.mu.Lock()
	c := m.channels[remuxKey{channel: channelID, plan: plan}]
	m.mu.Unlock()
	if c == nil {
		return "", false
	}
	return filepath.Join(c.dir, rel), true
}

func (m *PackagerHLS) StopChannel(channelID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, c := range m.channels {
		if key.channel == channelID {
			m.removeLocked(key, c)
		}
	}
}

func (m *PackagerHLS) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, c := range m.channels {
		m.removeLocked(key, c)
	}
}

// slate returns the house slate for an encode, encoding it once: a black, silent source run through
// the same builder and encoder as the items, so its sample descriptions match theirs.
func (m *PackagerHLS) slate(ctx context.Context, host HostProfile, out OutputProfile) (*packager.Slate, error) {
	key := fmt.Sprintf("%+v|%+v", host, out)
	m.slateMu.Lock()
	defer m.slateMu.Unlock()
	if s := m.slates[key]; s != nil {
		return s, nil
	}
	src := filepath.Join(m.root, fmt.Sprintf("slate-%dx%d-%d.mkv", out.Width, out.Height, out.FPS))
	if _, err := os.Stat(src); err != nil {
		tmp := src + ".tmp.mkv"
		cmd := exec.CommandContext(ctx, m.ffmpeg, "-hide_banner", "-nostdin", "-loglevel", "error", "-y",
			"-f", "lavfi", "-i", fmt.Sprintf("color=c=black:s=%dx%d:r=%d", out.Width, out.Height, out.FPS),
			"-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo", "-t", "2",
			"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-c:a", "aac", tmp)
		if o, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("packager hls: slate source: %w: %s", err, bytes.TrimSpace(o))
		}
		if err := os.Rename(tmp, src); err != nil {
			return nil, err
		}
	}
	format := MediaFormat{VideoCodec: "h264", Width: out.Width, Height: out.Height, FrameRate: float64(out.FPS),
		PixelFormat: "yuv420p", AudioCodec: "aac", AudioChannels: 2, AudioSampleRate: 48000, Container: "matroska,webm"}
	pl, err := Build(host, format, out)
	if err != nil {
		return nil, fmt.Errorf("packager hls: slate pipeline: %w", err)
	}
	gop := int64(out.gop())
	args := pl.FragmentArgs(src, 0, 0, gop, gop*audioRateHz/int64(out.FPS)/1024+1, out.FPS, 0)
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, m.ffmpeg, args...)
	cmd.Stderr = &stderr
	encoded, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("packager hls: slate encode: %w: %s", err, bytes.TrimSpace(stderr.Bytes()))
	}
	s, err := packager.NewSlate(encoded)
	if err != nil {
		return nil, err
	}
	m.slates[key] = s
	return s, nil
}

const audioRateHz = 48000

// startFragmentEncoder runs one item's ffmpeg and returns its stdout. Close (or cancelling ctx)
// stops it; a failure that was not a stop is logged with ffmpeg's last words.
func startFragmentEncoder(ctx context.Context, ffmpeg string, args []string, log *slog.Logger) (io.ReadCloser, error) {
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	cmd.WaitDelay = 2 * time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &limitedWriter{w: &stderr, n: 4096}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &encoderOutput{ReadCloser: stdout, ctx: ctx, cmd: cmd, stderr: &stderr, log: log}, nil
}

type encoderOutput struct {
	io.ReadCloser
	ctx    context.Context
	cmd    *exec.Cmd
	stderr *bytes.Buffer
	log    *slog.Logger
	once   sync.Once
}

func (e *encoderOutput) Close() error {
	e.once.Do(func() {
		_ = e.ReadCloser.Close()
		if e.cmd.Process != nil {
			_ = e.cmd.Process.Kill() // the packager is done with this item, finished or not
		}
		if err := e.cmd.Wait(); err != nil && e.ctx.Err() == nil && e.stderr.Len() > 0 {
			e.log.Warn("packager hls: item encoder failed", "err", err, "stderr", strings.TrimSpace(e.stderr.String()))
		}
	})
	return nil
}

type limitedWriter struct {
	w io.Writer
	n int
}

func (l *limitedWriter) Write(b []byte) (int, error) {
	if l.n > 0 {
		k := min(len(b), l.n)
		_, _ = l.w.Write(b[:k])
		l.n -= k
	}
	return len(b), nil
}

// switchedHLS picks the channel packager or the remux per new tune, by a live setting. Assets and
// stops go to both, so a channel keeps its origin until it stops even if the setting flips.
type switchedHLS struct {
	remux, packaged hlsOrigin
	usePackager     func() bool
}

func (s switchedHLS) acquirePlaylist(channelID string, plan EncodePlan, speculative bool) (hlsPlaylistLease, error) {
	if s.remux == nil || s.usePackager() {
		return s.packaged.acquirePlaylist(channelID, plan, speculative)
	}
	return s.remux.acquirePlaylist(channelID, plan, speculative)
}

func (s switchedHLS) AssetPath(channelID string, plan EncodePlan, rel string) (string, bool) {
	if p, ok := s.packaged.AssetPath(channelID, plan, rel); ok {
		return p, true
	}
	if s.remux == nil {
		return "", false
	}
	return s.remux.AssetPath(channelID, plan, rel)
}

func (s switchedHLS) StopChannel(channelID string) {
	s.packaged.StopChannel(channelID)
	if s.remux != nil {
		s.remux.StopChannel(channelID)
	}
}

func (s switchedHLS) StopAll() {
	s.packaged.StopAll()
	if s.remux != nil {
		s.remux.StopAll()
	}
}

// Stop ends every channel packager and removes the scratch root.
func (m *PackagerHLS) Stop() {
	m.StopAll()
	_ = os.RemoveAll(m.root)
	if m.unlock != nil {
		m.unlock()
	}
}
