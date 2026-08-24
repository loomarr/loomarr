package mediatools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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
	// Boundaries runs blackdetect + silencedetect over [startMs,endMs) in one pass. The sources
	// stay separate because detector agreement is the strongest boundary-confidence evidence.
	Boundaries(ctx context.Context, file string, startMs, endMs int64) (black, silence []Interval, err error)
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
	// returns bounded near-full-resolution JPEGs from representative windows,
	// including a closing-card-biased window. The 320px artwork rendition is
	// presentation data, never OCR/semantic evidence. Order is start→end of clip.
	Keyframes(ctx context.Context, file string, n int) ([][]byte, error)
	// KeyframesIn is Keyframes scoped to the measured [startMs,endMs) of ONE segment inside a
	// compilation, before that segment exists as a file. An invalid or unbounded span is refused:
	// the closing-card window and resource ceiling both depend on an honest duration.
	//
	// ⚠ This is what lets the auto-confirm gate be answered from a segment's own pixels rather
	// than from its generated name (§10 V54). `Transcribe` has always taken a span for the same
	// reason; vision only ever needed one because the vision RUNG runs on whole clips.
	KeyframesIn(ctx context.Context, file string, startMs, endMs int64, n int) ([][]byte, error)
	// Cut writes [startMs,endMs) to out with stream copy (no re-encode — §10).
	Cut(ctx context.Context, file string, startMs, endMs int64, out string) error
}

// SpanTranscriber is the swappable speech-to-text seam inside FFmpegTools. The local adapter is
// whisper-cli; the hosted adapter extracts the same span and calls the selected provider. A
// closure selects it so changing the provider setting applies to the next clip without restart.
type SpanTranscriber interface {
	Transcribe(ctx context.Context, file string, startMs, endMs int64) ([]TranscriptSegment, error)
}

// FFmpegTools is the real MediaTools over the vendored binaries (§14).
type FFmpegTools struct {
	FFmpegPath  string
	FFprobePath string
	// WhisperPath/WhisperModel empty ⇒ Transcribe always errors, which the
	// pipeline turns into Unsplittable segments — never into guessed cuts (§15).
	WhisperPath  string
	WhisperModel string
	transcriber  func() SpanTranscriber
	// tmpDir holds whisper's wav/json intermediates; os.MkdirTemp when empty.
	tmpDir string
}

// WithTranscriber attaches a live-selected override. Returning nil from selectTranscriber keeps
// the local whisper implementation, so one FFmpegTools value can hot-switch both directions.
func (t *FFmpegTools) WithTranscriber(selectTranscriber func() SpanTranscriber) *FFmpegTools {
	t.transcriber = selectTranscriber
	return t
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

func (t *FFmpegTools) Boundaries(ctx context.Context, file string, startMs, endMs int64) ([]Interval, []Interval, error) {
	if startMs < 0 || endMs <= startMs {
		return nil, nil, fmt.Errorf("invalid boundary span %d..%d", startMs, endMs)
	}
	// pix_th=0.20, not the 0.10 default: measured (plan §6.4), two of six
	// compilations fade to dark GREY rather than black and score 60–67% at 0.10.
	// Both detectors report on stderr; the null muxer discards the decode. Reset timestamps so
	// detector output is relative to this seeked span, then offset it back to the file timeline.
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, t.FFmpegPath,
		"-nostdin", "-hide_banner", "-nostats", "-v", "info",
		"-ss", msToSeconds(startMs), "-t", msToSeconds(endMs-startMs),
		"-i", file,
		"-vf", "setpts=PTS-STARTPTS,blackdetect=d=0.1:pix_th=0.20",
		"-af", "asetpts=PTS-STARTPTS,silencedetect=n=-35dB:d=0.3",
		"-f", "null", "-")
	cmd.Stderr = &stderr
	// ffmpeg exits 0 on a successful null-muxer pass; the detector output is on
	// stderr either way, so a nonzero exit is the only failure signal.
	if err := cmd.Run(); err != nil {
		return nil, nil, fmt.Errorf("ffmpeg boundary detect %s at %d..%d: %w: %s", file, startMs, endMs, err, stderr.String())
	}
	black, silence := boundaryGaps(stderr.String(), startMs, endMs)
	return black, silence, nil
}

func (t *FFmpegTools) Transcribe(ctx context.Context, file string, startMs, endMs int64) ([]TranscriptSegment, error) {
	if t.transcriber != nil {
		if transcriber := t.transcriber(); transcriber != nil {
			return transcriber.Transcribe(ctx, file, startMs, endMs)
		}
	}
	if t.WhisperPath == "" || t.WhisperModel == "" {
		return nil, fmt.Errorf("whisper not configured (ingest.whisper_path / ingest.whisper_model, §15)")
	}
	dir, err := os.MkdirTemp(t.tmpDir, "loomarr-whisper-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	// whisper.cpp wants 16kHz mono wav; use the shared span extractor so local and hosted
	// transcription cannot drift on seek/duration semantics.
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

// Keyframes samples the WHOLE clip. Prefer KeyframesIn when the caller already owns the measured
// duration: probing again is needless work and makes it impossible for a fake to pin the window.
func (t *FFmpegTools) Keyframes(ctx context.Context, file string, n int) ([][]byte, error) {
	if n <= 0 {
		return nil, nil
	}
	if t.FFprobePath == "" {
		return nil, fmt.Errorf("ffprobe is required to bound semantic frames for %s", file)
	}
	out, err := exec.CommandContext(ctx, t.FFprobePath,
		"-v", "error", "-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", file).Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe semantic-frame duration %s: %w", file, err)
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil || seconds <= 0 {
		return nil, fmt.Errorf("ffprobe semantic-frame duration %s: invalid duration %q", file, strings.TrimSpace(string(out)))
	}
	return t.KeyframesIn(ctx, file, 0, int64(seconds*1000), n)
}

// KeyframesIn is Keyframes scoped to a measured [startMs,endMs).
//
// ⚠ `-ss` before `-i` is an INPUT seek and is the only form fast enough here: it jumps the
// demuxer to the span instead of decoding everything before it. On a 16-minute compilation the
// difference between input- and output-seeking is the difference between per-segment framing
// being viable at all and costing a full decode per segment (§10 V51g's budget rule).
func (t *FFmpegTools) KeyframesIn(ctx context.Context, file string, startMs, endMs int64, n int) ([][]byte, error) {
	if n <= 0 {
		return nil, nil
	}
	if startMs < 0 || endMs <= startMs {
		return nil, fmt.Errorf("semantic frames require a measured span, got %d..%d for %s", startMs, endMs, file)
	}

	// Each small window is decoded independently. `thumbnail` chooses a representative frame
	// inside that window rather than a fade or blurred transition, and the final window begins at
	// 90% so brief closing logos are not systematically missed. `thumbnail=n` means INPUT FRAMES
	// PER GROUP; the retired code passed the requested output count there and consequently took
	// all of its outputs from roughly the opening 0.3 seconds.
	spanMs := endMs - startMs
	starts := semanticWindowStarts(startMs, endMs, n)
	frames := make([][]byte, 0, len(starts))
	for _, seekMs := range starts {
		windowMs := min(int64(3000), endMs-seekMs)
		if windowMs <= 0 {
			continue
		}
		var stdout bytes.Buffer
		cmd := exec.CommandContext(ctx, t.FFmpegPath,
			"-nostdin",
			"-ss", fmt.Sprintf("%.3f", float64(seekMs)/1000),
			"-i", file,
			"-t", fmt.Sprintf("%.3f", float64(windowMs)/1000),
			"-an",
			"-vf", fmt.Sprintf("thumbnail=n=%d,scale=w='min(iw,%d)':h=-2", semanticThumbnailFrames, SemanticFrameMaxWidth),
			"-frames:v", "1",
			"-q:v", "4",
			"-f", "image2pipe", "-c:v", "mjpeg", "-")
		cmd.Stdout = &stdout
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("ffmpeg semantic frame %s at %d/%dms: %w", file, seekMs-startMs, spanMs, err)
		}
		decoded := splitJPEGs(stdout.Bytes())
		if len(decoded) > 0 {
			frames = append(frames, decoded[0])
		}
	}
	return frames, nil
}

// SemanticFrameMaxWidth bounds upload and pixel cost while preserving native resolution for the
// SD/HD archive material Loomarr commonly sees. It is intentionally unrelated to PreviewWidth.
const SemanticFrameMaxWidth = 1920

// About three seconds at ordinary broadcast frame rates. The window itself is also capped at
// three seconds, so this never grows with clip duration or buffers an entire reel.
const semanticThumbnailFrames = 90

func semanticWindowStarts(startMs, endMs int64, n int) []int64 {
	spanMs := endMs - startMs
	if n <= 0 || spanMs <= 0 {
		return nil
	}
	if n == 1 {
		return []int64{startMs + spanMs*90/100}
	}
	first := spanMs * 5 / 100
	rangeMs := spanMs * 85 / 100
	starts := make([]int64, 0, n)
	for i := 0; i < n; i++ {
		starts = append(starts, startMs+first+int64(i)*rangeMs/int64(n-1))
	}
	return starts
}

func (t *FFmpegTools) Cut(ctx context.Context, file string, startMs, endMs int64, out string) error {
	// Stream copy (§10 — no re-encode): fast, lossless, and the boundaries being
	// cut at are scene changes, which is where keyframes cluster.
	//
	// ⚠ Do NOT add `-avoid_negative_ts make_zero`. An input seek with stream copy begins at the
	// preceding keyframe; MP4's edit list hides that preroll and presents the requested span. Forcing
	// timestamps to zero makes the preroll visible instead: measured on a sparse-GOP compilation, a
	// requested 31s segment became 40.683s. The next probe then persisted the inflated duration and
	// a clip that fit a break no longer did. `TestCut_MP4DoesNotExposeKeyframePreroll` pins this.
	if combined, err := exec.CommandContext(ctx, t.FFmpegPath,
		"-ss", msToSeconds(startMs), "-t", msToSeconds(endMs-startMs),
		"-i", file, "-c", "copy", "-y", out).CombinedOutput(); err != nil {
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
