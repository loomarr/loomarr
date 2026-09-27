package playout

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ToneMapApplies is the one decision about tone-mapping, shared by live playout and prepared
// media: the content is HDR AND the build can tone-map. Prepared media records the answer in its
// rendition contract so a publication is never reused across a different answer.
func ToneMapApplies(sourceHDR, buildCanTonemap bool) bool {
	return sourceHDR && buildCanTonemap
}

// demoteTonemapper drops the GPU tone-mapper a graph used, so the next build takes the next one
// for the curve: its preferred GPU tone-mapper, the other one, then the CPU (the maintainer order,
// #1512); false when it used none. The capacity probe's tone-map self-check walks this order, and
// the packager's item retry (itemFaults) demotes the same stages, so the startup check and the live
// ladder cannot disagree.
func demoteTonemapper(g *GPUFilters, used string) bool {
	switch used {
	case TonemapperOpenCL:
		g.TonemapOpenCL = false
	case TonemapperLibplacebo:
		g.Libplacebo = false
	default:
		return false
	}
	return true
}

// scaleFilterArgs normalizes any input geometry to the profile's.
//
// A REAL filter-graph failure, and exactly the class ErsatzTV warns about (prior-art §5b):
// `-vf scale=1280:720` straight into `h264_vulkan` fails with "Impossible to convert between
// the formats supported by the filter 'Parsed_scale_0' and the filter 'auto_scale_0'" and a
// 40-line pixel-format dump. The cause is not the scale — it is that `scale` emits CPU
// frames while a hardware encoder wants GPU frames, and nothing uploads between them.
//
// So hardware gets `scale=W:H,format=nv12,hwupload`: scale on the CPU, convert to a format
// the uploader accepts, THEN upload. Software needs no upload step and gets the bare scale
// plus an explicit pixel format.
//
// `force_original_aspect_ratio=decrease` + `pad` letterboxes rather than stretching, so a
// 4:3 episode in a 16:9 profile keeps its geometry. The pad is what preserves the profile's
// exact output dimensions.
//
// `tonemap` is the HDR→SDR chain, or "" for the overwhelmingly common SDR case. It is a PARAMETER
// rather than something derived here because a Profile describes the OUTPUT and tone-mapping is a
// fact about the INPUT — and because capability.go builds this same chain for its trial encode
// against a synthetic lavfi source that has no input to speak of.
func (p Profile) scaleFilterArgs(tonemap string) []string {
	if p.Width <= 0 || p.Height <= 0 {
		return nil
	}
	scale := fmt.Sprintf(
		"scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2",
		p.Width, p.Height, p.Width, p.Height)

	// Framerate is pinned too: a 24fps film and a 25fps episode must not produce different
	// output rates.
	fps := fmt.Sprintf("fps=%d", p.Framerate)

	parts := []string{scale, fps}

	// TONE-MAP AFTER THE SCALE, BEFORE THE FORMAT/UPLOAD STEP. Both placements are deliberate:
	//
	//   - After scale, because tone-mapping is per-pixel and the scale has already cut a 4K frame
	//     to 1080p by this point. Doing it first would tone-map four times the pixels for a
	//     result no viewer can distinguish, on a realtime budget.
	//   - Before the format/upload step, because the chain's last zscale preserves BIT DEPTH — a
	//     10-bit HDR source is still 10-bit when it leaves the tone-map, and `format=yuv420p`
	//     (or `format=nv12,hwupload`) is what takes it to 8. Reversed, the tone-map would work on
	//     an already-truncated picture, which is most of what this was meant to fix.
	//
	// Both orders RUN — neither errors — so nothing but the assertions in program_test.go protects
	// this. Measured against a real HDR10 source (2026-08-09, ffmpeg n9.0): swapping the tone-map
	// and the format step yields a different frame hash, so the order is doing work rather than
	// being a stylistic preference.
	if tonemap != "" {
		parts = append(parts, tonemap)
	}

	// The upload step comes from the capability prober's own helper, not a local copy. It is
	// per-encoder correct in ways a generic "format=nv12,hwupload" is not — QSV needs
	// `extra_hw_frames=64` or its lookahead intermittently fails to allocate frames, and the
	// families that accept CPU frames directly (nvenc, amf, videotoolbox, rkmpp, v4l2m2m)
	// must get NO upload at all.
	if up := hardwareUploadFilter(p.Encoder); up != "" {
		// These families upload to GPU memory, and their upload filter already pins the
		// pixel format (nv12) on the way.
		parts = append(parts, up)
	} else {
		// EVERY OTHER FAMILY gets an explicit 8-bit pixel format — software AND the hardware
		// encoders that take CPU frames directly (nvenc, amf, videotoolbox, rkmpp, v4l2m2m).
		// Without it a 10-bit source reaches h264_nvenc, which encodes 8-bit H.264 only, as
		// yuv420p10le, and it fails with "No capable devices found": a message that names the
		// DEVICE, not the pixel format, so it reads as "your GPU is missing" while the GPU is fine.
		parts = append(parts, "format=yuv420p")
	}
	return []string{"-vf", strings.Join(parts, ",")}
}

// seconds formats a duration for ffmpeg's -ss / -t, keeping millisecond precision.
//
// Not strconv on a float: %g would emit exponent notation for small values ("1e-06") which
// ffmpeg parses as a filename-ish token, and %f would pad to six decimals. Milliseconds are
// as fine as a seek is meaningfully accurate.
func seconds(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'f', 3, 64)
}
