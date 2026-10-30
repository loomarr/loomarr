package mediatools

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/loomarr/loomarr/internal/bgexec"

	"github.com/loomarr/loomarr/internal/diagnostics"
	"github.com/loomarr/loomarr/internal/encode"
	"github.com/loomarr/loomarr/internal/playout"
)

// Derivative transcoding (§10 V66) — source masters feed separately identified evidence and
// playback recipes so downstream code can rely on both the bytes and their intended role.
//
// ⚠ **This is a MEZZANINE normalisation, not a BROADCAST one, and the distinction is the whole
// design.** The obvious-looking move is to reuse `playout.DefaultProfile()`. Do not: that is
// 1280×720@25 with `-tune zerolatency` and a forced IDR every two seconds — a LIVE-STREAM profile
// whose job is making concat programs byte-compatible on the fly. Baking it into a file would
// permanently upscale a 480p 4:3 advert and resample a 29.97 NTSC capture to 25, destroying the
// original in the process. And per V50 the broadcast codec is chosen PER CHANNEL, while one clip
// airs on many channels — so there is no single channel codec to target at ingest even in
// principle.
//
// What this stage owes playout is a file that is decodable, seekable, loudness-correct and cheap
// to copy or re-encode. Nothing else.

// MezzanineProfile is the codec-level shape shared by a derivative recipe. Recipe identity and
// measured QC live separately so this type does not become a grab bag of provenance fields.
type MezzanineProfile struct {
	// VideoCodec is the universal floor — everything decodes h264.
	VideoCodec string
	// Family is the hardware family the video encodes on (encode.FileFamily, #2121); empty is
	// software.
	Family encode.Family
	// CRF is the family's constant-quality level (encode.FileLevel): x264's CRF in software, NVENC's
	// -cq, VAAPI's -qp. QUALITY-targeted rather than bitrate-targeted, so a 240p advert and a 1080p
	// one do not get the same bitrate budget: the small one stays small instead of being padded, and
	// the large one is not starved. Named for its software origin; the recipe's JSON key is `crf`.
	CRF int
	// Preset is x264's speed preset; the hardware families carry none.
	Preset string
	// PixelFormat is yuv420p, the format every hardware decoder handles.
	PixelFormat string
	AudioCodec  string
	AudioKbps   int
	AudioRateHz int
	AudioCh     int
	// KeyframeSeconds forces a keyframe at frame 0 and every N seconds, so a cut or a seek lands
	// instantly rather than decoding from the previous GOP.
	KeyframeSeconds int
}

// FileMezzanine is the playback profile for this host (#2121): the family encode.FileFamily picks,
// at the level encode.FileLevel measured for it.
//
// ⚠ **Resolution, framerate, aspect and SAR are deliberately ABSENT.** They are PRESERVED, never
// set — see the note at the top of this file. A profile field for any of them would be an
// invitation to fill it in.
func FileMezzanine(host encode.HostProfile) MezzanineProfile {
	family := encode.FileFamily(host)
	level, _ := encode.FileLevel(family)
	p := MezzanineProfile{
		VideoCodec: "h264", Family: family, CRF: level, PixelFormat: "yuv420p",
		AudioCodec: "aac", AudioKbps: 192, AudioRateHz: 48000, AudioCh: 2,
		KeyframeSeconds: 2,
	}
	if family == encode.FamilySoftware {
		p.Family, p.Preset = "", "veryfast"
	}
	return p
}

// DefaultMezzanine is the software playback profile: FileMezzanine on a host with no usable GPU.
func DefaultMezzanine() MezzanineProfile {
	return FileMezzanine(encode.HostProfile{Family: encode.FamilySoftware})
}

// ID is the profile's stable identity, recorded in the sidecar so a clip encoded under an older
// profile can be told apart from one that has never been transcoded. A hardware family is part of
// it: the same level on another encoder is a different picture.
func (p MezzanineProfile) ID() string {
	if p.Family != "" {
		return fmt.Sprintf("%s-%s-q%d-%s%dk", p.VideoCodec, p.Family, p.CRF, p.AudioCodec, p.AudioKbps)
	}
	return fmt.Sprintf("%s-crf%d-%s%dk", p.VideoCodec, p.CRF, p.AudioCodec, p.AudioKbps)
}

// encoderName is the ffmpeg encoder the profile's family runs (the recipe's VideoEncoder).
func (p MezzanineProfile) encoderName() string {
	return encode.FileVideoArgs(p.fileFamily(), encode.File{})[1]
}

func (p MezzanineProfile) fileFamily() encode.Family {
	if p.Family == "" {
		return encode.FamilySoftware
	}
	return p.Family
}

// TranscodeRequest is one derivative encode from an immutable input.
type TranscodeRequest struct {
	// In is the absolute path of the file to read.
	In string
	// Out is the absolute path to write. May differ in EXTENSION from In — the output is always
	// mp4 — which is what makes the sidecar move below necessary.
	Out string
	// DurationMs is the probed duration, used both for the progress percentage and for the
	// output verification.
	DurationMs int64
	// InputProbe carries optional preservation facts. V66 derivative builders always provide it;
	// legacy callers may leave it nil.
	InputProbe *Probed
	// HadAudio is whether the INPUT carried an audio stream, so the verification can require one
	// in the output only when there was one to begin with.
	HadAudio bool
	// TargetLUFS folds loudness normalisation into the same pass. Zero ⇒ no loudness filter.
	TargetLUFS float64
	Profile    MezzanineProfile
	// Host is this machine's hardware profile: the VAAPI render node a hardware Profile.Family opens.
	Host       encode.HostProfile
	FFmpegPath string
	// Probe re-measures the OUTPUT. Required: an unverified transcode is how a header-only file
	// replaces a good original.
	Probe Prober
	// Diagnostics observes the process best-effort; it is never part of transcode success.
	Diagnostics  *diagnostics.ProcessManager
	ProcessJobID string
}

// Transcode re-encodes one clip and verifies the result.
//
// ⚠ **Temp-then-rename, so a crash mid-encode leaves the original intact** — the pattern V42's
// loudness pass established, kept verbatim because the failure it guards against (a half-written
// clip in the catalog) is identical here. ffmpeg cannot read and write the same path either:
// pointed at its own input it produces a truncated or empty file.
func Transcode(ctx context.Context, req TranscodeRequest, onProgress func(percent int)) (MediaQuality, error) {
	if req.In == "" || req.Out == "" {
		return MediaQuality{}, fmt.Errorf("transcode: both an input and an output are required")
	}
	tmp := req.Out + ".mezz.tmp" + filepath.Ext(req.Out)
	defer func() { _ = os.Remove(tmp) }()

	// A hardware family decodes on its GPU first. A source the GPU cannot decode fails the attempt,
	// and the one retry decodes on the CPU into the same encoder: the playout ladder's rule, applied
	// per clip. Any failure retries, not only encode.IsHardwareDecodeFault: the probe records no
	// codec, and a codec the decoder lacks fails in the graph's format negotiation, not with a
	// decode-fault line. Both attempts are recorded by the process diagnostics.
	if gpuDecodes(req) {
		quality, gpuErr := transcodeOnce(ctx, req, tmp, true, onProgress)
		if gpuErr == nil || ctx.Err() != nil {
			return quality, gpuErr
		}
		quality, err := transcodeOnce(ctx, req, tmp, false, onProgress)
		if err != nil {
			return quality, fmt.Errorf("%w (after GPU decode failed: %v)", err, gpuErr)
		}
		return quality, nil
	}
	return transcodeOnce(ctx, req, tmp, false, onProgress)
}

// gpuDecodes reports whether the encode first tries decoding on the profile's GPU: the families
// whose graph keeps frames on the GPU from decode to encode.
func gpuDecodes(req TranscodeRequest) bool {
	switch req.Profile.fileFamily() {
	case encode.FamilyNVENC, encode.FamilyVAAPI:
		return true
	}
	return false
}

func transcodeOnce(ctx context.Context, req TranscodeRequest, tmp string, gpuDecode bool, onProgress func(percent int)) (MediaQuality, error) {
	args := transcodeArguments(req, tmp, gpuDecode)

	cmd := bgexec.FFmpeg(ctx, FFmpegOr(req.FFmpegPath), args...)
	var stderr boundedBytes
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return MediaQuality{}, fmt.Errorf("transcode %s: stderr pipe: %w", filepath.Base(req.In), err)
	}
	progress, err := cmd.StdoutPipe()
	if err != nil {
		return MediaQuality{}, fmt.Errorf("transcode %s: progress pipe: %w", filepath.Base(req.In), err)
	}
	run := req.Diagnostics.Begin(diagnostics.ProcessSpec{
		Purpose: "filler_transcode", JobID: req.ProcessJobID, Target: req.Profile.ID(),
		Executable: FFmpegOr(req.FFmpegPath), Args: args,
	})
	err = cmd.Start()
	if err != nil {
		_ = progress.Close()
		_ = stderrPipe.Close()
		if run != nil {
			run.Finish(diagnostics.ProcessResult{Err: err})
		}
		return MediaQuality{}, fmt.Errorf("transcode %s: %w: %s", filepath.Base(req.In), err, stderr.String())
	}

	progressDone := make(chan struct{})
	go func() {
		defer close(progressDone)
		// ⚠ The SHARED parser (`playout.ReadProgress`), not a second copy. `out_time_ms` reports
		// microseconds despite its name, and that is exactly the kind of fact that gets fixed in
		// one copy and not the other.
		playout.ReadProgress(progress, func(sample playout.Progress) {
			if run != nil {
				run.ObserveProgress(diagnostics.ProcessProgress{Frame: sample.Frame, Speed: sample.Speed, OutTimeMS: sample.OutTimeMS})
			}
			if onProgress == nil || req.DurationMs <= 0 {
				return
			}
			pct := int(float64(sample.OutTimeMS) / float64(req.DurationMs) * 100)
			if pct < 0 {
				pct = 0
			}
			if pct > 99 {
				// 100 is reserved for "verified and renamed". A bar that reaches 100 and then sits
				// there while the verification runs reads as a hang.
				pct = 99
			}
			onProgress(pct)
		})
	}()
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			line := scanner.Text()
			stderr.add(line)
			if run != nil {
				run.RecordOutput(line)
			}
		}
	}()

	runErr := cmd.Wait()
	<-progressDone
	<-stderrDone
	if run != nil {
		run.Finish(diagnostics.ProcessResult{
			Err: runErr, Cancelled: cmd.Stopped(), TerminationReason: transcodeTerminationReason(cmd.Stopped()),
		})
	}
	if cmd.Stopped() && ctx.Err() != nil {
		return MediaQuality{}, fmt.Errorf("transcode %s: %w: %s", filepath.Base(req.In), ctx.Err(), stderr.String())
	}
	if runErr != nil {
		return MediaQuality{}, fmt.Errorf("transcode %s: %w: %s", filepath.Base(req.In), runErr, stderr.String())
	}

	if err := verifyTranscode(ctx, req, tmp); err != nil {
		return MediaQuality{}, err
	}
	if err := os.Rename(tmp, req.Out); err != nil {
		return MediaQuality{}, fmt.Errorf("transcode %s: %w", filepath.Base(req.In), err)
	}
	if onProgress != nil {
		onProgress(100)
	}
	return qualityFromDetectorOutput(stderr.String(), req.DurationMs), nil
}

// transcodeArguments is one encode on the profile's family. The picture splits once: the encode
// branch keeps the source geometry (on the GPU from decode to encode when gpuDecode), and a
// detector branch brings full-size frames to the CPU, runs the quality detectors and ends.
//
// ⚠ The detectors watch the FULL picture, not a thumbnail. A 160-pixel copy matched blackdetect
// but averaged away the motion freezedetect measures: on 74 real masters it invented freezes in 4
// and doubled one clip's frozen time, enough to send good adverts to review (#2121). Downloading
// full frames cost ~13% more detector CPU and no wall time.
func transcodeArguments(req TranscodeRequest, output string, gpuDecode bool) []string {
	// Detector facts share this encode. `-v info` is required because the three filters report on
	// stderr at info level; `-nostats` keeps that capture bounded to diagnostics and measurements.
	args := []string{"-nostdin", "-hide_banner", "-nostats", "-v", "info"}
	// Background work: bound the DECODER (before -i) as well as the encoder below. See
	// BackgroundThreads.
	args = append(args, "-threads", strconv.Itoa(BackgroundThreads))

	p := req.Profile
	family := p.fileFamily()
	detect := qualityVideoFilters + ",nullsink"
	encodeTail := "format=" + p.PixelFormat
	switch family {
	case encode.FamilyNVENC:
		args = append(args, "-init_hw_device", "cuda=cu:0", "-filter_hw_device", "cu")
		if gpuDecode {
			args = append(args, "-hwaccel", "cuda", "-hwaccel_device", "cu", "-hwaccel_output_format", "cuda")
			detect = "scale_cuda=format=nv12,hwdownload,format=nv12," + detect
			// 8-bit: NVENC has no 10-bit H.264.
			encodeTail = "scale_cuda=format=nv12"
		}
	case encode.FamilyVAAPI:
		node := req.Host.RenderNode
		if node == "" {
			node = encode.RenderNode()
		}
		args = append(args, "-init_hw_device", "vaapi=va:"+node, "-filter_hw_device", "va")
		encodeTail = "format=nv12,hwupload"
		if gpuDecode {
			args = append(args, "-hwaccel", "vaapi", "-hwaccel_device", "va", "-hwaccel_output_format", "vaapi")
			detect = "scale_vaapi=format=nv12,hwdownload,format=nv12," + detect
			encodeTail = "scale_vaapi=format=nv12"
		}
	}
	// stdout carries progress; stderr remains exclusively diagnostics and detector evidence.
	args = append(args, "-progress", "pipe:1", "-i", req.In)
	args = append(args, "-filter_complex", "[0:v]split=2[enc][det];[det]"+detect+";[enc]"+encodeTail+"[v]",
		"-filter_threads", strconv.Itoa(BackgroundThreads), "-filter_complex_threads", strconv.Itoa(BackgroundThreads))
	// One explicit A/V pair is part of the recipe identity. Metadata, chapters, subtitles and data
	// are not semantic media and cannot silently cross into a derivative.
	args = append(args, "-map", "[v]", "-map", "0:a:0?", "-map_metadata", "-1", "-map_chapters", "-1", "-sn", "-dn")

	args = append(args, encode.FileVideoArgs(family, encode.File{Quality: p.CRF, KeyframeSeconds: p.KeyframeSeconds, Preset: p.Preset})...)
	if family == encode.FamilySoftware {
		args = append(args, "-threads", strconv.Itoa(BackgroundThreads), "-x264-params", x264BackgroundParams())
	}
	if req.HadAudio {
		args = append(args, "-c:a", p.AudioCodec,
			"-b:a", strconv.Itoa(p.AudioKbps)+"k",
			"-ar", strconv.Itoa(p.AudioRateHz), "-ac", strconv.Itoa(p.AudioCh))
		audioFilter := qualityAudioFilter
		if req.TargetLUFS != 0 {
			// Loudness belongs only to the playback recipe. It rides the one audio encode rather
			// than creating another lossy generation.
			audioFilter += ",loudnorm=I=" + strconv.FormatFloat(req.TargetLUFS, 'f', -1, 64) + ":TP=-1:LRA=11"
		}
		args = append(args, "-af", audioFilter)
	} else {
		args = append(args, "-an")
	}
	// Preserve cadence and geometry, normalize negative timestamp origin without independently
	// resetting A/V streams, and put the moov atom first for bounded startup/seek behavior.
	args = append(args, "-fps_mode", "passthrough", "-avoid_negative_ts", "make_zero", "-movflags", "+faststart", "-y", output)
	return args
}

type boundedBytes struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *boundedBytes) add(line string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, _ = b.buf.WriteString(line + "\n")
	const limit = 256 << 10
	if b.buf.Len() > limit {
		data := append([]byte(nil), b.buf.Bytes()[b.buf.Len()-limit:]...)
		b.buf.Reset()
		_, _ = b.buf.Write(data)
	}
}

func (b *boundedBytes) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func transcodeTerminationReason(stopped bool) string {
	if stopped {
		return "process tree stopped"
	}
	return ""
}

// verifyTranscodeToleranceMs is how far the output's duration may drift from the input's.
//
// Half a second: container rounding and a trailing partial frame both move the number slightly,
// while the failure being caught here — ffmpeg exiting 0 having written a header and no samples —
// misses by the whole clip.
const verifyTranscodeToleranceMs = 500

// verifyTranscode re-probes the output and refuses an implausible one.
//
// ⚠ **This deliberately does NOT reuse the retired loudness pass's `size < orig/10` guard, and
// the divergence is the point.** That check is right for an audio-only rewrite, where the video is
// copied and the file size can barely move. It is wrong for a real transcode: a bloated MJPEG or
// uncompressed original legitimately drops well below a tenth of its size at CRF 20, so the guard
// would reject good encodes — and rejecting a good encode here means refusing the clip
// (`transcode` is fatal). Re-probing catches the same real failure with no false positive: a
// header with no samples has no streams and no duration.
func verifyTranscode(ctx context.Context, req TranscodeRequest, tmp string) error {
	if _, err := os.Stat(tmp); err != nil {
		return fmt.Errorf("transcode %s: no output was written: %w", filepath.Base(req.In), err)
	}
	if req.Probe == nil {
		return fmt.Errorf("transcode %s: refusing to install an unverified encode", filepath.Base(req.In))
	}
	out, err := req.Probe(ctx, tmp)
	if err != nil {
		return fmt.Errorf("transcode %s: the output could not be probed: %w", filepath.Base(req.In), err)
	}
	if out.Height <= 0 {
		return fmt.Errorf("transcode %s: the output has no video stream", filepath.Base(req.In))
	}
	if req.HadAudio && out.Silent {
		return fmt.Errorf("transcode %s: the input had audio and the output has none", filepath.Base(req.In))
	}
	if req.DurationMs > 0 {
		if drift := math.Abs(float64(out.DurationMs - req.DurationMs)); drift > verifyTranscodeToleranceMs {
			return fmt.Errorf("transcode %s: the output is %.1fs against an input of %.1fs",
				filepath.Base(req.In), float64(out.DurationMs)/1000, float64(req.DurationMs)/1000)
		}
	}
	if req.InputProbe != nil {
		if err := verifyPreservedMedia(*req.InputProbe, out); err != nil {
			return fmt.Errorf("transcode %s: %w", filepath.Base(req.In), err)
		}
	}
	return nil
}

func verifyPreservedMedia(input, output Probed) error {
	if (input.Width > 0 && output.Width != input.Width) || (input.Height > 0 && output.Height != input.Height) {
		return fmt.Errorf("geometry changed from %dx%d to %dx%d", input.Width, input.Height, output.Width, output.Height)
	}
	for _, value := range []struct {
		name   string
		before string
		after  string
	}{
		{name: "cadence", before: input.Cadence, after: output.Cadence},
		{name: "sample aspect", before: input.SampleAspect, after: output.SampleAspect},
		{name: "display aspect", before: input.DisplayAspect, after: output.DisplayAspect},
	} {
		if value.before != "" && !equivalentMediaRatio(value.before, value.after) {
			return fmt.Errorf("%s changed from %q to %q", value.name, value.before, value.after)
		}
	}
	if input.FieldOrder != "" && output.FieldOrder != input.FieldOrder {
		return fmt.Errorf("field order changed from %q to %q without a declared interlace recipe", input.FieldOrder, output.FieldOrder)
	}
	if input.VideoTimingKnown && input.AudioTimingKnown {
		if !output.VideoTimingKnown || !output.AudioTimingKnown {
			return errors.New("A/V timing became unavailable")
		}
		inputStartSkew := input.AudioStartMs - input.VideoStartMs
		outputStartSkew := output.AudioStartMs - output.VideoStartMs
		if math.Abs(float64(outputStartSkew-inputStartSkew)) > 50 {
			return fmt.Errorf("A/V start skew changed from %dms to %dms", inputStartSkew, outputStartSkew)
		}
		inputEndSkew := input.AudioStartMs + input.AudioDurationMs - input.VideoStartMs - input.VideoDurationMs
		outputEndSkew := output.AudioStartMs + output.AudioDurationMs - output.VideoStartMs - output.VideoDurationMs
		if math.Abs(float64(outputEndSkew-inputEndSkew)) > verifyTranscodeToleranceMs {
			return fmt.Errorf("A/V end skew changed from %dms to %dms", inputEndSkew, outputEndSkew)
		}
	}
	return nil
}

func equivalentMediaRatio(before, after string) bool {
	if before == "" || after == "" {
		return false
	}
	parse := func(value string) (*big.Rat, bool) {
		value = strings.ReplaceAll(value, ":", "/")
		ratio, ok := new(big.Rat).SetString(value)
		return ratio, ok && ratio.Sign() > 0
	}
	left, leftOK := parse(before)
	right, rightOK := parse(after)
	return leftOK && rightOK && left.Cmp(right) == 0
}

// BackgroundThreads caps every filler ffmpeg's worker threads (#1512 G5).
//
// ⚠ Production ran two libx264 software encodes per clip with NO thread cap and measured about
// 350% CPU in a 4-CPU container — every page and every live stream slowed for the duration. One
// thread per stage keeps a batch encode to roughly one core plus the muxer; it is slower per clip,
// which is the right trade for work nobody is waiting on. Paired with proctree.LowPriority so that
// even that core yields to playback.
//
// Measured on a 4-CPU pin (720p): uncapped ~393%, two threads ~196–200% (no margin, and 1080p
// exceeds it), one thread ~135–168%.
const BackgroundThreads = 1

// x264BackgroundParams bounds libx264's own thread pool, which `-threads` alone does not: with
// two frame threads libx264 also runs a separate lookahead thread and peaked at ~256%. One thread
// runs the lookahead synchronously on the encoding thread, so there is nothing else to bound.
func x264BackgroundParams() string {
	return "threads=" + strconv.Itoa(BackgroundThreads)
}
