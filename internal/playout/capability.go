package playout

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/diagnostics"
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
//
// ⚠ **This is the CROSS-VENDOR default, used when the GPU vendor is unknown.** When the vendor IS
// known (preferenceFor), that vendor's NATIVE encoder is moved to the front — see the comment there.
// Vulkan sits high because it sustains high measured throughput and is cross-vendor, but it is NOT
// the right default on a card with a mature native encoder: the Vulkan video-encode drivers are
// young (h264/hevc encode landed only recently, ~ffmpeg 8.x) and vary by driver, and on NVIDIA the
// native NVENC path is more mature and better quality per bit. Raw throughput is not the constraint
// for live playout — even ~14x realtime is many channels of headroom — so maturity and quality win.
// The representative trial (trialEncode) still refuses anything that does not actually feed the HLS
// remux, so a young Vulkan driver that stalls is demoted automatically rather than by exclusion.
//
// Anything ahead of software here must EARN its place by working; ordering only decides
// who gets asked first.
var encoderPreference = []Encoder{
	EncoderVulkan,       // cross-vendor; high measured throughput, but young drivers → not native-first
	EncoderNVENC,        // NVIDIA — mature, good quality per bit
	EncoderQSV,          // Intel Quick Sync — mature
	EncoderVAAPI,        // Intel AND AMD on Linux; the broadest Linux path
	EncoderAMF,          // AMD on Windows
	EncoderVideoToolbox, // Apple Silicon / Intel Macs
	EncoderRKMPP,        // Rockchip SBCs
	EncoderV4L2M2M,      // Raspberry Pi and other V4L2 stateful encoders
	EncoderSoftware,     // always viable — the floor, not a candidate
}

// nativeEncoders maps a GPU vendor to its MATURE native H.264 encoders, most-preferred first. The
// key is matched as a lowercase substring of the probed GPU name ("NVIDIA GeForce RTX 3080 Ti" →
// "nvidia"), so no device-file inspection is needed — the same nvidia-smi/GPU-name signal the rest of
// the app already probes is the only input. A vendor not listed here (or an empty name) keeps the
// cross-vendor encoderPreference order, so unknown hardware still trials everything.
var nativeEncoders = map[string][]Encoder{
	"nvidia": {EncoderNVENC},
	"intel":  {EncoderQSV, EncoderVAAPI}, // QSV is Intel's native; VAAPI is the broader Linux path
	"amd":    {EncoderVAAPI, EncoderAMF}, // VAAPI on Linux, AMF on Windows
	"apple":  {EncoderVideoToolbox},
}

// preferenceFor returns the trial order for a known GPU vendor: that vendor's native encoders first
// (so a mature vendor path is chosen over young cross-vendor Vulkan), then the cross-vendor default
// for everything else, de-duplicated. An unknown/empty vendor returns the default order unchanged.
func preferenceFor(gpuVendor string) []Encoder {
	v := strings.ToLower(strings.TrimSpace(gpuVendor))
	var native []Encoder
	for key, encs := range nativeEncoders {
		if strings.Contains(v, key) {
			native = encs
			break
		}
	}
	if len(native) == 0 {
		return encoderPreference
	}
	seen := make(map[Encoder]bool, len(encoderPreference))
	ordered := make([]Encoder, 0, len(encoderPreference))
	for _, e := range native {
		if !seen[e] {
			seen[e] = true
			ordered = append(ordered, e)
		}
	}
	for _, e := range encoderPreference {
		if !seen[e] {
			seen[e] = true
			ordered = append(ordered, e)
		}
	}
	return ordered
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
	// MaxChannels is the automatic admission budget (§15), derived from the measured speed with
	// headroom (channelsFromSpeed). `playout.max_channels` can only cap this result downward.
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
// Detect picks the encoder and channel capacity for this box. gpuVendor is the probed GPU name (or
// vendor substring); when it names a vendor with a mature native encoder, that encoder is trialled
// first (preferenceFor). Pass "" when the GPU is unknown — the cross-vendor default order applies and
// the trial still decides what actually works.
func Detect(ctx context.Context, ffmpegPath string, p Profile, gpuVendor string) Capacity {
	return DetectObserved(ctx, ffmpegPath, p, gpuVendor, nil)
}

// DetectObserved is Detect with best-effort Process-run diagnostics for each external probe.
func DetectObserved(ctx context.Context, ffmpegPath string, p Profile, gpuVendor string,
	manager *diagnostics.ProcessManager,
) Capacity {
	listed := listEncodersObserved(ctx, ffmpegPath, manager)
	out := Capacity{Chosen: EncoderSoftware, MaxChannels: 1}

	for _, enc := range preferenceFor(gpuVendor) {
		if !listed[enc] {
			// This ffmpeg build has no such encoder. Distinct from a failure: it means
			// "wrong build", not "wrong hardware".
			out.All = append(out.All, Capability{Encoder: enc, Err: "not in this ffmpeg build"})
			continue
		}
		got := trialEncodeObserved(ctx, ffmpegPath, enc, p, trialSeconds, manager)
		got.Available = true
		out.All = append(out.All, got)

		// First working encoder in preference order wins. Software is last in that
		// order, so hardware is preferred whenever it genuinely works.
		if got.Works && out.Chosen == EncoderSoftware && out.MaxChannels == 1 {
			out.Chosen = got.Encoder
			out.MaxChannels = channelsFromSpeed(got.Speed)
		}
	}

	// Re-measure the CHOSEN encoder's speed WARM (§9.1 V49). The pass/fail loop above uses a short
	// probe that reads the cold ramp and under-counts a capable GPU (the 3080-Ti-reads-as-1 bug); a
	// single longer trial on just the winner clears the ramp and reads the sustained peak — paid once,
	// at boot, not per candidate. Software is left on its short-probe figure: it has no cold ramp to
	// clear (the CPU is already warm) and channelsFromSpeed governs it honestly.
	if out.Chosen != EncoderSoftware {
		if warm := trialEncodeObserved(ctx, ffmpegPath, out.Chosen, p, trialSecondsWarm, manager); warm.Works && warm.Speed > 0 {
			out.MaxChannels = channelsFromSpeed(warm.Speed)
		}
		// Clamp to [floor, ceiling] for any hardware encoder: the floor stops a still-low reading from
		// throttling a real GPU to 1; the ceiling stands in for the driver session cap and the
		// test-pattern-is-cheaper-than-film caveat. Software is deliberately NOT clamped.
		if out.MaxChannels < capacityFloor {
			out.MaxChannels = capacityFloor
		}
		if out.MaxChannels > capacityCeiling {
			out.MaxChannels = capacityCeiling
		}
	}
	return out
}

// listEncoders returns the h264 encoders the local ffmpeg build carries.
//
// This is the ONLY honest "could it possibly work here" signal, and it is why the detector
// needs no per-vendor device knowledge: a Pi's build lists h264_v4l2m2m, a Mac's lists
// h264_videotoolbox, and an unknown future encoder is probed the day its build ships.
func listEncodersObserved(ctx context.Context, ffmpegPath string, manager *diagnostics.ProcessManager) map[Encoder]bool {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	out := map[Encoder]bool{}
	args := []string{"-hide_banner", "-encoders"}
	run := manager.Begin(diagnostics.ProcessSpec{Purpose: "encoder_list_probe", Target: "encoders", Executable: ffmpegPath, Args: args})
	raw, err := exec.CommandContext(ctx, ffmpegPath, args...).Output()
	recordExitStderr(run, err)
	if run != nil {
		run.Finish(diagnostics.ProcessResult{Err: err, Cancelled: ctx.Err() != nil, TerminationReason: capabilityTerminationReason(ctx)})
	}
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
//
// ⚠ **The trial mirrors the LIVE pipeline shape — same scale filter, muxed to MPEG-TS, and the
// output is then checked for a keyframe.** It used to encode to `-f null` (decode/encode only,
// output discarded), which is NOT representative: an encoder whose driver produces a stream the
// mpegts muxer cannot segment on a keyframe passed the trial and was then CHOSEN, only to stall the
// live `-c copy -f hls` remux and black the channel for the full 45s timeout. Muxing to a real .ts
// and asserting a keyframe is present makes the probe fail that encoder here instead — which is what
// lets ordering prefer a vendor-native encoder while an immature cross-vendor driver is demoted
// automatically rather than by a hard-coded exclusion.
func trialEncodeObserved(ctx context.Context, ffmpegPath string, enc Encoder, p Profile, seconds int,
	manager *diagnostics.ProcessManager,
) Capability {
	// Bounded: a wedged GPU driver can hang an encode indefinitely, and boot waits on this. The
	// timeout scales with the trial length (a 15s warm trial needs more than a 5s probe's headroom).
	ctx, cancel := context.WithTimeout(ctx, time.Duration(seconds+25)*time.Second)
	defer cancel()

	probe := p
	probe.Encoder = enc

	// A real MPEG-TS output file, mirroring the live child (which muxes mpegts). Removed after the
	// check. Stdout stays reserved for `-progress`, so the mux target must be a file, not pipe:1.
	out, err := os.CreateTemp("", "loomarr-trial-*.ts")
	if err != nil {
		return Capability{Encoder: enc, Err: err.Error()}
	}
	outPath := out.Name()
	_ = out.Close()
	defer func() { _ = os.Remove(outPath) }()

	// ⚠ `-y` IS LOAD-BEARING, and its absence made this entire function vacuous from the commit
	// that introduced it until 2026-08-09.
	//
	// os.CreateTemp above CREATES the file, so the path handed to ffmpeg always exists. Without
	// `-y` ffmpeg refuses to overwrite it — and it does so by EXITING ZERO:
	//
	//	File '/tmp/loomarr-trial-123.ts' already exists. Overwrite? [y/N] Not overwriting - exiting
	//	Error opening output file /tmp/loomarr-trial-123.ts.
	//	$ echo $?
	//	0
	//
	// Exit 0 means `cmd.Wait()` returns nil, so the failure branch never runs; no frames are
	// encoded so `-progress` emits no `speed=` line and Speed stays 0; and the output is 0 bytes,
	// which ffprobe cannot read — so hasKeyframe's deliberate best-effort ("do NOT fail on a probe
	// that cannot run") rubber-stamps it. Three independently reasonable decisions compose into
	// `Works: true` for EVERY encoder the build lists, including ones with no hardware present.
	//
	// Measured consequence on an RTX 3080 Ti: h264_amf reported WORKS (it exits 171 —
	// "DLL libamfrt64.so.1 failed to open"), h264_nvenc reported speed 0.0 (it really runs at
	// 16.2×), and MaxChannels sat at capacityFloor. The nine-family probe never encoded anything.
	//
	// Nothing caught it because the failure is silent in the success direction:
	// TestLive_DetectChoosesSomethingThatActuallyWorks asserts the chosen encoder has Works=true,
	// which is exactly what a vacuous probe reports. The header comment above quotes real per-
	// encoder results, but those were measured by running ffmpeg BY HAND — a measurement the code
	// never reproduced.
	args := []string{"-hide_banner", "-loglevel", "error", "-y", "-progress", "pipe:1", "-nostats"}
	args = append(args, deviceInitArgs(enc)...)
	// `testsrc` rather than a flat colour field: it has detail and motion, so the encoder
	// does representative work. A flat colour compresses to nearly nothing and would
	// report a speed no real program achieves.
	args = append(args, "-f", "lavfi", "-i",
		fmt.Sprintf("testsrc=duration=%d:size=%dx%d:rate=%d",
			seconds, probe.Width, probe.Height, probe.Framerate))
	// The SAME scale/format/upload filter the live child builds (scaleFilterArgs), not just the bare
	// upload — a filter-graph mismatch (CPU frames into a GPU encoder) is one of the real cold-path
	// failures, so the trial must exercise it.
	// No tone-map: the source is a synthetic SDR `testsrc`, so there is no HDR to map and adding
	// the step would make the trial fail on a build without zscale — which is a real, working
	// encoder configuration for every SDR program on that box.
	args = append(args, probe.scaleFilterArgs("")...)
	args = append(args, probe.videoEncodeArgs()...)
	// Mux to MPEG-TS exactly like the child, so an encoder that cannot feed the muxer fails HERE.
	args = append(args, "-f", "mpegts", outPath)

	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Capability{Encoder: enc, Err: err.Error()}
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return Capability{Encoder: enc, Err: err.Error()}
	}
	run := manager.Begin(diagnostics.ProcessSpec{
		Purpose: "encoder_trial_probe", Target: string(enc), Executable: ffmpegPath, Args: args,
	})
	if err := cmd.Start(); err != nil {
		if run != nil {
			run.Finish(diagnostics.ProcessResult{Err: err})
		}
		return Capability{Encoder: enc, Err: err.Error()}
	}

	var stderr strings.Builder
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			line := scanner.Text()
			if stderr.Len() < 64<<10 {
				_, _ = stderr.WriteString(line + "\n")
			}
			if run != nil {
				run.RecordOutput(line)
			}
		}
	}()
	speed := lastSpeedObserved(stdout, run)
	err = cmd.Wait()
	<-stderrDone
	if run != nil {
		run.Finish(diagnostics.ProcessResult{Err: err, Cancelled: ctx.Err() != nil, TerminationReason: capabilityTerminationReason(ctx)})
	}

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

	// The encode exited 0 — but Works also requires the muxed stream to carry a keyframe the HLS
	// remux could cut on. An empty/keyframeless .ts here is exactly the live stall, caught early.
	probeCtx := ctx
	if run != nil {
		probeCtx = diagnostics.WithProcessSpec(ctx, diagnostics.ProcessSpec{ParentRunID: run.ID()})
	}
	if !hasKeyframeObserved(probeCtx, ffmpegPath, outPath, manager) {
		return Capability{Encoder: enc, Err: "encoded but produced no keyframe the HLS remux could segment on"}
	}
	return Capability{Encoder: enc, Works: true, Speed: speed}
}

// hasKeyframe reports whether an MPEG-TS file carries at least one video keyframe. This is the
// property the HLS remux needs to cut a segment; an encoder whose output has none would stall the
// live pipeline, so the trial rejects it. Best-effort: if ffprobe cannot run we do NOT fail the
// encoder on that basis (the encode itself already exited 0) — we only fail on a definitive "no
// keyframe" answer.
func hasKeyframeObserved(ctx context.Context, ffmpegPath, path string, manager *diagnostics.ProcessManager) bool {
	probeBin := ffprobeFor(ffmpegPath)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-select_streams", "v",
		"-show_entries", "packet=flags",
		"-read_intervals", "%+#5", // first few packets are enough; a frame-0 IDR is the target
		"-of", "csv=p=0", path,
	}
	spec, _ := diagnostics.ProcessSpecFromContext(ctx)
	spec.Purpose, spec.Target, spec.Executable, spec.Args = "media_probe", "trial_keyframes", probeBin, args
	run := manager.Begin(spec)
	raw, err := exec.CommandContext(ctx, probeBin, args...).Output()
	recordExitStderr(run, err)
	if run != nil {
		run.Finish(diagnostics.ProcessResult{Err: err, Cancelled: ctx.Err() != nil, TerminationReason: capabilityTerminationReason(ctx)})
	}
	if err != nil {
		return true // can't probe → don't punish an encode that already succeeded
	}
	return strings.Contains(string(raw), "K")
}

// ffprobeFor derives the ffprobe path from the ffmpeg path (they ship together), so a build that
// points FFMPEG at a custom location finds the matching probe rather than a PATH default.
func ffprobeFor(ffmpegPath string) string {
	if ffmpegPath == "" || ffmpegPath == "ffmpeg" {
		return "ffprobe"
	}
	return strings.TrimSuffix(ffmpegPath, "ffmpeg") + "ffprobe"
}

// trialSeconds is the PASS/FAIL probe length — long enough to prove an encoder produces a
// keyframe-bearing stream, short enough that probing every candidate does not stall boot. It is NOT
// long enough to measure sustained SPEED: a hardware encoder ramps from a cold context over ~15–20s
// (measured: nvenc 8.3×→13.3× across 30s), so a 5s probe reads the cold ramp and under-counts a
// capable GPU as ~1 channel. Speed is therefore re-measured warm on the CHOSEN encoder only
// (trialSecondsWarm), which bounds the extra boot cost to a single trial rather than one per candidate.
const trialSeconds = 5

// trialSecondsWarm is the length of the SPEED re-measure on the winning encoder — long enough to
// clear the cold ramp and read the sustained peak. Paid once, at boot, only for the chosen encoder.
const trialSecondsWarm = 15

// capacityFloor / capacityCeiling clamp the throughput-derived channel count for ANY hardware
// encoder (§9.1 V49). The floor stops a mis-measured or momentarily-slow trial from throttling a real
// GPU to 1 (the 3080-Ti-reads-as-1 bug); the ceiling stops an over-optimistic throughput reading (a
// cheap test pattern encodes faster than real film grain) from admitting more than the box sustains —
// it also stands in for NVENC's driver session cap (~8 on modern drivers) without a per-GPU table.
// Software (no hardware) is not floored — it is honestly CPU-bound and channelsFromSpeed governs it.
const (
	capacityFloor   = 2
	capacityCeiling = 12
)

// deviceInitArgs returns args that must appear BEFORE the input — hardware device setup is
// a global option, and placing it after `-i` silently applies to nothing.
func deviceInitArgs(enc Encoder) []string {
	switch engineOf(enc) {
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
	switch engineOf(enc) {
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

// hardwareDecodeArgs returns the `-hwaccel` args that move DECODING onto the GPU.
//
// Decoding, not encoding, is where the CPU actually goes on high-resolution sources. Measured
// on a 4K 10-bit HEVC film with an RTX 3080 Ti:
//
//	CPU decode + CPU scale + CPU encode (libx264)   260% CPU
//	CPU decode + CPU scale + GPU encode (nvenc)     341% CPU   ← encode moved, cost went UP
//	GPU decode + CPU scale + GPU encode (nvenc)     ~0% CPU    ← this
//
// The middle row is the instructive one: moving only the encode made CPU *rise*, because the
// decode was always the real cost and it had simply been throttled by waiting on a slow
// software encoder. "GPU encoding" without GPU decoding buys very little on 4K.
//
// ⚠ NO `-hwaccel_output_format`, deliberately. Setting it keeps decoded frames in GPU memory,
// which is faster still — and turns any unsupported input into a HARD FAILURE instead of a
// silent, correct fallback to software decode. ffmpeg cannot hardware-decode every codec
// (VC-1, some VP9 profiles, anything the GPU generation predates), and a channel must not die
// because one film in its lineup is an odd codec. Without the flag ffmpeg downloads frames to
// CPU memory after decoding, which costs a copy and keeps the CPU filters below working.
//
// That copy is also what makes the SCALE stay on the CPU, and that is a correctness
// requirement rather than an oversight: `scale_cuda` has NO pad option (verified —
// `ffmpeg -h filter=scale_cuda` lists w/h/format/interp_algo/force_original_aspect_ratio and
// nothing else), so 4:3 content through it emits 1440x1080 rather than a letterboxed
// 1920x1080. Measured, not assumed. A channel mixing 4:3 and 16:9 would then break the concat
// parent's `-c copy` mid-stream — the exact failure §5d predicted for a bare
// aspect-preserving scale.
func hardwareDecodeArgs(enc Encoder) []string {
	switch engineOf(enc) {
	case EncoderNVENC:
		return []string{"-hwaccel", "cuda"}
	case EncoderVAAPI, EncoderQSV:
		// Both decode through VAAPI on Linux. The device comes from deviceInitArgs, which
		// already ran — `-hwaccel vaapi` reuses it rather than opening a second one.
		return []string{"-hwaccel", "vaapi"}
	case EncoderVideoToolbox:
		return []string{"-hwaccel", "videotoolbox"}
	case EncoderVulkan:
		return []string{"-hwaccel", "vulkan"}
	default:
		// Software, AMF, RKMPP and V4L2M2M: no hardware decode offered here.
		//
		// AMF is an ENCODE-only interface — its decode path is a separate AMD stack that this
		// ffmpeg build does not wire up. RKMPP and V4L2M2M are SBC encoders whose decoders are
		// unreliable enough across kernels that asking for them costs more failures than it
		// saves cycles. And for libx264 a GPU decode would mean decoding on the GPU only to
		// download every frame for a CPU encode — strictly slower than decoding on the CPU.
		return nil
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
// lastSpeed returns the PEAK realtime multiple across a trial encode's progress samples — the
// encoder's sustained capability once warmed, not whichever sample happened to be last.
//
// ⚠ **Peak, not last, and that is the fix for a capacity under-count.** ffmpeg's early progress
// samples are depressed by cold encoder init (CUDA/VAAPI context setup, filter-graph warmup); for a
// short 5s trial the LAST sample can still be one of those cold ones, or `N/A`/`0x` during teardown.
// Taking the last collapsed a warm ~8x NVENC to ~1x → channelsFromSpeed → 1 hardware channel on a
// GPU that sustains several, which then made the admission gate cap the box at one transcode. The
// peak is stable against the cold ramp and is the honest "how fast can this encoder go" signal.
func lastSpeed(r interface{ Read([]byte) (int, error) }) float64 {
	return lastSpeedObserved(r, nil)
}

func lastSpeedObserved(r interface{ Read([]byte) (int, error) }, run *diagnostics.ProcessHandle) float64 {
	var speed float64
	var current Progress
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		_, complete := consumeProgressLine(strings.TrimSpace(scanner.Text()), &current)
		if current.Speed > speed {
			speed = current.Speed
		}
		if complete && run != nil {
			run.ObserveProgress(diagnostics.ProcessProgress{
				Frame: current.Frame, Speed: current.Speed, OutTimeMS: current.OutTimeMS,
			})
		}
	}
	return speed
}

func capabilityTerminationReason(ctx context.Context) string {
	if ctx.Err() != nil {
		return "context cancelled"
	}
	return ""
}

func recordExitStderr(run *diagnostics.ProcessHandle, err error) {
	if run == nil || err == nil {
		return
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return
	}
	for _, line := range strings.Split(string(exitErr.Stderr), "\n") {
		run.RecordOutput(line)
	}
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
