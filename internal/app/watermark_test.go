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
	wm := c.For(context.Background(), "ch", playout.EncoderNVENC, 1920, 1080)
	if wm == nil {
		t.Fatal("no bug")
	}
	sameBug(t, wm.Straight, call, watermark.StyleText, 0.40)

	set.svc.SetDB(map[string]string{"playout.watermark_look": "plate"})
	again := c.For(context.Background(), "ch", playout.EncoderNVENC, 1920, 1080)
	if again == nil || again.Straight == wm.Straight {
		t.Fatalf("a changed look did not re-render the bug: %+v", again)
	}
	sameBug(t, again.Straight, call, watermark.StylePlate, 0.40)

	ch := store.Channel{Channel: schedule.Channel{Name: name, Number: number}}
	ch.Policy.Watermark = &schedule.WatermarkPolicy{Look: "small-plate"}
	over := testWatermarksWith(t, set, ch, true).For(context.Background(), "ch", playout.EncoderNVENC, 1920, 1080)
	sameBug(t, over.Straight, call, watermark.StyleSmallPlate, 0.40)
}

// ONE LIST, THREE COPIES: the renderer's styles, the policy's looks and the setting's options are
// declared in three packages that must not import each other; this pins them equal.
func TestWatermarkLooks_AgreeAcrossRendererPolicyAndSetting(t *testing.T) {
	var styles []string
	for _, s := range watermark.Styles {
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
	wm := c.For(context.Background(), "ch", playout.EncoderNVENC, 1920, 1080)
	if wm == nil {
		t.Fatal("no bug")
	}
	if got, want := peakWhite(t, wm.Straight), uint32(140); got < want-1 || got > want+1 { // 0.55 × 255
		t.Errorf("bug peaks at %d/255, want %d (the install setting, 55%%)", got, want)
	}
	set.svc.SetDB(map[string]string{"playout.watermark_opacity_pct": "70"})
	again := c.For(context.Background(), "ch", playout.EncoderNVENC, 1920, 1080)
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
	over := testWatermarksWith(t, set, ch, true).For(context.Background(), "ch", playout.EncoderNVENC, 1920, 1080)
	if got, want := peakWhite(t, over.Straight), uint32(64); got < want-1 || got > want+1 { // 0.25 × 255
		t.Errorf("channel override: bug peaks at %d/255, want %d", got, want)
	}
}

// ON BY DEFAULT: a channel with no watermark policy gets the generated bug at the approved placement.
func TestChannelWatermarks_DefaultBug(t *testing.T) {
	c := testWatermarks(t, store.Channel{Channel: schedule.Channel{Name: "Retro Cartoons", Number: 7}}, true)
	wm := c.For(context.Background(), "ch", playout.EncoderNVENC, 1920, 1080)
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
	again := c.For(context.Background(), "ch", playout.EncoderNVENC, 1920, 1080)
	if again == nil || again.Straight != wm.Straight {
		t.Errorf("not cached: %+v vs %+v", again, wm)
	}
}

func TestChannelWatermarks_OffOrUnverifiedMeansNoBug(t *testing.T) {
	off := false
	ch := store.Channel{Channel: schedule.Channel{Name: "Kids", Number: 3}}
	ch.Policy.Watermark = &schedule.WatermarkPolicy{Enabled: &off}
	if wm := testWatermarks(t, ch, true).For(context.Background(), "ch", playout.EncoderNVENC, 1920, 1080); wm != nil {
		t.Error("a channel that turned the bug off got one")
	}
	ch.Policy.Watermark = nil
	if wm := testWatermarks(t, ch, false).For(context.Background(), "ch", playout.EncoderNVENC, 1920, 1080); wm != nil {
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
