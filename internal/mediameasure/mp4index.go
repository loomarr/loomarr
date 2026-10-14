package mediameasure

import (
	"encoding/binary"
	"io"

	"github.com/loomarr/loomarr/internal/inventory"
)

const mp4MaxMoovBytes = 64 << 20

type mp4Box struct {
	kind       string
	start, end int64 // the whole box
	body       int64 // first byte after the header
}

// mp4Boxes lists the child boxes in [from, to) by reading headers only, so a huge mdat costs one
// 16-byte read.
func mp4Boxes(r io.ReaderAt, from, to int64) []mp4Box {
	var boxes []mp4Box
	for pos := from; pos+8 <= to && len(boxes) < 4096; {
		var h [16]byte
		n, _ := r.ReadAt(h[:], pos)
		if n < 8 {
			break
		}
		size, hdr := int64(binary.BigEndian.Uint32(h[0:4])), int64(8)
		switch {
		case size == 1 && n >= 16:
			size, hdr = int64(binary.BigEndian.Uint64(h[8:16])), 16
		case size == 0:
			size = to - pos
		}
		if size < hdr || pos+size > to {
			break
		}
		boxes = append(boxes, mp4Box{kind: string(h[4:8]), start: pos, end: pos + size, body: pos + hdr})
		pos += size
	}
	return boxes
}

func find(boxes []mp4Box, kind string) (mp4Box, bool) {
	for _, b := range boxes {
		if b.kind == kind {
			return b, true
		}
	}
	return mp4Box{}, false
}

// mp4Keyframes reads the keyframe index of an MP4/MOV from the first video track's sample tables:
// sync samples (stss), their decode times (stts) and exact byte offsets (stsc, stsz, stco/co64).
// Only moov is read (it may sit at the end of the file); mdat is skipped by its size. Timestamps
// are decode times: composition offsets and edit lists are ignored, which shifts a keyframe by
// at most the reorder delay. ok is false for other containers or a track without stss.
func mp4Keyframes(r io.ReaderAt, size int64) (frames []inventory.Keyframe, ok bool, err error) {
	top := mp4Boxes(r, 0, size)
	if len(top) == 0 || (top[0].kind != "ftyp" && top[0].kind != "moov") {
		return nil, false, nil
	}
	moovBox, found := find(top, "moov")
	if !found || moovBox.end-moovBox.body > mp4MaxMoovBytes {
		return nil, false, nil
	}
	moov := make([]byte, moovBox.end-moovBox.body)
	if _, rerr := r.ReadAt(moov, moovBox.body); rerr != nil && rerr != io.EOF {
		return nil, false, nil
	}
	m := bytesReaderAt(moov)
	for _, trak := range mp4Boxes(m, 0, int64(len(moov))) {
		if trak.kind != "trak" {
			continue
		}
		mdia, _ := find(mp4Boxes(m, trak.body, trak.end), "mdia")
		mdiaKids := mp4Boxes(m, mdia.body, mdia.end)
		hdlr, hasHdlr := find(mdiaKids, "hdlr")
		if !hasHdlr || string(moov[hdlr.body+8:hdlr.body+12]) != "vide" {
			continue
		}
		mdhd, _ := find(mdiaKids, "mdhd")
		timescale := int64(binary.BigEndian.Uint32(moov[mdhd.body+12 : mdhd.body+16]))
		if moov[mdhd.body] == 1 {
			timescale = int64(binary.BigEndian.Uint32(moov[mdhd.body+20 : mdhd.body+24]))
		}
		minf, _ := find(mdiaKids, "minf")
		stbl, _ := find(mp4Boxes(m, minf.body, minf.end), "stbl")
		tables := map[string][]byte{}
		for _, b := range mp4Boxes(m, stbl.body, stbl.end) {
			tables[b.kind] = moov[b.body+4 : b.end] // skip version+flags
		}
		frames, ok = sampleTableKeyframes(tables, timescale)
		return frames, ok, nil
	}
	return nil, false, nil
}

func u32(b []byte, i int) int64 { return int64(binary.BigEndian.Uint32(b[i*4 : i*4+4])) }

func sampleTableKeyframes(t map[string][]byte, timescale int64) ([]inventory.Keyframe, bool) {
	stss, stts, stsc, stsz := t["stss"], t["stts"], t["stsc"], t["stsz"]
	chunkOffsets, wide := t["stco"], false
	if chunkOffsets == nil {
		chunkOffsets, wide = t["co64"], true
	}
	if timescale <= 0 || len(stss) < 4 || len(stts) < 4 || len(stsc) < 4 || len(stsz) < 8 || len(chunkOffsets) < 4 {
		return nil, false
	}
	nSync, nStts, nStsc := int(u32(stss, 0)), int(u32(stts, 0)), int(u32(stsc, 0))
	nChunks := int(u32(chunkOffsets, 0))
	fixed, nSamples := u32(stsz, 0), int(u32(stsz, 1))
	if 4+4*nSync > len(stss) || 4+8*nStts > len(stts) || 4+12*nStsc > len(stsc) || nSync == 0 || nChunks == 0 ||
		(fixed == 0 && 8+4*nSamples > len(stsz)) {
		return nil, false
	}
	entry := 4
	if wide {
		entry = 8
	}
	if 4+entry*nChunks > len(chunkOffsets) {
		return nil, false
	}
	sampleSize := func(i int) int64 {
		if fixed != 0 {
			return fixed
		}
		return u32(stsz, 2+i)
	}
	chunkOffset := func(c int) int64 {
		if wide {
			return int64(binary.BigEndian.Uint64(chunkOffsets[4+8*c:]))
		}
		return int64(binary.BigEndian.Uint32(chunkOffsets[4+4*c:]))
	}

	var frames []inventory.Keyframe
	sync, syncIdx := stss[4:], 0 // 1-based sample numbers, ascending
	sttsIdx, sttsLeft, decode := 0, int64(0), int64(0)
	var delta int64
	sample, stscIdx := 0, 0
	for c := 0; c < nChunks && syncIdx < nSync; c++ {
		for stscIdx+1 < nStsc && int(u32(stsc[4:], 3*(stscIdx+1)))-1 <= c {
			stscIdx++
		}
		perChunk := int(u32(stsc[4:], 3*stscIdx+1))
		offset := chunkOffset(c)
		for k := 0; k < perChunk && sample < nSamples && syncIdx < nSync; k++ {
			for sttsLeft == 0 && sttsIdx < nStts {
				sttsLeft, delta = u32(stts[4:], 2*sttsIdx), u32(stts[4:], 2*sttsIdx+1)
				sttsIdx++
			}
			if int(u32(sync, syncIdx)) == sample+1 {
				frames = append(frames, inventory.Keyframe{PTSMs: decode * 1000 / timescale, Offset: offset})
				syncIdx++
			}
			offset += sampleSize(sample)
			decode += delta
			sttsLeft--
			sample++
		}
	}
	return frames, len(frames) > 0
}

type bytesReaderAt []byte

func (b bytesReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(b)) {
		return 0, io.EOF
	}
	n := copy(p, b[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}
