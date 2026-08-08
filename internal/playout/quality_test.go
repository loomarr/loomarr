package playout

import (
	"strings"
	"testing"
)

// The policy under test: "best picture the hardware sustains, then adapt as channels are
// added." These assert the two halves separately, because they fail in different ways — a
// broken "best" is a permanently soft picture, a broken "adapt" is universal stutter.

// Half one: the FIRST channel on an idle box gets the top rung. If this regresses, every
// install quietly delivers reduced quality and nobody can tell why.
func TestResolve_FirstChannelOnAnIdleBoxGetsTheBestRung(t *testing.T) {
	for _, tier := range []Tier{TierEfficient, TierBalanced, TierQuality} {
		got := Resolve(tier, EncoderNVENC, 9, 0)
		best := ladders[tier][0]
		if got.Width != best.width || got.VideoBitrate != best.videoBitrate {
			t.Errorf("%s: first channel got %dx%d @%dk, want the top rung %dx%d @%dk",
				tier, got.Width, got.Height, got.VideoBitrate,
				best.width, best.height, best.videoBitrate)
		}
	}
}

// Half two: quality steps DOWN as capacity fills, and never up.
func TestResolve_DegradesMonotonicallyAsLoadRises(t *testing.T) {
	const capacity = 8
	var lastBitrate int
	for active := 0; active <= capacity; active++ {
		got := Resolve(TierBalanced, EncoderNVENC, capacity, active)
		if active > 0 && got.VideoBitrate > lastBitrate {
			t.Errorf("active=%d: bitrate went UP (%dk after %dk) — degradation must be monotonic",
				active, got.VideoBitrate, lastBitrate)
		}
		lastBitrate = got.VideoBitrate
	}
	// And the last one must genuinely be lower than the first, or "adapt" does nothing.
	first := Resolve(TierBalanced, EncoderNVENC, capacity, 0)
	full := Resolve(TierBalanced, EncoderNVENC, capacity, capacity)
	if full.VideoBitrate >= first.VideoBitrate {
		t.Errorf("a full box (%dk) must encode below an idle one (%dk)",
			full.VideoBitrate, first.VideoBitrate)
	}
}

// A bigger box degrades LATER. The step-down is proportional to committed capacity, so a
// machine that measured 12 channels should still be on the top rung where a 2-channel
// machine has already stepped down.
func TestResolve_LargerCapacityHoldsQualityLonger(t *testing.T) {
	small := Resolve(TierBalanced, EncoderNVENC, 2, 2)
	large := Resolve(TierBalanced, EncoderNVENC, 12, 2)
	if large.VideoBitrate <= small.VideoBitrate {
		t.Errorf("with 2 active: big box %dk should beat small box %dk",
			large.VideoBitrate, small.VideoBitrate)
	}
}

// Unmeasured capacity must not be read as "unlimited". 0 or 1 means we could not measure,
// or the box barely manages one channel — either way the bottom rung is the honest answer,
// not the top.
func TestResolve_UnmeasuredCapacityTakesTheBottomRung(t *testing.T) {
	for _, capacity := range []int{0, 1} {
		got := Resolve(TierBalanced, EncoderNVENC, capacity, 0)
		bottom := ladders[TierBalanced][len(ladders[TierBalanced])-1]
		if got.VideoBitrate != bottom.videoBitrate {
			t.Errorf("capacity=%d gave %dk, want the bottom rung %dk — an unmeasured box must not be treated as unlimited",
				capacity, got.VideoBitrate, bottom.videoBitrate)
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

// THE refusal. Admitting an N+1th channel that makes all N stutter is worse than declining
// it — and this is the admission bound viewra lacked, where a new session EVICTED others.
func TestAtCapacity_RefusesRatherThanDegradingEveryone(t *testing.T) {
	if AtCapacity(4, 3) {
		t.Error("3 of 4 must be admitted")
	}
	if !AtCapacity(4, 4) {
		t.Error("4 of 4 must be REFUSED, not admitted at reduced quality")
	}
	if !AtCapacity(4, 5) {
		t.Error("over capacity must be refused")
	}
	// An unconfigured cap must not block playout entirely.
	if AtCapacity(0, 99) {
		t.Error("an unset max_channels must not refuse everything")
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

// CRF is software-only. Hardware rate control handles a bitrate target far better than a
// quality one, and v4l2m2m has no usable CRF at all — emitting it would fail at init.
func TestQualityArgs_CrfIsSoftwareOnly(t *testing.T) {
	sw := strings.Join(Profile{Encoder: EncoderSoftware, VideoBitrate: 5000}.qualityArgs(), " ")
	if !strings.Contains(sw, "-crf") {
		t.Errorf("software should get a CRF target, got %q", sw)
	}
	for _, enc := range []Encoder{
		EncoderNVENC, EncoderQSV, EncoderVAAPI, EncoderAMF,
		EncoderVideoToolbox, EncoderRKMPP, EncoderV4L2M2M, EncoderVulkan,
	} {
		p := Profile{Encoder: enc, VideoBitrate: 5000}
		if got := p.qualityArgs(); len(got) != 0 {
			t.Errorf("%s must not get CRF args, got %v", enc, got)
		}
	}
}

// The resolved profile must carry the chosen encoder through — a ladder that silently reset
// it to software would undo the whole detection step.
func TestResolve_KeepsTheChosenEncoder(t *testing.T) {
	got := Resolve(TierBalanced, EncoderVulkan, 8, 0)
	if got.Encoder != EncoderVulkan {
		t.Errorf("Resolve dropped the encoder: %q", got.Encoder)
	}
}

// lastSpeed must return the PEAK sample, not the last — a cold encoder ramps, and taking whichever
// sample landed last collapsed a warm ~8x to ~1x and capped the box at one hardware channel.
func TestLastSpeed_TakesThePeakNotTheLast(t *testing.T) {
	// A realistic cold ramp that then falls off at teardown: peak is 8.66, last is a cold 0.90.
	progress := strings.NewReader(strings.Join([]string{
		"frame=10", "speed=0.75x",
		"frame=60", "speed=6.20x",
		"frame=140", "speed=8.66x", // the peak — the honest capability
		"frame=150", "speed=0.90x", // a depressed final sample
		"progress=end",
	}, "\n"))
	if got := lastSpeed(progress); got != 8.66 {
		t.Errorf("lastSpeed = %v, want the peak 8.66", got)
	}
}

// A trial that never emitted a usable speed (all N/A) reports 0, which channelsFromSpeed floors to 1.
func TestLastSpeed_NoUsableSampleIsZero(t *testing.T) {
	r := strings.NewReader("speed=N/A\nspeed=0x\nprogress=end\n")
	if got := lastSpeed(r); got != 0 {
		t.Errorf("lastSpeed = %v, want 0 when no usable sample", got)
	}
}
