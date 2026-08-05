package filler

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Clip artwork (V39): the still frame AND the animated hover preview, from ONE ffmpeg pass.
//
// The catalog is a grid of single frames, and one frame of a commercial is often ambiguous — a
// title card, a face, a product shot. "Is this actually the advert it says it is?" is the question
// the whole Filler page exists to answer, and a still cannot answer it. A short animation can,
// without the operator opening anything.
//
// ⚠ **A generated ASSET, not the clip streamed on hover.** Playing the source file on hover was
// the cheaper option and the wrong one at catalog scale: a grid of 50 cards would open a range
// request per hovered card against files tens of megabytes each, to show six seconds of a
// 30-second advert. This pays once, per clip, and the hover then costs one cached asset.
//
// ⚠ **One exec, two outputs, ONE decode** (maintainer, 2026-08-03). ffmpeg writes both files from
// a single `split` filter, so the first scan of a large drop-folder costs one process and one
// decode per clip rather than two. Measured at ~0.47s per clip for both together on a 3MB spot.
//
// The consequence of sharing an invocation is that they must share a SEEK — see previewStartFor.
// That was the deliberate tradeoff: the still is now the animation's opening frame, so the two
// always show the same moment. Before this they could disagree, which was never a feature.

// ThumbDirName is the artwork cache directory, created inside FILLER_DIR.
//
// Inside the drop-folder rather than beside the database: the files are derived from what is in
// that folder and share its lifecycle — if an operator moves or empties FILLER_DIR, stale artwork
// goes with it instead of accumulating somewhere else. Dot-prefixed so it does not look like a
// clip subdirectory to a person browsing, and the scan's extension allowlist ignores `.jpg` and
// `.webp` anyway.
const ThumbDirName = ".loomarr-thumbs"

// PreviewDirName is the same directory. Both assets are derived from the same folder, share its
// lifecycle, and are invalidated by the same events; a second dot-directory would be a second
// thing to explain, to clean up, and to forget in the backup exclusion list.
const PreviewDirName = ThumbDirName

// previewSeconds is how much motion a preview carries.
//
// Six seconds is long enough to show what an advert IS — a scene change or two, usually the
// product — and short enough that the loop does not become something an operator sits and
// watches. It is also under the length of the shortest clips worth cataloguing, so the window is
// rarely longer than the clip it comes from.
const previewSeconds = 6

// previewFPS and previewQuality: the maintainer chose smoothness over cache size (2026-08-03),
// measured at ~460KB per 30-second spot. That is ~230MB across a 500-clip catalog — all of it
// regenerable, all of it excluded from the §16 backup, and none of it in the database.
//
// Measured alternatives, kept here so the tradeoff does not have to be rediscovered:
// q=50/10fps ≈ 110KB, q=40/8fps ≈ 79KB. Below ~8fps camera pans start to strobe.
const previewFPS = 12
const previewQuality = 55

// previewWidth is shared by both outputs. The card renders the still and the animation in the
// same box and swaps between them on hover, so differing widths would visibly jump at the moment
// of the swap — the one frame where the two are compared directly.
const previewWidth = 320

// thumbAtSeconds is the seek used when a clip's duration is UNKNOWN.
//
// Not frame 0: the first frame of a commercial is very often a black fade-in or a slate, so a
// catalog thumbnailed at 0 is a wall of black rectangles. Three seconds is past the fade on a 15s
// spot while still inside the shortest clips worth cataloguing.
const thumbAtSeconds = 3

// previewStartFor returns how many seconds into a clip the artwork window begins.
//
// ⚠ **Proportional, not a constant.** A flat offset does not survive contact with real clips: on
// a 15-second spot, starting at 3s means the window is the last 40% of the advert including the
// end card, and on a 10-second bumper there is no 6-second window at 3s at all — ffmpeg writes a
// truncated file or nothing.
//
// ⚠ This is now the STILL's seek too, which is the price of the single-pass build. The still used
// to take a flat 3s; it now takes the animation's first frame. On a 15s spot that moves it from
// 3.0s to 2.25s — marginally closer to a fade-in — and in exchange the card's still and its hover
// can never show different moments.
func previewStartFor(durationMs int64) float64 {
	if durationMs <= 0 {
		// Unprobed — a real state, for a clip ffprobe could not read. Guessing a proportion of an
		// unknown length is meaningless, so fall back to the known-good constant.
		return thumbAtSeconds
	}
	seconds := float64(durationMs) / 1000
	// A clip shorter than the window plays from the very start; there is nothing to trim to.
	if seconds <= previewSeconds {
		return 0
	}

	start := seconds * 0.15
	// The floor: on a short clip 15% lands inside the fade this offset exists to skip.
	if start < 1 {
		start = 1
	}
	// ⚠ **The end-clamp is applied LAST, and must be, because the two constraints can CONTRADICT
	// each other.** On a 6.5s clip the floor wants ≥1s and this wants ≤0.5s, and there is no value
	// satisfying both. Returning early from the floor let it win that argument silently — ffmpeg
	// was then asked for a 6s window with 5.5s of clip left and wrote a short file.
	//
	// This one wins because the failures are not symmetric: overrunning the end produces a
	// truncated or empty preview, while starting earlier than ideal only produces a duller one.
	// Found by a test that asserted the window fits, on a duration between the two thresholds.
	if latest := seconds - previewSeconds; start > latest {
		start = latest
	}
	return start
}

// ArtworkRenderer writes a still to stillDst and an animation to animDst from one source.
// Injected so the scanner is testable without executing a binary — the same seam Prober uses.
//
// ⚠ Both destinations in ONE call, deliberately: that is what makes the single-decode
// implementation expressible behind the interface. Two methods would let a caller invoke them
// separately and quietly reintroduce the second decode this replaced.
type ArtworkRenderer func(ctx context.Context, src, stillDst, animDst string, startSeconds float64) error

// ThumbPathFor returns a clip's still path RELATIVE to the artwork directory, mirroring how a
// clip's own id is relative to FILLER_DIR (see 00013): the mount differs between host and
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

// PreviewPathFor is the animation's equivalent, same rules.
func PreviewPathFor(clipPath string) string {
	if clipPath == "" {
		return ""
	}
	ext := filepath.Ext(clipPath)
	return strings.TrimSuffix(clipPath, ext) + ".webp"
}

// GenerateArtwork fills in Thumbnail and Preview for every clip missing either, and returns how
// many failed.
//
// Existing files are NOT regenerated: a scan runs periodically (FILLER_SYNC_EVERY), and
// re-rendering each pass would spend an ffmpeg exec per clip per cycle forever to reproduce files
// that are already correct. A changed clip changes its hash — which is its identity — so
// stale-but-matching artwork is not a state this can be in.
//
// ⚠ Failures must be COUNTED, not swallowed. The scan already has a cautionary tale: an earlier
// version handed the ffmpeg path to something that ran it as ffprobe, every probe failed, and
// because unprobeable files are skipped by design the catalog came back silently empty with no
// error anywhere (see FFprobeNextTo). This pass has exactly that shape — best-effort, failures
// skipped — so it returns a count and the caller logs it. A misconfigured ffmpeg then reads as
// "0 of 412 generated" rather than as artwork mysteriously not existing.
func GenerateArtwork(ctx context.Context, dir string, clips []RawClip, render ArtworkRenderer) (failed int) {
	if dir == "" || len(clips) == 0 {
		return 0
	}
	if render == nil {
		render = FFmpegArtwork("")
	}
	cacheDir := filepath.Join(dir, ThumbDirName)

	for i := range clips {
		stillRel, animRel := ThumbPathFor(clips[i].Path), PreviewPathFor(clips[i].Path)
		if stillRel == "" {
			continue
		}
		stillDst := filepath.Join(cacheDir, filepath.FromSlash(stillRel))
		animDst := filepath.Join(cacheDir, filepath.FromSlash(animRel))

		haveStill, haveAnim := nonEmpty(stillDst), nonEmpty(animDst)
		// Both already there: adopt and move on, the steady state on every scan after the first.
		if haveStill && haveAnim {
			clips[i].Thumbnail, clips[i].Preview = stillRel, animRel
			continue
		}
		if err := os.MkdirAll(filepath.Dir(stillDst), 0o750); err != nil {
			failed++
			continue
		}

		if err := render(ctx, filepath.Join(dir, filepath.FromSlash(clips[i].Path)),
			stillDst, animDst, previewStartFor(clips[i].DurationMs)); err != nil {
			failed++
			// ⚠ Partial files from a killed ffmpeg would otherwise be adopted as "already
			// generated" on the next pass and never retried. Only the ones that are actually
			// empty or absent are removed — a render that failed on the ANIMATION must not
			// destroy a still that came out fine.
			removeIfEmpty(stillDst)
			removeIfEmpty(animDst)
		}

		// ⚠ Recorded per-file, from what is actually ON DISK, rather than from whether the exec
		// returned an error. A single command with two outputs can half-succeed: ffmpeg writes
		// the still, fails on the webp, and exits non-zero. Trusting the error code alone would
		// discard a perfectly good thumbnail — and an empty `Thumbnail` renders as no image,
		// which on a catalog-wide failure is a grid of blank cards.
		if nonEmpty(stillDst) {
			clips[i].Thumbnail = stillRel
		}
		if nonEmpty(animDst) {
			clips[i].Preview = animRel
		}
	}
	return failed
}

// nonEmpty reports whether a path is a file with bytes in it. A zero-length file is what ffmpeg
// leaves when it exits 0 having decoded nothing (an audio-only source, or one shorter than the
// seek), so "exists" is not the question worth asking.
func nonEmpty(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Size() > 0
}

func removeIfEmpty(path string) {
	if !nonEmpty(path) {
		_ = os.Remove(path)
	}
}

// FFmpegArtwork returns an ArtworkRenderer using the given ffmpeg binary ("" ⇒ "ffmpeg").
//
// Takes the ffmpeg path directly rather than deriving it, unlike FFprobeNextTo — this genuinely
// wants ffmpeg, so `playout.ffmpeg_path` is the right value and there is nothing to translate.
//
// Animated WebP rather than GIF or a muted MP4:
//   - GIF is 256 colours and dithers video badly — the content here is film grain and gradients,
//     which is GIF's worst case, and the files come out LARGER than WebP anyway.
//   - An MP4 would be smaller still, but a <video> on every card in a grid means dozens of media
//     elements the browser must decode and keep alive; an <img> is a decode the browser already
//     manages, and it swaps with the still without a second element.
func FFmpegArtwork(ffmpegPath string) ArtworkRenderer {
	bin := ffmpegOr(ffmpegPath)
	return func(ctx context.Context, src, stillDst, animDst string, startSeconds float64) error {
		// ⚠ **`-ss` BEFORE `-i`** seeks by keyframe without decoding everything up to that point,
		// which on a 30s clip is the difference between milliseconds and a full decode pass.
		//
		// The filter graph is what makes this one decode: `split` forks the decoded frames into
		// two chains. `[still]` takes frame 0 of the window; `[anim]` resamples the whole window
		// to previewFPS. Two `-map`s then write two files from that single input.
		//
		// ⚠ `-update 1` on the JPEG is REQUIRED, not cosmetic. Without it the image2 muxer warns
		// that a single-image output needs either a `%03d` pattern or this flag, and future
		// ffmpeg versions promise to make that an error rather than a warning.
		cmd := exec.CommandContext(ctx, bin,
			"-nostdin",
			"-ss", fmt.Sprintf("%.2f", startSeconds),
			"-t", fmt.Sprintf("%d", previewSeconds),
			"-i", src,
			"-an",
			"-filter_complex", fmt.Sprintf(
				"[0:v]split=2[still][anim];[still]select=eq(n\\,0),scale=%d:-1[s];[anim]fps=%d,scale=%d:-2:flags=lanczos[a]",
				previewWidth, previewFPS, previewWidth),
			"-map", "[s]", "-frames:v", "1", "-q:v", "6", "-update", "1", "-y", stillDst,
			// ⚠ `-loop 0` makes the WebP loop FOREVER, which is what makes it a hover preview
			// rather than a six-second animation that stops and leaves a still.
			"-map", "[a]", "-c:v", "libwebp", "-lossless", "0",
			"-q:v", fmt.Sprintf("%d", previewQuality), "-compression_level", "4", "-loop", "0",
			"-y", animDst,
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			// ffmpeg's diagnostics go to stderr and are the only clue why a clip has no artwork;
			// truncated because a failure here is one line in a log, not an incident report.
			return fmt.Errorf("artwork %s: %w: %s", src, err, truncate(string(out), 200))
		}
		// A clip with no video stream, or one shorter than the seek point, exits 0 having written
		// nothing. Reported as a failure so the caller does not record paths to empty files —
		// which would render as BROKEN images rather than as absent ones.
		if !nonEmpty(stillDst) || !nonEmpty(animDst) {
			return fmt.Errorf("artwork %s: ffmpeg wrote no frames (audio-only, or shorter than %.2fs)",
				src, startSeconds)
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
