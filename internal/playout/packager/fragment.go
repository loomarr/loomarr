package packager

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4/seekablebuffer"
)

// The encoder contract (Pipeline.FragmentArgs): track 1 is video on a 90 kHz timescale, track 2 is
// AAC-LC at 48 kHz, one fragment per closed GOP.
const (
	videoTrack   = 1
	audioTrack   = 2
	videoRate    = 90000
	audioRate    = 48000
	aacFrame     = 1024
	maxBoxSize   = 256 << 20 // a 1 s fragment of 4K HEVC is a few MB; anything this large is a broken stream
	minBoxHeader = 8
)

var errBadBox = errors.New("packager: malformed fMP4 box")

// readBox reads one top-level box, header included.
func readBox(r *bufio.Reader) (typ string, b []byte, err error) {
	var h [16]byte
	if _, err = io.ReadFull(r, h[:8]); err != nil {
		return "", nil, err
	}
	size := uint64(binary.BigEndian.Uint32(h[:4]))
	typ = string(h[4:8])
	hdr := 8
	if size == 1 {
		if _, err = io.ReadFull(r, h[8:16]); err != nil {
			return "", nil, io.ErrUnexpectedEOF
		}
		size = binary.BigEndian.Uint64(h[8:16])
		hdr = 16
	}
	if size < uint64(hdr) || size > maxBoxSize {
		return "", nil, fmt.Errorf("%w: %q size %d", errBadBox, typ, size)
	}
	b = make([]byte, size)
	copy(b, h[:hdr])
	if _, err = io.ReadFull(r, b[hdr:]); err != nil {
		return "", nil, io.ErrUnexpectedEOF
	}
	return typ, b, nil
}

// children calls fn for each child box of the container body b[off:end].
func children(b []byte, off, end int, fn func(typ string, start, size int)) {
	for off+minBoxHeader <= end {
		sz := int(binary.BigEndian.Uint32(b[off:]))
		if sz < minBoxHeader || off+sz > end {
			return
		}
		fn(string(b[off+4:off+8]), off, sz)
		off += sz
	}
}

// fragmentCounts returns the sample count per track in a moof+mdat without changing it.
func fragmentCounts(b []byte) (map[uint32]int64, error) {
	return patchFragment(b, 0, nil)
}

// patchFragment rewrites mfhd.sequence_number and each traf's tfdt in place onto the channel
// timeline (base per track id; nil leaves them) and returns the sample count per track. This is
// the hot path: no allocation beyond the map, no re-marshal (spike cp2: 0.0017 cores per stream).
func patchFragment(b []byte, seq uint32, base map[uint32]uint64) (map[uint32]int64, error) {
	if len(b) < minBoxHeader || string(b[4:8]) != "moof" {
		return nil, fmt.Errorf("%w: fragment does not start with moof", errBadBox)
	}
	moofSize := int(binary.BigEndian.Uint32(b))
	if moofSize > len(b) {
		return nil, fmt.Errorf("%w: moof overruns fragment", errBadBox)
	}
	counts := map[uint32]int64{}
	bad := false
	children(b, 8, moofSize, func(typ string, s, sz int) {
		switch typ {
		case "mfhd":
			if base != nil && sz >= 16 {
				binary.BigEndian.PutUint32(b[s+12:], seq)
			}
		case "traf":
			var id uint32
			children(b, s+8, s+sz, func(t string, cs, csz int) {
				switch t {
				case "tfhd":
					if csz < 16 {
						bad = true
						return
					}
					id = binary.BigEndian.Uint32(b[cs+12:])
				case "tfdt":
					if base == nil {
						return
					}
					switch {
					case b[cs+8] == 1 && csz >= 20:
						binary.BigEndian.PutUint64(b[cs+12:], base[id])
					case b[cs+8] == 0 && csz >= 16:
						binary.BigEndian.PutUint32(b[cs+12:], uint32(base[id]))
					default:
						bad = true
					}
				case "trun":
					if csz < 16 {
						bad = true
						return
					}
					counts[id] += int64(binary.BigEndian.Uint32(b[cs+12:]))
				}
			})
		}
	})
	if bad {
		return nil, fmt.Errorf("%w: short tfhd/tfdt/trun", errBadBox)
	}
	return counts, nil
}

// rewriteFragment re-marshals a fragment through mediacommon, used only where samples must change:
//   - an item's first fragment: drop the AAC priming frame and even out the two video durations
//     ffmpeg's start shift leaves uneven (their sum is exact);
//   - a fragment that overfills its slot: ffmpeg's -frames:v/-frames:a are not exact (spike: 1198
//     of 1199 video, ±2 AAC), so the encoder over-produces and this trims to keep[id] samples.
func rewriteFragment(b []byte, seq uint32, base map[uint32]uint64, first bool, frameDur uint32,
	keep map[uint32]int64,
) ([]byte, map[uint32]int64, error) {
	parts, err := unmarshalFragment(b)
	if err != nil {
		return nil, nil, err
	}
	step := map[uint32]uint64{videoTrack: uint64(frameDur), audioTrack: aacFrame}
	counts := map[uint32]int64{}
	for _, part := range parts {
		part.SequenceNumber = seq
		for _, tr := range part.Tracks {
			id := uint32(tr.ID)
			if first && id == audioTrack && len(tr.Samples) > 0 {
				tr.Samples = tr.Samples[1:]
			}
			if first && id == videoTrack {
				for _, s := range tr.Samples {
					s.Duration, s.PTSOffset = frameDur, 0
				}
			}
			if n := keep[id] - counts[id]; int64(len(tr.Samples)) > n {
				tr.Samples = tr.Samples[:max(n, 0)]
			}
			tr.BaseTime = base[id] + uint64(counts[id])*step[id]
			counts[id] += int64(len(tr.Samples))
		}
	}
	var w seekablebuffer.Buffer
	if err := parts.Marshal(&w); err != nil {
		return nil, nil, fmt.Errorf("packager: marshal fragment: %w", err)
	}
	return w.Bytes(), counts, nil
}

const (
	trunDataOffset       = 0x001
	trunFirstSampleFlags = 0x004
	sampleNonSync        = 1 << 16
)

// unmarshalFragment parses a moof+mdat, restoring what mediacommon v2.9.5 drops: trun's
// first_sample_flags. ffmpeg marks each fragment's IDR only there, over a non-sync tfhd default,
// so without this every re-marshalled item start would be labelled non-sync and an MSE player
// would discard it as not a random-access point.
func unmarshalFragment(b []byte) (fmp4.Parts, error) {
	var parts fmp4.Parts
	if err := parts.Unmarshal(b); err != nil {
		return nil, fmt.Errorf("packager: unmarshal fragment: %w", err)
	}
	sync := map[uint32]bool{}
	if len(b) >= minBoxHeader {
		moofSize := min(int(binary.BigEndian.Uint32(b)), len(b))
		children(b, 8, moofSize, func(typ string, s, sz int) {
			if typ != "traf" {
				return
			}
			var id uint32
			children(b, s+8, s+sz, func(t string, cs, csz int) {
				switch {
				case t == "tfhd" && csz >= 16:
					id = binary.BigEndian.Uint32(b[cs+12:])
				case t == "trun" && csz >= 16:
					flags := binary.BigEndian.Uint32(b[cs+8:]) & 0xffffff
					off := cs + 16
					if flags&trunDataOffset != 0 {
						off += 4
					}
					if flags&trunFirstSampleFlags != 0 && off+4 <= cs+csz {
						sync[id] = binary.BigEndian.Uint32(b[off:])&sampleNonSync == 0
					}
				}
			})
		})
	}
	for _, part := range parts {
		for _, tr := range part.Tracks {
			if sync[uint32(tr.ID)] && len(tr.Samples) > 0 {
				tr.Samples[0].IsNonSyncSample = false
			}
		}
	}
	return parts, nil
}

// sampleDescriptions returns every trak's stsd box. A later item joins the channel's init segment
// only if these match byte for byte; otherwise a player would have to re-initialise its decoder.
func sampleDescriptions(init []byte) [][]byte {
	var out [][]byte
	var walk func(off, end int)
	walk = func(off, end int) {
		children(init, off, end, func(t string, s, sz int) {
			switch t {
			case "moov", "trak", "mdia", "minf", "stbl":
				walk(s+8, s+sz)
			case "stsd":
				out = append(out, init[s:s+sz])
			}
		})
	}
	walk(0, len(init))
	return out
}

// sameDecoderConfig reports whether two init segments describe identical sample entries.
func sameDecoderConfig(a, b []byte) bool {
	sa, sb := sampleDescriptions(a), sampleDescriptions(b)
	if len(sa) == 0 || len(sa) != len(sb) {
		return false
	}
	for i := range sa {
		if !bytes.Equal(sa[i], sb[i]) {
			return false
		}
	}
	return true
}
