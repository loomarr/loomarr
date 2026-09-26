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
// overlay self-check per encoder, and the programme route's per-item resolver.
//
// The self-check runs once per encoder, in the background, on the first programme that encoder
// airs: the encoder itself is only known once capability detection has run, which is lazy. Until
// the check passes, programmes air WITHOUT the bug (the safe direction: a bug never costs the
// picture). A failed check disables the watermark on the host and says so in Diagnostics.
type channelWatermarks struct {
	ffmpeg   func() string
	tonemap  func() bool
	gpu      func() playout.GPUFilters
	dir      string
	channels interface {
		GetChannel(context.Context, string) (store.Channel, error)
	}
	originals interface {
		Original(context.Context, string) (string, error)
	}
	startup  *diagnostics.Startup
	log      *slog.Logger
	lifetime context.Context

	mu     sync.Mutex
	gates  map[playout.Encoder]*watermarkGate
	render sync.Mutex // one render at a time; each is cached on disk
}

type watermarkGate struct{ works atomic.Bool }

// newChannelWatermarks wires the service from the HTTP build. Rendered bugs are derived files,
// kept beside the image store rather than inside it (the image GC owns that tree).
func newChannelWatermarks(deps httpBuild, set resolved, imgs *images.Service) *channelWatermarks {
	ffmpeg := set.str("playout.ffmpeg_path")
	c := &channelWatermarks{
		ffmpeg:   func() string { return ffmpeg },
		tonemap:  playout.TonemapperFor(ffmpeg),
		gpu:      playout.GPUFiltersFor(ffmpeg),
		dir:      filepath.Join(filepath.Dir(filepath.Clean(set.str("images.dir"))), "watermarks"),
		channels: deps.store, startup: deps.foundation.startup, log: deps.log, lifetime: deps.rootCtx,
	}
	if imgs != nil {
		c.originals = imgs
	}
	return c
}

// renderVersion changes every cached bug when the renderer's output changes.
const renderVersion = "1"

// For is the programme route's resolver: the channel's rendered bug for this output, or nil.
func (c *channelWatermarks) For(ctx context.Context, channelID string, enc playout.Encoder, width, height int) *playout.Watermark {
	if !c.gate(enc).works.Load() {
		return nil
	}
	ch, err := c.channels.GetChannel(ctx, channelID)
	if err != nil {
		return nil
	}
	res := schedule.ResolveWatermark(ch.Policy.Watermark)
	if !res.Enabled {
		return nil
	}
	look := watermark.Look{Size: res.Size, Opacity: res.Opacity, Shadow: true}
	key, render := c.source(ctx, ch, res)
	straight, pm, w, h, err := c.cached(fmt.Sprintf("%s|%d|%g|%g|%s", key, height, look.Size, look.Opacity, renderVersion),
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
	return &playout.Watermark{Straight: straight, Premultiplied: pm, Width: w, Height: h,
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
			defer f.Close()
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

// deriveCallsign is the Plate's text when the channel has none: the channel name's first word of
// two or more letters or digits, upper-cased, at most 8 characters; else "CH<number>".
func deriveCallsign(name string, number int) string {
	for _, word := range strings.FieldsFunc(name, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if r := []rune(strings.ToUpper(word)); len(r) >= 2 {
			return string(r[:min(len(r), 8)])
		}
	}
	return fmt.Sprintf("CH%d", number)
}

// cached returns the rendered bug's two files for key, rendering them once.
func (c *channelWatermarks) cached(key string, render func() (watermark.Bug, error)) (string, string, int, int, error) {
	sum := sha256.Sum256([]byte(key))
	base := filepath.Join(c.dir, hex.EncodeToString(sum[:10]))
	straight, pm := base+".png", base+".pm.png"
	c.render.Lock()
	defer c.render.Unlock()
	if f, err := os.Open(straight); err == nil {
		cfg, derr := png.DecodeConfig(f)
		_ = f.Close()
		if _, serr := os.Stat(pm); derr == nil && serr == nil {
			return straight, pm, cfg.Width, cfg.Height, nil
		}
	}
	bug, err := render()
	if err != nil {
		return "", "", 0, 0, err
	}
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return "", "", 0, 0, err
	}
	for path, img := range map[string]image.Image{pm: bug.Premultiplied, straight: bug.Straight} {
		if err := writePNGAtomic(path, img); err != nil {
			return "", "", 0, 0, err
		}
	}
	w, h := bug.Size()
	return straight, pm, w, h, nil
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
	status, detail := diagnostics.StartupPassed, "GPU overlay verified on "+string(enc)+": "+r.Detail
	if !r.Works {
		status = diagnostics.StartupWarning
		detail = "Channel watermarks are off on this host (" + string(enc) + "): " + r.Detail +
			". Watermarks are drawn only by a GPU overlay that passes its picture check, never on the CPU."
		c.log.Warn("watermark: disabled on this host", "encoder", enc, "reason", r.Detail)
	} else {
		c.log.Info("watermark: GPU overlay verified", "encoder", enc, "detail", r.Detail)
	}
	if c.startup != nil {
		c.startup.Complete(diagnostics.StartupCheckWatermark, status, detail, "/settings/system/playback", "")
	}
}

func evenRound(v float64) int { return int(math.Round(v/2)) * 2 }
