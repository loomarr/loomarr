package packager

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4/seekablebuffer"
)

// HDR10 is a premium HDR10 channel's static metadata, which the packager writes because no
// encoder can (#1512 G10, #1527): every item's own HDR side data is stripped, so the stream
// carries one fixed set and a TV never re-evaluates its tone curve at a seam.
type HDR10 struct {
	// SEI is the Annex B prefix SEI NAL (start code included) carrying the mastering display and
	// content light level messages, inserted once per IDR access unit.
	SEI []byte
	// MDCV and CLLI are the payloads of the init sample entry's mdcv and clli boxes.
	MDCV, CLLI []byte
	// Colr is the nclx colr payload, written only when the encoder's sample entry has no colr.
	Colr []byte
}

// visualSampleEntryHeader is a VisualSampleEntry's size before its child boxes: the box header,
// 6 reserved bytes, the data reference index and 70 bytes of visual fields.
const visualSampleEntryHeader = 8 + 6 + 2 + 70

// init returns init with the static boxes appended to each HEVC sample entry, every enclosing
// box grown to match. Boxes the entry already has are kept, so boxing twice changes nothing.
func (h *HDR10) init(init []byte) ([]byte, error) {
	type entry struct {
		ancestors  []int // start offsets of moov … stsd
		start, end int
	}
	var found []entry
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
						found = append(found, entry{here, es, es + esz})
					}
				})
			}
		})
	}
	walk(0, len(init), nil)
	if len(found) == 0 {
		return nil, fmt.Errorf("%w: an HDR10 init has no HEVC sample entry", errBadBox)
	}
	out := init
	for i := len(found) - 1; i >= 0; i-- { // last first: earlier offsets stay valid
		e := found[i]
		have := map[string]bool{}
		children(out, e.start+visualSampleEntryHeader, e.end, func(t string, _, _ int) { have[t] = true })
		var add []byte
		for _, b := range []struct {
			typ     string
			payload []byte
		}{{"colr", h.Colr}, {"mdcv", h.MDCV}, {"clli", h.CLLI}} {
			if !have[b.typ] && len(b.payload) > 0 {
				add = binary.BigEndian.AppendUint32(add, uint32(8+len(b.payload)))
				add = append(append(add, b.typ...), b.payload...)
			}
		}
		if len(add) == 0 {
			continue
		}
		grown := make([]byte, 0, len(out)+len(add))
		grown = append(append(append(grown, out[:e.end]...), add...), out[e.end:]...)
		for _, s := range append(e.ancestors, e.start) {
			binary.BigEndian.PutUint32(grown[s:], binary.BigEndian.Uint32(grown[s:])+uint32(len(add)))
		}
		out = grown
	}
	return out, nil
}

// fragment returns frag with the static SEI in each IDR access unit, just before its first slice.
func (h *HDR10) fragment(frag []byte) ([]byte, error) {
	parts, err := unmarshalFragment(frag)
	if err != nil {
		return nil, err
	}
	nal := bytes.TrimLeft(h.SEI, "\x00")
	if len(nal) < 2 || nal[0] != 1 {
		return nil, fmt.Errorf("packager: the HDR10 SEI is not an Annex B NAL")
	}
	sei := binary.BigEndian.AppendUint32(nil, uint32(len(nal)-1))
	sei = append(sei, nal[1:]...)
	changed := false
	for _, part := range parts {
		for _, tr := range part.Tracks {
			if tr.ID != videoTrack {
				continue
			}
			for _, s := range tr.Samples {
				if s.IsNonSyncSample {
					continue
				}
				if s.Payload, err = withPrefixSEI(s.Payload, sei); err != nil {
					return nil, err
				}
				changed = true
			}
		}
	}
	if !changed {
		return frag, nil
	}
	var w seekablebuffer.Buffer
	if err := parts.Marshal(&w); err != nil {
		return nil, fmt.Errorf("packager: marshal HDR10 fragment: %w", err)
	}
	return w.Bytes(), nil
}

// withPrefixSEI inserts sei, a length-prefixed NAL unit, before the first slice (VCL, types 0–31)
// of a 4-byte length-prefixed HEVC access unit. An access unit that already has it is returned as
// it is.
func withPrefixSEI(au, sei []byte) ([]byte, error) {
	for off := 0; off+4 <= len(au); {
		n := int(binary.BigEndian.Uint32(au[off:]))
		if n < 2 || off+4+n > len(au) {
			return nil, fmt.Errorf("%w: HEVC NAL length overruns its sample", errBadBox)
		}
		if bytes.HasPrefix(au[off:], sei) {
			return au, nil
		}
		if (au[off+4]>>1)&0x3f <= 31 {
			out := make([]byte, 0, len(au)+len(sei))
			return append(append(append(out, au[:off]...), sei...), au[off:]...), nil
		}
		off += 4 + n
	}
	return nil, fmt.Errorf("%w: an IDR sample with no slice", errBadBox)
}
