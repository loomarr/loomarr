package app

import (
	"context"
	"log/slog"
	"os"
	"testing"

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
