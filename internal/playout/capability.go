package playout

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Encoder capability detection (§9.1) — "which encoder should this box use, and how many
// channels can it sustain?"
//
// THE RULE: the ffmpeg build says what is POSSIBLE; a real encode says what WORKS. Nothing
// else gets a vote — in particular, not device files.
//
// Measured on this development box, 2026-07-25, ffmpeg 7.x, and the reason for that rule:
//
//	/dev/dri/renderD128 EXISTS, and h264_vaapi + h264_qsv are both LISTED by -encoders …
//	  h264_vaapi → "No usable encoding entrypoint found for profile VAEntrypointEncSlice"
//	  h264_qsv   → "Error creating a MFX session: -9"
//	  h264_nvenc → works, 14x at 720p25
//
// because renderD128 there belongs to an NVIDIA card: no VAAPI encode entrypoint, not
// Intel. A detector that gated on that stat — as viewra's does, treating it as the
// Intel/AMD signal — picks a broken encoder on this exact hardware.
//
// So candidates are enumerated from `ffmpeg -encoders` and every listed one is TRIED. That
// is also what makes this portable to hardware nobody here has: an encoder we have never
// heard of still gets probed if the local build carries it.

// Encoder families, ordered by preference. The order encodes a judgement about picture
// quality and CPU offload, not availability — availability is measured.
//
// Coverage mirrors ErsatzTV's eight pipeline families, because that is the breadth real
// deployments need: NVIDIA, Intel, AMD (both OSes), Apple Silicon, and ARM SBCs.
// Vulkan leads on MEASURED throughput. On this box it sustains 23.7x against nvenc's
// 13.7x at 720p25 — nearly double the channel capacity — and it is cross-vendor, so the
// same path serves NVIDIA, AMD and Intel. It was originally ordered last as "newer, least
// proven", which is a reasonable prior and the wrong call once there is a number: the
// trial encode already refuses anything that does not work, so the cost of trying Vulkan
// first is one failed probe, and the benefit is roughly twice the channels.
//
// Anything ahead of software here must EARN its place by working; ordering only decides
// who gets asked first.
var encoderPreference = []Encoder{
	EncoderVulkan,       // cross-vendor, and fastest measured here (23.7x vs nvenc 13.7x)
	EncoderNVENC,        // NVIDIA — mature, good quality per bit
	EncoderQSV,          // Intel Quick Sync — mature
	EncoderVAAPI,        // Intel AND AMD on Linux; the broadest Linux path
	EncoderAMF,          // AMD on Windows
	EncoderVideoToolbox, // Apple Silicon / Intel Macs
	EncoderRKMPP,        // Rockchip SBCs
	EncoderV4L2M2M,      // Raspberry Pi and other V4L2 stateful encoders
	EncoderSoftware,     // always viable — the floor, not a candidate
}

// Capability is what one encoder can actually do here.
type Capability struct {
	Encoder Encoder
	// Works is earned by a real encode. Never inferred.
	Works bool
	// Speed is the realtime multiple at the probed profile — ffmpeg's `speed=N x`. 14
	// means ~14s of video per second of wall-clock. 0 when unmeasured.
	Speed float64
	// Err is why it failed, verbatim from ffmpeg, for the wizard's transcode check
	// (§13 V21). Kept as text rather than classified into a category — see trialEncode.
	Err string
	// Available reports whether the local ffmpeg lists this encoder at all. False means
	// "this build cannot", which is a different and more useful message to an operator
	// than "it failed".
	Available bool
}

// Capacity is the whole answer: which encoder to use, and how many channels it sustains.
type Capacity struct {
	// Chosen never ends up zero — worst case EncoderSoftware, which needs no hardware.
	Chosen Encoder
	// MaxChannels is the default for `playout.max_channels` (§15), derived from the
	// measured speed with headroom (channelsFromSpeed).
	MaxChannels int
	// All is every probe result in preference order, so the wizard can show "every
	// option, measured" and an operator can see WHY their GPU was skipped.
	All []Capability
}

// Detect probes what the local ffmpeg can do and returns the best working encoder with its
// measured capacity. "No hardware" is never an error — it is the expected outcome on most
// machines, and software is a correct answer.
//
// HARDWARE WINS by default when it works (maintainer's call). The reason is CPU HEADROOM
// rather than raw throughput: measured here, nvenc sustains 14x and libx264 veryfast 13.6x
// at 720p25 — nearly identical channel counts, but libx264 burns cores the rest of the app
// needs while nvenc offloads to otherwise-idle silicon. The gap widens with resolution,
// which is why the probe runs at the PROFILE's resolution rather than a fixed one.
func Detect(ctx context.Context, ffmpegPath string, p Profile) Capacity {
	listed := listEncoders(ctx, ffmpegPath)
	out := Capacity{Chosen: EncoderSoftware, MaxChannels: 1}

	for _, enc := range encoderPreference {
		if !listed[enc] {
			// This ffmpeg build has no such encoder. Distinct from a failure: it means
			// "wrong build", not "wrong hardware".
			out.All = append(out.All, Capability{Encoder: enc, Err: "not in this ffmpeg build"})
			continue
		}
		got := trialEncode(ctx, ffmpegPath, enc, p)
		got.Available = true
		out.All = append(out.All, got)

		// First working encoder in preference order wins. Software is last in that
		// order, so hardware is preferred whenever it genuinely works.
		if got.Works && out.Chosen == EncoderSoftware && out.MaxChannels == 1 {
			out.Chosen = got.Encoder
			out.MaxChannels = channelsFromSpeed(got.Speed)
		}
	}
	return out
}

// listEncoders returns the h264 encoders the local ffmpeg build carries.
//
// This is the ONLY honest "could it possibly work here" signal, and it is why the detector
// needs no per-vendor device knowledge: a Pi's build lists h264_v4l2m2m, a Mac's lists
// h264_videotoolbox, and an unknown future encoder is probed the day its build ships.
func listEncoders(ctx context.Context, ffmpegPath string) map[Encoder]bool {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	out := map[Encoder]bool{}
	raw, err := exec.CommandContext(ctx, ffmpegPath, "-hide_banner", "-encoders").Output()
	if err != nil {
		// No ffmpeg, or it refused to run. Software is still claimed as available so
		// Detect returns a usable answer; the trial encode will fail honestly and the
		// wizard shows why.
		out[EncoderSoftware] = true
		return out
	}
	text := string(raw)
	for _, enc := range encoderPreference {
		// Word-ish match on the encoder name as ffmpeg prints it in the flags table.
		if strings.Contains(text, " "+string(enc)+" ") {
			out[enc] = true
		}
	}
	return out
}

// trialEncode encodes a few seconds of synthetic video and reports whether it worked and
// how fast. This is the only thing that decides Works.
//
// Failure is reported by ffmpeg's EXIT CODE, with its stderr kept VERBATIM for display
// rather than pattern-matched into a category. Viewra classified by substring — matching
// "not found" and "cannot open" among others — which also matches a missing input file, so
// an unrelated failure could permanently demote a box to software.
func trialEncode(ctx context.Context, ffmpegPath string, enc Encoder, p Profile) Capability {
	// Bounded: a wedged GPU driver can hang an encode indefinitely, and the wizard waits
	// on this.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	probe := p
	probe.Encoder = enc

	args := []string{"-hide_banner", "-loglevel", "error", "-progress", "pipe:1", "-nostats"}
	args = append(args, deviceInitArgs(enc)...)
	// `testsrc` rather than a flat colour field: it has detail and motion, so the encoder
	// does representative work. A flat colour compresses to nearly nothing and would
	// report a speed no real program achieves.
	args = append(args, "-f", "lavfi", "-i",
		fmt.Sprintf("testsrc=duration=%d:size=%dx%d:rate=%d",
			trialSeconds, probe.Width, probe.Height, probe.Framerate))
	if vf := hardwareUploadFilter(enc); vf != "" {
		args = append(args, "-vf", vf)
	}
	args = append(args, probe.videoEncodeArgs()...)
	args = append(args, "-f", "null", "-")

	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Capability{Encoder: enc, Err: err.Error()}
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return Capability{Encoder: enc, Err: err.Error()}
	}

	speed := lastSpeed(stdout)
	err = cmd.Wait()

	if err != nil {
		msg := strings.TrimSpace(firstLine(stderr.String()))
		if msg == "" {
			msg = err.Error()
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			msg = "timed out — the driver may be wedged"
		}
		return Capability{Encoder: enc, Err: msg}
	}
	return Capability{Encoder: enc, Works: true, Speed: speed}
}

// trialSeconds is long enough for the speed figure to settle past encoder startup, short
// enough that probing every candidate does not stall the wizard.
const trialSeconds = 5

// deviceInitArgs returns args that must appear BEFORE the input — hardware device setup is
// a global option, and placing it after `-i` silently applies to nothing.
func deviceInitArgs(enc Encoder) []string {
	switch enc {
	case EncoderVAAPI:
		return []string{"-vaapi_device", renderNode()}
	case EncoderQSV:
		// QSV needs an EXPLICIT device when a filter uploads frames to it. Found by the
		// probe on this box: without it the failure is not a QSV error at all but
		// "[hwupload] A hardware device reference is required to upload frames to" —
		// which reads like a filter bug and sends you looking in the wrong place.
		return []string{"-init_hw_device", "qsv=hw:" + renderNode(), "-filter_hw_device", "hw"}
	case EncoderVulkan:
		return []string{"-init_hw_device", "vulkan=vk:0", "-filter_hw_device", "vk"}
	default:
		// nvenc, amf, videotoolbox, rkmpp and v4l2m2m initialise their own context.
		return nil
	}
}

// hardwareUploadFilter returns the filter that moves frames into GPU memory, which some
// hardware encoders require and which is the most common reason a "supported" encoder
// fails at init.
//
// Kept minimal deliberately: playout encodes a NORMALIZED profile, so there is no scaling
// to do here. That sidesteps the per-encoder scaler differences that dominate viewra's and
// ErsatzTV's filter code — no `pad_qsv` exists, `scale_qsv` ignores
// force_original_aspect_ratio, and `scale_cuda` needs a conditional hwupload when the
// source was software-decoded.
func hardwareUploadFilter(enc Encoder) string {
	switch enc {
	case EncoderVAAPI:
		// nv12 first: hwupload will not accept the yuv420p a lavfi source produces.
		return "format=nv12,hwupload"
	case EncoderQSV:
		// extra_hw_frames gives the encoder a pool deep enough for its lookahead; the
		// default is too small and shows up as intermittent frame-allocation failures.
		return "format=nv12,hwupload=extra_hw_frames=64"
	case EncoderVulkan:
		return "format=nv12,hwupload"
	default:
		// nvenc, amf, videotoolbox, rkmpp and v4l2m2m accept CPU frames directly;
		// software needs nothing.
		return ""
	}
}

// Env overrides for machine-specific paths. Deliberately NOT registry settings (§15): they
// describe this machine's filesystem, like the ffmpeg path, not app configuration that
// should round-trip through a database.
const (
	// RenderNodeEnv picks the DRM render node for VAAPI. A multi-GPU box has renderD129
	// and up, and the right one is not guessable.
	RenderNodeEnv = "PLAYOUT_RENDER_NODE"
)

func renderNode() string {
	if d := os.Getenv(RenderNodeEnv); d != "" {
		return d
	}
	return "/dev/dri/renderD128"
}

// lastSpeed reads ffmpeg's `-progress` key=value stream and returns the final speed.
//
// Structured k=v on its own pipe, NOT stderr scraping: viewra read stderr in 4096-byte
// chunks looking for substrings, and a chunked read can split a token across the buffer
// boundary. A bufio.Scanner over the progress pipe cannot.
func lastSpeed(r interface{ Read([]byte) (int, error) }) float64 {
	var speed float64
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		v, ok := strings.CutPrefix(sc.Text(), "speed=")
		if !ok {
			continue
		}
		// Padded, with a trailing x: "speed=  14x".
		v = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(v), "x"))
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			speed = f
		}
	}
	return speed
}

// channelsFromSpeed converts a realtime multiple into a channel count, with headroom.
//
// A box that encodes at 14x does NOT sustain 14 channels: each is realtime-paced, so 14x
// is the ceiling with nothing left for decode, mux, the rest of the app, or the fact that
// real programs are harder to encode than a test pattern. Two thirds, floored at 1 —
// deliberately conservative, because the operator is told this is a starting estimate they
// can raise ("a test pattern is cheaper to encode than film grain").
func channelsFromSpeed(speed float64) int {
	if speed <= 0 {
		return 1
	}
	if n := int(speed * 2 / 3); n > 1 {
		return n
	}
	return 1
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
