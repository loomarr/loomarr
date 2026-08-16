package mediatools

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
)

// Shared ffmpeg invocation helpers.
//
// ⚠ This file is deliberately SMALL, and the things it does NOT unify are the point. The
// package execs ffmpeg/ffprobe from several places, and most of those calls only look alike:
// they pass different flags because they ask different questions. `ffprobeWith` reads
// duration + stream height (`-show_entries`), `FFmpegTools.Chapters` reads chapter markers
// (`-show_chapters`), `Boundaries` runs two detection filters into a null muxer. A helper
// parameterised by every flag would save no code and would hide what each caller is actually
// asking the binary for.
//
// What IS shared here is the pair that was duplicated character-for-character, and the
// path fallback every caller repeated.

// FFmpegOr returns the configured ffmpeg path, or the bare binary name so the OS resolves it
// from PATH.
//
// ⚠ The empty string is the ORDINARY case, not a misconfiguration: `ffmpeg.path` is unset on
// every install that uses the bundled binary, which is most of them. Four call sites wrote this
// three-line fallback out by hand, which is four chances to write `ffmpeg` as a path and get an
// exec error instead of a PATH lookup.
func FFmpegOr(path string) string {
	if path == "" {
		return "ffmpeg"
	}
	return path
}

// ExtractSpanWAV cuts [startMs, endMs) out of `file` into a 16 kHz mono WAV at `dst`.
//
// ⚠ 16 kHz mono is whisper's required input shape, not a preference — both language backends
// feed this to a model that expects it, which is why they share the extraction even though one
// runs whisper locally and the other posts the bytes to a hosted API. The two had drifted into
// byte-identical copies of these six lines plus the same error string; a change to the flags
// would have had to be made twice, correctly, by someone who noticed the second copy.
//
// ⚠ `-ss` and `-t` BEFORE `-i`, so ffmpeg seeks by keyframe rather than decoding up to the span.
// On the ~10s spans the language gate uses that is the difference between milliseconds and a
// full decode of the clip.
func ExtractSpanWAV(ctx context.Context, ffmpegPath, file string, startMs, endMs int64, dst string) error {
	cut := exec.CommandContext(ctx, FFmpegOr(ffmpegPath),
		"-nostdin", "-v", "error",
		"-ss", MsToFFmpegTime(startMs), "-t", MsToFFmpegTime(endMs-startMs),
		"-i", file, "-vn", "-ac", "1", "-ar", "16000", "-y", dst)
	if out, err := cut.CombinedOutput(); err != nil {
		return fmt.Errorf("extract audio for language: %w: %s", err, truncate(string(out), 200))
	}
	return nil
}

// SpanWAVPath is where a backend stages its extracted span. Shared so the two backends cannot
// disagree about the filename inside their own temp dirs — harmless today, confusing the first
// time someone debugs one by listing the other's directory.
func SpanWAVPath(dir string) string { return filepath.Join(dir, "span.wav") }

// MsToFFmpegTime renders milliseconds as ffmpeg's seconds-with-decimals.
func MsToFFmpegTime(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	return fmt.Sprintf("%d.%03d", ms/1000, ms%1000)
}

// truncate shortens a string for a log line. A private copy rather than an import: filler has
// its own, and a shared five-line string helper is not worth a dependency edge in either
// direction.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
