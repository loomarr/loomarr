package playout

import (
	"sort"
	"strconv"
)

// Quality selection (§9.1). The policy is "best picture the hardware sustains, then adapt
// as channels are added" — so quality is a RUNTIME property derived from
// (target tier, measured capacity, current load), not a value fixed when a channel is made.
//
// That is why Resolve takes load: a box comfortably encoding one channel at 1080p should do
// exactly that, and the same box with five channels running should step down rather than
// deliver five stuttering ones. The alternative — a fixed per-channel profile — makes the
// operator predict their own peak load at setup time, which nobody can do.

// Tier is the quality target an operator asks for. Three, because more would be a false
// choice: the meaningful axis is "how much am I willing to spend per channel", and past
// three points the labels stop mapping to anything an operator can reason about.
type Tier string

const (
	// TierEfficient favours channel count. 720p, lower bitrate — the right answer for a
	// NAS running six channels for a household.
	TierEfficient Tier = "efficient"
	// TierBalanced is the default: 1080p at a bitrate that looks good on a TV without
	// monopolising the encoder.
	TierBalanced Tier = "balanced"
	// TierQuality favours picture. 1080p at a high bitrate; fewer channels.
	TierQuality Tier = "quality"
)

// rung is one step on the ladder, best first. Degradation walks DOWN this list as load
// rises, dropping bitrate before resolution — a softer 1080p picture is less objectionable
// on a TV than a sudden resolution switch, which some clients handle by re-buffering.
type rung struct {
	width, height, framerate int
	videoBitrate             int // kbit/s
	audioBitrate             int
}

// ladders are the ordered degradation paths per tier.
//
// Every rung is 16:9 and an even multiple of 2 in both axes — h264 requires even
// dimensions, and an odd height fails at encoder init with a message that does not mention
// the height.
var ladders = map[Tier][]rung{
	TierQuality: {
		{1920, 1080, 30, 8000, 192},
		{1920, 1080, 30, 6000, 160},
		{1920, 1080, 25, 4500, 128},
		{1280, 720, 25, 3000, 128},
	},
	TierBalanced: {
		{1920, 1080, 25, 5000, 160},
		{1920, 1080, 25, 3500, 128},
		{1280, 720, 25, 2500, 128},
		{1280, 720, 25, 1800, 96},
	},
	TierEfficient: {
		{1280, 720, 25, 2500, 128},
		{1280, 720, 25, 1800, 96},
		{854, 480, 25, 1200, 96},
		{854, 480, 25, 800, 64},
	},
}

// DefaultTier is what an install gets without an explicit choice.
const DefaultTier = TierBalanced

// TierFor maps a stored setting value to a Tier, falling back to the default. An unknown
// value degrades rather than erroring: this is read on the path to starting a channel, and
// refusing to play because a setting is misspelled is worse than playing at the default.
func TierFor(s string) Tier {
	switch Tier(s) {
	case TierEfficient, TierBalanced, TierQuality:
		return Tier(s)
	default:
		return DefaultTier
	}
}

func ladderFor(tier Tier) []rung {
	if l := ladders[tier]; len(l) > 0 {
		return l
	}
	return ladders[DefaultTier]
}

// LadderHeights are the tier's rung output heights, best first: the ResourceBudget's rungs.
func LadderHeights(tier Tier) []int {
	l := ladderFor(tier)
	out := make([]int, len(l))
	for i, r := range l {
		out[i] = r.height
	}
	return out
}

// Resolve is the profile at a ladder rung. The rung is the one the ResourceBudget admitted the
// session at (the best that fits; it drops a rung before refusing) and stays pinned for the
// session's lifetime. A rung past the bottom clamps to the bottom.
func Resolve(tier Tier, enc Encoder, rungIndex int) Profile {
	l := ladderFor(tier)
	r := l[min(max(rungIndex, 0), len(l)-1)]
	return Profile{
		Width: r.width, Height: r.height, Framerate: r.framerate,
		VideoBitrate: r.videoBitrate, AudioBitrate: r.audioBitrate,
		Encoder: enc,
	}
}

// qualityArgs returns rate-control args for software encoders, which do better with a
// quality target than a hard bitrate.
//
// Software gets CRF: for a live stream at a fixed resolution, CRF holds picture quality
// steady and lets bitrate vary, which is the better trade when the transport can absorb it.
// Hardware encoders take the bitrate ladder instead — most hardware rate-control
// implementations handle a bitrate target far better than a quality one, and several
// (v4l2m2m especially) have no usable CRF equivalent at all.
//
// ⚠ Keyed on the FAMILY, not the encoder value. `p.Encoder != EncoderSoftware` excluded
// `libx265` — so an HEVC software encode got `-b:v`/`-maxrate`/`-bufsize` and no `-crf` at all,
// which is the bitrate ladder this function exists to override. Its sibling defect in
// capability.go had the same shape and the same cause: nine HEVC encoders added in V49, and every
// switch already written against the h264 constants silently kept excluding them.
//
// The test that should have caught it enumerated the eight h264 hardware encoders — a set that
// could not contain the failing case, so it was green by construction. It now derives its
// iteration from h264Engines and covers both codecs (TestQualityArgs_CrfIsSoftwareOnly).
func (p Profile) qualityArgs() []string {
	if familyOf(p.Encoder) != familySoftware {
		return nil
	}
	// A CRF derived from the rung's bitrate: the ladder already encodes the operator's
	// intent, so this maps it onto the software encoder's scale rather than adding a
	// second knob that could disagree with the first.
	var crf int
	switch {
	case p.VideoBitrate >= 6000:
		crf = 19
	case p.VideoBitrate >= 4000:
		crf = 21
	case p.VideoBitrate >= 2000:
		crf = 23
	default:
		crf = 26
	}
	return []string{"-crf", strconv.Itoa(crf)}
}

// ProbeOutputs are the outputs the class probe measures: every rung height any tier uses, tallest
// first, each at the highest frame rate a tier gives it, so a tier change never meets an
// unmeasured height and no rung is costed below what it runs at.
func ProbeOutputs(enc Encoder) []Profile {
	best := map[int]rung{}
	for _, l := range ladders {
		for _, r := range l {
			if b, ok := best[r.height]; !ok || r.framerate > b.framerate {
				best[r.height] = r
			}
		}
	}
	out := make([]Profile, 0, len(best))
	for _, r := range best {
		out = append(out, Profile{Width: r.width, Height: r.height, Framerate: r.framerate,
			VideoBitrate: r.videoBitrate, AudioBitrate: r.audioBitrate, Encoder: enc})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Height > out[j].Height })
	return out
}
