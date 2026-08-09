package playout

import (
	"bytes"
	"context"
	"os/exec"
	"sync"
	"time"
)

// What filters THIS ffmpeg build carries (§9.1).
//
// ⚠ **Filter availability is a per-BUILD fact, not a per-machine one**, and getting that wrong
// kills channels rather than degrading them. A filter ffmpeg does not have is rejected at
// GRAPH-INIT with "Filter not found" — the encode exits before a single frame, so the failure
// lands on every program that needs it, forever, with a message that never mentions the build.
//
// This package already learned that once, for `drawtext`: it is a compile-time option
// (libfreetype, plus libharfbuzz on ffmpeg 8), Homebrew's bottle ships without it, and because
// macOS HAS a font file the font check passed and every card died anyway (see CardFontFor). The
// answer there was to ask the binary. This file is that answer, generalized — so the next optional
// filter does not repeat the lesson.
//
// Both probes here follow the same three rules:
//
//  1. Ask the BINARY (`-filters`), never a version number or a platform assumption. An operator
//     can point `playout.ffmpeg_path` at anything, and the image's pinned build is not the only
//     build that runs this code.
//  2. Memoise per process. The exec is cheap but it is on the path to starting a channel.
//  3. A probe that cannot RUN resolves to "no", never to an assumed yes. The failure being
//     prevented is a dead channel; degrading the picture is the safe direction.

// filterProbeTimeout bounds the `-filters` exec. Generous — it is a local process listing static
// data — but bounded, because this runs on the path to starting a channel and a hung binary must
// not hang the request.
const filterProbeTimeout = 10 * time.Second

// hasFilter reports whether an ffmpeg build carries a named filter.
//
// Not memoised itself: callers wrap it (CardFontFor, TonemapperFor) because each caches a
// DECISION, not the raw answer, and those decisions combine more than one probe.
func hasFilter(ffmpegPath, name string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), filterProbeTimeout)
	defer cancel()

	raw, err := exec.CommandContext(ctx, ffmpegPath, "-hide_banner", "-filters").Output()
	if err != nil {
		return false
	}
	return parseHasFilter(raw, name)
}

// parseHasFilter is the pure half of hasFilter, split out so the column matching can be tested
// without a binary to exec.
//
// Matched against the FILTER-NAME COLUMN rather than the whole line: `-filters` output carries a
// description per row, so a substring search would also match a filter that merely MENTIONS the
// name in its prose. `tonemap` is the case that makes this concrete — `tonemap_opencl`,
// `tonemap_vaapi` and the plain `tonemap` are three different filters, and only exact
// column-matching tells them apart.
func parseHasFilter(raw []byte, name string) bool {
	want := []byte(name)
	for _, line := range bytes.Split(raw, []byte("\n")) {
		// Rows look like " TS. drawtext  V->V  Draw text on top of video frames."
		// — flags, name, signature, description.
		if f := bytes.Fields(line); len(f) >= 2 && bytes.Equal(f[1], want) {
			return true
		}
	}
	return false
}

// TonemapperFor reports whether this ffmpeg build can tone-map HDR to SDR, memoised for the
// process. Bound to an ffmpeg path the same way CardFontFor is, and for the same reason.
//
// BOTH filters are required and neither is universal:
//
//   - `zscale` is libzimg, a compile-time dependency. It does the colour-space and transfer
//     conversions either side of the tone-map.
//   - `tonemap` performs the actual dynamic-range compression, and it operates on LINEAR light —
//     which is why it cannot be used without zscale to get there and back.
//
// ⚠ **Do not go looking for a CUDA tone-map filter; there isn't one.** ErsatzTV's NVIDIA answer is
// decode-via-Vulkan into libplacebo and back out through `hwupload_cuda`, which is a whole
// hardware-frames pipeline. Loomarr needs none of it, and the reason is a decision already made
// elsewhere in this package for unrelated motives: capability.go REFUSES `-hwaccel_output_format`,
// so decoded frames are always in system memory by the time filters run. A CPU tone-map is
// therefore available on every encoder family, including nvenc, at no plumbing cost. The
// constraint that looked like a limitation is what makes this cheap.
func TonemapperFor(ffmpegPath string) func() bool {
	var (
		once sync.Once
		ok   bool
	)
	return func() bool {
		once.Do(func() {
			ok = hasFilter(ffmpegPath, "zscale") && hasFilter(ffmpegPath, "tonemap")
		})
		return ok
	}
}

// hdrToSDRChain is the HDR→SDR filter chain.
//
// The three steps are not interchangeable and the order is the whole trick:
//
//  1. `zscale=t=linear:npl=100` — decode the source's transfer curve (PQ or HLG; zscale reads
//     which from the stream tags, so one chain covers both) into LINEAR light. Tone-mapping any
//     other representation compresses the wrong quantity. `npl` is the peak luminance the result
//     is normalised against; 100 nits is the SDR reference white this output is headed for.
//  2. `tonemap=tonemap=hable:desat=0` — the actual range compression. `hable` is the filmic curve:
//     it rolls highlights off gradually instead of clipping them, which matters most on exactly
//     the content that ships as HDR (specular highlights, skies, practical lights). `desat=0`
//     because ffmpeg's default desaturation visibly washes skin tones out, and a flat picture is
//     the complaint this change exists to fix.
//  3. `zscale=p=bt709:t=bt709:m=bt709:r=tv` — re-encode into the SDR space the Profile actually
//     targets. Without this the data would be linear-light, which nothing downstream expects.
//
// Step 3 is also what FIXES THE LABELS, which was the more damaging half of the defect.
//
// Truncation (8-bit SDR pixels from an HDR source, never tone-mapped) was the visible half.
// Mislabelling was the half no client could recover from: ffmpeg carries the source's colour
// metadata to the output when nothing changes it, so an HDR source through the old
// `…,fps=25,format=yuv420p` chain produced
//
//	yuv420p,bt2020nc,smpte2084,bt2020
//
// — SDR-range data still announcing itself as PQ/BT.2020. A player that believes the tags applies
// an HDR transfer to SDR pixels, which is worse than doing nothing.
//
// ⚠ **No `-colorspace`/`-color_trc`/`-color_primaries`/`-color_range` output flags are needed, and
// adding them would be cargo.** Measured 2026-08-09 against a real HDR10 source: this chain alone
// yields `yuv420p,tv,bt709,bt709,bt709`, byte-identical to the same run with all four flags set.
// zscale rewrites the frames' colour properties and ffmpeg propagates them to the stream. An
// earlier draft of this change DID emit the four flags; a sabotage check found the live test
// passed with them removed, because they had never been doing anything. They are left out
// deliberately — a flag that asserts what the pixels already prove is the "captured now so a later
// feature can use it" pattern that produced this bug in the first place.
//
// The corollary matters too: on a build with no zscale, the output keeps the SOURCE's HDR tags,
// and that is CORRECT. The pixels really are still (truncated) BT.2020/PQ data, so tagging them
// bt709 there would replace one lie with another.
//
// It emits no pixel FORMAT either: zscale preserves bit depth, so a 10-bit source is still 10-bit
// here. The existing `format=yuv420p` / `format=nv12,hwupload` step that follows is what takes it
// to 8 bits, which is why this must be inserted BEFORE that step and not after.
const hdrToSDRChain = "zscale=t=linear:npl=100," +
	"tonemap=tonemap=hable:desat=0," +
	"zscale=p=bt709:t=bt709:m=bt709:r=tv"
