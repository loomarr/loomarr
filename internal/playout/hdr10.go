package playout

// The channel-level HDR10 static metadata (#1512 G10, spike 0b decision 2). A 4K HDR channel airs
// titles mastered on different displays plus converted SDR items. If each item carried its own
// mastering-display and light-level SEI, the TV would re-evaluate its tone curve at every seam. So
// the builder strips every item's HDR metadata (hdrStrip) and the stream carries ONE fixed set.
//
// No ffmpeg encoder can write fixed values: hevc_vaapi, hevc_nvenc and hevc_videotoolbox only copy
// the metadata attached to each input frame, and no filter or bitstream filter (ffmpeg 8.1/9.0)
// attaches or inserts it. So the static SEI is data here, and the packager inserts it: this NAL
// before the first slice of every IDR access unit, and the same values as the init segment's
// mdcv/clli boxes.

// HDR10Metadata is SMPTE ST 2086 mastering display colour volume plus CTA-861.3 content light
// level, in the units the HEVC SEI carries.
type HDR10Metadata struct {
	// Primaries are the display primaries in 0.00002 units, in the SEI's G, B, R order.
	Primaries [3][2]uint16
	// WhitePoint is in 0.00002 units.
	WhitePoint [2]uint16
	// MaxLuminance and MinLuminance are in 0.0001 cd/m² units.
	MaxLuminance, MinLuminance uint32
	// MaxCLL and MaxFALL are in cd/m².
	MaxCLL, MaxFALL uint16
}

// ChannelHDR10 is the maintainer's fixed channel metadata: BT.2020 primaries, D65 white, a
// 1000-nit / 0.0001-nit mastering display, MaxCLL 1000, MaxFALL 400.
var ChannelHDR10 = HDR10Metadata{
	Primaries: [3][2]uint16{
		{8500, 39850},  // G (0.170, 0.797)
		{6550, 2300},   // B (0.131, 0.046)
		{35400, 14600}, // R (0.708, 0.292)
	},
	WhitePoint:   [2]uint16{15635, 16450}, // D65 (0.3127, 0.3290)
	MaxLuminance: 1000 * 10000,
	MinLuminance: 1,
	MaxCLL:       1000,
	MaxFALL:      400,
}

// HEVC SEI payload types (ITU-T H.265 Annex D) and the prefix SEI NAL unit type.
const (
	seiMasteringDisplay = 137
	seiContentLight     = 144
	nalPrefixSEI        = 39
)

// SEI is the Annex B prefix SEI NAL unit (start code included) carrying m's mastering display
// colour volume and content light level messages.
func (m HDR10Metadata) SEI() []byte {
	var mdcv []byte
	for _, p := range m.Primaries {
		mdcv = be16(be16(mdcv, p[0]), p[1])
	}
	mdcv = be16(be16(mdcv, m.WhitePoint[0]), m.WhitePoint[1])
	mdcv = be32(be32(mdcv, m.MaxLuminance), m.MinLuminance)
	clli := be16(be16(nil, m.MaxCLL), m.MaxFALL)

	rbsp := append([]byte{seiMasteringDisplay, byte(len(mdcv))}, mdcv...)
	rbsp = append(rbsp, seiContentLight, byte(len(clli)))
	rbsp = append(rbsp, clli...)
	rbsp = append(rbsp, 0x80) // rbsp_trailing_bits

	// NAL header: forbidden_zero_bit 0, nal_unit_type, nuh_layer_id 0, nuh_temporal_id_plus1 1.
	nal := []byte{0, 0, 0, 1, nalPrefixSEI << 1, 1}
	return append(nal, escapeRBSP(rbsp)...)
}

// escapeRBSP inserts emulation_prevention_three_byte wherever two zero bytes precede a byte <= 3,
// so the payload never imitates a start code.
func escapeRBSP(rbsp []byte) []byte {
	out := make([]byte, 0, len(rbsp)+4)
	zeros := 0
	for _, b := range rbsp {
		if zeros >= 2 && b <= 3 {
			out = append(out, 3)
			zeros = 0
		}
		out = append(out, b)
		if b == 0 {
			zeros++
		} else {
			zeros = 0
		}
	}
	return out
}

func be16(b []byte, v uint16) []byte { return append(b, byte(v>>8), byte(v)) }

func be32(b []byte, v uint32) []byte { return append(b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v)) }
