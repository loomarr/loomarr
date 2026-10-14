package playout

import (
	"strconv"
	"time"
)

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

// seconds formats a duration for ffmpeg's -ss / -t, keeping millisecond precision.
//
// Not strconv on a float: %g would emit exponent notation for small values ("1e-06") which
// ffmpeg parses as a filename-ish token, and %f would pad to six decimals. Milliseconds are
// as fine as a seek is meaningfully accurate.
func seconds(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'f', 3, 64)
}
