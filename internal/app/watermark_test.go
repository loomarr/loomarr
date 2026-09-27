package app

import (
	"context"
	"image/color"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/diagnostics"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
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
		opacity: watermarkOpacity(set), gates: map[playout.Encoder]*watermarkGate{playout.EncoderNVENC: g}}
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

// ON BY DEFAULT: a channel with no watermark policy gets the Plate bug at the approved placement.
func TestChannelWatermarks_DefaultPlateBug(t *testing.T) {
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
