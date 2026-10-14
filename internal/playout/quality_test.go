package playout

import (
	"strings"
	"testing"
)

// Resolve is the profile at the rung the ResourceBudget admitted; the rung choice itself is
// tested in budget_test.go. Rung 0 is the top rung, and a rung past the bottom clamps.
func TestResolve_RungIndexesTheLadder(t *testing.T) {
	for _, tier := range []Tier{TierEfficient, TierBalanced, TierQuality} {
		l := ladders[tier]
		for i, want := range l {
			got := Resolve(tier, EncoderNVENC, i)
			if got.Width != want.width || got.Height != want.height || got.VideoBitrate != want.videoBitrate {
				t.Errorf("%s rung %d = %dx%d @%dk, want %dx%d @%dk", tier, i,
					got.Width, got.Height, got.VideoBitrate, want.width, want.height, want.videoBitrate)
			}
		}
		if got, bottom := Resolve(tier, EncoderNVENC, 99), l[len(l)-1]; got.VideoBitrate != bottom.videoBitrate {
			t.Errorf("%s: rung past the bottom = %dk, want the bottom rung", tier, got.VideoBitrate)
		}
		if heights := LadderHeights(tier); len(heights) != len(l) || heights[0] != l[0].height {
			t.Errorf("%s: LadderHeights = %v", tier, heights)
		}
	}
}

// h264 requires EVEN dimensions in both axes; an odd height fails at encoder init with a
// message that never mentions the height. Every rung on every ladder must be safe.
func TestLadders_EveryRungIsEvenAnd16By9(t *testing.T) {
	for tier, l := range ladders {
		if len(l) == 0 {
			t.Errorf("%s: empty ladder", tier)
		}
		for i, r := range l {
			if r.width%2 != 0 || r.height%2 != 0 {
				t.Errorf("%s rung %d: %dx%d — h264 needs even dimensions", tier, i, r.width, r.height)
			}
			if r.videoBitrate <= 0 || r.audioBitrate <= 0 {
				t.Errorf("%s rung %d: non-positive bitrate", tier, i)
			}
			if i > 0 && r.videoBitrate > l[i-1].videoBitrate {
				t.Errorf("%s rung %d is HIGHER bitrate than rung %d — the ladder must descend", tier, i, i-1)
			}
		}
	}
}

// Bitrate drops before resolution: a softer 1080p is less objectionable on a TV than a
// resolution switch, which some clients handle by re-buffering.
func TestLadders_BitrateFallsBeforeResolution(t *testing.T) {
	for tier, l := range ladders {
		if len(l) < 2 {
			continue
		}
		if l[0].width != l[1].width || l[0].height != l[1].height {
			t.Errorf("%s: the first step changes resolution (%dx%d → %dx%d); it should drop bitrate first",
				tier, l[0].width, l[0].height, l[1].width, l[1].height)
		}
	}
}

// An unknown tier degrades to the default rather than erroring: this is read on the path to
// starting a channel, and refusing to play because a setting is misspelled is worse than
// playing at the default.
func TestTierFor_UnknownDegradesToDefault(t *testing.T) {
	if got := TierFor("nonsense"); got != DefaultTier {
		t.Errorf("TierFor(nonsense) = %q, want %q", got, DefaultTier)
	}
	if got := TierFor(""); got != DefaultTier {
		t.Errorf("TierFor(empty) = %q, want %q", got, DefaultTier)
	}
	if got := TierFor("quality"); got != TierQuality {
		t.Errorf("a valid tier must survive: %q", got)
	}
}

// The resolved profile must carry the chosen encoder through — a ladder that silently reset
// it to software would undo the whole detection step.
func TestResolve_KeepsTheChosenEncoder(t *testing.T) {
	got := Resolve(TierBalanced, EncoderVulkan, 0)
	if got.Encoder != EncoderVulkan {
		t.Errorf("Resolve dropped the encoder: %q", got.Encoder)
	}
}

// lastSpeedObserved must return the PEAK sample, not the last — a cold encoder ramps, and taking
// whichever sample landed last collapsed a warm ~8x to ~1x and capped the box at one hardware channel.
func TestLastSpeed_TakesThePeakNotTheLast(t *testing.T) {
	// A realistic cold ramp that then falls off at teardown: peak is 8.66, last is a cold 0.90.
	progress := strings.NewReader(strings.Join([]string{
		"frame=10", "speed=0.75x",
		"frame=60", "speed=6.20x",
		"frame=140", "speed=8.66x", // the peak — the honest capability
		"frame=150", "speed=0.90x", // a depressed final sample
		"progress=end",
	}, "\n"))
	if got := lastSpeedObserved(progress, nil); got != 8.66 {
		t.Errorf("lastSpeedObserved = %v, want the peak 8.66", got)
	}
}

// A trial that never emitted a usable speed (all N/A) reports 0, which channelsFromSpeed floors to 1.
func TestLastSpeed_NoUsableSampleIsZero(t *testing.T) {
	r := strings.NewReader("speed=N/A\nspeed=0x\nprogress=end\n")
	if got := lastSpeedObserved(r, nil); got != 0 {
		t.Errorf("lastSpeedObserved = %v, want 0 when no usable sample", got)
	}
}
