package app

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/diagnostics"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/settings"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/watermark"
)

func testWatermarks(t *testing.T, ch store.Channel, gatePassed bool) *channelWatermarks {
	return testWatermarksWith(t, visionSet(t, nil), ch, gatePassed)
}

// testWatermarksWith reads the install-wide opacity from set, a live settings service over the
// declared registry, the way the build wires it.
func testWatermarksWith(t *testing.T, set resolved, ch store.Channel, gatePassed bool) *channelWatermarks {
	g := &watermarkGate{}
	g.works.Store(gatePassed)
	return &channelWatermarks{dir: t.TempDir(), channels: staticChannelReader{channel: ch}, log: slog.New(slog.DiscardHandler),
		install: watermarkInstall(set), gates: map[playout.Encoder]*watermarkGate{playout.EncoderNVENC: g}}
}

// sameBug fails unless the aired PNG at path is exactly the renderer's bug for the callsign in the
// style at 1080 lines, the default size and opacity.
func sameBug(t *testing.T, path, callsign string, style watermark.Style, opacity float64) {
	t.Helper()
	mask, err := watermark.CallsignMask(callsign, style)
	if err != nil {
		t.Fatal(err)
	}
	want := watermark.Render(mask, 1080, style.Adjust(watermark.Look{Size: schedule.WatermarkDefaultSize, Opacity: opacity, Shadow: true})).Straight
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(f)
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	got := image.NewNRGBA(img.Bounds())
	draw.Draw(got, got.Rect, img, img.Bounds().Min, draw.Src)
	if got.Rect.Size() != want.Rect.Size() || !bytes.Equal(got.Pix, want.Pix) {
		t.Errorf("aired bug %v is not the %s bug %v", got.Rect.Size(), style, want.Rect.Size())
	}
}

// THE LOOK (#1617): playout.watermark_look (Text by default) styles every channel's generated bug
// unless the channel overrides it, and a change applies to the next bug asked for.
func TestChannelWatermarks_LookFollowsTheInstallSettingLive(t *testing.T) {
	name, number := "Retro Cartoons", 7
	call := deriveCallsign(name, number)
	set := visionSet(t, nil)
	c := testWatermarksWith(t, set, store.Channel{Channel: schedule.Channel{Name: name, Number: number}}, true)
	wm := c.For(context.Background(), "ch", playout.EncoderNVENC, 1920, 1080, offSeason)
	if wm == nil {
		t.Fatal("no bug")
	}
	sameBug(t, wm.Straight, call, watermark.StyleText, 0.40)

	set.svc.SetDB(map[string]string{"playout.watermark_look": "plate"})
	again := c.For(context.Background(), "ch", playout.EncoderNVENC, 1920, 1080, offSeason)
	if again == nil || again.Straight == wm.Straight {
		t.Fatalf("a changed look did not re-render the bug: %+v", again)
	}
	sameBug(t, again.Straight, call, watermark.StylePlate, 0.40)

	ch := store.Channel{Channel: schedule.Channel{Name: name, Number: number}}
	ch.Policy.Watermark = &schedule.WatermarkPolicy{Look: "small-plate"}
	over := testWatermarksWith(t, set, ch, true).For(context.Background(), "ch", playout.EncoderNVENC, 1920, 1080, offSeason)
	sameBug(t, over.Straight, call, watermark.StyleSmallPlate, 0.40)
}

// ONE LIST, THREE COPIES: the renderer's styles, the policy's looks and the setting's options are
// declared in three packages that must not import each other; this pins them equal.
func TestWatermarkLooks_AgreeAcrossRendererPolicyAndSetting(t *testing.T) {
	var styles []string
	for _, s := range watermark.DesignStyles {
		styles = append(styles, string(s))
	}
	s, ok := settings.NewRegistry().Get("playout.watermark_look")
	if !ok {
		t.Fatal("playout.watermark_look not declared")
	}
	var options []string
	for _, o := range s.Enum {
		options = append(options, o.Value)
	}
	if !slices.Equal(styles, schedule.WatermarkLooks) || !slices.Equal(options, schedule.WatermarkLooks) {
		t.Errorf("renderer %v, policy %v, setting %v", styles, schedule.WatermarkLooks, options)
	}
	field, _ := reflect.TypeFor[schedule.WatermarkPolicy]().FieldByName("Look")
	if tag := field.Tag.Get("enum"); tag != strings.Join(schedule.WatermarkLooks, ",") {
		t.Errorf("policy enum tag %q, want %v", tag, schedule.WatermarkLooks)
	}
}

// peakWhite is a rendered bug's peak premultiplied white out of 255: its opacity (the shadow beneath
// adds alpha, not light).
func peakWhite(t *testing.T, path string) uint32 {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("rendered file missing: %v", err)
	}
	img, err := png.Decode(f)
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	var peak uint32
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			peak = max(peak, (uint32(c.R)*uint32(c.A)+127)/255)
		}
	}
	return peak
}

// THE INSTALL SETTING (#1617): playout.watermark_opacity_pct is every channel's opacity unless the
// channel overrides it, and a change applies to the next bug asked for, without a restart.
func TestChannelWatermarks_OpacityFollowsTheInstallSettingLive(t *testing.T) {
	set := visionSet(t, map[string]string{"playout.watermark_opacity_pct": "55"})
	c := testWatermarksWith(t, set, store.Channel{Channel: schedule.Channel{Name: "Retro Cartoons", Number: 7}}, true)
	wm := c.For(context.Background(), "ch", playout.EncoderNVENC, 1920, 1080, offSeason)
	if wm == nil {
		t.Fatal("no bug")
	}
	if got, want := peakWhite(t, wm.Straight), uint32(140); got < want-1 || got > want+1 { // 0.55 × 255
		t.Errorf("bug peaks at %d/255, want %d (the install setting, 55%%)", got, want)
	}
	set.svc.SetDB(map[string]string{"playout.watermark_opacity_pct": "70"})
	again := c.For(context.Background(), "ch", playout.EncoderNVENC, 1920, 1080, offSeason)
	if again == nil || again.Straight == wm.Straight {
		t.Fatalf("a changed setting did not re-render the bug: %+v", again)
	}
	if got, want := peakWhite(t, again.Straight), uint32(179); got < want-1 || got > want+1 { // 0.70 × 255
		t.Errorf("after the change the bug peaks at %d/255, want %d", got, want)
	}
	// The channel's own opacity overrides the install setting.
	own := 0.25
	ch := store.Channel{Channel: schedule.Channel{Name: "Retro Cartoons", Number: 7}}
	ch.Policy.Watermark = &schedule.WatermarkPolicy{Opacity: &own}
	over := testWatermarksWith(t, set, ch, true).For(context.Background(), "ch", playout.EncoderNVENC, 1920, 1080, offSeason)
	if got, want := peakWhite(t, over.Straight), uint32(64); got < want-1 || got > want+1 { // 0.25 × 255
		t.Errorf("channel override: bug peaks at %d/255, want %d", got, want)
	}
}

// ON BY DEFAULT: a channel with no watermark policy gets the generated bug at the approved placement.
func TestChannelWatermarks_DefaultBug(t *testing.T) {
	c := testWatermarks(t, store.Channel{Channel: schedule.Channel{Name: "Retro Cartoons", Number: 7}}, true)
	wm := c.For(context.Background(), "ch", playout.EncoderNVENC, 1920, 1080, offSeason)
	if wm == nil {
		t.Fatal("no bug for a channel on the defaults")
	}
	if wm.Corner != playout.CornerTopRight || wm.MarginX != 96 || wm.MarginY != 54 || wm.Width%2 != 0 || wm.Height%2 != 0 {
		t.Errorf("placement %+v", wm)
	}
	// The install setting's default, 40% (#1617), baked into the aired PNG.
	if peak, want := peakWhite(t, wm.Straight), uint32(102); peak < want-1 || peak > want+1 { // 0.40 × 255
		t.Errorf("default bug peaks at %d/255, want %d (opacity 0.40)", peak, want)
	}
	// Cached: the second ask returns the same files without re-rendering.
	again := c.For(context.Background(), "ch", playout.EncoderNVENC, 1920, 1080, offSeason)
	if again == nil || again.Straight != wm.Straight {
		t.Errorf("not cached: %+v vs %+v", again, wm)
	}
}

func TestChannelWatermarks_OffOrUnverifiedMeansNoBug(t *testing.T) {
	off := false
	ch := store.Channel{Channel: schedule.Channel{Name: "Kids", Number: 3}}
	ch.Policy.Watermark = &schedule.WatermarkPolicy{Enabled: &off}
	if wm := testWatermarks(t, ch, true).For(context.Background(), "ch", playout.EncoderNVENC, 1920, 1080, offSeason); wm != nil {
		t.Error("a channel that turned the bug off got one")
	}
	ch.Policy.Watermark = nil
	if wm := testWatermarks(t, ch, false).For(context.Background(), "ch", playout.EncoderNVENC, 1920, 1080, offSeason); wm != nil {
		t.Error("a host whose overlay has not passed its self-check got a bug")
	}
}

type recordedEvents struct{ got []diagnostics.Event }

func (r *recordedEvents) Record(_ context.Context, e diagnostics.Event) { r.got = append(r.got, e) }

// NO SILENT DROP (#1595): a host whose overlay fails its self-check (a wrong picture, or too slow
// to air) keeps every programme bug-free, and the Diagnostics event says so with the check's own
// reason, which is where the UI shows it.
func TestChannelWatermarks_FailedCheckIsAWarningWithItsReason(t *testing.T) {
	ffmpeg := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(ffmpeg, []byte("#!/bin/sh\necho 'no such encoder' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	events := &recordedEvents{}
	c := &channelWatermarks{dir: t.TempDir(), channels: staticChannelReader{channel: store.Channel{}}, log: slog.New(slog.DiscardHandler),
		ffmpeg: func() string { return ffmpeg }, tonemap: func() bool { return false }, gpu: func() playout.GPUFilters { return playout.GPUFilters{} },
		events: events, lifetime: t.Context()}
	g := &watermarkGate{}
	c.check(playout.EncoderVAAPI, g)

	if g.works.Load() {
		t.Fatal("a failed self-check left the watermark on")
	}
	if len(events.got) != 1 {
		t.Fatalf("%d Diagnostics events, want 1: %+v", len(events.got), events.got)
	}
	e := events.got[0]
	detail, _ := e.Attributes["detail"].(string)
	if e.Level != diagnostics.LevelWarn || e.Name != "watermark.disabled" || !strings.Contains(detail, "no such encoder") ||
		!strings.Contains(e.Message, detail) || e.Attributes["encoder"] != string(playout.EncoderVAAPI) {
		t.Errorf("the event does not say why the watermark is off: %+v", e)
	}
}

// The Plate's callsign is broadcast-short, deterministic from the channel name (the issue's LLM
// pick is off): an acronym the name already has, else a decade or number, else the initials of
// its significant words, else its one word.
func TestDeriveCallsign(t *testing.T) {
	for name, want := range map[string]string{
		"Saturday Cartoons":                  "SC",
		"Retro Cartoons":                     "RC",
		"TGIF Sitcoms":                       "TGIF",
		"80s Horror":                         "80s",
		"The Sci-Fi Vault":                   "SV",
		"A Midnight Movies":                  "MM",
		"Movies & More":                      "MM",
		"Friday Night Horror Movie Marathon": "FNHM",
		"Ciné Club":                          "CC",
		"RETRO CARTOONS":                     "RC",
		"Westerns":                           "WESTERNS",
		"Supercalifragilistic":               "SUPERCAL",
		"The":                                "THE",
		"!":                                  "CH4",
	} {
		if got := deriveCallsign(name, 4); got != want {
			t.Errorf("%q: %q, want %q", name, got, want)
		}
	}
}

// offSeason is an airing start with no calendar holiday on.
var offSeason = time.Date(2026, time.July, 4, 20, 0, 0, 0, time.UTC)

// airedPixels decodes an aired bug PNG to NRGBA.
func airedPixels(t *testing.T, path string) *image.NRGBA {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	out := image.NewNRGBA(img.Bounds())
	draw.Draw(out, out.Rect, img, img.Bounds().Min, draw.Src)
	return out
}

// sameDesign fails unless the aired PNG is exactly the renderer's design at 1080 lines.
func sameDesign(t *testing.T, path string, d watermark.Design) {
	t.Helper()
	want, err := watermark.RenderDesign(d, 1080, watermark.Look{Size: schedule.WatermarkDefaultSize, Opacity: 0.40, Shadow: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := airedPixels(t, path); !bytes.Equal(got.Pix, want.Straight.Pix) {
		t.Errorf("aired bug is not %+v", d)
	}
}

// HOLIDAY VERSIONS (#1659): the airing's start picks the version, so a programme that starts on
// Oct 1 airs the Halloween logo and one that started Sep 30 keeps the plain one; the channel's
// picture choice and "Match the era" take the holiday colours; Ignore holidays and an own image
// never get one.
func TestChannelWatermarks_HolidayVersionFollowsTheAiringStart(t *testing.T) {
	ch := store.Channel{Channel: schedule.Channel{Name: "Saturday Cartoons", Number: 7}}
	ch.Policy.Watermark = &schedule.WatermarkPolicy{Look: "tint", Motifs: map[string]string{"halloween": "ghost"}}
	ch.Policy.Scope.Dates = schedule.EraDates(schedule.Range{From: 1980, To: 1989})
	c := testWatermarks(t, ch, true)
	colour(c)
	sep30 := time.Date(2026, time.September, 30, 23, 30, 0, 0, time.UTC)
	oct1 := time.Date(2026, time.October, 1, 0, 30, 0, 0, time.UTC)

	plain := c.For(t.Context(), "ch", playout.EncoderNVENC, 1920, 1080, sep30)
	sameDesign(t, plain.Straight, watermark.Design{Style: watermark.StyleTint, Callsign: "SC", Number: 7,
		Colour: watermark.Red, Palette: watermark.EraPalette(1980)})
	spooky := c.For(t.Context(), "ch", playout.EncoderNVENC, 1920, 1080, oct1)
	h := watermark.HolidayLooks["halloween"]
	sameDesign(t, spooky.Straight, watermark.Design{Style: watermark.StyleTint, Callsign: "SC", Number: 7,
		Colour: h.Colour, Palette: h.Palette, Motif: watermark.MotifGhost})

	// A fixed colour gives way to the holiday's too: the holiday version is one design (the mock's holOpts).
	fixed := ch
	fixed.Policy.Watermark = &schedule.WatermarkPolicy{Look: "tint", Color: "cyan", Motifs: map[string]string{"halloween": "ghost"}}
	if wm := colour(testWatermarks(t, fixed, true)).For(t.Context(), "ch", playout.EncoderNVENC, 1920, 1080, oct1); wm == nil {
		t.Fatal("no bug")
	} else {
		sameDesign(t, wm.Straight, watermark.Design{Style: watermark.StyleTint, Callsign: "SC", Number: 7,
			Colour: h.Colour, Palette: h.Palette, Motif: watermark.MotifGhost})
	}

	ignore := ch
	ignore.Policy.Seasonal = schedule.SeasonalPolicy{Mode: schedule.SeasonalOff}
	if wm := colour(testWatermarks(t, ignore, true)).For(t.Context(), "ch", playout.EncoderNVENC, 1920, 1080, oct1); wm == nil {
		t.Fatal("no bug")
	} else {
		sameDesign(t, wm.Straight, watermark.Design{Style: watermark.StyleTint, Callsign: "SC", Number: 7,
			Colour: watermark.Red, Palette: watermark.EraPalette(1980)})
	}

	off := false
	optedOut := ch
	optedOut.Policy.Watermark = &schedule.WatermarkPolicy{Look: "tint", Color: "cyan", Holiday: &off}
	if wm := colour(testWatermarks(t, optedOut, true)).For(t.Context(), "ch", playout.EncoderNVENC, 1920, 1080, oct1); wm == nil {
		t.Fatal("no bug")
	} else {
		sameDesign(t, wm.Straight, watermark.Design{Style: watermark.StyleTint, Callsign: "SC", Number: 7,
			Colour: watermark.Cyan, Palette: watermark.EraPalette(1980)})
	}
}

// colour marks the test service's encoder as passing the coloured self-check.
func colour(c *channelWatermarks) *channelWatermarks {
	c.gates[playout.EncoderNVENC].colour.Store(true)
	return c
}

// COLOUR FALLBACK (#1659 Q2): a host whose coloured self-check failed airs the same look in white,
// never no logo, and the white bug is cached apart from the coloured one.
func TestChannelWatermarks_ColourFallsBackToWhite(t *testing.T) {
	ch := store.Channel{Channel: schedule.Channel{Name: "Saturday Cartoons", Number: 7}}
	ch.Policy.Watermark = &schedule.WatermarkPolicy{Look: "badge", Color: "pink"}
	white := testWatermarks(t, ch, true).For(t.Context(), "ch", playout.EncoderNVENC, 1920, 1080, offSeason)
	if white == nil {
		t.Fatal("a failed coloured check dropped the logo")
	}
	sameDesign(t, white.Straight, watermark.Design{Style: watermark.StyleBadge, Callsign: "SC", Number: 7,
		Colour: watermark.Pink, Palette: watermark.NoEraPalette, White: true})
	c := testWatermarks(t, ch, true)
	coloured := colour(c).For(t.Context(), "ch", playout.EncoderNVENC, 1920, 1080, offSeason)
	if filepath.Base(coloured.Straight) == filepath.Base(white.Straight) {
		t.Error("the white and coloured bugs share a cache key")
	}
	sameDesign(t, coloured.Straight, watermark.Design{Style: watermark.StyleBadge, Callsign: "SC", Number: 7,
		Colour: watermark.Pink, Palette: watermark.NoEraPalette})
}

// ONE LIST, TWO COPIES: the policy's colours and holiday pictures are the renderer's.
func TestWatermarkColoursAndMotifs_AgreeAcrossRendererAndPolicy(t *testing.T) {
	colours := []string{schedule.WatermarkDefaultColor}
	for _, c := range watermark.Colours {
		colours = append(colours, string(c))
	}
	if !slices.Equal(colours, schedule.WatermarkColors) {
		t.Errorf("renderer colours %v, policy %v", colours, schedule.WatermarkColors)
	}
	field, _ := reflect.TypeFor[schedule.WatermarkPolicy]().FieldByName("Color")
	if tag := field.Tag.Get("enum"); tag != strings.Join(schedule.WatermarkColors, ",") {
		t.Errorf("policy color enum tag %q", tag)
	}
	var motifs []string
	for holiday, choices := range schedule.WatermarkMotifs {
		if _, ok := watermark.HolidayLooks[holiday]; !ok {
			t.Errorf("holiday %q has no colours", holiday)
		}
		motifs = append(motifs, choices...)
	}
	slices.Sort(motifs)
	var drawn []string
	for _, m := range watermark.Motifs {
		drawn = append(drawn, string(m))
	}
	slices.Sort(drawn)
	if !slices.Equal(motifs, drawn) {
		t.Errorf("policy motifs %v, renderer %v", motifs, drawn)
	}
}

// The section's options come from the backend (#1659 W2c): the derived name, the era and its
// colours, and only the holidays the channel honours, with the scheduler's "in effect today".
func TestChannelWatermarks_Options(t *testing.T) {
	ch := store.Channel{Channel: schedule.Channel{ID: "ch-1", Name: "Saturday Cartoons", Number: 7}}
	ch.Policy.Scope.Dates = schedule.EraDates(schedule.Range{From: 1980, To: 1989})
	ch.Policy.Seasonal = schedule.SeasonalPolicy{Holidays: []string{"christmas", "halloween"}}
	ch.Policy.Watermark = &schedule.WatermarkPolicy{Motifs: map[string]string{"halloween": "ghost"}}
	oct13 := time.Date(2026, time.October, 13, 20, 0, 0, 0, time.UTC)

	got, err := testWatermarks(t, ch, true).Options(context.Background(), "ch-1", oct13)
	if err != nil {
		t.Fatal(err)
	}
	if got.AutoCallsign != "SC" || got.Era != "1980s" || !slices.Equal(got.EraPalette, []string{"red", "amber", "cyan"}) {
		t.Fatalf("name/era = %q %q %v, want SC 1980s [red amber cyan]", got.AutoCallsign, got.Era, got.EraPalette)
	}
	if len(got.Holidays) != 2 {
		t.Fatalf("holidays = %+v, want halloween and christmas only", got.Holidays)
	}
	hw, xm := got.Holidays[0], got.Holidays[1]
	if hw.ID != "halloween" || hw.Name != "Halloween" || hw.Start != "2026-10-01" || hw.End != "2026-10-31" || !hw.Active ||
		hw.Picture != "ghost" || len(hw.Pictures) != 3 || hw.Pictures[0] != (api.WatermarkPicture{ID: "pumpkin", Name: "Pumpkin"}) {
		t.Fatalf("halloween = %+v", hw)
	}
	if xm.ID != "christmas" || xm.Active || xm.Picture != "holly" || xm.Start != "2026-12-01" || xm.End != "2026-12-26" {
		t.Fatalf("christmas = %+v", xm)
	}

	// Ignore holidays: none, and no era is amber's palette.
	ch.Policy.Seasonal = schedule.SeasonalPolicy{Mode: schedule.SeasonalOff}
	ch.Policy.Scope.Dates = nil
	got, err = testWatermarks(t, ch, true).Options(context.Background(), "ch-1", oct13)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Holidays) != 0 || got.Holidays == nil || got.Era != "" || got.EraPalette[0] != "amber" {
		t.Fatalf("ignore holidays = %+v, want an empty list, no era, amber", got)
	}
}

// Preview draws the holiday version with the channel's picture, or the one the request names (the
// section's picture thumbnails), without changing the saved policy; the everyday logo has none.
func TestChannelWatermarks_PreviewHolidayPicture(t *testing.T) {
	ch := store.Channel{Channel: schedule.Channel{ID: "ch-1", Name: "Saturday Cartoons", Number: 7}}
	read := func(path string, err error) []byte {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	preview := func(c store.Channel, change schedule.WatermarkPolicy, holiday string) []byte {
		path, _, err := testWatermarks(t, c, true).Preview(context.Background(), "ch-1", change, holiday)
		return read(path, err)
	}
	everyday := preview(ch, schedule.WatermarkPolicy{}, "")
	pumpkin := preview(ch, schedule.WatermarkPolicy{}, "halloween")
	asked := preview(ch, schedule.WatermarkPolicy{Motifs: map[string]string{"halloween": "bat"}}, "halloween")
	saved := ch
	saved.Policy.Watermark = &schedule.WatermarkPolicy{Motifs: map[string]string{"halloween": "bat"}}
	if bytes.Equal(everyday, pumpkin) || bytes.Equal(pumpkin, asked) {
		t.Fatal("the holiday version and its pictures must differ from each other and from the everyday logo")
	}
	if !bytes.Equal(asked, preview(saved, schedule.WatermarkPolicy{}, "halloween")) {
		t.Fatal("a requested picture must draw exactly what the channel airs once it is saved")
	}
	if ch.Policy.Watermark != nil {
		t.Fatal("a preview must not change the channel's policy")
	}
}
