package packager

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	mp4codecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
)

// CodecsAttr is the HLS CODECS attribute of a channel init: the RFC 6381 name of each track, read
// from the init itself rather than assumed from the output profile. It is "" when a track has no
// name here; the master then omits the attribute and players probe.
func CodecsAttr(init []byte) string {
	var in fmp4.Init
	if err := in.Unmarshal(bytes.NewReader(init)); err != nil {
		return ""
	}
	names := make([]string, 0, len(in.Tracks))
	for _, tr := range in.Tracks {
		switch c := tr.Codec.(type) {
		case *mp4codecs.H264:
			if len(c.SPS) < 4 {
				return ""
			}
			// profile_idc, constraint flags, level_idc.
			names = append(names, fmt.Sprintf("avc1.%02x%02x%02x", c.SPS[1], c.SPS[2], c.SPS[3]))
		case *mp4codecs.H265:
			name := hevcCodecName(init, c.SPS)
			if name == "" {
				return ""
			}
			names = append(names, name)
		case *mp4codecs.MPEG4Audio:
			names = append(names, fmt.Sprintf("mp4a.40.%d", c.Config.Type))
		default:
			return ""
		}
	}
	return strings.Join(names, ",")
}

// hevcCodecName is an HEVC track's RFC 6381 name (ISO/IEC 14496-15 E.3): the sample entry's 4CC
// (hvc1 or hev1, whichever the encoder wrote), then the SPS's profile space and idc, compatibility
// flags in reverse bit order, tier and level, and the constraint bytes up to the last non-zero one.
func hevcCodecName(init, rawSPS []byte) string {
	entries := hevcEntries(init)
	var sps h265.SPS
	if len(entries) == 0 || sps.Unmarshal(rawSPS) != nil {
		return ""
	}
	ptl := sps.ProfileTierLevel
	var compat uint32
	for j, set := range ptl.GeneralProfileCompatibilityFlag {
		if set {
			compat |= 1 << j
		}
	}
	tier := "L"
	if ptl.GeneralTierFlag == 1 {
		tier = "H"
	}
	name := fmt.Sprintf("%s.%s%d.%X.%s%d", entries[0].typ, [4]string{"", "A", "B", "C"}[ptl.GeneralProfileSpace&3],
		ptl.GeneralProfileIdc, compat, tier, ptl.GeneralLevelIdc)
	bit := func(b bool, shift uint) byte {
		if b {
			return 1 << shift
		}
		return 0
	}
	constraints := []byte{
		bit(ptl.GeneralProgressiveSourceFlag, 7) | bit(ptl.GeneralInterlacedSourceFlag, 6) |
			bit(ptl.GeneralNonPackedConstraintFlag, 5) | bit(ptl.GeneralFrameOnlyConstraintFlag, 4) |
			bit(ptl.GeneralMax12bitConstraintFlag, 3) | bit(ptl.GeneralMax10bitConstraintFlag, 2) |
			bit(ptl.GeneralMax8bitConstraintFlag, 1) | bit(ptl.GeneralMax422ChromeConstraintFlag, 0),
		bit(ptl.GeneralMax420ChromaConstraintFlag, 7) | bit(ptl.GeneralMaxMonochromeConstraintFlag, 6) |
			bit(ptl.GeneralIntraConstraintFlag, 5) | bit(ptl.GeneralOnePictureOnlyConstraintFlag, 4) |
			bit(ptl.GeneralLowerBitRateConstraintFlag, 3) | bit(ptl.GeneralMax14BitConstraintFlag, 2),
	}
	for len(constraints) > 1 && constraints[len(constraints)-1] == 0 {
		constraints = constraints[:len(constraints)-1]
	}
	for _, b := range constraints {
		name += fmt.Sprintf(".%X", b)
	}
	return name
}

// hevcEntry is one HEVC visual sample entry of an init: its 4CC, its span, and the start offsets of
// the boxes enclosing it (moov … stsd).
type hevcEntry struct {
	typ        string
	ancestors  []int
	start, end int
}

// hevcEntries finds every hvc1/hev1 sample entry in init, in file order.
func hevcEntries(init []byte) []hevcEntry {
	var found []hevcEntry
	var walk func(off, end int, path []int)
	walk = func(off, end int, path []int) {
		children(init, off, end, func(t string, s, sz int) {
			here := append(path[:len(path):len(path)], s)
			switch t {
			case "moov", "trak", "mdia", "minf", "stbl":
				walk(s+8, s+sz, here)
			case "stsd": // full box header, then entry_count
				children(init, s+16, s+sz, func(et string, es, esz int) {
					if (et == "hvc1" || et == "hev1") && esz >= visualSampleEntryHeader {
						found = append(found, hevcEntry{et, here, es, es + esz})
					}
				})
			}
		})
	}
	walk(0, len(init), nil)
	return found
}
