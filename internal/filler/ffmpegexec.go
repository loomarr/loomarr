package filler

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
// (`-show_chapters`), `BlackSilence` runs two detection filters into a null muxer. A helper
// parameterised by every flag would save no code and would hide what each caller is actually
// asking the binary for.
//
// What IS shared here is the pair that was duplicated character-for-character, and the
// path fallback every caller repeated.

// ffmpegOr returns the configured ffmpeg path, or the bare binary name so the OS resolves it
// from PATH.
//
// ⚠ The empty string is the ORDINARY case, not a misconfiguration: `ffmpeg.path` is unset on
// every install that uses the bundled binary, which is most of them. Four call sites wrote this
// three-line fallback out by hand, which is four chances to write `ffmpeg` as a path and get an
// exec error instead of a PATH lookup.
func ffmpegOr(path string) string {
	if path == "" {
		return "ffmpeg"
	}
	return path
}

// extractSpanWAV cuts [startMs, endMs) out of `file` into a 16 kHz mono WAV at `dst`.
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
func extractSpanWAV(ctx context.Context, ffmpegPath, file string, startMs, endMs int64, dst string) error {
	cut := exec.CommandContext(ctx, ffmpegOr(ffmpegPath),
		"-nostdin", "-v", "error",
		"-ss", msToFFmpegTime(startMs), "-t", msToFFmpegTime(endMs-startMs),
		"-i", file, "-vn", "-ac", "1", "-ar", "16000", "-y", dst)
	if out, err := cut.CombinedOutput(); err != nil {
		return fmt.Errorf("extract audio for language: %w: %s", err, truncate(string(out), 200))
	}
	return nil
}

// spanWAVPath is where a backend stages its extracted span. Shared so the two backends cannot
// disagree about the filename inside their own temp dirs — harmless today, confusing the first
// time someone debugs one by listing the other's directory.
func spanWAVPath(dir string) string { return filepath.Join(dir, "span.wav") }
