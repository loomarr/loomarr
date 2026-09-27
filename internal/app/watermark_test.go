package app

import (
	"context"
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
	g := &watermarkGate{}
	g.works.Store(gatePassed)
	return &channelWatermarks{dir: t.TempDir(), channels: staticChannelReader{channel: ch}, log: slog.New(slog.DiscardHandler),
		gates: map[playout.Encoder]*watermarkGate{playout.EncoderNVENC: g}}
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
	if _, err := os.Stat(wm.Straight); err != nil {
		t.Errorf("rendered file missing: %v", err)
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
