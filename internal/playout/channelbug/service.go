// Package channelbug turns a channel's watermark policy into the bug its programmes air with: it
// picks the look, colour and holiday version, renders through playout/watermark, caches the files,
// and runs the per-encoder GPU overlay self-check that gates the bug. It lives apart from the
// renderer because it drives internal/playout, which tests the renderer.
package channelbug

import (
	"context"
	"fmt"
	"image"
	"log/slog"
	"maps"
	"math"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	_ "golang.org/x/image/webp" // custom watermark uploads may be WebP

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/platform/diagnostics"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/playout/watermark"
)

// Service is the channel-bug service (#1512): the GPU overlay self-check per encoder, and
// the channel packager's per-item resolver.
//
// The self-check runs once per encoder, in the background, on the first programme that encoder
// airs: the encoder itself is only known once capability detection has run, which is lazy. Until
// the check passes, programmes air WITHOUT the bug (the safe direction: a bug never costs the
// picture). A failed check disables the watermark on the host and says so in Diagnostics.
type Service struct {
	ffmpeg  func() string
	tonemap func() bool
	gpu     func() encode.GPUFilters
	dir     string
	// install is the live install-wide opacity and look, read on every For.
	install func() schedule.WatermarkInstall
	// calendar is the channels' holiday calendar (the engine's too), so a holiday version flips at
	// the same local instant as the schedule's holiday. nil = the zero Calendar (UTC, the US).
	calendar  func() schedule.Calendar
	channels  ChannelReader
	originals Originals
	events    EventRecorder
	log       *slog.Logger
	lifetime  context.Context

	mu     sync.Mutex
	gates  map[encode.Encoder]*watermarkGate
	render sync.Mutex // one aired render at a time; each is cached on disk
	// preview serialises the channel page's previews apart from render, so a slow preview never
	// delays a tune (#1949). Previews live in their own pruned directory, never sharing a file.
	preview sync.Mutex

	kernelOnce sync.Once
	kernel     string // the VAAPI family's blend kernel beside the bugs, written on first use
}

// blendKernel is the path of the VAAPI blend kernel, or "" when it could not be written: the VAAPI
// graph then airs the programme bug-free and says why.
func (c *Service) blendKernel() string {
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

// ChannelReader is the one store read the service makes.
type ChannelReader interface {
	GetChannel(context.Context, string) (channelstore.Channel, error)
}

// Originals reads a custom upload's original file by image id.
type Originals interface {
	Original(context.Context, string) (string, error)
}

// EventRecorder receives the Diagnostics events the self-check raises.
type EventRecorder interface {
	Record(context.Context, diagnostics.Event)
}

// Config is what New builds a Service from. Rendered bugs are derived files: Dir is kept beside the
// image store rather than inside it (the image GC owns that tree).
type Config struct {
	// FFmpeg is the binary the overlay self-check runs.
	FFmpeg string
	// Dir holds the rendered bugs and the self-check scratch directories.
	Dir string
	// Install is the install-wide opacity and look, resolved per call so a change applies to the next
	// programme item without a restart (the rendered-bug cache is keyed by both).
	Install func() schedule.WatermarkInstall
	// Calendar is the channels' holiday calendar; nil is the zero Calendar.
	Calendar func() schedule.Calendar
	Channels ChannelReader
	// Events is optional; nil records no Diagnostics events.
	Events EventRecorder
	Log    *slog.Logger
	// Lifetime bounds the background self-check; it is the application's lifetime.
	Lifetime context.Context
}

// New builds the service before the channel packager that asks it for bugs. A custom-upload reader
// is bound later with SetOriginals.
func New(cfg Config) *Service {
	c := &Service{
		ffmpeg:   func() string { return cfg.FFmpeg },
		tonemap:  playout.TonemapperFor(cfg.FFmpeg),
		gpu:      playout.GPUFiltersFor(cfg.FFmpeg),
		dir:      cfg.Dir,
		install:  cfg.Install,
		calendar: cfg.Calendar,
		channels: cfg.Channels, log: cfg.Log, lifetime: cfg.Lifetime,
	}
	if cfg.Events != nil {
		c.events = cfg.Events
	}
	return c
}

// SetOriginals binds the reader a custom upload is read from. The build calls it before the server
// takes a request, so no tune can race it; unbound, a channel's bug is the generated Plate.
func (c *Service) SetOriginals(originals Originals) { c.originals = originals }

// SetGateResult records an encoder's self-check verdict without running the check. It is the seam
// for tests that need a passed or failed gate without ffmpeg.
func (c *Service) SetGateResult(enc encode.Encoder, works, colour bool) {
	g := &watermarkGate{}
	g.works.Store(works)
	g.colour.Store(colour)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gates == nil {
		c.gates = map[encode.Encoder]*watermarkGate{}
	}
	c.gates[enc] = g
}

// holidayCalendar is the calendar holiday versions are dated in; unbound (tests), the zero one.
func (c *Service) holidayCalendar() schedule.Calendar {
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
func (c *Service) For(ctx context.Context, channelID string, enc encode.Encoder, width, height int, at time.Time) *playout.Watermark {
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

// previewKeep bounds the preview cache: the most recently used previews kept on disk.
const previewKeep = 64

// Preview is the bug a channel airs with its saved policy and the given changes (#1659 W2, the
// channel page's On-screen logo preview): the same source, colours, holiday version and PNG as For,
// drawn for a 1080-line frame into the bounded preview cache. holiday is a calendar holiday's id to
// show its version, or "" for the everyday logo. drawable is false while no encoder's self-check has
// passed, when programmes air without the bug. The logo is white when any working encoder airs it
// white (#1949): For whitens per encoder, and the channel may air on that one.
func (c *Service) Preview(ctx context.Context, channelID string, change schedule.WatermarkPolicy, holiday string) (path string, drawable bool, err error) {
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
	drawable, white := c.drawable()
	design := bugDesign{decade: schedule.WatermarkEraDecade(ch.Policy.Scope), white: white}
	if motif := schedule.WatermarkMotif(&p, holiday); motif != "" && res.Image == "" {
		design.holiday, design.motif = holiday, watermark.Motif(motif)
	}
	look := watermark.Look{Size: res.Size, Opacity: res.Opacity, Shadow: true}
	key, render := c.source(ctx, ch, res, design)
	path, err = c.cachedPreview(fmt.Sprintf("%s|%d|%g|%g|%s", key, previewFrameHeight, look.Size, look.Opacity, renderVersion),
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
// the channel's choice. White follows the encoders' coloured self-checks, like Preview.
func (c *Service) PreviewMotif(ctx context.Context, channelID, holiday, picture string) (string, error) {
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
	_, white := c.drawable()
	return c.cachedPreview(fmt.Sprintf("motif|%s|%t|%d|%s", motif, white, motifChipPx, renderVersion),
		func() (watermark.Bug, error) {
			img, err := watermark.RenderMotif(watermark.Motif(motif), white, motifChipPx)
			return watermark.Bug{Straight: img}, err
		})
}

// ChannelOptions is what the channel page's On-screen logo section shows beside the choices.
type ChannelOptions struct {
	// AutoCallsign is the text the logo shows when the name is left blank.
	AutoCallsign string
	// Era is the channel's decade such as 1980s, empty when it has none.
	Era string
	// EraPalette is the colours "Match the era" uses, in order.
	EraPalette []string
	// Holidays are the holidays the channel's logo follows, in calendar order.
	Holidays []HolidayOption
}

// HolidayOption is one holiday the channel's logo follows.
type HolidayOption struct {
	ID, Name string
	// Start and End are the dates of the holiday's window that is on today or comes next.
	Start, End string
	// Active reports the holiday is in effect today for this channel.
	Active bool
	// Picture is the picture this channel shows for the holiday; Pictures are its choices, the
	// default first.
	Picture  string
	Pictures []HolidayPicture
}

// HolidayPicture is a holiday picture's id and plain name.
type HolidayPicture struct{ ID, Name string }

// Options is what the channel page's On-screen logo section shows beside the choices at now (#1659
// W2c): the derived name, the era and the colours "Match the era" uses, and the holidays the logo
// follows (those with a picture), each in effect through the scheduler's own ActiveHolidayAt so the
// page and what airs agree.
func (c *Service) Options(ctx context.Context, channelID string, now time.Time) (ChannelOptions, error) {
	ch, err := c.channels.GetChannel(ctx, channelID)
	if err != nil {
		return ChannelOptions{}, err
	}
	decade := schedule.WatermarkEraDecade(ch.Policy.Scope)
	out := ChannelOptions{AutoCallsign: watermark.DeriveCallsign(ch.Name, ch.Number), Holidays: []HolidayOption{}}
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
		h := HolidayOption{ID: w.ID, Name: names[w.ID], Start: w.Start.Format(time.DateOnly),
			End: w.End.Format(time.DateOnly), Active: w.ID == active, Picture: schedule.WatermarkMotif(ch.Policy.Watermark, w.ID)}
		for _, m := range schedule.WatermarkMotifs[w.ID] {
			h.Pictures = append(h.Pictures, HolidayPicture{ID: m, Name: schedule.WatermarkMotifNames[m]})
		}
		out.Holidays = append(out.Holidays, h)
	}
	return out, nil
}

// drawable reports whether any encoder's self-check has passed, and whether any working encoder
// draws the bug white (its coloured check failed). No encoder checked yet (nothing has aired since
// start) counts as drawable, in colour.
func (c *Service) drawable() (works, white bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.gates) == 0 {
		return true, false
	}
	for _, g := range c.gates {
		if g.works.Load() {
			works = true
			white = white || !g.colour.Load()
		}
	}
	return works, white
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
// version. There is no automatic TMDB network logo step between the two.
func (c *Service) source(ctx context.Context, ch channelstore.Channel, res schedule.ResolvedWatermark, design bugDesign) (string, func() (bugRenderer, error)) {
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
		call = watermark.DeriveCallsign(ch.Name, ch.Number)
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

// cached returns the aired bug's file for key, rendering it once.
func (c *Service) cached(key string, render func() (watermark.Bug, error)) (string, int, int, error) {
	c.render.Lock()
	defer c.render.Unlock()
	path, w, h, _, err := watermark.CacheIn(c.dir, key, render)
	return path, w, h, err
}

// cachedPreview returns the preview's file for key from the preview cache, rendering it once under
// the preview lock (never the tune's). A hit marks the file used; a new file prunes the cache to the
// previewKeep most recently used.
func (c *Service) cachedPreview(key string, render func() (watermark.Bug, error)) (string, error) {
	dir := filepath.Join(c.dir, "preview")
	c.preview.Lock()
	defer c.preview.Unlock()
	path, _, _, hit, err := watermark.CacheIn(dir, key, render)
	if err != nil {
		return "", err
	}
	if hit {
		now := time.Now()
		_ = os.Chtimes(path, now, now)
		return path, nil
	}
	watermark.PruneLeastRecentlyUsed(dir, previewKeep, path, c.log)
	return path, nil
}

// gate returns the encoder's self-check state, starting the check on first use.
func (c *Service) gate(enc encode.Encoder) *watermarkGate {
	c.mu.Lock()
	defer c.mu.Unlock()
	if g, ok := c.gates[enc]; ok {
		return g
	}
	g := &watermarkGate{}
	if c.gates == nil {
		c.gates = map[encode.Encoder]*watermarkGate{}
	}
	c.gates[enc] = g
	go c.check(enc, g)
	return g
}

func (c *Service) check(enc encode.Encoder, g *watermarkGate) {
	host := encode.HostFor(enc, c.tonemap(), c.gpu())
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
