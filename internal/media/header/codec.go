package header

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/loomarr/loomarr/internal/library/inventory"
)

// statesDepth reports whether an audio codec has a sample size to state: lossless and PCM audio.
// A lossy codec has none, and ffprobe reports none for it, whatever the container says.
func statesDepth(codec string) bool {
	return codec == "flac" || codec == "alac" || strings.HasPrefix(codec, "pcm_")
}

// flacDepth reads bits per sample from FLAC's STREAMINFO, the first metadata block: as Matroska's
// CodecPrivate holds it ("fLaC", then the blocks) or MP4's dfLa after its full-box header. 0 when
// the block is not there.
func flacDepth(b []byte) int {
	b = bytes.TrimPrefix(b, []byte("fLaC"))
	if len(b) < 4+14 || b[0]&0x7F != 0 { // block type 0 is STREAMINFO
		return 0
	}
	info := b[4:]
	return int(info[12]&0x01)<<4 | int(info[13]>>4) + 1
}

// alacDepth reads bits per sample from an ALACSpecificConfig: frameLength (4), compatibleVersion
// (1), then bitDepth. 0 when it is too short.
func alacDepth(config []byte) int {
	if len(config) < 6 {
		return 0
	}
	return int(config[5])
}

// colourInfo is ISO/IEC 23091-2 colour description (H.273 code points). 0 means not signalled
// (2, "unspecified", is treated the same).
type colourInfo struct {
	primaries, transfer, matrix int
	present                     bool
}

func (c colourInfo) signalled() bool {
	return c.present || (c.primaries != 0 && c.primaries != 2) || (c.transfer != 0 && c.transfer != 2) ||
		(c.matrix != 2 && c.matrix != 0)
}

// ffprobe's names for H.273 code points (libavutil/pixdesc.c).
var (
	transferNames = map[int]string{
		1: "bt709", 4: "gamma22", 5: "gamma28", 6: "smpte170m", 7: "smpte240m", 8: "linear",
		9: "log100", 10: "log316", 11: "iec61966-2-4", 12: "bt1361e", 13: "iec61966-2-1",
		14: "bt2020-10", 15: "bt2020-12", 16: "smpte2084", 17: "smpte428", 18: "arib-std-b67",
	}
	primariesNames = map[int]string{
		1: "bt709", 4: "bt470m", 5: "bt470bg", 6: "smpte170m", 7: "smpte240m", 8: "film",
		9: "bt2020", 10: "smpte428", 11: "smpte431", 12: "smpte432", 22: "jedec-p22",
	}
	matrixNames = map[int]string{
		0: "gbr", 1: "bt709", 4: "fcc", 5: "bt470bg", 6: "smpte170m", 7: "smpte240m", 8: "ycgco",
		9: "bt2020nc", 10: "bt2020c", 11: "smpte2085", 12: "chroma-derived-nc",
		13: "chroma-derived-c", 14: "ictcp",
	}
)

// videoBits is what a codec's configuration record says about the picture.
type videoBits struct {
	chroma     int // 0 monochrome, 1 4:2:0, 2 4:2:2, 3 4:4:4; -1 unknown
	depth      int // bits per luma sample; 0 unknown
	colour     colourInfo
	colourRead bool // the bitstream's colour description was parsed (present or not)
	interlaced bool // the bitstream allows field coding, so only decoding can tell
	profile    string
	level      string
	fullRange  bool
}

// dolbyVisionProfile reads a Dolby Vision configuration record (dvcC, dvvC or dvwC; a Matroska
// BlockAdditionMapping holds the same bytes): version major and minor, then the profile in the
// top seven bits. 0 for a record too short to say.
func dolbyVisionProfile(record []byte) int {
	if len(record) < 4 {
		return 0
	}
	return int(record[2] >> 1)
}

// videoConfig reads a codec's configuration record as both containers store it: avcC, hvcC and
// av1C records, an MPEG sequence header or VOL. VP9 differs between them, so the Matroska form is
// read by matroskaVP9 and the MP4 form (vpcC) by parseVPConfig.
func videoConfig(codec string, private []byte) (videoBits, bool) {
	switch codec {
	case "h264":
		return parseAVCConfig(private)
	case "hevc":
		return parseHEVCConfig(private)
	case "av1":
		return parseAV1Config(private)
	case "vp8":
		return videoBits{chroma: 1, depth: 8, colourRead: true}, true
	case "mpeg2video", "mpeg1video":
		return parseMPEG2Sequence(private)
	case "mpeg4":
		return parseMPEG4VOL(private)
	}
	return videoBits{}, false
}

// describeVideo fills the stream's profile, level, pixel format and colour from what the codec's
// configuration record said. The bitstream's colour description wins over the container's, as it
// does in ffprobe. ok is false when a fact playout needs (pixel format, HDR transfer) is not
// determined without decoding.
func describeVideo(stream *inventory.Stream, bits videoBits, container colourInfo) bool {
	if bits.depth <= 0 || bits.chroma < 0 {
		return false
	}
	colour := container
	if bits.colourRead && bits.colour.signalled() {
		colour = bits.colour
	}
	// An HDR transfer is only ever signalled; ≥10-bit video whose colour nobody signalled might
	// still carry it in a message only a decoder sees, so let ffprobe look.
	if !colour.signalled() && !bits.colourRead && bits.depth > 8 {
		return false
	}
	stream.Profile, stream.Level, stream.BitDepth = bits.profile, bits.level, bits.depth
	stream.PixelFormat = pixelFormat(bits.chroma, bits.depth, stream.Codec == "h264" && bits.fullRange)
	if stream.PixelFormat == "" {
		return false
	}
	stream.ColorTransfer = transferNames[colour.transfer]
	stream.ColorPrimaries = primariesNames[colour.primaries]
	if colour.matrix != 2 && (colour.matrix != 0 || colour.present) {
		stream.ColorSpace = matrixNames[colour.matrix]
	}
	stream.HDR = stream.ColorTransfer == "smpte2084" || stream.ColorTransfer == "arib-std-b67"
	return true
}

// pixelFormat spells ffmpeg's pixel format for a chroma format and bit depth. ffmpeg's H.264
// decoder still reports full-range 8-bit as the yuvj formats.
func pixelFormat(chroma, depth int, jpegRange bool) string {
	base := map[int]string{0: "gray", 1: "yuv420p", 2: "yuv422p", 3: "yuv444p"}[chroma]
	if base == "" {
		return ""
	}
	switch depth {
	case 8:
		if jpegRange && chroma != 0 {
			return "yuvj" + base[3:]
		}
		return base
	case 9, 10, 12, 14, 16:
		if chroma == 0 {
			return "gray" + strconv.Itoa(depth) + "le"
		}
		return base + strconv.Itoa(depth) + "le"
	}
	return ""
}

// --- H.264 -----------------------------------------------------------------------------------

var h264Profiles = map[int]string{
	66: "Baseline", 77: "Main", 88: "Extended", 100: "High", 110: "High 10", 122: "High 4:2:2",
	244: "High 4:4:4 Predictive", 44: "CAVLC 4:4:4", 118: "Multiview High", 128: "Stereo High",
}

// parseAVCConfig reads an AVCDecoderConfigurationRecord's first SPS.
func parseAVCConfig(b []byte) (videoBits, bool) {
	if len(b) < 8 || b[0] != 1 {
		return videoBits{}, false
	}
	count := int(b[5] & 0x1F)
	if count == 0 {
		return videoBits{}, false
	}
	n := int(b[6])<<8 | int(b[7])
	if n < 4 || 8+n > len(b) {
		return videoBits{}, false
	}
	return parseH264SPS(b[8 : 8+n])
}

// parseH264SPS reads an H.264 sequence parameter set NAL (with its header byte) far enough for
// the chroma format, bit depth, field coding and the VUI colour description.
func parseH264SPS(nal []byte) (videoBits, bool) {
	if len(nal) < 4 || nal[0]&0x1F != 7 {
		return videoBits{}, false
	}
	br := newBitReader(unescapeRBSP(nal[1:]))
	profileIdc := int(br.u(8))
	constraints := br.u(8)
	levelIdc := int(br.u(8))
	br.ue() // seq_parameter_set_id
	bits := videoBits{chroma: 1, depth: 8, colourRead: true}
	switch profileIdc {
	case 100, 110, 122, 244, 44, 83, 86, 118, 128, 138, 139, 134, 135:
		bits.chroma = int(br.ue())
		if bits.chroma == 3 {
			br.u(1) // separate_colour_plane_flag
		}
		bits.depth = int(br.ue()) + 8
		br.ue()           // bit_depth_chroma_minus8
		br.u(1)           // qpprime_y_zero_transform_bypass_flag
		if br.u(1) == 1 { // seq_scaling_matrix_present_flag
			lists := 8
			if bits.chroma == 3 {
				lists = 12
			}
			for i := 0; i < lists; i++ {
				if br.u(1) == 1 {
					size := 16
					if i >= 6 {
						size = 64
					}
					skipScalingList(br, size)
				}
			}
		}
	}
	br.ue()          // log2_max_frame_num_minus4
	switch br.ue() { // pic_order_cnt_type
	case 0:
		br.ue()
	case 1:
		br.u(1)
		br.se()
		br.se()
		cycle := br.ue()
		if cycle > 255 {
			return videoBits{}, false
		}
		for i := uint64(0); i < cycle; i++ {
			br.se()
		}
	}
	br.ue()           // max_num_ref_frames
	br.u(1)           // gaps_in_frame_num_value_allowed_flag
	br.ue()           // pic_width_in_mbs_minus1
	br.ue()           // pic_height_in_map_units_minus1
	if br.u(1) == 0 { // frame_mbs_only_flag
		bits.interlaced = true
		br.u(1) // mb_adaptive_frame_field_flag
	}
	br.u(1)           // direct_8x8_inference_flag
	if br.u(1) == 1 { // frame_cropping_flag
		br.ue()
		br.ue()
		br.ue()
		br.ue()
	}
	if br.u(1) == 1 { // vui_parameters_present_flag
		readVUIColour(br, &bits)
	}
	if br.err {
		return videoBits{}, false
	}
	name := h264Profiles[profileIdc]
	switch {
	case profileIdc == 66 && constraints&0x40 != 0:
		name = "Constrained Baseline"
	case (profileIdc == 110 || profileIdc == 122) && constraints&0x10 != 0:
		name += " Intra"
	case profileIdc == 244 && constraints&0x10 != 0:
		name = "High 4:4:4 Intra"
	}
	bits.profile = name
	bits.level = strconv.Itoa(levelIdc)
	return bits, true
}

// readVUIColour reads the start of H.264/HEVC VUI: aspect ratio, overscan, video signal type.
func readVUIColour(br *bitReader, bits *videoBits) {
	if br.u(1) == 1 { // aspect_ratio_info_present_flag
		if br.u(8) == 255 {
			br.u(16)
			br.u(16)
		}
	}
	if br.u(1) == 1 { // overscan_info_present_flag
		br.u(1)
	}
	if br.u(1) == 1 { // video_signal_type_present_flag
		br.u(3) // video_format
		bits.fullRange = br.u(1) == 1
		if br.u(1) == 1 { // colour_description_present_flag
			bits.colour = colourInfo{
				primaries: int(br.u(8)), transfer: int(br.u(8)), matrix: int(br.u(8)), present: true,
			}
		}
	}
}

func skipScalingList(br *bitReader, size int) {
	last, next := int64(8), int64(8)
	for j := 0; j < size && !br.err; j++ {
		if next != 0 {
			next = (last + br.se() + 256) % 256
		}
		if next != 0 {
			last = next
		}
	}
}

// --- HEVC ------------------------------------------------------------------------------------

var hevcProfiles = map[int]string{1: "Main", 2: "Main 10", 3: "Main Still Picture", 4: "Rext"}

// parseHEVCConfig reads an HEVCDecoderConfigurationRecord, then its SPS for the VUI colour and
// field coding.
func parseHEVCConfig(b []byte) (videoBits, bool) {
	if len(b) < 23 || b[0] != 1 {
		return videoBits{}, false
	}
	profile := int(b[1] & 0x1F)
	bits := videoBits{
		chroma:  int(b[16] & 0x03),
		depth:   int(b[17]&0x07) + 8,
		profile: hevcProfiles[profile],
		level:   strconv.Itoa(int(b[12])),
	}
	arrays, pos := int(b[22]), 23
	for i := 0; i < arrays; i++ {
		if pos+3 > len(b) {
			return videoBits{}, false
		}
		nalType := b[pos] & 0x3F
		count := int(b[pos+1])<<8 | int(b[pos+2])
		pos += 3
		for j := 0; j < count; j++ {
			if pos+2 > len(b) {
				return videoBits{}, false
			}
			n := int(b[pos])<<8 | int(b[pos+1])
			pos += 2
			if pos+n > len(b) {
				return videoBits{}, false
			}
			if nalType == 33 && !bits.colourRead {
				sps, ok := parseHEVCSPS(b[pos : pos+n])
				if !ok {
					return videoBits{}, false // field coding and colour unknown: ffprobe decides
				}
				bits.colour, bits.interlaced, bits.fullRange, bits.colourRead = sps.colour, sps.interlaced, sps.fullRange, true
				bits.chroma, bits.depth = sps.chroma, sps.depth
			}
			pos += n
		}
	}
	return bits, bits.colourRead
}

// parseHEVCSPS reads an HEVC SPS NAL (with its two-byte header) as far as the VUI's colour
// description and field_seq_flag (H.265 §7.3.2.2, ffmpeg hevc_ps.c).
func parseHEVCSPS(nal []byte) (videoBits, bool) {
	if len(nal) < 3 {
		return videoBits{}, false
	}
	br := newBitReader(unescapeRBSP(nal[2:]))
	br.u(4) // sps_video_parameter_set_id
	maxSubLayers := int(br.u(3))
	br.u(1) // sps_temporal_id_nesting_flag
	// profile_tier_level(1, maxSubLayers)
	br.u(8)  // profile space, tier, profile idc
	br.u(32) // compatibility flags
	br.u(32)
	br.u(16) // constraint flags (48 bits)
	br.u(8)  // general_level_idc
	profilePresent := make([]bool, maxSubLayers)
	levelPresent := make([]bool, maxSubLayers)
	for i := 0; i < maxSubLayers; i++ {
		profilePresent[i], levelPresent[i] = br.u(1) == 1, br.u(1) == 1
	}
	if maxSubLayers > 0 {
		for i := maxSubLayers; i < 8; i++ {
			br.u(2)
		}
	}
	for i := 0; i < maxSubLayers; i++ {
		if profilePresent[i] {
			br.u(32)
			br.u(32)
			br.u(24) // 88 bits
		}
		if levelPresent[i] {
			br.u(8)
		}
	}
	br.ue() // sps_seq_parameter_set_id
	bits := videoBits{chroma: int(br.ue())}
	if bits.chroma == 3 {
		br.u(1)
	}
	br.ue()           // pic_width_in_luma_samples
	br.ue()           // pic_height_in_luma_samples
	if br.u(1) == 1 { // conformance_window_flag
		br.ue()
		br.ue()
		br.ue()
		br.ue()
	}
	bits.depth = int(br.ue()) + 8
	br.ue() // bit_depth_chroma_minus8
	log2MaxPOC := int(br.ue()) + 4
	orderingAll := br.u(1) == 1
	first := maxSubLayers
	if orderingAll {
		first = 0
	}
	for i := first; i <= maxSubLayers; i++ {
		br.ue()
		br.ue()
		br.ue()
	}
	for range 6 { // log2_min_luma_coding_block … max_transform_hierarchy_depth_intra
		br.ue()
	}
	scalingListEnabled := br.u(1) == 1
	if scalingListEnabled && br.u(1) == 1 { // sps_scaling_list_data_present_flag
		for size := 0; size < 4; size++ {
			step := 1
			if size == 3 {
				step = 3
			}
			for matrix := 0; matrix < 6; matrix += step {
				if br.u(1) == 0 { // scaling_list_pred_mode_flag
					br.ue()
					continue
				}
				coefs := min(64, 1<<(4+(size<<1)))
				if size > 1 {
					br.se()
				}
				for k := 0; k < coefs; k++ {
					br.se()
				}
			}
		}
	}
	br.u(1)           // amp_enabled_flag
	br.u(1)           // sample_adaptive_offset_enabled_flag
	if br.u(1) == 1 { // pcm_enabled_flag
		br.u(4)
		br.u(4)
		br.ue()
		br.ue()
		br.u(1)
	}
	sets := int(br.ue())
	if sets > 64 {
		return videoBits{}, false
	}
	deltas := make([]int, sets)
	for i := 0; i < sets && !br.err; i++ {
		if i != 0 && br.u(1) == 1 { // inter_ref_pic_set_prediction_flag
			br.u(1) // delta_rps_sign
			br.ue() // abs_delta_rps_minus1
			count := 0
			for j := 0; j <= deltas[i-1]; j++ {
				used := br.u(1) == 1
				if used || br.u(1) == 1 { // use_delta_flag
					count++
				}
			}
			deltas[i] = count
			continue
		}
		negative, positive := br.ue(), br.ue()
		if negative > 16 || positive > 16 {
			return videoBits{}, false
		}
		for j := uint64(0); j < negative+positive; j++ {
			br.ue()
			br.u(1)
		}
		deltas[i] = int(negative + positive)
	}
	if br.u(1) == 1 { // long_term_ref_pics_present_flag
		n := br.ue()
		if n > 32 {
			return videoBits{}, false
		}
		for i := uint64(0); i < n; i++ {
			br.u(log2MaxPOC)
			br.u(1)
		}
	}
	br.u(1)           // sps_temporal_mvp_enabled_flag
	br.u(1)           // strong_intra_smoothing_enabled_flag
	if br.u(1) == 1 { // vui_parameters_present_flag
		readVUIColour(br, &bits)
		if br.u(1) == 1 { // chroma_loc_info_present_flag
			br.ue()
			br.ue()
		}
		br.u(1)                        // neutral_chroma_indication_flag
		bits.interlaced = br.u(1) == 1 // field_seq_flag
	}
	if br.err {
		return videoBits{}, false
	}
	return bits, true
}

// --- AV1 and VP9 -----------------------------------------------------------------------------

var av1Profiles = map[int]string{0: "Main", 1: "High", 2: "Professional"}

// parseAV1Config reads an AV1CodecConfigurationRecord. Its colour lives in the sequence header,
// which is not parsed: ≥10-bit AV1 without container colour goes to ffprobe.
func parseAV1Config(b []byte) (videoBits, bool) {
	if len(b) < 4 || b[0] != 0x81 {
		return videoBits{}, false
	}
	bits := videoBits{depth: 8, profile: av1Profiles[int(b[1]>>5)], level: strconv.Itoa(int(b[1] & 0x1F))}
	high, twelve, mono := b[2]&0x40 != 0, b[2]&0x20 != 0, b[2]&0x10 != 0
	if high {
		bits.depth = 10
		if twelve {
			bits.depth = 12
		}
	}
	x, y := b[2]&0x08 != 0, b[2]&0x04 != 0
	switch {
	case mono:
		bits.chroma = 0
	case x && y:
		bits.chroma = 1
	case x:
		bits.chroma = 2
	default:
		bits.chroma = 3
	}
	return bits, true
}

// vpChroma maps a VP chroma-subsampling code to the chroma format.
func vpChroma(code byte) int {
	switch code {
	case 0, 1:
		return 1
	case 2:
		return 2
	case 3:
		return 3
	}
	return -1
}

// parseVPConfig reads a VPCodecConfigurationRecord (the vpcC box, version 1, from its version
// byte): bit depth, chroma, range and the colour description.
func parseVPConfig(b []byte) (videoBits, bool) {
	if len(b) < 10 || b[0] != 1 {
		return videoBits{}, false
	}
	return videoBits{
		chroma: vpChroma((b[6] >> 1) & 0x07), depth: int(b[6] >> 4), colourRead: true, fullRange: b[6]&1 == 1,
		profile: "Profile " + strconv.Itoa(int(b[4])),
		colour:  colourInfo{primaries: int(b[7]), transfer: int(b[8]), matrix: int(b[9])},
	}, b[6]>>4 > 0
}

// matroskaVP9 reads Matroska's VP9 CodecPrivate, a list of (id, length, value) features: 1
// profile, 3 bit depth, 4 chroma subsampling. It carries no colour; profile 0 alone still fixes
// 8-bit 4:2:0.
func matroskaVP9(b []byte) (videoBits, bool) {
	bits := videoBits{chroma: -1}
	profile := -1
	for i := 0; i+2 <= len(b); {
		id, n := b[i], int(b[i+1])
		if i+2+n > len(b) || n != 1 {
			return videoBits{}, false
		}
		v := b[i+2]
		switch id {
		case 1:
			profile = int(v)
		case 3:
			bits.depth = int(v)
		case 4:
			bits.chroma = vpChroma(v)
		}
		i += 2 + n
	}
	if profile == 0 {
		bits.depth, bits.chroma = 8, 1
	}
	if profile >= 0 {
		bits.profile = "Profile " + strconv.Itoa(profile)
	}
	return bits, bits.depth > 0 && bits.chroma >= 0
}

// --- MPEG-1/2 and MPEG-4 Part 2 ----------------------------------------------------------------

// parseMPEG2Sequence reads a sequence header and its extensions: chroma format, progressive_sequence
// and the sequence display extension's colour.
func parseMPEG2Sequence(b []byte) (videoBits, bool) {
	bits := videoBits{chroma: 1, depth: 8, colourRead: true}
	seen := false
	for i := 0; i+5 < len(b); i++ {
		if b[i] != 0 || b[i+1] != 0 || b[i+2] != 1 {
			continue
		}
		switch b[i+3] {
		case 0xB3:
			seen = true
		case 0xB5:
			switch b[i+4] >> 4 {
			case 1: // sequence extension
				if i+6 >= len(b) {
					return videoBits{}, false
				}
				bits.interlaced = (b[i+5]>>3)&1 == 0
				bits.chroma = int((b[i+5] >> 1) & 0x03)
			case 2: // sequence display extension
				br := newBitReader(b[i+4:])
				br.u(4)
				br.u(3)
				if br.u(1) == 1 {
					bits.colour = colourInfo{
						primaries: int(br.u(8)), transfer: int(br.u(8)), matrix: int(br.u(8)), present: true,
					}
				}
			}
		}
	}
	return bits, seen
}

// parseMPEG4VOL reads the video_object_layer header for its interlaced flag.
func parseMPEG4VOL(b []byte) (videoBits, bool) {
	for i := 0; i+4 < len(b); i++ {
		if b[i] != 0 || b[i+1] != 0 || b[i+2] != 1 || b[i+3]&0xF0 != 0x20 {
			continue
		}
		br := newBitReader(b[i+4:])
		br.u(1) // random_accessible_vol
		br.u(8) // video_object_type_indication
		verid := uint64(1)
		if br.u(1) == 1 { // is_object_layer_identifier
			verid = br.u(4)
			br.u(3)
		}
		if br.u(4) == 15 { // aspect_ratio_info: extended PAR
			br.u(8)
			br.u(8)
		}
		chroma := 1
		if br.u(1) == 1 { // vol_control_parameters
			if br.u(2) != 1 {
				chroma = -1
			}
			br.u(1)
			if br.u(1) == 1 { // vbv_parameters
				br.u(16)
				br.u(16)
				br.u(16)
				br.u(3)
				br.u(12)
				br.u(16)
			}
		}
		shape := br.u(2)
		if shape == 3 && verid != 1 {
			br.u(4)
		}
		br.u(1)
		resolution := br.u(16)
		br.u(1)
		if br.u(1) == 1 { // fixed_vop_rate
			n := 1
			for (uint64(1) << n) < resolution {
				n++
			}
			br.u(n)
		}
		if shape != 0 { // only rectangular video is described here
			return videoBits{}, false
		}
		br.u(1)
		br.u(13)
		br.u(1)
		br.u(13)
		br.u(1)
		interlaced := br.u(1) == 1
		if br.err {
			return videoBits{}, false
		}
		return videoBits{chroma: chroma, depth: 8, colourRead: true, interlaced: interlaced}, chroma > 0
	}
	return videoBits{}, false
}

// --- AAC -------------------------------------------------------------------------------------

type audioConfig struct {
	profile    string
	sampleRate int
	channels   int
}

var aacRates = []int{96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050, 16000, 12000, 11025, 8000, 7350}

var aacProfiles = map[uint64]string{1: "Main", 2: "LC", 3: "SSR", 4: "LTP", 5: "HE-AAC", 29: "HE-AACv2", 23: "LD", 39: "ELD"}

// parseAudioSpecificConfig reads an AAC AudioSpecificConfig. An explicit SBR signal doubles the
// output rate, as ffprobe reports it.
func parseAudioSpecificConfig(b []byte) (audioConfig, bool) {
	if len(b) < 2 {
		return audioConfig{}, false
	}
	br := newBitReader(b)
	objectType := func() uint64 {
		t := br.u(5)
		if t == 31 {
			t = 32 + br.u(6)
		}
		return t
	}
	rate := func() int {
		i := br.u(4)
		if i == 15 {
			return int(br.u(24))
		}
		if int(i) < len(aacRates) {
			return aacRates[i]
		}
		return 0
	}
	aot := objectType()
	cfg := audioConfig{sampleRate: rate()}
	channelConfig := int(br.u(4))
	cfg.profile = aacProfiles[aot]
	if aot == 5 || aot == 29 { // explicit SBR: the extension rate, then the core object type
		cfg.sampleRate = rate()
		objectType()
	}
	switch channelConfig {
	case 1, 2, 3, 4, 5, 6:
		cfg.channels = channelConfig
	case 7:
		cfg.channels = 8
	}
	if br.err {
		return audioConfig{}, false
	}
	return cfg, true
}

// --- shared arithmetic -----------------------------------------------------------------------

// avReduce is libavutil's av_reduce: num/den as the closest fraction with both terms ≤ max.
func avReduce(num, den, limit int64) (int64, int64) {
	if den == 0 {
		return 0, 0
	}
	a0n, a0d, a1n, a1d := int64(0), int64(1), int64(1), int64(0)
	if num < 0 {
		num = -num
	}
	if den < 0 {
		den = -den
	}
	if g := gcd(num, den); g != 0 {
		num, den = num/g, den/g
	}
	if num <= limit && den <= limit {
		a1n, a1d = num, den
		den = 0
	}
	for den != 0 {
		x := num / den
		nextDen := num - den*x
		a2n, a2d := x*a1n+a0n, x*a1d+a0d
		if a2n > limit || a2d > limit {
			if a1n != 0 {
				x = (limit - a0n) / a1n
			}
			if a1d != 0 {
				x = min(x, (limit-a0d)/a1d)
			}
			if den*(2*x*a1d+a0d) > num*a1d {
				a1n, a1d = x*a1n+a0n, x*a1d+a0d
			}
			break
		}
		a0n, a0d, a1n, a1d = a1n, a1d, a2n, a2d
		num, den = den, nextDen
	}
	return a1n, a1d
}

func gcd(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func rational(num, den int64) string {
	return strconv.FormatInt(num, 10) + "/" + strconv.FormatInt(den, 10)
}

// bitrateOf is ffmpeg's estimate for a container that does not state one: the file's bits over
// its duration, rounded.
func bitrateOf(size, micros int64) int64 {
	if size <= 0 || micros <= 0 {
		return 0
	}
	bits := float64(size) * 8 * 1_000_000 / float64(micros)
	return int64(bits + 0.5)
}

// --- bit reading -----------------------------------------------------------------------------

// unescapeRBSP drops H.264/HEVC emulation-prevention bytes (00 00 03 → 00 00).
func unescapeRBSP(b []byte) []byte {
	out := make([]byte, 0, len(b))
	zeros := 0
	for _, c := range b {
		if zeros >= 2 && c == 3 {
			zeros = 0
			continue
		}
		out = append(out, c)
		if c == 0 {
			zeros++
		} else {
			zeros = 0
		}
	}
	return out
}

// bitReader reads MSB-first. Reading past the end sets err and yields zeros, so a parser checks
// err once at the end.
type bitReader struct {
	b   []byte
	pos int
	err bool
}

func newBitReader(b []byte) *bitReader { return &bitReader{b: b} }

func (r *bitReader) u(n int) uint64 {
	var v uint64
	for i := 0; i < n; i++ {
		if r.pos >= len(r.b)*8 {
			r.err = true
			return 0
		}
		bit := (r.b[r.pos/8] >> (7 - r.pos%8)) & 1
		v = v<<1 | uint64(bit)
		r.pos++
	}
	return v
}

// ue is an Exp-Golomb unsigned value.
func (r *bitReader) ue() uint64 {
	zeros := 0
	for r.u(1) == 0 {
		if r.err || zeros >= 32 {
			r.err = true
			return 0
		}
		zeros++
	}
	return (uint64(1)<<zeros - 1) + r.u(zeros)
}

// se is an Exp-Golomb signed value.
func (r *bitReader) se() int64 {
	v := r.ue()
	if v%2 == 1 {
		return int64(v/2) + 1
	}
	return -int64(v / 2)
}
