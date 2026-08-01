package filler

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
// — CLAUDE.md §19).
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

	// whisper.cpp wants 16kHz mono wav; ffmpeg extracts just the span.
	wav := filepath.Join(dir, "span.wav")
	if out, err := exec.CommandContext(ctx, t.FFmpegPath,
		"-ss", msToSeconds(startMs), "-to", msToSeconds(endMs),
		"-i", file, "-ar", "16000", "-ac", "1", "-y", wav).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg wav extract: %w: %s", err, out)
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
		"-ss", msToSeconds(startMs), "-to", msToSeconds(endMs),
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

func (t *FFmpegTools) Cut(ctx context.Context, file string, startMs, endMs int64, out string) error {
	// Stream copy (§10 — no re-encode): fast, lossless, and the boundaries being
	// cut at are scene changes, which is where keyframes cluster.
	if combined, err := exec.CommandContext(ctx, t.FFmpegPath,
		"-ss", msToSeconds(startMs), "-to", msToSeconds(endMs),
		"-i", file, "-c", "copy", "-avoid_negative_ts", "make_zero", "-y", out).CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg cut %s: %w: %s", out, err, combined)
	}
	return nil
}

// msToSeconds renders milliseconds as ffmpeg's seconds-with-decimals timestamp.
func msToSeconds(ms int64) string {
	return fmt.Sprintf("%d.%03d", ms/1000, ms%1000)
}
