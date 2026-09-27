package playout

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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
	// Watermark is set only for a PROGRAMME airing (never filler, bumpers or IDs): the channel's bug
	// sized for the packager's output, resolved per item because one item airs in every format.
	Watermark WatermarkFor
}

// WatermarkFor returns the channel's rendered bug for an encoder and output size, or nil when the
// channel turned it off or this host's GPU overlay failed its self-check (WatermarkCheck); the item
// then airs bug-free.
type WatermarkFor func(ctx context.Context, enc Encoder, width, height int) *Watermark

// PackagerSource is the application's side of the channel packager (#1512 phase 2): the schedule,
// and the host and output one of a channel's formats encodes with at an output ladder rung (the
// lease's).
type PackagerSource interface {
	ItemAt(ctx context.Context, channelID string, at time.Time) (PackagerItem, error)
	Output(ctx context.Context, channelID string, class FormatClass, rung int) (HostProfile, OutputProfile)
	// Premium is the premium format the channel airs on this host (its lineup's, after
	// ChannelFormats.OnHost); empty when none.
	Premium(ctx context.Context, channelID string) FormatClass
}

// packagedKey is one packager: a channel at one output format. Every client and plan reads the
// baseline (H.264 1080p SDR); a channel adds at most one premium format (G10), read only by a
// client that opts in to it.
type packagedKey struct {
	channel string
	format  FormatClass
}

// PackagerHLS serves in-app HLS from one channel packager per (channel, format): one encoder per
// scheduled item at a time, stitched in-process, no continuous transcode and no remux. It is an
// hlsOrigin, so Origin serves it exactly as it serves the remux: master.m3u8 is a master playlist
// naming each format's media playlist, and every asset is flat, named `<format>-<file>`.
type PackagerHLS struct {
	ffmpeg, root string
	unlock       func() // drops root's owner lock
	grace        time.Duration
	source       PackagerSource
	log          *slog.Logger
	// budget is the one admission ledger (#1520): one lease per running (channel, format) packager.
	// Nil admits everything (tests, builds without internal playout).
	budget *ResourceBudget
	// ladderCfg tunes each airing's RungMonitor; zero is the defaults (a test seam for its dwell times).
	ladderCfg RungMonitorConfig

	// life bounds background work that outlives a viewer (the slate encodes); Stop ends it.
	life    context.Context
	endLife context.CancelFunc

	slateMu sync.Mutex
	slates  map[string]*slateEncode // keyed by the encode (host, output)

	mu       sync.Mutex
	channels map[packagedKey]*packagedChannel

	// observer and onChange see packagers start and stop. Both are set before the first tune.
	observer SessionObserver
	onChange func()
}

type packagedChannel struct {
	p       *packager.Packager
	host    HostProfile
	out     OutputProfile
	started time.Time
	dir     string
	cancel  context.CancelFunc
	done    chan struct{}
	viewers int
	idle    *time.Timer
}

// DefaultGrace is how long a channel packager survives its last viewer.
//
// Long enough to absorb channel surfing and a client reconnecting after a network blip, both
// common on a TV, which would otherwise pay a cold start again. Short enough that a genuinely
// abandoned channel stops burning an encoder promptly.
const DefaultGrace = 30 * time.Second

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
	life, endLife := context.WithCancel(context.Background())
	return &PackagerHLS{ffmpeg: ffmpeg, root: root, unlock: unlock, grace: grace, source: source, log: log,
		life: life, endLife: endLife,
		slates: map[string]*slateEncode{}, channels: map[packagedKey]*packagedChannel{}}, nil
}

// WithBudget admits every channel packager through the ResourceBudget. Call before the packager
// serves.
func (m *PackagerHLS) WithBudget(budget *ResourceBudget) *PackagerHLS {
	m.budget = budget
	return m
}

// acquirePlaylist serves the channel's master playlist. Every plan gets the baseline variant: a
// PlanBaseline browser on a channel whose profile is HEVC still gets H.264 (#1512 phase 2). Beside
// it the master names the channel's premium format when it airs one here and the ledger has room
// (#1512 G10). The premium is described from its output alone: only a client that plays it starts
// its packager. Its lineup lookup runs beside the baseline's cold start, off the tune path.
func (m *PackagerHLS) acquirePlaylist(channelID string, _ EncodePlan, _ bool) (hlsPlaylistLease, error) {
	premium, found := new(*hlsVariant), make(chan struct{})
	go func() {
		defer close(found)
		ctx, cancel := context.WithTimeout(m.life, premiumLookupTimeout)
		defer cancel()
		*premium = m.premiumVariant(ctx, channelID)
	}()
	c, release, err := m.acquire(channelID, FormatBaseline)
	if err != nil {
		return hlsPlaylistLease{}, err
	}
	return hlsPlaylistLease{
		path:    filepath.Join(c.dir, "master.m3u8"),
		release: release,
		await:   c.p.AwaitPlaylist,
		snapshot: func(ctx context.Context) ([]byte, error) {
			vs := []hlsVariant{c.variant(FormatBaseline)}
			select {
			case <-found:
				if *premium != nil {
					vs = append(vs, **premium)
				}
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return masterPlaylist(vs), nil
		},
	}, nil
}

// premiumLookupTimeout bounds the master's premium lookup (the lineup's formats); a lookup that
// fails or times out lists the baseline alone.
const premiumLookupTimeout = 5 * time.Second

// premiumVariant is the master's entry for the channel's premium format, or nil when it airs none
// on this host or the ledger has no room for another one now (#1520: premium is admitted only on
// its own measured cost). A running premium is described from its own init.
func (m *PackagerHLS) premiumVariant(ctx context.Context, channelID string) *hlsVariant {
	class := m.source.Premium(ctx, channelID)
	if class == "" {
		return nil
	}
	m.mu.Lock()
	c := m.channels[packagedKey{channel: channelID, format: class}]
	m.mu.Unlock()
	if c != nil {
		v := c.variant(class)
		return &v
	}
	if m.budget != nil && !m.budget.Fits(premiumAdmission) {
		m.log.Info("packager hls: premium not offered: no measured room for it", "channel", channelID, "format", string(class))
		return nil
	}
	_, out := m.source.Output(ctx, channelID, class, 0)
	v := variantOf(class, out, nil)
	return &v
}

// premiumAdmission is a premium packager's lease: the premium class at its measured cost for the
// channel's life, whatever each item's source is, and never a lower output rung (a premium's
// geometry is fixed).
var premiumAdmission = AdmitRequest{Class: ClassPremium4K, NoRungDrop: true}

// MediaPlaylist serves a format's live media playlist (`<format>.m3u8`, named by the master). A
// player polls it, not the master, so each poll counts as a viewer and keeps the packager past its
// grace. A premium playlist is the client's opt-in (#1512 G10): it starts the channel's premium
// packager, and only for the premium the channel airs on this host.
func (m *PackagerHLS) MediaPlaylist(ctx context.Context, channelID string, _ EncodePlan, rel string) ([]byte, bool, error) {
	class := FormatClass(strings.TrimSuffix(rel, ".m3u8"))
	if filepath.Base(rel) != rel || !strings.HasSuffix(rel, ".m3u8") {
		return nil, false, nil
	}
	switch class {
	case FormatBaseline:
	case Format4KSDR, Format4KHDR:
		m.mu.Lock()
		running := m.channels[packagedKey{channel: channelID, format: class}] != nil
		m.mu.Unlock()
		if !running && m.source.Premium(ctx, channelID) != class {
			return nil, false, nil
		}
	default:
		return nil, false, nil
	}
	c, release, err := m.acquire(channelID, class)
	if err != nil {
		return nil, false, err
	}
	defer release()
	if err := c.p.AwaitPlaylist(ctx); err != nil {
		return nil, false, err
	}
	return c.p.Playlist(), true, nil
}

// acquire counts a viewer (browser or tuner) on a channel format's packager, starting it if needed.
func (m *PackagerHLS) acquire(channelID string, class FormatClass) (*packagedChannel, func(), error) {
	key := packagedKey{channel: channelID, format: class}
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
		// Admission is the ledger's (#1520): start books the packager's lease, and a full host
		// refuses it with ErrAtCapacity. Viewers of a running packager never count against it.
		var err error
		if c, err = m.start(key); err != nil {
			result := "spawn_error"
			if errors.Is(err, ErrAtCapacity) {
				result = "capacity"
			}
			m.observe(func(o SessionObserver) { o.PlayoutSessionStarted(result) })
			return nil, nil, err
		}
	}
	m.mu.Lock()
	if cur := m.channels[key]; cur != c { // lost a start race: use the winner
		if cur != nil {
			c.cancel()
			c = cur
		} else {
			m.channels[key] = c
			m.observe(func(o SessionObserver) { o.PlayoutSessionStarted("success"); o.PlayoutSessionActive(1) })
			m.changed()
		}
	}
	c.viewers++
	if c.idle != nil {
		c.idle.Stop()
		c.idle = nil
	}
	m.mu.Unlock()
	return c, onceRelease(func() { m.release(key, c) }), nil
}

func (m *PackagerHLS) start(key packagedKey) (*packagedChannel, error) {
	ctx, cancel := context.WithCancel(context.Background())
	// Admission (#1520) is priced for the item airing now: the first manifest waits for the first
	// real item anyway, so resolving it here costs the tune nothing, and the schedule's first lookup
	// reuses it. A 4K HDR first item on a nearly full host is demoted or refused before any encoder
	// starts; a card slot or a failed lookup books SDR until a real item reclasses the lease.
	// ErrAtCapacity reaches the viewer as 503. The lease is released when the run ends.
	// A premium packager is priced by its own class instead (premiumAdmission).
	resolvedAt := time.Now()
	first, ferr := m.source.ItemAt(ctx, key.channel, resolvedAt)
	req := AdmitRequest{Class: ClassSDR}
	if key.format != FormatBaseline {
		req = premiumAdmission
	} else if ferr == nil && first.Input != "" {
		req.Class = ClassOf(first.Format)
	}
	var pre *prefetchedItem
	if ferr == nil {
		pre = &prefetchedItem{at: resolvedAt, item: first}
	}
	var lease *Lease
	if m.budget != nil {
		var err error
		if lease, err = m.budget.Admit(ctx, req); err != nil {
			cancel()
			return nil, err
		}
	}
	rung := 0
	if lease != nil {
		rung = lease.Rung()
	}
	t0 := time.Now()
	host, out := m.source.Output(ctx, key.channel, key.format, rung)
	t1 := time.Now()
	// The tune-in (G2) split before the packager runs: the encode profile. The slate is waited for
	// only by a slot that needs it.
	m.log.Info("packager hls: channel start", "channel", key.channel, "output_ms", t1.Sub(t0).Milliseconds())
	slate := m.slate(host, out)
	dir, err := os.MkdirTemp(m.root, "ch-")
	if err != nil {
		cancel()
		lease.Release()
		return nil, fmt.Errorf("packager hls: channel dir: %w", err)
	}
	log := m.log.With("channel", key.channel, "format", string(key.format))
	cfg := packager.Config{FPS: out.FPS, Dir: dir, DVR: DVRHorizon, URIPrefix: string(key.format) + "-", Log: log}
	if out.HDR {
		cfg.HDR10 = ChannelHDR10.Packager() // no encoder writes the channel's fixed metadata (#1527)
	}
	p, err := packager.New(cfg, m.schedule(key, host, out, lease, pre, log), slate)
	if err != nil {
		cancel()
		lease.Release()
		_ = os.RemoveAll(dir)
		return nil, err
	}
	c := &packagedChannel{p: p, host: host, out: out, started: t0, dir: dir, cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(c.done)
		defer lease.Release() // before done: a restart must not count this run
		if err := p.Run(ctx); err != nil {
			log.Error("packager hls: channel packager stopped", "err", err)
			m.observe(func(o SessionObserver) { o.PlayoutProcessFailure("parent") })
		}
		_ = os.RemoveAll(dir)
	}()
	go func() {
		// Prewarm the slate once the channel is on air, not beside the tune-in item's encoder.
		if p.AwaitPlaylist(ctx) == nil {
			m.slateEncodeFor(host, out)
		}
	}()
	log.Info("packager hls: started", "dir", dir, "output", fmt.Sprintf("%dx%d@%d", out.Width, out.Height, out.FPS))
	return c, nil
}

// schedule adapts the application's items to the packager: each Open builds the item's command
// with the phase-1a builder and starts its encoder. Each item moves the channel's lease to its
// class, and encodes at the software ladder rung the ledger picks for it (#1517, #1520).
func (m *PackagerHLS) schedule(
	key packagedKey, host HostProfile, out OutputProfile, lease *Lease, pre *prefetchedItem, log *slog.Logger,
) packager.Schedule {
	faults := &itemFaults{by: map[string]itemFault{}}
	ladder := &itemLadder{}
	return func(ctx context.Context, at time.Time) (packager.Item, error) {
		it, ok := pre.take(at)
		if !ok {
			var err error
			if it, err = m.source.ItemAt(ctx, key.channel, at); err != nil {
				return packager.Item{}, err
			}
		}
		item := packager.Item{Label: it.Label, Duration: it.Remaining}
		if it.Input == "" {
			return item, nil
		}
		// A CPU too slow for the source degrades the picture instead of refusing it (#1517): the
		// ledger picks the item's software rung (GPU families ignore it); with no ledger, the
		// unmeasured start (full quality up to 1080p SDR, keyframes-only for 4K or HDR).
		//
		// On a software host the rung then follows the item encoder's measured speed (#1517): one
		// RungMonitor per airing, bound to the lease so every step re-prices it. A step ends the
		// encoder at a fragment boundary; the packager asks again at its own clock, which resolves
		// the same airing at the position the channel reached, and it resumes here on the monitor's
		// rung instead of a fresh pick.
		itemOut := out
		// A premium lease keeps its class: it was priced for the channel's whole lineup.
		itemClass := ClassOf(it.Format)
		if key.format != FormatBaseline {
			itemClass = ClassPremium4K
		}
		airing := it.Label + "\x00" + it.Input
		rung, resumed, stepped := ladder.resume(airing)
		switch {
		case resumed:
			itemOut.SoftwareRung = rung
		case lease != nil:
			lease.ReclassItem(ctx, itemClass) // a transcoding lease always moves
			itemOut.SoftwareRung = lease.SoftwareRung()
		default:
			itemOut.SoftwareRung = StartRung(it.Format, RungCost{})
		}
		var monitor *RungMonitor
		switch {
		case host.Family != FamilySoftware:
			ladder.begin("", nil)
		case resumed:
			monitor = ladder.current()
		case lease != nil:
			monitor = lease.NewRungMonitor(m.ladderCfg)
		default:
			cfg := m.ladderCfg
			cfg.Costs = RungCostsFor(it.Format)
			monitor = NewRungMonitor(itemOut.SoftwareRung, cfg)
		}
		if monitor != nil && !resumed {
			ladder.begin(airing, monitor)
		}
		if stepped {
			item.Wait = ladderResumeWait
		}
		// The channel's bug, programmes only (#1512 phase 1d): resolved once per item at this
		// packager's encoder and output size, so a retried Open reuses it.
		var wm *Watermark
		if it.Watermark != nil {
			wm = it.Watermark(ctx, host.Encoder, out.Width, out.Height)
		}
		// The frames the packager took from this item (0 until it reports, and never for an item whose
		// channel stopped): the cost sample's media.
		var delivered atomic.Int64
		item.Open = func(ctx context.Context, slot packager.Slot) (io.ReadCloser, error) {
			pl, args, err := packagerItemArgs(host, itemOut, it, wm, slot, faults.get(it.Input))
			if err != nil {
				return nil, err
			}
			if len(pl.Fallbacks) > 0 {
				log.Info("packager hls: item leaves the GPU", "item", it.Label, "fallbacks", strings.Join(pl.Fallbacks, "; "))
			}
			return startFragmentEncoder(ctx, m.ffmpeg, args, log.With("item", it.Label), func(decodeFault bool) {
				if faults.record(it.Input, pl, decodeFault) {
					log.Warn("packager hls: item failed on the GPU; its next attempt demotes the failing stage",
						"item", it.Label, "decode_fault", decodeFault, "tonemapper", pl.Tonemapper)
				}
			}, func(cpu time.Duration) {
				// The ledger learns the class's real CPU cost from delivered items (#1520), as it
				// did from the retired chain's finished programmes: CPU over the media the item put
				// on the timeline, not its slot, so an encoder closed early cannot over-count.
				if frames := delivered.Load(); frames > 0 && itemOut.FPS > 0 {
					media := time.Duration(frames) * time.Second / time.Duration(itemOut.FPS)
					lease.ObserveCPU(itemClass, cpu, media)
				}
			}, ladder.watch(monitor, log, it.Label))
		}
		item.Delivered = func(frames int64) { delivered.Store(frames) }
		return item, nil
	}
}

// ladderResumeWait is how long an airing resumed on a new rung gets to produce, as long as a tune-in's
// first item (FirstItemWait): the encoder it replaces was too slow, so the timeline has no lead left,
// and a hold beats a slate.
const ladderResumeWait = 10 * time.Second

// itemLadder is one channel packager's software ladder (#1517): the RungMonitor of the airing on
// air, which outlives its encoder restarts (a new airing starts a new monitor). The schedule reads
// it; the item encoder's progress goroutine feeds it.
type itemLadder struct {
	mu      sync.Mutex
	airing  string
	monitor *RungMonitor
	stepped bool // a step ended the airing's encoder: its next open resumes on the monitor's rung
}

func (l *itemLadder) begin(airing string, m *RungMonitor) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.airing, l.monitor, l.stepped = airing, m, false
}

func (l *itemLadder) current() *RungMonitor {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.monitor
}

// resume reports whether airing is the one on air (a restart of it), on which rung, and whether a
// step caused the restart.
func (l *itemLadder) resume(airing string) (rung SoftwareRung, resumed, stepped bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.monitor == nil || l.airing != airing {
		return RungFull, false, false
	}
	stepped, l.stepped = l.stepped, false
	return l.monitor.Rung(), true, stepped
}

// watch feeds an item encoder's samples to m and ends that encoder on a step. Samples from an
// encoder of an airing no longer on air are ignored. Nil when there is no monitor (GPU hosts).
func (l *itemLadder) watch(m *RungMonitor, log *slog.Logger, label string) func(SpeedSample) bool {
	if m == nil {
		return nil
	}
	return func(s SpeedSample) bool {
		l.mu.Lock()
		defer l.mu.Unlock()
		if l.monitor != m || l.stepped {
			return false
		}
		d := m.Observe(s)
		if d.Step {
			l.stepped = true
			log.Info("packager hls: software rung step", "item", label, "rung", d.Rung.String(), "reason", d.Reason,
				"encoder_media", s.OutTime.Round(time.Millisecond))
		}
		return d.Step
	}
}

// prefetchedItem is the item the channel start resolved for admission, handed to the schedule's
// first lookup so it is not resolved twice. The schedule runs on one goroutine.
type prefetchedItem struct {
	at   time.Time
	item PackagerItem
	used bool
}

// take is the prefetched item as it airs at at, advanced by the time since it was resolved; false
// once taken, or when at is outside that airing.
func (p *prefetchedItem) take(at time.Time) (PackagerItem, bool) {
	if p == nil || p.used {
		return PackagerItem{}, false
	}
	p.used = true
	d := at.Sub(p.at)
	if d < 0 || d >= p.item.Remaining {
		return PackagerItem{}, false
	}
	it := p.item
	it.Remaining -= d
	if it.Input != "" {
		it.Seek += d
	}
	return it, true
}

// itemFault is what a failed encode of one source proved about this host. After a failed item the
// packager asks the schedule again, so the next attempt at the same source builds without the stage
// that failed, as the retired chain's retry ladder did (§9.1 V47). The encoder never changes: the
// channel's init must still match.
type itemFault struct {
	// cpuDecode: the GPU decoder faulted on this source (IsHardwareDecodeFault); retrying the same
	// -hwaccel path fails identically.
	cpuDecode bool
	// noOpenCL, noLibplacebo: that GPU tone-mapper failed this source, so the next one for the
	// curve is taken, ending at the CPU (the maintainer order, #1512).
	noOpenCL, noLibplacebo bool
}

func (f itemFault) apply(h HostProfile) HostProfile {
	if f.cpuDecode {
		h.DecodeCodecs = nil
	}
	h.TonemapOpenCL = h.TonemapOpenCL && !f.noOpenCL
	h.Libplacebo = h.Libplacebo && !f.noLibplacebo
	return h
}

// itemFaults holds one channel packager's faults by source, for its life. Only a failure adds one.
type itemFaults struct {
	mu sync.Mutex
	by map[string]itemFault
}

func (f *itemFaults) get(input string) itemFault {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.by[input]
}

// record demotes one stage for an encode that failed before its first output: the decode on a GPU
// decode fault, otherwise the GPU tone-mapper the pipeline used. It reports false when there is
// nothing left to demote, so the item keeps failing into slate rather than retrying blindly.
func (f *itemFaults) record(input string, pl Pipeline, decodeFault bool) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur := f.by[input]
	switch {
	case decodeFault && !cur.cpuDecode:
		cur.cpuDecode = true
	case pl.Tonemapper == TonemapperOpenCL:
		cur.noOpenCL = true
	case pl.Tonemapper == TonemapperLibplacebo:
		cur.noLibplacebo = true
	default:
		return false
	}
	f.by[input] = cur
	return true
}

// packagerItemArgs is one item's encoder command for its slot: the phase-1a builder's pipeline on
// the host less any stage this source faulted, the channel's bug when wm is non-nil (a programme on
// a host whose overlay passed its self-check), the filler gain in its audio stage, fMP4 out.
func packagerItemArgs(host HostProfile, out OutputProfile, it PackagerItem, wm *Watermark, slot packager.Slot, fault itemFault) (Pipeline, []string, error) {
	h := fault.apply(host)
	h.Overlay = wm != nil
	pl, err := BuildItem(h, it.Format, out, wm)
	if err != nil {
		return Pipeline{}, nil, err
	}
	pl = pl.WithGain(it.GainDB)
	return pl, pl.FragmentArgs(it.Input, it.Seek, slot.Offset, slot.Frames, slot.AudioFrames, out.FPS, it.AudioTrack), nil
}

func (m *PackagerHLS) release(key packagedKey, c *packagedChannel) {
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

func (m *PackagerHLS) removeLocked(key packagedKey, c *packagedChannel) {
	if m.channels[key] == c {
		delete(m.channels, key)
		m.observe(func(o SessionObserver) { o.PlayoutSessionActive(-1) })
		m.changed()
	}
	if c.idle != nil {
		c.idle.Stop()
	}
	c.cancel()
}

// AssetPath resolves `<format>-<file>`, the init segment or a media segment of one of a running
// channel's packagers.
func (m *PackagerHLS) AssetPath(channelID string, _ EncodePlan, rel string) (string, bool) {
	class, file, ok := strings.Cut(rel, "-"+packager.InitName)
	if ok && file == "" {
		file = packager.InitName
	} else if i := strings.LastIndex(rel, "-seg"); i > 0 && strings.HasSuffix(rel, ".m4s") {
		class, file = rel[:i], rel[i+1:]
	} else {
		return "", false
	}
	if filepath.Base(file) != file {
		return "", false
	}
	m.mu.Lock()
	c := m.channels[packagedKey{channel: channelID, format: FormatClass(class)}]
	m.mu.Unlock()
	if c == nil {
		return "", false
	}
	return filepath.Join(c.dir, file), true
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

// slateEncode is one (host, output) slate, encoding or encoded; done closes when s or err is set.
type slateEncode struct {
	done chan struct{}
	s    *packager.Slate
	err  error
}

// slateEncodeTimeout bounds one slate encode (0.8 s cold on NVENC live).
const slateEncodeTimeout = time.Minute

// slate returns the house slate for an encode as a SlateSource: the packager waits on it only when a
// slot needs slate (#1512 G2: never on the tune path; the first manifest waits for a real item
// anyway). The encode starts on that first need or, when prewarmed, once the channel is on air.
func (m *PackagerHLS) slate(host HostProfile, out OutputProfile) packager.SlateSource {
	return func(ctx context.Context) (*packager.Slate, error) {
		e := m.slateEncodeFor(host, out)
		select {
		case <-e.done:
			return e.s, e.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// slateEncodeFor returns the (host, output) slate encode, starting it in the background if none is
// running or done. Encodes are shared across channels and sessions; a failed one is forgotten, so
// the next need encodes it again.
func (m *PackagerHLS) slateEncodeFor(host HostProfile, out OutputProfile) *slateEncode {
	key := fmt.Sprintf("%+v|%+v", host, out)
	m.slateMu.Lock()
	e := m.slates[key]
	if e == nil {
		e = &slateEncode{done: make(chan struct{})}
		m.slates[key] = e
		go func() {
			defer close(e.done)
			ctx, cancel := context.WithTimeout(m.life, slateEncodeTimeout)
			defer cancel()
			t0 := time.Now()
			if e.s, e.err = m.encodeSlate(ctx, host, out); e.err != nil {
				m.log.Error("packager hls: slate encode failed", "err", e.err)
				m.slateMu.Lock()
				if m.slates[key] == e {
					delete(m.slates, key)
				}
				m.slateMu.Unlock()
				return
			}
			m.log.Info("packager hls: slate encoded", "output", fmt.Sprintf("%dx%d@%d", out.Width, out.Height, out.FPS),
				"ms", time.Since(t0).Milliseconds())
		}()
	}
	m.slateMu.Unlock()
	return e
}

// encodeSlate encodes a house slate: a black, silent source run through the same builder and
// encoder as the items, so its sample descriptions match theirs.
func (m *PackagerHLS) encodeSlate(ctx context.Context, host HostProfile, out OutputProfile) (*packager.Slate, error) {
	src := filepath.Join(m.root, fmt.Sprintf("slate-%dx%d-%d.mkv", out.Width, out.Height, out.FPS))
	if _, err := os.Stat(src); err != nil {
		// Two hosts with one output size may make the source at once: each writes its own temp.
		f, err := os.CreateTemp(m.root, "slate-*.tmp.mkv")
		if err != nil {
			return nil, err
		}
		tmp := f.Name()
		_ = f.Close()
		cmd := exec.CommandContext(ctx, m.ffmpeg, "-hide_banner", "-nostdin", "-loglevel", "error", "-y",
			"-f", "lavfi", "-i", fmt.Sprintf("color=c=black:s=%dx%d:r=%d", out.Width, out.Height, out.FPS),
			"-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo", "-t", "2",
			"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-c:a", "aac", tmp)
		if o, err := cmd.CombinedOutput(); err != nil {
			_ = os.Remove(tmp)
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
	return packager.NewSlate(encoded)
}

const audioRateHz = 48000

// startFragmentEncoder runs one item's ffmpeg and returns its stdout. Close (or cancelling ctx)
// stops it; a failure that was not a stop is logged with ffmpeg's last words.
// startFragmentEncoder starts one item's encoder. failed, when set, hears an encoder that exited
// on its own with an error before any output: a fault of this source on this host, not a slow
// start (a late item's context is cancelled first) and not an item the packager finished.
//
// watch, when set, is fed the encoder's -progress, one SpeedSample per block with the process's
// CPU time (RungMonitor, #1517). When it returns true the stream ends cleanly at the next top-level
// box, so the packager takes the fragments produced so far and none of what the killed encoder had
// in flight (#1533's contract: no flush reaches the channel); the schedule then resumes the item on
// the new rung.
func startFragmentEncoder(ctx context.Context, ffmpeg string, args []string, log *slog.Logger,
	failed func(decodeFault bool), done func(cpu time.Duration), watch func(SpeedSample) bool,
) (io.ReadCloser, error) {
	var progress, progressW *os.File
	if watch != nil {
		var err error
		if progress, progressW, err = os.Pipe(); err != nil {
			return nil, err
		}
		args = append([]string{"-progress", "pipe:3"}, args...)
	}
	closeProgress := func() {
		if progress != nil {
			_ = progress.Close()
			_ = progressW.Close()
		}
	}
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	cmd.WaitDelay = 2 * time.Second
	var stderr bytes.Buffer
	faults := &limitedWriter{w: &stderr, n: 4096}
	cmd.Stderr = faults
	if progressW != nil {
		cmd.ExtraFiles = []*os.File{progressW}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		closeProgress()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		closeProgress()
		return nil, err
	}
	cut := &boxCut{r: stdout}
	if progress != nil {
		_ = progressW.Close() // the child holds its own copy
		pid := cmd.Process.Pid
		go ReadProgress(progress, func(p Progress) {
			if cut.stop.Load() {
				return
			}
			cpu, _ := processCPUTime(pid)
			if watch(SpeedSample{At: time.Now(), OutTime: time.Duration(p.OutTimeMS) * time.Millisecond, CPU: cpu}) {
				cut.stop.Store(true)
			}
		})
	}
	return &encoderOutput{ReadCloser: stdout, cut: cut, ctx: ctx, cmd: cmd, stderr: &stderr, watch: faults, failed: failed, done: done, log: log}, nil
}

// boxCut passes an fMP4 stream through whole top-level boxes and, once stopped, ends it with a
// clean EOF at the next box boundary: the reader never sees a partial box, so the packager's
// stream ends as if the encoder had finished there.
type boxCut struct {
	r       io.Reader
	stop    atomic.Bool
	left    int64 // bytes left in the current box; 0 at a boundary, -1 for a box that runs to EOF
	pending []byte
}

func (c *boxCut) Read(p []byte) (int, error) {
	if len(c.pending) > 0 {
		n := copy(p, c.pending)
		c.pending = c.pending[n:]
		return n, nil
	}
	if c.left == 0 {
		if c.stop.Load() {
			return 0, io.EOF
		}
		var h [16]byte
		if _, err := io.ReadFull(c.r, h[:8]); err != nil {
			return 0, err // io.EOF here is the encoder finishing at a boundary
		}
		size, hl := int64(binary.BigEndian.Uint32(h[:4])), 8
		if size == 1 {
			if _, err := io.ReadFull(c.r, h[8:16]); err != nil {
				return 0, io.ErrUnexpectedEOF
			}
			size, hl = int64(binary.BigEndian.Uint64(h[8:16])), 16
		}
		switch {
		case size == 0:
			c.left = -1
		case size < int64(hl):
			return 0, fmt.Errorf("packager hls: box size %d", size)
		default:
			c.left = size - int64(hl)
		}
		c.pending = append([]byte(nil), h[:hl]...)
		return c.Read(p)
	}
	if c.left > 0 && int64(len(p)) > c.left {
		p = p[:c.left]
	}
	n, err := c.r.Read(p)
	if c.left > 0 {
		c.left -= int64(n)
		if err == io.EOF && c.left > 0 {
			err = io.ErrUnexpectedEOF
		}
	}
	return n, err
}

type encoderOutput struct {
	io.ReadCloser // the encoder's stdout; reads go through cut
	cut           *boxCut
	ctx           context.Context
	cmd           *exec.Cmd
	stderr        *bytes.Buffer
	watch         *limitedWriter
	failed        func(decodeFault bool)
	// done receives the CPU time of an encoder that produced, for the ledger's measured class cost
	// (Lease.ObserveCPU, #1520); the schedule divides it by the frames the packager took.
	done     func(cpu time.Duration)
	produced bool // read on the packager's reader goroutine, then by Close after it
	log      *slog.Logger
	once     sync.Once
}

func (e *encoderOutput) Read(p []byte) (int, error) {
	n, err := e.cut.Read(p)
	if n > 0 {
		e.produced = true
	}
	return n, err
}

func (e *encoderOutput) Close() error {
	e.once.Do(func() {
		_ = e.ReadCloser.Close()
		if e.cmd.Process != nil {
			_ = e.cmd.Process.Kill() // the packager is done with this item, finished or not
		}
		err := e.cmd.Wait()
		// Every encoder that produced reports its CPU, however it ended (the packager cancels the
		// item's context before closing it): the schedule counts it only over the frames the
		// packager reported delivered.
		if e.produced && e.done != nil && e.cmd.ProcessState != nil {
			e.done(e.cmd.ProcessState.UserTime() + e.cmd.ProcessState.SystemTime())
		}
		if e.ctx.Err() != nil || err == nil {
			return // ended by the packager or its channel: not a failure
		}
		if e.stderr.Len() > 0 {
			e.log.Warn("packager hls: item encoder failed", "err", err, "stderr", strings.TrimSpace(e.stderr.String()))
		}
		if !e.produced && e.failed != nil {
			e.failed(e.watch.decodeFault)
		}
	})
	return nil
}

// limitedWriter keeps the first n bytes of an encoder's stderr for its failure log, and notes a GPU
// decode fault anywhere in it. ffmpeg writes one log line per write, so a line is never split.
// The fault is read only after cmd.Wait, which waits for the copy into this writer.
type limitedWriter struct {
	w           io.Writer
	n           int
	decodeFault bool
}

func (l *limitedWriter) Write(b []byte) (int, error) {
	if !l.decodeFault && IsHardwareDecodeFault(string(b)) {
		l.decodeFault = true
	}
	if l.n > 0 {
		k := min(len(b), l.n)
		_, _ = l.w.Write(b[:k])
		l.n -= k
	}
	return len(b), nil
}

// mediaPlaylister is an hlsOrigin whose Tune answer is a master playlist: its variant playlists are
// live documents, rendered per request, not files.
type mediaPlaylister interface {
	MediaPlaylist(ctx context.Context, channelID string, plan EncodePlan, rel string) ([]byte, bool, error)
}

// hlsVariant is one EXT-X-STREAM-INF entry of a channel's master playlist.
type hlsVariant struct {
	uri                 string
	bandwidth, average  int // bits/s: the peak and the target, video plus audio
	codecs              string
	width, height, rate int
	videoRange          string // SDR or PQ
}

// variant describes a running format's media playlist from its output and its channel init.
func (c *packagedChannel) variant(class FormatClass) hlsVariant {
	return variantOf(class, c.out, c.p.Init())
}

// variantOf describes a format's media playlist from its output and, once it has one, its channel
// init. A premium with no init yet names its predicted CODECS (premiumCodecs), because a web player
// offers it only when it can decode that exact string.
func variantOf(class FormatClass, o OutputProfile, init []byte) hlsVariant {
	v := hlsVariant{
		uri:       string(class) + ".m3u8",
		bandwidth: (o.MaxKbps + o.AudioKbps) * 1000, average: (o.TargetKbps + o.AudioKbps) * 1000,
		codecs: packager.CodecsAttr(init),
		width:  o.Width, height: o.Height, rate: o.FPS, videoRange: "SDR",
	}
	if o.HDR {
		v.videoRange = "PQ"
	}
	if v.codecs == "" && o.premium() {
		v.codecs = o.premiumCodecs()
	}
	return v
}

// masterPlaylist names each variant's media playlist. CODECS is omitted when the init could not
// name every track, so a player probes instead of trusting a wrong string.
func masterPlaylist(vs []hlsVariant) []byte {
	var b bytes.Buffer
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-INDEPENDENT-SEGMENTS\n")
	for _, v := range vs {
		fmt.Fprintf(&b, "#EXT-X-STREAM-INF:BANDWIDTH=%d,AVERAGE-BANDWIDTH=%d", v.bandwidth, v.average)
		if v.codecs != "" {
			fmt.Fprintf(&b, ",CODECS=%q", v.codecs)
		}
		fmt.Fprintf(&b, ",RESOLUTION=%dx%d,FRAME-RATE=%.3f", v.width, v.height, float64(v.rate))
		if v.videoRange != "" {
			fmt.Fprintf(&b, ",VIDEO-RANGE=%s", v.videoRange)
		}
		fmt.Fprintf(&b, "\n%s\n", v.uri)
	}
	return b.Bytes()
}

// Stop ends every channel packager and removes the scratch root.
func (m *PackagerHLS) Stop() {
	if m.endLife != nil {
		m.endLife()
	}
	m.StopAll()
	_ = os.RemoveAll(m.root)
	if m.unlock != nil {
		m.unlock()
	}
}
