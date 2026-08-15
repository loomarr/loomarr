package mediatools

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/mantonx/loomarr/internal/playout"
)

// Mezzanine transcoding (§10 V51b) — every clip is re-encoded once, to one profile, so that
// everything downstream can rely on what it is holding.
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

// MezzanineProfile is the one shape every clip is re-encoded to.
type MezzanineProfile struct {
	// VideoCodec is the universal floor — everything decodes h264.
	VideoCodec string
	// CRF is QUALITY-targeted rather than bitrate-targeted, so a 240p advert and a 1080p one do
	// not get the same bitrate budget: the small one stays small instead of being padded, and the
	// large one is not starved.
	CRF int
	// Preset trades encode time for size. `medium` — this is a batch job with nobody waiting.
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

// DefaultMezzanine is the profile V51b ships.
//
// ⚠ **Resolution, framerate, aspect and SAR are deliberately ABSENT.** They are PRESERVED, never
// set — see the note at the top of this file. A profile field for any of them would be an
// invitation to fill it in.
func DefaultMezzanine() MezzanineProfile {
	return MezzanineProfile{
		VideoCodec: "h264", CRF: 20, Preset: "medium", PixelFormat: "yuv420p",
		AudioCodec: "aac", AudioKbps: 192, AudioRateHz: 48000, AudioCh: 2,
		KeyframeSeconds: 2,
	}
}

// ID is the profile's stable identity, recorded in the sidecar so a clip encoded under an older
// profile can be told apart from one that has never been transcoded.
func (p MezzanineProfile) ID() string {
	return fmt.Sprintf("%s-crf%d-%s%dk", p.VideoCodec, p.CRF, p.AudioCodec, p.AudioKbps)
}

// TranscodeRequest is one clip's re-encode.
type TranscodeRequest struct {
	// In is the absolute path of the file to read.
	In string
	// Out is the absolute path to write. May differ in EXTENSION from In — the output is always
	// mp4 — which is what makes the sidecar move below necessary.
	Out string
	// DurationMs is the probed duration, used both for the progress percentage and for the
	// output verification.
	DurationMs int64
	// HadAudio is whether the INPUT carried an audio stream, so the verification can require one
	// in the output only when there was one to begin with.
	HadAudio bool
	// TargetLUFS folds loudness normalisation into the same pass. Zero ⇒ no loudness filter.
	TargetLUFS float64
	Profile    MezzanineProfile
	FFmpegPath string
	// Probe re-measures the OUTPUT. Required: an unverified transcode is how a header-only file
	// replaces a good original.
	Probe Prober
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

	// Detector facts share this encode. `-v info` is required because the three filters report on
	// stderr at info level; `-nostats` keeps that capture bounded to diagnostics and measurements.
	args := []string{"-nostdin", "-hide_banner", "-nostats", "-v", "info"}
	// ⚠ fd 3 carries the progress stream. `-progress pipe:3` keeps it OFF stderr, which is where
	// ffmpeg also writes real errors — scraping the two out of one stream is what viewra did, and
	// a chunked read there can split a token across the buffer boundary.
	args = append(args, "-progress", "pipe:3")
	args = append(args, "-i", req.In)

	p := req.Profile
	args = append(args,
		"-c:v", "libx264", "-crf", strconv.Itoa(p.CRF), "-preset", p.Preset,
		"-pix_fmt", p.PixelFormat,
		"-vf", qualityVideoFilters)
	if p.KeyframeSeconds > 0 {
		// A keyframe at frame 0 and every N seconds. `expr:gte(t,n_forced*N)` is the form that
		// also guarantees the FIRST frame is an IDR — a clip whose first keyframe arrives late is
		// the black-screen-on-start class §9.1 records.
		args = append(args, "-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%d)", p.KeyframeSeconds))
	}
	if req.HadAudio {
		args = append(args, "-c:a", p.AudioCodec,
			"-b:a", strconv.Itoa(p.AudioKbps)+"k",
			"-ar", strconv.Itoa(p.AudioRateHz), "-ac", strconv.Itoa(p.AudioCh))
		audioFilter := qualityAudioFilter
		if req.TargetLUFS != 0 {
			// ⚠ Loudness rides along in the pass that is already re-encoding the audio, rather
			// than as a second rewrite of the same file. V42's standalone loudness pass existed
			// for the case where only the audio changes and the video is copied; here we are
			// re-encoding anyway, so a separate pass would be a second generation of loss for
			// nothing. (That pass is retired — it had no production caller, so this is the first
			// time `filler.autofile.normalize_loudness` has actually done anything.)
			//
			// Single-pass loudnorm, unlike the two-pass form that pass used: two-pass wants a
			// measurement run over the whole file, which here would mean decoding the source
			// twice on top of an already-expensive encode. The first-second ramp single-pass
			// leaves is the accepted cost, and playout normalises again anyway.
			audioFilter += ",loudnorm=I=" + strconv.FormatFloat(req.TargetLUFS, 'f', -1, 64) + ":TP=-1:LRA=11"
		}
		args = append(args, "-af", audioFilter)
	} else {
		args = append(args, "-an")
	}
	// faststart puts the moov atom first, so a player (and Tunarr's probe) can start without
	// reading to the end of the file.
	args = append(args, "-movflags", "+faststart", "-y", tmp)

	cmd := exec.CommandContext(ctx, FFmpegOr(req.FFmpegPath), args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	pr, pw, err := os.Pipe()
	if err != nil {
		return MediaQuality{}, fmt.Errorf("transcode %s: progress pipe: %w", filepath.Base(req.In), err)
	}
	cmd.ExtraFiles = []*os.File{pw} // becomes fd 3 in the child

	done := make(chan struct{})
	go func() {
		defer close(done)
		// ⚠ The SHARED parser (`playout.ReadProgress`), not a second copy. `out_time_ms` reports
		// microseconds despite its name, and that is exactly the kind of fact that gets fixed in
		// one copy and not the other.
		playout.ReadProgress(pr, func(sample playout.Progress) {
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

	runErr := cmd.Run()
	_ = pw.Close() // unblocks the reader; the child no longer holds fd 3
	<-done
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
	return nil
}
