package mediatools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The shared span extraction (§10 V40/V41). Both language backends feed the result to a model
// that requires 16 kHz mono, so the WAV's shape is a contract, not a detail.
//
// ⚠ These assert what the file IS, via ffprobe — not merely that ffmpeg exited 0. That
// distinction is the whole reason the file exists: extracting at the wrong sample rate, or with
// two channels, succeeds silently. The two backends carried byte-identical copies of this
// invocation with NO test between them, so changing `-ar 16000` to `-ar 8000` was caught by
// nothing at all until this file.
//
// Skipped without ffmpeg, matching `languagesilence_test.go`. Skipping is honest here — the
// binary genuinely is not there — as opposed to faking a probe while leaving a real PATH lookup
// running, which is the shape that shipped a red-on-CI test in V38c.

// probeWAV reports (sampleRate, channels, codec) for a file.
func probeWAV(t *testing.T, path string) (int, int, string) {
	t.Helper()
	out, err := exec.Command("ffprobe",
		"-v", "error", "-select_streams", "a:0",
		"-show_entries", "stream=sample_rate,channels,codec_name",
		"-of", "json", path).Output()
	if err != nil {
		t.Fatalf("ffprobe %s: %v", path, err)
	}
	var probed struct {
		Streams []struct {
			SampleRate string `json:"sample_rate"`
			Channels   int    `json:"channels"`
			CodecName  string `json:"codec_name"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &probed); err != nil {
		t.Fatalf("parse ffprobe json: %v", err)
	}
	if len(probed.Streams) == 0 {
		t.Fatalf("%s has no audio stream", path)
	}
	s := probed.Streams[0]
	rate, err := strconv.Atoi(s.SampleRate)
	if err != nil {
		t.Fatalf("sample_rate %q: %v", s.SampleRate, err)
	}
	return rate, s.Channels, s.CodecName
}

// srcWithTone writes a 3-second stereo 44.1kHz source — deliberately NOT the target shape, so a
// pass proves the extraction converted rather than copied.
func srcWithTone(t *testing.T, dir string) string {
	t.Helper()
	src := filepath.Join(dir, "src.wav")
	cmd := exec.Command("ffmpeg", "-nostdin", "-v", "error",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=3:sample_rate=44100",
		"-ac", "2", "-y", src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build source: %v: %s", err, out)
	}
	return src
}

func TestExtractSpanWAV_ProducesWhatWhisperRequires(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe unavailable")
	}
	dir := t.TempDir()
	src := srcWithTone(t, dir)
	dst := SpanWAVPath(dir)

	if err := ExtractSpanWAV(context.Background(), "", src, 500, 1500, dst); err != nil {
		t.Fatalf("extract: %v", err)
	}

	rate, channels, codec := probeWAV(t, dst)
	// ⚠ 16 kHz MONO is whisper's required input. The source is 44.1kHz stereo, so equality here
	// proves a conversion happened rather than a copy.
	if rate != 16000 {
		t.Errorf("sample rate = %d, want 16000 — whisper's required input shape", rate)
	}
	if channels != 1 {
		t.Errorf("channels = %d, want 1 (mono)", channels)
	}
	if codec != "pcm_s16le" {
		t.Errorf("codec = %q, want pcm_s16le", codec)
	}
}

// ⚠ The span bounds must be HONOURED, not merely accepted: dropping `-t` converts the whole
// file, which still exits 0 and still feeds the model something plausible.
//
// ⚠ It does NOT pin the `-ss`-before-`-i` ORDER, and that limit is worth stating rather than
// leaving for someone to discover. Moving `-ss` after `-i` produces the same correct cut — it
// is a PERFORMANCE property (seek by keyframe vs decode up to the span), not a correctness one,
// so no assertion about the output bytes can catch it. Verified by sabotage: the reorder leaves
// this test green. The comment on `extractSpanWAV` is what guards it.
func TestExtractSpanWAV_CutsOnlyTheRequestedSpan(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe unavailable")
	}
	dir := t.TempDir()
	src := srcWithTone(t, dir) // 3 seconds
	dst := SpanWAVPath(dir)

	if err := ExtractSpanWAV(context.Background(), "", src, 0, 1000, dst); err != nil {
		t.Fatalf("extract: %v", err)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	// 1s of 16kHz mono s16 is ~32KB. The 3s source at 44.1kHz stereo is ~529KB, so anything
	// near that means the span was ignored and the whole file was converted.
	if info.Size() > 80_000 {
		t.Errorf("extracted %d bytes for a 1s span — the whole file came through", info.Size())
	}
	if info.Size() < 8_000 {
		t.Errorf("extracted only %d bytes — the span produced (almost) no samples", info.Size())
	}
}

func TestCut_LateSpanUsesDurationNotAbsoluteEnd(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe unavailable")
	}
	dir := t.TempDir()
	src := srcWithTone(t, dir) // 3 seconds
	dst := filepath.Join(dir, "late.wav")
	tools := NewFFmpegTools("ffmpeg", "ffprobe", "", "", "")
	if err := tools.Cut(context.Background(), src, 1_000, 2_000, dst); err != nil {
		t.Fatal(err)
	}
	raw, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", dst).Output()
	if err != nil {
		t.Fatal(err)
	}
	duration, err := strconv.ParseFloat(strings.TrimSpace(string(raw)), 64)
	if err != nil {
		t.Fatal(err)
	}
	if duration < 0.8 || duration > 1.2 {
		t.Fatalf("late 1s cut duration = %.3fs; absolute -to would produce about 2s", duration)
	}
}

// A stream-copy input seek lands on the preceding keyframe. MP4 hides that preroll with an edit
// list, but `-avoid_negative_ts make_zero` defeats the edit and makes the preroll visible. The
// production symptom was a 31s compilation segment becoming 41-51s after the next probe, with the
// exact inflation depending on where the preceding GOP began. A WAV has no GOP, so the duration
// test above could never catch this class.
func TestCut_MP4DoesNotExposeKeyframePreroll(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe unavailable")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "sparse-gop.mp4")
	// 12fps with a 250-frame GOP puts keyframes about 20.8s apart. Starting at 30.489s therefore
	// has roughly 9.7s of preroll available — large enough that this fails unmistakably if exposed.
	cmd := exec.Command("ffmpeg", "-nostdin", "-v", "error",
		"-f", "lavfi", "-i", "testsrc2=size=160x120:rate=12:duration=70",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=70",
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "250", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-shortest", "-y", src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build sparse-GOP source: %v: %s", err, out)
	}

	dst := filepath.Join(dir, "segment.mp4")
	tools := NewFFmpegTools("ffmpeg", "ffprobe", "", "", "")
	if err := tools.Cut(context.Background(), src, 30_489, 61_489, dst); err != nil {
		t.Fatal(err)
	}
	raw, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", dst).Output()
	if err != nil {
		t.Fatal(err)
	}
	duration, err := strconv.ParseFloat(strings.TrimSpace(string(raw)), 64)
	if err != nil {
		t.Fatal(err)
	}
	if duration < 30.5 || duration > 31.5 {
		t.Fatalf("31s MP4 cut duration = %.3fs; keyframe preroll became visible", duration)
	}
}

// ⚠ A negative duration must not reach ffmpeg. `extractSpanWAV` passes `endMs-startMs`, so an
// inverted span is a subtraction away — and ffmpeg given a negative `-t` writes nothing while
// exiting 0, which the caller reads as a successful extraction of silence.
func TestExtractSpanWAV_ClampsAnInvertedSpan(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	if got := MsToFFmpegTime(-500); got != "0.000" {
		t.Errorf("MsToFFmpegTime(-500) = %q, want 0.000 — a negative -t makes ffmpeg emit nothing", got)
	}
	// And the shared alias must clamp too: it used to be a second, unclamped implementation.
	if got := msToSeconds(-500); got != "0.000" {
		t.Errorf("msToSeconds(-500) = %q, want 0.000 — the two helpers must not disagree", got)
	}
}

func TestFFmpegOr_FallsBackToPathLookup(t *testing.T) {
	if got := FFmpegOr(""); got != "ffmpeg" {
		t.Errorf(`FFmpegOr("") = %q, want "ffmpeg" so the OS resolves it from PATH`, got)
	}
	if got := FFmpegOr("/opt/ffmpeg/bin/ffmpeg"); got != "/opt/ffmpeg/bin/ffmpeg" {
		t.Errorf("ffmpegOr overrode a configured path: %q", got)
	}
}
