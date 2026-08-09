package images

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"strings"
)

// AVIF encoding, which is the one rendition Loomarr does NOT produce in-process (§22).
//
// ⚠ **This is a subprocess and a background job on purpose — but NOT for the reason usually
// given, and the difference matters if you are tempted to revisit it.**
//
// The common claim is that AVIF is an order of magnitude slower than WebP (300–1200ms per image).
// **Measured here, it is not.** With `libaom -still-picture -cpu-used 6`, a 500px poster encodes in
// **~86ms** against WebP's **~67ms** on the same box (i9-12900K, `BenchmarkAVIFEncode` vs
// `BenchmarkEncodeWebP -tags nodynamic`) — about 1.3×, not 10×. The scary numbers in circulation
// come from running a VIDEO encoder at video defaults, which is exactly what SVT-AV1 does here:
// see FFmpegAVIF's note on the 2.34 GB it allocated for one frame.
//
// The reason that DOES survive measurement is **concurrency, not latency**. Each encode is a forked,
// natively-multithreaded process. Generating AVIF lazily would mean a cold grid of fifty posters
// forks fifty of them at once, which will thrash a four-core NAS whatever the per-image figure is.
// A job runs them at a controlled rate; a request cannot.
//
// The consequence, stated in §22 and enforced by codec.go refusing FormatAVIF inline: AVIF coverage
// is EVENTUALLY CONSISTENT. A serve that finds no AVIF derivative omits the <source> and the
// browser takes WebP, so nothing waits and no surface has to know whether the job has caught up.

// AVIFEncoder writes an AVIF rendition of img to dst.
//
// Injected as a func rather than called directly so the service is testable without executing a
// binary — the same seam `filler.ArtworkRenderer` and `filler.Prober` already use, kept identical
// so there is one way to fake an external tool in this codebase rather than two.
type AVIFEncoder func(ctx context.Context, img image.Image, dst string) error

// avifCPUUsed is libaom's speed/quality dial: 0 is slowest and best, 8 fastest and worst.
//
// 6 is chosen for a BACKGROUND job that may have a catalog-sized backlog to work through. Lower
// values buy a few percent of file size for multiples of the CPU time, which is the wrong trade
// when the alternative rendition (WebP) is already on disk and being served — nothing is waiting
// for this, but the box has other work to do.
const avifCPUUsed = 6

// avifCRF is the quality target. 32 sits where AVIF is comfortably smaller than the WebP rendition
// it competes with while staying visually indistinguishable on artwork at these sizes; going lower
// erases the size advantage that is AVIF's entire reason for existing here.
const avifCRF = 32

// FFmpegAVIF returns an AVIFEncoder backed by the vendored ffmpeg ("" ⇒ "ffmpeg" on PATH).
//
// ⚠ **`libaom-av1` with `-still-picture`, NOT `libsvtav1`** — measured, not assumed. SVT-AV1 is a
// VIDEO encoder: asked for a single 1000×1500 frame it allocated **2.34 GB** and spawned **82
// threads**, and still produced a file **78% larger** than libaom did (6220 vs 3494 bytes). It is
// the better choice for a stream of frames and the wrong one for one picture. libaom's
// `-still-picture` path is built for exactly this and behaves like a still encoder should.
//
// This is why the design doc's dependency row says "libaom-av1 / libsvtav1" but the code picks:
// the image must *contain* an AV1 encoder, and this is the one worth using.
func FFmpegAVIF(ffmpegPath string) AVIFEncoder {
	bin := ffmpegPath
	if bin == "" {
		bin = "ffmpeg"
	}
	return func(ctx context.Context, img image.Image, dst string) error {
		// PNG on stdin rather than a temp file: lossless (so the AVIF encoder sees exactly the
		// pixels we resized, not a JPEG's approximation of them) and it avoids a scratch file we
		// would then have to clean up on every failure path.
		var in bytes.Buffer
		if err := png.Encode(&in, img); err != nil {
			return fmt.Errorf("avif: encode intermediate png: %w", err)
		}

		cmd := exec.CommandContext(ctx, bin,
			"-nostdin",
			"-v", "error",
			"-f", "image2pipe", "-i", "-",
			"-c:v", "libaom-av1",
			"-still-picture", "1",
			"-cpu-used", fmt.Sprint(avifCPUUsed),
			"-crf", fmt.Sprint(avifCRF),
			// yuv420p because some decoders reject 4:4:4 AVIF, and artwork does not benefit
			// enough from full chroma to be worth a rendition a client might refuse.
			"-pix_fmt", "yuv420p",
			"-y", dst,
		)
		cmd.Stdin = &in
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("avif %s: %w: %s", dst, err, truncate(string(out), 200))
		}

		// ⚠ Verified from what is ON DISK, not from the exit code. ffmpeg can exit 0 having
		// written nothing, and an empty derivative recorded as present renders as a BROKEN image
		// rather than an absent one — which is the failure `filler.GenerateArtwork` already
		// learned to guard against by checking size rather than existence.
		info, statErr := os.Stat(dst)
		if statErr != nil || info.Size() == 0 {
			_ = os.Remove(dst)
			return fmt.Errorf("avif %s: ffmpeg exited 0 but wrote no bytes", dst)
		}
		return nil
	}
}

// HasAVIFEncoder reports whether the ffmpeg binary actually carries libaom-av1.
//
// ⚠ Exists because the image build must ASSERT this rather than assume it. A Dockerfile that
// silently ships an ffmpeg without an AV1 encoder produces an install where every AVIF job fails
// forever and the only symptom is that clients quietly keep taking WebP — a degradation nobody
// would notice for months. The same reasoning made the image prove whisper by transcribing at
// build time rather than by `--help`.
func HasAVIFEncoder(ctx context.Context, ffmpegPath string) bool {
	bin := ffmpegPath
	if bin == "" {
		bin = "ffmpeg"
	}
	out, err := exec.CommandContext(ctx, bin, "-hide_banner", "-encoders").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "libaom-av1")
}

// truncate bounds a subprocess's diagnostics to one log line. ffmpeg's stderr is the only clue why
// a rendition is missing, but a failure here is a line in a log, not an incident report.
func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
