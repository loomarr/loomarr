package media

// StreamClass is a source's cost class: what one stream of it costs this host to transcode into the
// channel output. Costs are measured per host by the capability probe on real content (#1512 G5).
type StreamClass string

const (
	// ClassCopy is a session that copies video. It costs no encoder and is always admitted.
	ClassCopy StreamClass = "copy"
	// ClassSDR is 8-bit SDR up to 1080p (56% of the household library). Also the class of a source
	// with no stream facts, which is the common case among unmeasured sources.
	ClassSDR StreamClass = "sdr_1080p"
	// ClassHEVC10 is 10-bit SDR up to 1080p: a heavier decode, the same filter graph.
	ClassHEVC10 StreamClass = "hevc10_1080p"
	// ClassHDR4K is anything above 1080p or HDR: a 4K decode, a downscale and (for HDR) the tone-map
	// stage. 4K SDR shares the class because its decode, not the tone-map, dominates.
	ClassHDR4K StreamClass = "uhd_hdr_tonemap"
	// ClassPremium4K is G10's premium 4K encode: one channel's premium packager, priced for its
	// whole lineup. The probe measures it on GPU hosts at 2160p (measurePremium); a measured host
	// that has no premium cell never admits it.
	ClassPremium4K StreamClass = "premium_4k"
)

// TranscodeClasses are the source classes the capability probe measures at each output rung,
// cheapest first. It also measures ClassPremium4K, once, at the premium's own height.
var TranscodeClasses = []StreamClass{ClassSDR, ClassHEVC10, ClassHDR4K}

// ClassOf is the cost class of a source.
func ClassOf(f MediaFormat) StreamClass {
	switch {
	case f.HDR() || f.Height > 1080 || f.Width > 1920:
		return ClassHDR4K
	case isTenBit(f.PixelFormat):
		return ClassHEVC10
	default:
		return ClassSDR
	}
}

func isTenBit(pixfmt string) bool {
	return len(pixfmt) > 4 && (pixfmt[len(pixfmt)-4:] == "10le" || pixfmt[len(pixfmt)-4:] == "10be")
}

// Rect is a rectangle in pixels.
type Rect struct {
	X, Y, W, H int
}
