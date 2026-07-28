package playout

import (
	"bytes"
	"os"
	"testing"
)

// The bug this file exists for: a host with a font file but an ffmpeg built WITHOUT drawtext.
// The old guard asked only "is there a font?", so it emitted `-vf drawtext=…`, ffmpeg rejected
// the graph with "Filter not found", the encode exited 8 — and the channel was dead, which is
// the one outcome font.go promises never to cause. Homebrew's ffmpeg bottle is that host:
// macOS ships Arial.ttf and the bottle carries no libfreetype.

// Real `ffmpeg -filters` rows, verbatim from ffmpeg 8.1.2 — flags, name, signature, description.
const filtersWithDrawText = `Filters:
  T.. colorhold          V->V       Turns a certain color range into gray.
 TS. drawtext            V->V       Draw text on top of video frames.
 ... anullsrc            |->A       Null audio source, return empty audio frames.
`

// The same build minus drawtext. `drawbox` stays: it shares a prefix, so a substring match
// would call this a pass.
const filtersWithoutDrawText = `Filters:
  T.. colorhold          V->V       Turns a certain color range into gray.
 T.. drawbox             V->V       Draw a colored box on the input video.
 ... anullsrc            |->A       Null audio source, return empty audio frames.
`

func TestParseHasDrawText_ReadsTheNameColumn(t *testing.T) {
	if !parseHasDrawText([]byte(filtersWithDrawText)) {
		t.Error("drawtext is listed in this build and was not found")
	}
	if parseHasDrawText([]byte(filtersWithoutDrawText)) {
		t.Error("drawtext is absent from this build and was reported present")
	}
}

// The name must match the NAME COLUMN, not the line. `-filters` carries a prose description per
// row, and a filter whose description merely mentions drawtext is not drawtext.
func TestParseHasDrawText_IgnoresTheDescriptionColumn(t *testing.T) {
	mentionOnly := ` T.. subtitles  V->V  Render text subtitles, unlike drawtext, onto the video.
`
	if parseHasDrawText([]byte(mentionOnly)) {
		t.Error("matched the description column — a substring search, not a column match")
	}
}

func TestParseHasDrawText_EmptyOutputIsNotAYes(t *testing.T) {
	if parseHasDrawText(nil) {
		t.Error("no output must never resolve to 'the filter is present'")
	}
}

// The fail-safe direction, which is the whole point: when the probe cannot run at all, the
// answer is "" (unlabelled card), never a font that would produce a dead channel.
func TestCardFontFor_UnrunnableProbeYieldsNoFont(t *testing.T) {
	font := CardFontFor(t.TempDir() + "/definitely-not-ffmpeg")()
	if font != "" {
		t.Errorf("a probe that cannot execute must degrade to an unlabelled card, got %q", font)
	}
}

// A build that DOES carry drawtext resolves to a real font, and the probe runs ONCE however
// many cards are served — the offline card is re-requested on a loop for as long as a channel
// has nothing scheduled, so a per-card process spawn would be a spawn every few seconds.
//
// Uses a stub standing in for ffmpeg rather than the host's: this must assert the same thing on
// a machine whose real ffmpeg has drawtext and one whose has not.
func TestCardFontFor_ResolvesAFontAndProbesOnce(t *testing.T) {
	dir := t.TempDir()
	font, calls := dir+"/stub.ttf", dir+"/calls"
	if err := os.WriteFile(font, []byte("not really a font"), 0o600); err != nil {
		t.Fatal(err)
	}
	// PLAYOUT_FONT_PATH is checked before the candidate list, so the font side is pinned
	// rather than depending on which fonts this host happens to ship.
	t.Setenv(FontPathEnv, font)

	stub := dir + "/fake-ffmpeg"
	script := "#!/bin/sh\necho x >> " + calls + "\n" +
		"echo ' TS. drawtext            V->V       Draw text on top of video frames.'\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil { //nolint:gosec // a test stub must be executable
		t.Fatal(err)
	}

	probe := CardFontFor(stub)
	for range 3 {
		if got := probe(); got != font {
			t.Fatalf("want the configured font %q, got %q", font, got)
		}
	}

	ran, err := os.ReadFile(calls)
	if err != nil {
		t.Fatalf("the stub never ran: %v", err)
	}
	if n := bytes.Count(ran, []byte("x")); n != 1 {
		t.Errorf("probed %d times across 3 cards — the answer is not memoised", n)
	}
}
