package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"maps"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	_ "golang.org/x/image/webp" // custom watermark uploads may be WebP

	"github.com/loomarr/loomarr/internal/api"
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
	// install is the live install-wide opacity and look (watermarkInstall), read on every For.
	install func() schedule.WatermarkInstall
	// calendar is the channels' holiday calendar (householdCalendar, the engine's too), so a
	// holiday version flips at the same local instant as the schedule's holiday. nil = the zero
	// Calendar (UTC, the US).
	calendar func() schedule.Calendar
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

// watermarkGate is an encoder's self-check verdict: works draws the bug at all, colour draws the
// coloured looks in colour (otherwise they air in white, #1659 Q2).
type watermarkGate struct{ works, colour atomic.Bool }

// newChannelWatermarks builds the service before the channel packager that asks it for bugs. The
// image service is built later and bound with withOriginals. Rendered bugs are derived files, kept
// beside the image store rather than inside it (the image GC owns that tree).
func newChannelWatermarks(rootCtx context.Context, st store.Store, set resolved, calendar func() schedule.Calendar, events *diagnostics.Recorder, log *slog.Logger) *channelWatermarks {
	ffmpeg := set.str("playout.ffmpeg_path")
	c := &channelWatermarks{
		ffmpeg:   func() string { return ffmpeg },
		tonemap:  playout.TonemapperFor(ffmpeg),
		gpu:      playout.GPUFiltersFor(ffmpeg),
		dir:      filepath.Join(filepath.Dir(filepath.Clean(set.str("images.dir"))), "watermarks"),
		install:  watermarkInstall(set),
		calendar: calendar,
		channels: st, log: log, lifetime: rootCtx,
	}
	if events != nil {
		c.events = events
	}
	return c
}

// watermarkInstall is the install-wide bug settings: playout.watermark_opacity_pct as a fraction
// and playout.watermark_look, resolved per call so a change applies to the next programme item
// without a restart (the rendered-bug cache is keyed by both, so a new value re-renders).
func watermarkInstall(set resolved) func() schedule.WatermarkInstall {
	return func() schedule.WatermarkInstall {
		return schedule.WatermarkInstall{Opacity: float64(set.intv("playout.watermark_opacity_pct")) / 100,
			Look: set.str("playout.watermark_look")}
	}
}

// withOriginals binds the image service a custom upload is read from. The build calls it before the
// server takes a request, so no tune can race it; unbound, a channel's bug is the generated Plate.
func (c *channelWatermarks) withOriginals(imgs *images.Service) {
	if imgs != nil {
		c.originals = imgs
	}
}

// holidayCalendar is the calendar holiday versions are dated in; unbound (tests), the zero one.
func (c *channelWatermarks) holidayCalendar() schedule.Calendar {
	if c.calendar == nil {
		return schedule.Calendar{}
	}
	return c.calendar()
}

// renderVersion changes every cached bug when the renderer's output changes.
const renderVersion = "1"

// For is the channel packager's resolver: the channel's rendered bug for this output, or nil. at is
// the airing's start (playout.Airing.StartedAt), which picks the holiday version: stable across a
// retried open and a mid-programme tune-in, so the version changes only at a programme boundary.
func (c *channelWatermarks) For(ctx context.Context, channelID string, enc playout.Encoder, width, height int, at time.Time) *playout.Watermark {
	gate := c.gate(enc)
	if !gate.works.Load() {
		return nil
	}
	ch, err := c.channels.GetChannel(ctx, channelID)
	if err != nil {
		return nil
	}
	res := schedule.ResolveWatermark(ch.Policy.Watermark, c.install())
	if !res.Enabled {
		return nil
	}
	look := watermark.Look{Size: res.Size, Opacity: res.Opacity, Shadow: true}
	holiday, motif := schedule.WatermarkHoliday(ch.Policy.Watermark, res, ch.Policy.Seasonal, at, c.holidayCalendar())
	key, render := c.source(ctx, ch, res, bugDesign{holiday: holiday, motif: watermark.Motif(motif),
		decade: schedule.WatermarkEraDecade(ch.Policy.Scope), white: !gate.colour.Load()})
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

// previewFrameHeight is the frame a preview bug is drawn for: 1080p, the size most programmes air.
const previewFrameHeight = 1080

// Preview is the bug a channel airs with its saved policy and the given changes (#1659 W2, the
// channel page's On-screen logo preview): the same source, colours, holiday version, cache and PNG
// as For, drawn for a 1080-line frame. holiday is a calendar holiday's id to show its version, or ""
// for the everyday logo. drawable is false while no encoder's self-check has passed, when
// programmes air without the bug.
func (c *channelWatermarks) Preview(ctx context.Context, channelID string, change schedule.WatermarkPolicy, holiday string) (path string, drawable bool, err error) {
	ch, err := c.channels.GetChannel(ctx, channelID)
	if err != nil {
		return "", false, err
	}
	p := schedule.WatermarkPolicy{}
	if ch.Policy.Watermark != nil {
		p = *ch.Policy.Watermark
	}
	if change.Look != "" {
		p.Look = change.Look
	}
	if change.Color != "" {
		p.Color = change.Color
	}
	if change.Size != nil {
		p.Size = change.Size
	}
	if len(change.Motifs) > 0 {
		p.Motifs = maps.Clone(p.Motifs)
		if p.Motifs == nil {
			p.Motifs = map[string]string{}
		}
		maps.Copy(p.Motifs, change.Motifs)
	}
	res := schedule.ResolveWatermark(&p, c.install())
	drawable, colour := c.drawable()
	design := bugDesign{decade: schedule.WatermarkEraDecade(ch.Policy.Scope), white: drawable && !colour}
	if motif := schedule.WatermarkMotif(&p, holiday); motif != "" && res.Image == "" {
		design.holiday, design.motif = holiday, watermark.Motif(motif)
	}
	look := watermark.Look{Size: res.Size, Opacity: res.Opacity, Shadow: true}
	key, render := c.source(ctx, ch, res, design)
	path, _, _, err = c.cached(fmt.Sprintf("%s|%d|%g|%g|%s", key, previewFrameHeight, look.Size, look.Opacity, renderVersion),
		func() (watermark.Bug, error) {
			mask, err := render()
			if err != nil {
				return watermark.Bug{}, err
			}
			return mask(previewFrameHeight, look)
		})
	return path, drawable, err
}

// motifChipPx is the picture-only preview's square: the channel page's 24 px chip icon at 2×.
const motifChipPx = 48

// PreviewMotif is the holiday's picture alone, as the channel's holiday bug carries it (#1659 W2f,
// the picture chips beside each honoured holiday): picture is one of the holiday's own, or "" for
// the channel's choice. White follows the host's coloured self-check, like Preview.
func (c *channelWatermarks) PreviewMotif(ctx context.Context, channelID, holiday, picture string) (string, error) {
	ch, err := c.channels.GetChannel(ctx, channelID)
	if err != nil {
		return "", err
	}
	p := schedule.WatermarkPolicy{}
	if ch.Policy.Watermark != nil {
		p = *ch.Policy.Watermark
	}
	if picture != "" {
		p.Motifs = map[string]string{holiday: picture}
	}
	motif := schedule.WatermarkMotif(&p, holiday)
	if motif == "" {
		return "", fmt.Errorf("watermark: holiday %q has no picture", holiday)
	}
	drawable, colour := c.drawable()
	white := drawable && !colour
	path, _, _, err := c.cached(fmt.Sprintf("motif|%s|%t|%d|%s", motif, white, motifChipPx, renderVersion),
		func() (watermark.Bug, error) {
			img, err := watermark.RenderMotif(watermark.Motif(motif), white, motifChipPx)
			return watermark.Bug{Straight: img}, err
		})
	return path, err
}

// Options is what the channel page's On-screen logo section shows beside the choices at now (#1659
// W2c): the derived name, the era and the colours "Match the era" uses, and the holidays the logo
// follows (those with a picture), each in effect through the scheduler's own ActiveHolidayAt so the
// page and what airs agree.
func (c *channelWatermarks) Options(ctx context.Context, channelID string, now time.Time) (api.WatermarkOptions, error) {
	ch, err := c.channels.GetChannel(ctx, channelID)
	if err != nil {
		return api.WatermarkOptions{}, err
	}
	decade := schedule.WatermarkEraDecade(ch.Policy.Scope)
	out := api.WatermarkOptions{AutoCallsign: deriveCallsign(ch.Name, ch.Number), Holidays: []api.WatermarkHolidayOption{}}
	if decade > 0 {
		out.Era = fmt.Sprintf("%ds", decade)
	}
	for _, col := range watermark.EraPalette(decade) {
		out.EraPalette = append(out.EraPalette, string(col))
	}
	cal := c.holidayCalendar()
	names := map[string]string{}
	for _, d := range cal.Vocab().Definitions() {
		names[d.ID] = d.Label
	}
	active := schedule.ActiveHolidayAt(now, ch.Policy.Seasonal, cal)
	for _, w := range schedule.HonouredHolidays(now, ch.Policy.Seasonal, cal) {
		if len(schedule.WatermarkMotifs[w.ID]) == 0 {
			continue // no holiday version to show: the logo airs plain through it
		}
		h := api.WatermarkHolidayOption{ID: w.ID, Name: names[w.ID], Start: w.Start.Format(time.DateOnly),
			End: w.End.Format(time.DateOnly), Active: w.ID == active, Picture: schedule.WatermarkMotif(ch.Policy.Watermark, w.ID)}
		for _, m := range schedule.WatermarkMotifs[w.ID] {
			h.Pictures = append(h.Pictures, api.WatermarkPicture{ID: m, Name: schedule.WatermarkMotifNames[m]})
		}
		out.Holidays = append(out.Holidays, h)
	}
	return out, nil
}

// drawable reports whether any encoder's self-check has passed, and whether one draws colour. No
// encoder checked yet (nothing has aired since start) counts as drawable, in colour.
func (c *channelWatermarks) drawable() (works, colour bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.gates) == 0 {
		return true, true
	}
	for _, g := range c.gates {
		works = works || g.works.Load()
		colour = colour || (g.works.Load() && g.colour.Load())
	}
	return works, colour
}

type bugRenderer func(frameHeight int, look watermark.Look) (watermark.Bug, error)

// bugDesign is what a generated bug takes beyond the policy: the holiday version at the airing's
// start, the channel's era, and whether the host draws colour.
type bugDesign struct {
	holiday string
	motif   watermark.Motif
	decade  int
	white   bool
}

// colours is the design's fill and palette (#1659 Q8): while a holiday is on its colours and palette
// apply whatever colour is chosen (the approved mock's holOpts); otherwise "era" is the era's
// (amber without one) and a named colour fills the look, with the Era stripe keeping the era palette.
func (b bugDesign) colours(color string) (watermark.Colour, watermark.Palette) {
	if h, ok := watermark.HolidayLooks[b.holiday]; ok {
		return h.Colour, h.Palette
	}
	pal := watermark.EraPalette(b.decade)
	fill := pal[0]
	if color != schedule.WatermarkDefaultColor {
		fill = watermark.Colour(color)
	}
	return fill, pal
}

// source picks the bug's image: a custom upload (its own shape; the look does not apply, and no
// holiday version), else the generated callsign bug in the resolved look, colour and holiday
// version. The automatic TMDB network logo (between the two in the maintainer's order) is not
// wired yet.
func (c *channelWatermarks) source(ctx context.Context, ch store.Channel, res schedule.ResolvedWatermark, design bugDesign) (string, func() (bugRenderer, error)) {
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
	style := watermark.Style(res.Look)
	fill, pal := design.colours(res.Color)
	d := watermark.Design{Style: style, Callsign: call, Number: ch.Number, Colour: fill, Palette: pal,
		Motif: design.motif, White: design.white}
	// The key names only what the look draws, so a built look with no holiday keeps its cached bug.
	key := "callsign:" + res.Look + ":" + call
	if style.Coloured() {
		key += fmt.Sprintf("|%s:%s:%s:%s:%s:white=%t", fill, pal[0], pal[1], pal[2], res.Color, design.white)
	}
	if style == watermark.StyleTwotone || style == watermark.StyleBadge || style == watermark.StyleLockup {
		key += fmt.Sprintf("|n%d", ch.Number)
	}
	if design.motif != "" {
		key += fmt.Sprintf("|hol:%s:%s:white=%t", design.holiday, design.motif, design.white)
	}
	return key, func() (bugRenderer, error) {
		return func(h int, look watermark.Look) (watermark.Bug, error) { return watermark.RenderDesign(d, h, look) }, nil
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
	g.colour.Store(r.Colour)
	g.works.Store(r.Works)
	level, name, msg := diagnostics.LevelInfo, "watermark.overlay_verified", "Channel watermark GPU overlay verified on "+string(enc)
	switch {
	case r.Works && !r.Colour:
		// Logged once, here: the gate keeps the verdict, so every later bug is simply white.
		level, name = diagnostics.LevelWarn, "watermark.colour_disabled"
		msg = "Coloured on-screen logos air in white on this host (" + string(enc) + "): " + r.ColourDetail +
			". The white logo passed its picture check, so every programme still carries one."
		c.log.Warn("watermark: coloured looks air in white on this host", "encoder", enc, "reason", r.ColourDetail)
	case !r.Works:
		level, name = diagnostics.LevelWarn, "watermark.disabled"
		msg = "Channel watermarks are off on this host (" + string(enc) + "): " + r.Detail +
			". Watermarks are drawn only by a GPU overlay that passes its picture check, never on the CPU."
		c.log.Warn("watermark: disabled on this host", "encoder", enc, "reason", r.Detail)
	default:
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
