package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"

	_ "golang.org/x/image/webp" // custom watermark uploads may be WebP

	"github.com/loomarr/loomarr/internal/diagnostics"
	"github.com/loomarr/loomarr/internal/images"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/watermark"
)

// channelWatermarks is the composition root's channel-bug service (#1512 phase 1d): the GPU
// overlay self-check per encoder, and the channel packager's per-item resolver.
//
// The self-check runs once per encoder, in the background, on the first programme that encoder
// airs: the encoder itself is only known once capability detection has run, which is lazy. Until
// the check passes, programmes air WITHOUT the bug (the safe direction: a bug never costs the
// picture). A failed check disables the watermark on the host and says so in Diagnostics.
type channelWatermarks struct {
	ffmpeg  func() string
	tonemap func() bool
	gpu     func() playout.GPUFilters
	dir     string
	// opacity is the live install-wide opacity (watermarkOpacity), read on every For.
	opacity  func() float64
	channels interface {
		GetChannel(context.Context, string) (store.Channel, error)
	}
	originals interface {
		Original(context.Context, string) (string, error)
	}
	events interface {
		Record(context.Context, diagnostics.Event)
	}
	log      *slog.Logger
	lifetime context.Context

	mu     sync.Mutex
	gates  map[playout.Encoder]*watermarkGate
	render sync.Mutex // one render at a time; each is cached on disk

	kernelOnce sync.Once
	kernel     string // the VAAPI family's blend kernel beside the bugs, written on first use
}

// blendKernel is the path of the VAAPI blend kernel, or "" when it could not be written: the VAAPI
// graph then airs the programme bug-free and says why.
func (c *channelWatermarks) blendKernel() string {
	c.kernelOnce.Do(func() {
		k, err := playout.WriteBlendKernel(c.dir)
		if err != nil {
			c.log.Warn("watermark: the VAAPI blend kernel could not be written; VAAPI airs without the bug", "err", err)
			return
		}
		c.kernel = k
	})
	return c.kernel
}

type watermarkGate struct{ works atomic.Bool }

// newChannelWatermarks builds the service before the channel packager that asks it for bugs. The
// image service is built later and bound with withOriginals. Rendered bugs are derived files, kept
// beside the image store rather than inside it (the image GC owns that tree).
func newChannelWatermarks(rootCtx context.Context, st store.Store, set resolved, events *diagnostics.Recorder, log *slog.Logger) *channelWatermarks {
	ffmpeg := set.str("playout.ffmpeg_path")
	c := &channelWatermarks{
		ffmpeg:   func() string { return ffmpeg },
		tonemap:  playout.TonemapperFor(ffmpeg),
		gpu:      playout.GPUFiltersFor(ffmpeg),
		dir:      filepath.Join(filepath.Dir(filepath.Clean(set.str("images.dir"))), "watermarks"),
		opacity:  watermarkOpacity(set),
		channels: st, log: log, lifetime: rootCtx,
	}
	if events != nil {
		c.events = events
	}
	return c
}

// watermarkOpacity is the install-wide bug opacity, playout.watermark_opacity_pct as a fraction,
// resolved per call so a change applies to the next programme item without a restart (the
// rendered-bug cache is keyed by opacity, so the new value re-renders).
func watermarkOpacity(set resolved) func() float64 {
	return func() float64 { return float64(set.intv("playout.watermark_opacity_pct")) / 100 }
}

// withOriginals binds the image service a custom upload is read from. The build calls it before the
// server takes a request, so no tune can race it; unbound, a channel's bug is the generated Plate.
func (c *channelWatermarks) withOriginals(imgs *images.Service) {
	if imgs != nil {
		c.originals = imgs
	}
}

// renderVersion changes every cached bug when the renderer's output changes.
const renderVersion = "1"

// For is the channel packager's resolver: the channel's rendered bug for this output, or nil.
func (c *channelWatermarks) For(ctx context.Context, channelID string, enc playout.Encoder, width, height int) *playout.Watermark {
	if !c.gate(enc).works.Load() {
		return nil
	}
	ch, err := c.channels.GetChannel(ctx, channelID)
	if err != nil {
		return nil
	}
	res := schedule.ResolveWatermark(ch.Policy.Watermark, c.opacity())
	if !res.Enabled {
		return nil
	}
	look := watermark.Look{Size: res.Size, Opacity: res.Opacity, Shadow: true}
	key, render := c.source(ctx, ch, res)
	straight, w, h, err := c.cached(fmt.Sprintf("%s|%d|%g|%g|%s", key, height, look.Size, look.Opacity, renderVersion),
		func() (watermark.Bug, error) {
			mask, err := render()
			if err != nil {
				return watermark.Bug{}, err
			}
			return mask(height, look)
		})
	if err != nil {
		c.log.Warn("watermark: the channel's bug could not be rendered; airing without it", "channel", channelID, "err", err)
		return nil
	}
	return &playout.Watermark{Straight: straight, Kernel: c.blendKernel(), Width: w, Height: h,
		Corner: playout.Corner(res.Corner), MarginX: evenRound(res.Margin * float64(width)), MarginY: evenRound(res.Margin * float64(height))}
}

type bugRenderer func(frameHeight int, look watermark.Look) (watermark.Bug, error)

// source picks the bug's image: a custom upload, else the generated Plate. The automatic TMDB
// network logo (between the two in the maintainer's order) is not wired yet.
func (c *channelWatermarks) source(ctx context.Context, ch store.Channel, res schedule.ResolvedWatermark) (string, func() (bugRenderer, error)) {
	if res.Image != "" && c.originals != nil {
		return "image:" + res.Image, func() (bugRenderer, error) {
			path, err := c.originals.Original(ctx, res.Image)
			if err != nil {
				return nil, err
			}
			f, err := os.Open(path)
			if err != nil {
				return nil, err
			}
			defer func() { _ = f.Close() }()
			img, _, err := image.Decode(f)
			if err != nil {
				return nil, err
			}
			return func(h int, look watermark.Look) (watermark.Bug, error) { return watermark.RenderImage(img, h, look) }, nil
		}
	}
	call := res.Callsign
	if call == "" {
		call = deriveCallsign(ch.Name, ch.Number)
	}
	return "plate:" + call, func() (bugRenderer, error) {
		mask, err := watermark.PlateMask(call)
		if err != nil {
			return nil, err
		}
		return func(h int, look watermark.Look) (watermark.Bug, error) { return watermark.Render(mask, h, look), nil }, nil
	}
}

// CallsignProposer is the seam for the issue's LLM callsign pick: one small call after channel
// creation, off Add a channel's latency path, its answer stored as policy.watermark.callsign so
// airtime never waits on a model. It is deliberately not wired while the LLM is off; with no
// stored callsign, deriveCallsign names every Plate.
type CallsignProposer interface {
	ProposeCallsign(ctx context.Context, channelName string) (string, error)
}

// callsignStopwords are skipped for initials: "The Sci-Fi Vault" is SV.
var callsignStopwords = map[string]bool{"a": true, "an": true, "and": true, "the": true, "of": true,
	"for": true, "to": true, "in": true, "on": true, "with": true, "n": true}

// deriveCallsign is the Plate's text when the channel has none, broadcast-short and deterministic:
// an acronym the name already has (TGIF), else a decade or number (80s), else the initials of its
// significant words (Saturday Cartoons: SC, at most 4), else its one word upper-cased (at most 8),
// else "CH<number>". Words keep only letters and digits, so "Sci-Fi" is one word.
func deriveCallsign(name string, number int) string {
	var words, significant []string
	for _, f := range strings.Fields(name) {
		w := strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				return r
			}
			return -1
		}, f)
		if w == "" {
			continue
		}
		words = append(words, w)
		if !callsignStopwords[strings.ToLower(w)] {
			significant = append(significant, w)
		}
	}
	// An all-caps name is shouting, not a string of acronyms.
	if strings.ToUpper(name) != name {
		for _, w := range words {
			if r := []rune(w); len(r) >= 2 && len(r) <= 5 && unicode.IsLetter(r[0]) && w == strings.ToUpper(w) {
				return w
			}
		}
	}
	for _, w := range words {
		if r := []rune(w); unicode.IsDigit(r[0]) && len(r) <= 5 {
			return w
		}
	}
	if len(significant) >= 2 {
		var initials []rune
		for _, w := range significant[:min(len(significant), 4)] {
			initials = append(initials, unicode.ToUpper([]rune(w)[0]))
		}
		return string(initials)
	}
	if len(significant) == 0 {
		significant = words
	}
	if len(significant) > 0 {
		r := []rune(strings.ToUpper(significant[0]))
		return string(r[:min(len(r), 8)])
	}
	return fmt.Sprintf("CH%d", number)
}

// cached returns the rendered bug's file for key, rendering it once.
func (c *channelWatermarks) cached(key string, render func() (watermark.Bug, error)) (string, int, int, error) {
	sum := sha256.Sum256([]byte(key))
	path := filepath.Join(c.dir, hex.EncodeToString(sum[:10])+".png")
	c.render.Lock()
	defer c.render.Unlock()
	if f, err := os.Open(path); err == nil {
		cfg, derr := png.DecodeConfig(f)
		_ = f.Close()
		if derr == nil {
			return path, cfg.Width, cfg.Height, nil
		}
	}
	bug, err := render()
	if err != nil {
		return "", 0, 0, err
	}
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return "", 0, 0, err
	}
	if err := writePNGAtomic(path, bug.Straight); err != nil {
		return "", 0, 0, err
	}
	w, h := bug.Size()
	return path, w, h, nil
}

func writePNGAtomic(path string, img image.Image) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".bug-*")
	if err != nil {
		return err
	}
	if err := png.Encode(tmp, img); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// gate returns the encoder's self-check state, starting the check on first use.
func (c *channelWatermarks) gate(enc playout.Encoder) *watermarkGate {
	c.mu.Lock()
	defer c.mu.Unlock()
	if g, ok := c.gates[enc]; ok {
		return g
	}
	g := &watermarkGate{}
	if c.gates == nil {
		c.gates = map[playout.Encoder]*watermarkGate{}
	}
	c.gates[enc] = g
	go c.check(enc, g)
	return g
}

func (c *channelWatermarks) check(enc playout.Encoder, g *watermarkGate) {
	host := playout.HostFor(enc, c.tonemap(), c.gpu())
	dir := filepath.Join(c.dir, "check-"+string(enc))
	r := playout.WatermarkCheck(c.lifetime, c.ffmpeg(), host, dir)
	_ = os.RemoveAll(dir)
	g.works.Store(r.Works)
	level, name, msg := diagnostics.LevelInfo, "watermark.overlay_verified", "Channel watermark GPU overlay verified on "+string(enc)
	if !r.Works {
		level, name = diagnostics.LevelWarn, "watermark.disabled"
		msg = "Channel watermarks are off on this host (" + string(enc) + "): " + r.Detail +
			". Watermarks are drawn only by a GPU overlay that passes its picture check, never on the CPU."
		c.log.Warn("watermark: disabled on this host", "encoder", enc, "reason", r.Detail)
	} else {
		c.log.Info("watermark: GPU overlay verified", "encoder", enc, "detail", r.Detail)
	}
	// A Diagnostics event, not a startup check: the check runs when an encoder first airs a
	// programme, and the append-only startup report must complete on a host that never does.
	if c.events != nil {
		c.events.Record(c.lifetime, diagnostics.Event{Level: level, Source: diagnostics.SourceServer, Subsystem: "playout",
			Name: name, Message: msg, Attributes: map[string]any{"encoder": string(enc), "detail": r.Detail}})
	}
}

func evenRound(v float64) int { return int(math.Round(v/2)) * 2 }
