package filler

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Clip thumbnails (V28): one extracted frame per clip, written to disk beside the catalog.
//
// A SEPARATE pass from the ffprobe scan, not an extension of Probed. Probed is deliberately
// atomic — one exec learns duration and height together, and its comment warns that splitting
// them "would create a state where a clip has a duration but silently lost its quality".
// A thumbnail cannot join that bargain: reading metadata is ffprobe, extracting a frame is
// ffmpeg, so it is a second exec no matter what. Keeping it separate means a failed extraction
// costs a missing image, never a missing clip.
//
// ⚠ Failures must be COUNTED, not swallowed. The scan already has a cautionary tale: an earlier
// version handed the ffmpeg path to something that ran it as ffprobe, every probe failed, and
// because unprobeable files are skipped by design the catalog came back silently empty with no
// error anywhere (see FFprobeNextTo). This pass has exactly that shape — best-effort, failures
// skipped — so it returns a failure count and the caller logs it. A misconfigured ffmpeg then
// reads as "0 of 412 thumbnails generated" rather than as thumbnails mysteriously not existing.

// ThumbDirName is the cache directory, created inside FILLER_DIR.
//
// Inside the drop-folder rather than beside the database: the images are derived from what is
// in that folder and share its lifecycle — if an operator moves or empties FILLER_DIR, stale
// thumbnails go with it instead of accumulating somewhere else. Dot-prefixed so it does not
// look like a clip subdirectory to a person browsing, and the scan's extension allowlist
// ignores `.jpg` anyway.
const ThumbDirName = ".loomarr-thumbs"

// thumbAtSeconds is how far into a clip the frame is taken.
//
// Not frame 0: the first frame of a commercial is very often a black fade-in or a slate, so a
// catalog thumbnailed at 0 is a wall of black rectangles. Three seconds is past the fade on a
// 15s spot while still inside the shortest clips worth cataloguing.
const thumbAtSeconds = 3

// Thumbnailer extracts a single frame from src and writes a JPEG to dst. Injected so the
// scanner is testable without executing a binary — the same seam Prober uses.
type Thumbnailer func(ctx context.Context, src, dst string) error

// ThumbPathFor returns a clip's thumbnail path RELATIVE to the thumbnail directory, mirroring
// how a clip's own id is relative to FILLER_DIR (see 00013): the mount differs between host and
// container, so anything absolute would invalidate every row the first time it moves.
//
// The clip's directory structure is preserved rather than flattened, because two clips named
// `intro.mp4` in different era folders are different clips and a flattened name would collide —
// silently, with one overwriting the other's image.
func ThumbPathFor(clipPath string) string {
	if clipPath == "" {
		return ""
	}
	ext := filepath.Ext(clipPath)
	return strings.TrimSuffix(clipPath, ext) + ".jpg"
}

// GenerateThumbnails fills in the Thumbnail field of every clip that does not already have a
// usable image on disk, and returns how many failed.
//
// Existing images are NOT regenerated: a scan runs periodically (FILLER_SYNC_EVERY), and
// re-extracting every frame each pass would spend an ffmpeg exec per clip per cycle forever to
// reproduce a file that is already correct. A changed clip changes its path — which is its
// identity — so a stale-but-matching thumbnail is not a state this can be in.
func GenerateThumbnails(ctx context.Context, dir string, clips []RawClip, extract Thumbnailer) (failed int) {
	if dir == "" || len(clips) == 0 {
		return 0
	}
	if extract == nil {
		extract = FFmpegThumbnail("")
	}
	thumbDir := filepath.Join(dir, ThumbDirName)

	for i := range clips {
		rel := ThumbPathFor(clips[i].Path)
		if rel == "" {
			continue
		}
		dst := filepath.Join(thumbDir, filepath.FromSlash(rel))

		// Already generated: adopt it and move on.
		if info, err := os.Stat(dst); err == nil && info.Size() > 0 {
			clips[i].Thumbnail = rel
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
			failed++
			continue
		}
		if err := extract(ctx, filepath.Join(dir, filepath.FromSlash(clips[i].Path)), dst); err != nil {
			// Leave Thumbnail empty: an empty string renders as no image, which is the honest
			// state. Writing a path to a file that does not exist would render as broken.
			failed++
			// A partial file from a killed ffmpeg would otherwise be adopted as "already
			// generated" on the next pass and never retried.
			_ = os.Remove(dst)
			continue
		}
		clips[i].Thumbnail = rel
	}
	return failed
}

// FFmpegThumbnail returns a Thumbnailer using the given ffmpeg binary ("" ⇒ "ffmpeg").
//
// Takes the ffmpeg path directly rather than deriving it, unlike FFprobeNextTo — this one
// genuinely wants ffmpeg, so `playout.ffmpeg_path` is the right value and there is nothing to
// translate.
func FFmpegThumbnail(ffmpegPath string) Thumbnailer {
	bin := ffmpegPath
	if bin == "" {
		bin = "ffmpeg"
	}
	return func(ctx context.Context, src, dst string) error {
		// -ss BEFORE -i seeks by keyframe without decoding everything up to that point, which
		// on a 30s clip is the difference between milliseconds and a full decode pass.
		// -frames:v 1 stops after one frame; -vf scale bounds the width and lets height follow
		// the aspect ratio (-1), so a 4:3 capture is not stretched into a 16:9 box — the whole
		// point of cataloguing era-accurate commercials is that they look like what they are.
		// -y overwrites, since a partial file from an earlier kill is removed by the caller
		// but a racing pass could still find one.
		cmd := exec.CommandContext(ctx, bin,
			"-nostdin",
			"-ss", fmt.Sprintf("%d", thumbAtSeconds),
			"-i", src,
			"-frames:v", "1",
			"-vf", "scale=320:-1",
			"-q:v", "6",
			"-y", dst,
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			// ffmpeg's diagnostics go to stderr and are the only clue why a clip has no image;
			// truncated because a failure here is one line in a log, not an incident report.
			return fmt.Errorf("thumbnail %s: %w: %s", src, err, truncate(string(out), 200))
		}
		// A clip shorter than the seek point, or one with no video stream, exits 0 having
		// written nothing. Treated as a failure so the caller does not record a path to an
		// empty file.
		if info, statErr := os.Stat(dst); statErr != nil || info.Size() == 0 {
			return fmt.Errorf("thumbnail %s: ffmpeg wrote no frame (audio-only, or shorter than %ds)", src, thumbAtSeconds)
		}
		return nil
	}
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
