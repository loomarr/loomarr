package playout

import "strconv"

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

// Resolve picks the profile a channel should encode at right now.
//
//	tier     — what the operator asked for
//	enc      — the encoder Detect chose (or an operator override)
//	capacity — measured concurrent-channel headroom for this box
//	active   — channels already encoding, NOT counting this one
//
// The step-down is proportional to how much of the measured capacity is already committed,
// so a box that measured 9 channels degrades later than one that measured 2. Capacity of 0
// or 1 means "we could not measure, or this box barely manages one" — take the bottom rung
// and stop guessing.
func Resolve(tier Tier, enc Encoder, capacity, active int) Profile {
	l := ladders[tier]
	if len(l) == 0 {
		l = ladders[DefaultTier]
	}

	idx := 0
	switch {
	case capacity <= 1:
		// Unmeasured or genuinely tiny. The bottom rung is the honest answer.
		idx = len(l) - 1
	default:
		// Committed fraction of capacity → position on the ladder. At <50% committed we
		// stay on the best rung; each further quarter steps down one.
		//
		// `active` counts channels ALREADY running, so the first channel on an idle box
		// always gets the top rung — which is the "best picture" half of the policy.
		used := float64(active) / float64(capacity)
		switch {
		case used < 0.5:
			idx = 0
		case used < 0.75:
			idx = 1
		case used < 1.0:
			idx = 2
		default:
			idx = len(l) - 1
		}
	}
	if idx >= len(l) {
		idx = len(l) - 1
	}

	r := l[idx]
	return Profile{
		Width: r.width, Height: r.height, Framerate: r.framerate,
		VideoBitrate: r.videoBitrate, AudioBitrate: r.audioBitrate,
		Encoder: enc,
	}
}

// Admit reports whether a new session may start, given a COST-AWARE budget (§9.1 V49).
//
// The bound is not "how many sessions" but "how many concurrent TRANSCODES the box can sustain",
// because the transcode is what consumes the GPU. A `-c copy` session (an h264 channel, or an HEVC
// channel to an HEVC-capable client) costs ~nothing and is ALWAYS admitted — it never blocks another
// channel. Only a session that re-encodes video counts against `budget`.
//
// This is what makes a channel watched at two plans (baseline + hevc8) cost ONE, not two: the hevc8
// copy is free, only the baseline transcode counts. And `budget` is the box's MEASURED capacity
// (Detect), optionally shaded by live VRAM headroom, not a static magic number.
//
//   - newCost is the incoming session's estimated cost (1 if it will transcode video, else 0).
//   - committed is the summed cost of sessions already running.
//   - budget <= 0 means "unmeasured/unconfigured" — do not block playout on a missing number.
//
// Refusing is deliberate and is the one place this policy says no. Admitting an N+1th transcode that
// makes all N stutter is worse than declining it: the operator sees a clear "at capacity" message and
// can raise the cap or lower the tier, whereas universal stutter presents as "playout is broken".
// (This is the bound viewra lacked — its manager EVICTED sessions to make room, which for playout
// would mean one viewer tuning in kills someone else's channel.)
func Admit(budget, committed, newCost int) bool {
	if budget <= 0 {
		return true // unmeasured: never block playout on a missing/zero capacity
	}
	if newCost <= 0 {
		return true // a copy costs ~no GPU — always admit, it cannot starve a transcode
	}
	return committed+newCost <= budget
}

// qualityArgs returns rate-control args for software encoders, which do better with a
// quality target than a hard bitrate.
//
// Software gets CRF: for a live stream at a fixed resolution, CRF holds picture quality
// steady and lets bitrate vary, which is the better trade when the transport can absorb it.
// Hardware encoders take the bitrate ladder instead — most hardware rate-control
// implementations handle a bitrate target far better than a quality one, and several
// (v4l2m2m especially) have no usable CRF equivalent at all.
func (p Profile) qualityArgs() []string {
	if p.Encoder != EncoderSoftware {
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
