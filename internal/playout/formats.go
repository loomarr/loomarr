package playout

// Channel formats (#1512 G10). Every channel airs a 1080p SDR H.264 baseline. When its lineup
// warrants it, it also airs ONE premium HEVC format, so a channel costs at most two encodes, and a
// format's dynamic range never changes mid-stream.
//
// The premium is derived from the lineup's inventory facts, top resolution and dynamic range
// INDEPENDENTLY (maintainer rule): any 4K item and any HDR item make it 4K HDR, even when no single
// item is both; 4K items and no HDR make it 4K SDR; otherwise there is none. On a 4K HDR channel the
// SDR (and HLG) items are converted to HDR10, never the other way round, and a 4K SDR channel never
// carries an HDR item.

// FormatClass names one output format. The premium names are the capacity classes the resource
// budget measures per host.
type FormatClass string

const (
	FormatBaseline FormatClass = "1080p-h264-sdr"
	Format4KSDR    FormatClass = "4k-hevc-sdr"
	Format4KHDR    FormatClass = "4k-hevc-hdr"
)

// ChannelFormats is what one channel airs. Premium is empty when the channel has none.
type ChannelFormats struct {
	Baseline FormatClass
	Premium  FormatClass
}

// DeriveChannelFormats is the lineup-warranted formats. An item without inventory facts (zero
// geometry, unknown transfer) contributes nothing: it cannot prove 4K or HDR, so a lineup with no
// facts at all is baseline only.
func DeriveChannelFormats(items []MediaFormat) ChannelFormats {
	var uhd, hdr bool
	for _, it := range items {
		uhd = uhd || Is4K(it)
		hdr = hdr || it.HDR()
	}
	f := ChannelFormats{Baseline: FormatBaseline}
	switch {
	case uhd && hdr:
		f.Premium = Format4KHDR
	case uhd:
		f.Premium = Format4KSDR
	}
	return f
}

// Is4K reports whether a source has more detail than the 1080p baseline can carry: larger than
// QHD (2560x1440) in either dimension. Width alone catches scope films (3840x1600), height alone
// catches cropped-width 4K encodes.
func Is4K(f MediaFormat) bool {
	return f.Width > 2560 || f.Height > 1440
}

// OnHost is the formats this host can actually produce, and why the premium was dropped (empty
// when it was kept or there was none). The baseline is never dropped.
func (c ChannelFormats) OnHost(h HostProfile) (ChannelFormats, string) {
	if c.Premium == "" {
		return c, ""
	}
	drop := func(why string) (ChannelFormats, string) {
		return ChannelFormats{Baseline: c.Baseline}, why
	}
	switch h.Family {
	case FamilySoftware:
		return drop("software-only hosts never produce a premium format")
	case FamilyVAAPI, FamilyNVENC, FamilyVideoToolbox:
	default:
		return drop("no GPU graph for " + string(h.Encoder) + ": a 4K premium on CPU filters would not keep up")
	}
	if c.Premium == Format4KHDR && !h.Libplacebo {
		// A 4K HDR channel's SDR items (filler at least) are converted to HDR10 by libplacebo; the
		// Intel VPP conversion puts SDR white at ~2,600 nits (spike 0b) and is never used.
		return drop("4K HDR needs libplacebo to convert the channel's SDR items to HDR10")
	}
	return c, ""
}

// Premium output rate control (spike 0b, proposed there and adopted here): the baseline's q22
// quality target, with a 16M target and 24M cap. 4K is four times 1080p's pixels and HEVC needs
// roughly half H.264's bitrate for the same quality, so 4 x 8M / 2 = 16M; the cap keeps the
// baseline's 1.5x headroom (12M over 8M).
const (
	premiumWidth      = 3840
	premiumHeight     = 2160
	premiumTargetKbps = 16000
	premiumMaxKbps    = 24000
)

// PremiumOutput is the uniform output of a premium class on a channel whose baseline is base: HEVC
// at 3840x2160 on the baseline's cadence, GOP, quality target and audio; HDR10 (Main10, BT.2020 PQ)
// for 4K HDR. ok is false for the empty class.
func PremiumOutput(class FormatClass, base OutputProfile) (OutputProfile, bool) {
	if class != Format4KSDR && class != Format4KHDR {
		return OutputProfile{}, false
	}
	return OutputProfile{
		Width: premiumWidth, Height: premiumHeight, FPS: base.FPS,
		HEVC: true, HDR: class == Format4KHDR,
		Quality: base.Quality, TargetKbps: premiumTargetKbps, MaxKbps: premiumMaxKbps,
		GOPSeconds: base.GOPSeconds, AudioKbps: base.AudioKbps,
	}, true
}

// premium reports whether this output is a premium format: HDR, or more pixels than 1080p.
func (o OutputProfile) premium() bool {
	return o.HDR || o.Width*o.Height > 1920*1080
}
