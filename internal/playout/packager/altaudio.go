package packager

import (
	"encoding/binary"
	"fmt"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4/seekablebuffer"
)

// The alternate audio rendition (#2088 decision 6, as amended). The encoder writes it as track 3
// of the channel's one stream, so it shares every slot decision, trim and slate with the default
// audio; the packager then writes it out as a rendition of its own: alt-init.mp4 holding track 3
// alone, and beside every segment an alt- file holding track 3's samples. The main files lose
// track 3, so everything that read them before (the variant playlist, CODECS, the tuners' TS
// writer) reads exactly what it always did.
//
// A channel's master playlist is read once per tune, so the slot is always there: the encoder maps
// the default audio again when the programme carries no second language. Which language it carries
// is told beside the stream (GET …/tracks), never in the playlists.

// AltInitName is the alternate audio's init segment, referenced by its playlist's EXT-X-MAP.
const AltInitName = "alt-init.mp4"

func altSegmentName(seq uint32) string { return "alt-" + segmentName(seq) }

// box assembles one box from its type and body parts.
func box(typ string, body ...[]byte) []byte {
	n := 8
	for _, b := range body {
		n += len(b)
	}
	out := make([]byte, 8, n)
	binary.BigEndian.PutUint32(out, uint32(n))
	copy(out[4:], typ)
	for _, b := range body {
		out = append(out, b...)
	}
	return out
}

// splitInit splits an init carrying track 3 into the main init (every other track) and the
// alternate's (track 3 alone), each with the mvhd and its own trex. ok is false without a track 3.
func splitInit(init []byte) (main, alt []byte, ok bool) {
	var ftyp []byte
	moov, moovSize := -1, 0
	children(init, 0, len(init), func(t string, s, sz int) {
		switch t {
		case "ftyp":
			ftyp = init[s : s+sz]
		case "moov":
			moov, moovSize = s, sz
		}
	})
	if moov < 0 {
		return nil, nil, false
	}
	var mainBody, altBody, mainTrex, altTrex [][]byte
	var shared [][]byte // mvhd and anything else that is neither a trak nor mvex
	children(init, moov+8, moov+moovSize, func(t string, s, sz int) {
		b := init[s : s+sz]
		switch t {
		case "trak":
			if trakID(init, s, sz) == altTrack {
				altBody = append(altBody, b)
				ok = true
			} else {
				mainBody = append(mainBody, b)
			}
		case "mvex":
			children(init, s+8, s+sz, func(ct string, cs, csz int) {
				cb := init[cs : cs+csz]
				switch {
				case ct == "trex" && csz >= 16 && binary.BigEndian.Uint32(init[cs+12:]) == altTrack:
					altTrex = append(altTrex, cb)
				case ct == "trex":
					mainTrex = append(mainTrex, cb)
				default: // mehd: the fragment duration, the same for both
					mainTrex, altTrex = append(mainTrex, cb), append(altTrex, cb)
				}
			})
		default:
			shared = append(shared, b)
		}
	})
	if !ok {
		return nil, nil, false
	}
	moovOf := func(traks, trex [][]byte) []byte {
		body := append(append(append([][]byte(nil), shared...), traks...), box("mvex", trex...))
		return append(append([]byte(nil), ftyp...), box("moov", body...)...)
	}
	return moovOf(mainBody, mainTrex), moovOf(altBody, altTrex), true
}

// trakID reads a trak's tkhd track_ID (version 0 or 1), or 0.
func trakID(b []byte, s, sz int) uint32 {
	var id uint32
	children(b, s+8, s+sz, func(t string, cs, csz int) {
		if t != "tkhd" || csz < 32 {
			return
		}
		at := cs + 20 // version+flags, creation and modification time (32-bit each)
		if b[cs+8] == 1 {
			at = cs + 28 // 64-bit times
		}
		if at+4 <= cs+csz {
			id = binary.BigEndian.Uint32(b[at:])
		}
	})
	return id
}

// splitFragment splits a stamped moof+mdat into the main fragment (every track but 3) and the
// alternate's (track 3 alone); alt is nil when the fragment carries no track 3, and main is then b
// itself. Every segment passes here, so the usual case is byte surgery: ffmpeg (and mediacommon)
// write one trun per traf with its data_offset from the moof, and track 3's samples last in the
// mdat. Anything else is re-marshalled instead, never refused.
func splitFragment(b []byte) (main, alt []byte, err error) {
	if main, alt, ok := splitInPlace(b); ok {
		return main, alt, nil
	}
	return splitByMarshal(b)
}

type trafAt struct {
	start, size int
	id          uint32
	offAt       int // the trun data_offset field's position; -1 when absent
	runs        int
}

func splitInPlace(b []byte) (main, alt []byte, ok bool) {
	if len(b) < minBoxHeader || string(b[4:8]) != "moof" {
		return nil, nil, false
	}
	moofSize := int(binary.BigEndian.Uint32(b))
	if moofSize+minBoxHeader > len(b) || string(b[moofSize+4:moofSize+8]) != "mdat" ||
		int(binary.BigEndian.Uint32(b[moofSize:])) != len(b)-moofSize {
		return nil, nil, false
	}
	var mfhd []byte
	var trafs []trafAt
	sane := true
	children(b, 8, moofSize, func(t string, s, sz int) {
		switch t {
		case "mfhd":
			mfhd = b[s : s+sz]
		case "traf":
			tr := trafAt{start: s, size: sz, offAt: -1}
			children(b, s+8, s+sz, func(ct string, cs, csz int) {
				switch {
				case ct == "tfhd" && csz >= 16:
					tr.id = binary.BigEndian.Uint32(b[cs+12:])
					if binary.BigEndian.Uint32(b[cs+8:])&0x1 != 0 { // base-data-offset-present
						sane = false
					}
				case ct == "trun" && csz >= 16:
					tr.runs++
					if binary.BigEndian.Uint32(b[cs+8:])&trunDataOffset != 0 {
						tr.offAt = cs + 16
					}
				}
			})
			trafs = append(trafs, tr)
		default:
			sane = false
		}
	})
	altAt := -1
	for i, tr := range trafs {
		if tr.runs != 1 || tr.offAt < 0 {
			sane = false
		}
		if tr.id == altTrack {
			altAt = i
		}
	}
	if altAt < 0 {
		return b, nil, true // nothing to split, whatever the layout
	}
	if !sane || mfhd == nil {
		return nil, nil, false
	}
	at := trafs[altAt]
	dataAt := int(int32(binary.BigEndian.Uint32(b[at.offAt:])))
	if dataAt < moofSize+minBoxHeader || dataAt > len(b) {
		return nil, nil, false
	}
	for i, tr := range trafs {
		if i != altAt && int(int32(binary.BigEndian.Uint32(b[tr.offAt:]))) > dataAt {
			return nil, nil, false // track 3's samples are not last
		}
	}

	// Main: the moof without track 3's traf, every data_offset moved back by the bytes it lost,
	// and the mdat cut before track 3's samples.
	moof := append(append([]byte(nil), b[:at.start]...), b[at.start+at.size:moofSize]...)
	binary.BigEndian.PutUint32(moof, uint32(moofSize-at.size))
	for i, tr := range trafs {
		if i == altAt {
			continue
		}
		pos := tr.offAt
		if pos > at.start {
			pos -= at.size
		}
		binary.BigEndian.PutUint32(moof[pos:], uint32(int32(binary.BigEndian.Uint32(moof[pos:]))-int32(at.size)))
	}
	main = append(moof, box("mdat", b[moofSize+minBoxHeader:dataAt])...)

	// Alternate: the mfhd and track 3's traf, its samples right after its own moof.
	traf := append([]byte(nil), b[at.start:at.start+at.size]...)
	altMoof := box("moof", mfhd, traf)
	binary.BigEndian.PutUint32(altMoof[8+len(mfhd)+at.offAt-at.start:], uint32(len(altMoof)+minBoxHeader))
	alt = append(altMoof, box("mdat", b[dataAt:])...)
	return main, alt, true
}

// splitByMarshal splits through mediacommon, for a layout splitInPlace does not take.
func splitByMarshal(b []byte) (main, alt []byte, err error) {
	parts, err := unmarshalFragment(b)
	if err != nil {
		return nil, nil, err
	}
	var mainParts, altParts fmp4.Parts
	for _, part := range parts {
		m, a := *part, *part
		m.Tracks, a.Tracks = nil, nil
		for _, tr := range part.Tracks {
			if tr.ID == altTrack {
				a.Tracks = append(a.Tracks, tr)
			} else {
				m.Tracks = append(m.Tracks, tr)
			}
		}
		mainParts = append(mainParts, &m)
		if len(a.Tracks) > 0 {
			altParts = append(altParts, &a)
		}
	}
	if len(altParts) == 0 {
		return b, nil, nil
	}
	marshal := func(ps fmp4.Parts) ([]byte, error) {
		var w seekablebuffer.Buffer
		if err := ps.Marshal(&w); err != nil {
			return nil, fmt.Errorf("packager: marshal split fragment: %w", err)
		}
		return w.Bytes(), nil
	}
	if main, err = marshal(mainParts); err != nil {
		return nil, nil, err
	}
	if alt, err = marshal(altParts); err != nil {
		return nil, nil, err
	}
	return main, alt, nil
}
