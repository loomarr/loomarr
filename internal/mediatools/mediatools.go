package mediatools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// MediaTools is the split pipeline's exec boundary (§10 V34): everything the
// domain needs from ffmpeg/ffprobe/whisper-cli, nothing more. Unit tests fake
// it; the real implementation (FFmpegTools) shells the vendored binaries and
// is exercised only against real media (unit tests never require the binaries
// — AGENTS.md §19).
type MediaTools interface {
	// Chapters reads embedded chapters (triage — a pre-chaptered compilation
	// splits for free). An empty slice means none, which is the common case
	// (6 of 8 measured sources had none).
	Chapters(ctx context.Context, file string) ([]Chapter, error)
	// BlackSilence runs blackdetect + silencedetect in ONE pass and returns both
	// interval sets (boundaries the coarse split cuts at).
	BlackSilence(ctx context.Context, file string) (blacks, silences []Interval, err error)
	// Transcribe runs whisper over [startMs,endMs) and returns timestamped
	// utterances relative to that span. Errors when whisper is unrunnable — the
	// caller then marks the segment Unsplittable rather than guessing.
	Transcribe(ctx context.Context, file string, startMs, endMs int64) ([]TranscriptSegment, error)
	// GrayFrames decodes the span at 1/3fps to 9x8 grayscale frames (dHash input,
	// ~30 lines of pure Go over the bytes — no library).
	GrayFrames(ctx context.Context, file string, startMs, endMs int64) ([][]byte, error)
	// Keyframes samples n viewable JPEG frames spread across the clip — the vision
	// tier's input (§10 V44) and the frame-heuristic tier's (framehints.go).
	//
	// ⚠ **Deliberately NOT GrayFrames.** GrayFrames is 9x8 grayscale dHash food —
	// it has thrown away colour (so it cannot see a B&W transfer) and resolution
	// (so it cannot read a logo), the two things both V44 tiers exist to read. This
	// returns real ~320px JPEGs, the same asset FFmpegArtwork produces, but N of
	// them across the duration rather than one still. Order is start→end of clip.
	Keyframes(ctx context.Context, file string, n int) ([][]byte, error)
	// KeyframesIn is Keyframes scoped to [startMs,endMs) — the frames of ONE segment inside a
	// compilation, before that segment exists as a file. endMs <= startMs means "to the end".
	//
	// ⚠ This is what lets the auto-confirm gate be answered from a segment's own pixels rather
	// than from its generated name (§10 V54). `Transcribe` has always taken a span for the same
	// reason; vision only ever needed one because the vision RUNG runs on whole clips.
	KeyframesIn(ctx context.Context, file string, startMs, endMs int64, n int) ([][]byte, error)
	// Cut writes [startMs,endMs) to out with stream copy (no re-encode — §10).
	Cut(ctx context.Context, file string, startMs, endMs int64, out string) error
}

// FFmpegTools is the real MediaTools over the vendored binaries (§14).
type FFmpegTools struct {
	FFmpegPath  string
	FFprobePath string
	// WhisperPath/WhisperModel empty ⇒ Transcribe always errors, which the
	// pipeline turns into Unsplittable segments — never into guessed cuts (§15).
	WhisperPath  string
	WhisperModel string
	// tmpDir holds whisper's wav/json intermediates; os.MkdirTemp when empty.
	tmpDir string
}

// NewFFmpegTools builds the real tool bridge. tmpDir may be "" (system temp).
func NewFFmpegTools(ffmpegPath, ffprobePath, whisperPath, whisperModel, tmpDir string) *FFmpegTools {
	return &FFmpegTools{
		FFmpegPath: ffmpegPath, FFprobePath: ffprobePath,
		WhisperPath: whisperPath, WhisperModel: whisperModel, tmpDir: tmpDir,
	}
}

var _ MediaTools = (*FFmpegTools)(nil)

func (t *FFmpegTools) Chapters(ctx context.Context, file string) ([]Chapter, error) {
	out, err := exec.CommandContext(ctx, t.FFprobePath,
		"-v", "quiet", "-print_format", "json", "-show_chapters", file).Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe chapters %s: %w", file, err)
	}
	return parseFFprobeChapters(out)
}

func (t *FFmpegTools) BlackSilence(ctx context.Context, file string) ([]Interval, []Interval, error) {
	// pix_th=0.20, not the 0.10 default: measured (plan §6.4), two of six
	// compilations fade to dark GREY rather than black and score 60–67% at 0.10.
	// Both detectors report on stderr; the null muxer discards the decode.
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, t.FFmpegPath,
		"-i", file,
		"-vf", "blackdetect=d=0.1:pix_th=0.20",
		"-af", "silencedetect=n=-35dB:d=0.3",
		"-f", "null", "-")
	cmd.Stderr = &stderr
	// ffmpeg exits 0 on a successful null-muxer pass; the detector output is on
	// stderr either way, so a nonzero exit is the only failure signal.
	if err := cmd.Run(); err != nil {
		return nil, nil, fmt.Errorf("ffmpeg black/silence detect %s: %w: %s", file, err, stderr.String())
	}
	return parseBlackdetect(stderr.String()), parseSilencedetect(stderr.String()), nil
}

func (t *FFmpegTools) Transcribe(ctx context.Context, file string, startMs, endMs int64) ([]TranscriptSegment, error) {
	if t.WhisperPath == "" || t.WhisperModel == "" {
		return nil, fmt.Errorf("whisper not configured (ingest.whisper_path / ingest.whisper_model, §15)")
	}
	dir, err := os.MkdirTemp(t.tmpDir, "loomarr-whisper-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	// whisper.cpp wants 16kHz mono wav; use the shared span extractor so every caller has the same
	// seek/duration semantics.
	wav := filepath.Join(dir, "span.wav")
	if err := ExtractSpanWAV(ctx, t.FFmpegPath, file, startMs, endMs, wav); err != nil {
		return nil, err
	}
	base := filepath.Join(dir, "out")
	if out, err := exec.CommandContext(ctx, t.WhisperPath,
		"-m", t.WhisperModel, "-f", wav, "-oj", "-of", base, "-np").CombinedOutput(); err != nil {
		return nil, fmt.Errorf("whisper-cli: %w: %s", err, out)
	}
	raw, err := os.ReadFile(base + ".json")
	if err != nil {
		return nil, fmt.Errorf("whisper-cli wrote no JSON: %w", err)
	}
	return parseWhisperJSON(raw)
}

func (t *FFmpegTools) GrayFrames(ctx context.Context, file string, startMs, endMs int64) ([][]byte, error) {
	// 1/3fps over 9x8 gray pixels: the §6.4-measured duplicate detector (mean
	// per-frame Hamming 1.1 for a re-encoded duplicate vs 27.6–32.2 different).
	var stdout bytes.Buffer
	cmd := exec.CommandContext(ctx, t.FFmpegPath,
		"-ss", msToSeconds(startMs), "-t", msToSeconds(endMs-startMs),
		"-i", file, "-vf", "fps=1/3,scale=9:8", "-pix_fmt", "gray",
		"-f", "rawvideo", "-")
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg gray frames %s: %w", file, err)
	}
	const frameBytes = 9 * 8
	raw := stdout.Bytes()
	var frames [][]byte
	for len(raw) >= frameBytes {
		frame := make([]byte, frameBytes)
		copy(frame, raw[:frameBytes])
		frames = append(frames, frame)
		raw = raw[frameBytes:]
	}
	return frames, nil
}

// Keyframes samples the WHOLE clip — the span-less form every existing caller wants.
func (t *FFmpegTools) Keyframes(ctx context.Context, file string, n int) ([][]byte, error) {
	return t.KeyframesIn(ctx, file, 0, 0, n)
}

// KeyframesIn is Keyframes scoped to [startMs,endMs). endMs <= startMs means "to the end".
//
// ⚠ `-ss` before `-i` is an INPUT seek and is the only form fast enough here: it jumps the
// demuxer to the span instead of decoding everything before it. On a 16-minute compilation the
// difference between input- and output-seeking is the difference between per-segment framing
// being viable at all and costing a full decode per segment (§10 V51g's budget rule).
func (t *FFmpegTools) KeyframesIn(ctx context.Context, file string, startMs, endMs int64, n int) ([][]byte, error) {
	// n frames spread ACROSS the clip, decoded to real JPEGs — mirrors the
	// FFmpegArtwork still (scale ~320px wide, JPEG at -q:v) but produces several
	// samples rather than one. A commercial's brand card, its B&W transfer, its
	// end slate can each fall in a different part of the runtime, so one frame is
	// not enough signal for either V44 tier (vision or framehints).
	if n <= 0 {
		return nil, nil
	}
	// `thumbnail=n=…` is ffmpeg's own "most representative frame of a group"
	// selector: it scores frames within each window and picks the least-blurry,
	// most-distinct one, which beats a flat every-Kth-frame sample for a clip that
	// opens on a black fade. We ask for `n` groups by sizing the window to the
	// clip's frame count / n — but we do not know the frame count without probing,
	// so we use `select` on a normalised timeline instead: `n` evenly-spaced picks.
	//
	// The expression selects a frame when its presentation time crosses one of n
	// evenly-spaced marks over the (probed) duration. We avoid a second probe by
	// letting ffmpeg compute it: `select='isnan(prev_selected_t)+gte(t-prev_selected_t\,DUR/n)'`
	// is fragile across builds, so the robust portable form is fps-based — sample
	// at a rate that yields ~n frames, capped by `-frames:v n`. A clip's exact
	// length is unknown here, so we oversample slightly and let the frame cap trim.
	//
	// ⚠ `-vsync vfr` (a.k.a `-fps_mode vfr`) keeps the selected frames at their own
	// timestamps rather than duplicating to a constant rate — without it a short
	// clip sampled sparsely gets padded with repeats, and the heuristics would then
	// score the same frame n times.
	var stdout bytes.Buffer
	args := []string{"-nostdin"}
	// Input seek (before -i). Skipped entirely when no span is asked for, so the whole-clip
	// path emits byte-identical arguments to what it always did.
	if startMs > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", float64(startMs)/1000))
	}
	args = append(args, "-i", file)
	if endMs > startMs {
		// `-t` (duration), not `-to`: after an input seek `-to` is interpreted against the
		// ORIGINAL timeline on some builds and the seeked one on others, so a duration is the
		// portable way to say "this much of it".
		args = append(args, "-t", fmt.Sprintf("%.3f", float64(endMs-startMs)/1000))
	}
	args = append(args, "-an")
	cmd := exec.CommandContext(ctx, t.FFmpegPath, append(args,
		// thumbnail=n groups the decode into n buckets and emits the single most
		// representative frame of each — exactly n frames for a clip long enough to
		// fill the buckets, fewer for a very short one, which is fine (the caller
		// tolerates <n).
		"-vf", fmt.Sprintf("thumbnail=n=%d,scale=%d:-1", n, PreviewWidth),
		"-frames:v", fmt.Sprintf("%d", n),
		// mjpeg over image2pipe streams the JPEGs back to us concatenated; -q:v 6
		// matches the still's quality (artwork.go) — a vision model does not need
		// archival frames, and a B&W/aspect check needs even less.
		"-q:v", "6",
		"-f", "image2pipe", "-c:v", "mjpeg", "-",
	)...)
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg keyframes %s: %w", file, err)
	}
	// image2pipe concatenates whole JPEGs; split on the SOI/EOI markers so each
	// element is one decodable frame. A clip with no video stream yields nothing,
	// which the caller reads as "no frames to look at" rather than an error.
	return splitJPEGs(stdout.Bytes()), nil
}

func (t *FFmpegTools) Cut(ctx context.Context, file string, startMs, endMs int64, out string) error {
	// Stream copy (§10 — no re-encode): fast, lossless, and the boundaries being
	// cut at are scene changes, which is where keyframes cluster.
	if combined, err := exec.CommandContext(ctx, t.FFmpegPath,
		"-ss", msToSeconds(startMs), "-t", msToSeconds(endMs-startMs),
		"-i", file, "-c", "copy", "-avoid_negative_ts", "make_zero", "-y", out).CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg cut %s: %w: %s", out, err, combined)
	}
	return nil
}

// msToSeconds renders milliseconds as ffmpeg's seconds-with-decimals timestamp.
// msToSeconds renders milliseconds as ffmpeg's `SS.mmm`.
//
// ⚠ One implementation, in `msToFFmpegTime`. This was a second copy of the same body — same
// format string, same arithmetic — differing only in that it did NOT clamp negatives. Keeping
// the clamping one is strictly safer for both callers: the extraction helper passes a
// SUBTRACTION (`endMs-startMs`), and a negative `-t` makes ffmpeg emit nothing at all, silently.
func msToSeconds(ms int64) string { return MsToFFmpegTime(ms) }
