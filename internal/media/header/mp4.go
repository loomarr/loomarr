package header

import (
	"encoding/binary"
	"io"
	"math"

	"github.com/loomarr/loomarr/internal/inventory"
)

const (
	mp4MaxMoovBytes = 64 << 20
	mp4MaxBoxes     = 4096
)

type mp4Box struct {
	kind       string
	start, end int64 // the whole box
	body       int64 // first byte after the header
}

// mp4Boxes lists the child boxes in [from, to) by reading headers only, so a huge mdat costs one
// 16-byte read.
func mp4Boxes(r io.ReaderAt, from, to int64) []mp4Box {
	var boxes []mp4Box
	for pos := from; pos >= 0 && pos+8 <= to && len(boxes) < mp4MaxBoxes; {
		var h [16]byte
		n, _ := r.ReadAt(h[:], pos)
		if n < 8 {
			break
		}
		size, hdr := int64(binary.BigEndian.Uint32(h[0:4])), int64(8)
		switch {
		case size == 1 && n >= 16:
			wide := binary.BigEndian.Uint64(h[8:16])
			if wide > math.MaxInt64 {
				return boxes
			}
			size, hdr = int64(wide), 16
		case size == 0:
			size = to - pos
		}
		if size < hdr || size > to-pos {
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

// mp4Moov holds the moov box in memory; every lookup below is bounds-checked against it.
type mp4Moov []byte

func (m mp4Moov) children(parent mp4Box) []mp4Box {
	return mp4Boxes(bytesReaderAt(m), parent.body, parent.end)
}

func (m mp4Moov) child(parent mp4Box, kind string) (mp4Box, bool) {
	return find(m.children(parent), kind)
}

// path follows nested boxes by kind, e.g. path(trak, "mdia", "minf", "stbl").
func (m mp4Moov) path(from mp4Box, kinds ...string) (mp4Box, bool) {
	at := from
	for _, kind := range kinds {
		next, ok := m.child(at, kind)
		if !ok {
			return mp4Box{}, false
		}
		at = next
	}
	return at, true
}

func (m mp4Moov) body(b mp4Box) []byte {
	if b.body < 0 || b.end > int64(len(m)) || b.body > b.end {
		return nil
	}
	return m[b.body:b.end]
}

// readMoov finds and reads the moov box. ok is false for anything that is not an ISO BMFF file
// with a moov of reasonable size.
func readMoov(r io.ReaderAt, size int64) (mp4Moov, mp4Box, bool) {
	top := mp4Boxes(r, 0, size)
	if len(top) == 0 || (top[0].kind != "ftyp" && top[0].kind != "moov") {
		return nil, mp4Box{}, false
	}
	moovBox, found := find(top, "moov")
	if !found || moovBox.end-moovBox.body > mp4MaxMoovBytes {
		return nil, mp4Box{}, false
	}
	moov := make([]byte, moovBox.end-moovBox.body)
	if n, _ := r.ReadAt(moov, moovBox.body); n < len(moov) {
		return nil, mp4Box{}, false
	}
	// Re-base the box onto the in-memory copy: its children start at 0.
	return mp4Moov(moov), mp4Box{kind: "moov", start: -8, body: 0, end: int64(len(moov))}, true
}

// readMP4 returns the header facts of an MP4/MOV file. ok is false when the file is not ISO BMFF,
// carries a QuickTime chapter track (ffprobe reads those), or any fact playout relies on is not
// determined by the header.
func readMP4(r io.ReaderAt, size int64) (Header, bool) {
	moov, root, ok := readMoov(r, size)
	if !ok {
		return Header{}, false
	}
	mvhd, ok := moov.child(root, "mvhd")
	if !ok {
		return Header{}, false
	}
	timescale, duration, ok := fullBoxTiming(moov.body(mvhd))
	if !ok || timescale <= 0 || duration <= 0 {
		return Header{}, false
	}
	micros := durationMicros(duration, timescale)
	out := Header{
		Container:      "mov,mp4,m4a,3gp,3g2,mj2",
		DurationMillis: micros / 1000,
		Bitrate:        bitrateOf(size, micros),
	}
	index := 0
	for _, trak := range moov.children(root) {
		if trak.kind != "trak" {
			continue
		}
		if tref, has := moov.child(trak, "tref"); has {
			if _, chap := moov.child(tref, "chap"); chap {
				return Header{}, false // a QuickTime chapter track: ffprobe reads it
			}
		}
		stream, known := mp4Track(moov, trak)
		if !known {
			return Header{}, false
		}
		stream.Index = index
		index++
		if stream.Kind != "" {
			out.Streams = append(out.Streams, stream)
		}
	}
	if index == 0 {
		return Header{}, false
	}
	if udta, has := moov.child(root, "udta"); has {
		if chpl, has := moov.child(udta, "chpl"); has {
			out.Chapters = neroChapters(moov.body(chpl), out.DurationMillis)
		}
	}
	return out, true
}

// fullBoxTiming reads timescale and duration from an mvhd or mdhd body (version 0 or 1).
func fullBoxTiming(b []byte) (timescale, duration int64, ok bool) {
	if len(b) < 4 {
		return 0, 0, false
	}
	if b[0] == 1 {
		if len(b) < 32 {
			return 0, 0, false
		}
		d := binary.BigEndian.Uint64(b[24:32])
		if d > math.MaxInt64 {
			return 0, 0, false
		}
		return int64(binary.BigEndian.Uint32(b[20:24])), int64(d), true
	}
	if len(b) < 20 {
		return 0, 0, false
	}
	return int64(binary.BigEndian.Uint32(b[12:16])), int64(binary.BigEndian.Uint32(b[16:20])), true
}

func durationMicros(duration, timescale int64) int64 {
	return int64(float64(duration) * 1_000_000 / float64(timescale))
}

// mp4Track reads one trak. Kind is empty for a track ffprobe lists as neither video, audio nor
// subtitle (it still takes a stream index).
func mp4Track(moov mp4Moov, trak mp4Box) (stream inventory.Stream, known bool) {
	tkhd, ok := moov.child(trak, "tkhd")
	if !ok {
		return stream, false
	}
	tk := moov.body(tkhd)
	if len(tk) < 4 {
		return stream, false
	}
	stream.Disposition.Default = tk[3]&1 != 0 // track_enabled
	mdia, ok := moov.child(trak, "mdia")
	if !ok {
		return stream, false
	}
	hdlrBox, ok := moov.child(mdia, "hdlr")
	hdlr := moov.body(hdlrBox)
	if !ok || len(hdlr) < 12 {
		return stream, false
	}
	mdhdBox, ok := moov.child(mdia, "mdhd")
	mdhd := moov.body(mdhdBox)
	if !ok {
		return stream, false
	}
	timescale, _, ok := fullBoxTiming(mdhd)
	if !ok || timescale <= 0 {
		return stream, false
	}
	langAt := 20
	if mdhd[0] == 1 {
		langAt = 32
	}
	if len(mdhd) >= langAt+2 {
		stream.Language = mp4Language(binary.BigEndian.Uint16(mdhd[langAt:]))
	}
	stbl, ok := moov.path(mdia, "minf", "stbl")
	if !ok {
		return stream, false
	}
	stsdBox, ok := moov.child(stbl, "stsd")
	stsd := moov.body(stsdBox)
	if !ok || len(stsd) < 16 {
		return stream, false
	}
	entry := mp4Box{
		kind:  string(stsd[12:16]),
		start: stsdBox.body + 8,
		body:  stsdBox.body + 16,
		end:   stsdBox.body + 8 + int64(binary.BigEndian.Uint32(stsd[8:12])),
	}
	if entry.end > stsdBox.end || entry.end < entry.body {
		return stream, false
	}

	switch string(hdlr[8:12]) {
	case "vide":
		stream.Kind = inventory.StreamVideo
		return mp4Video(moov, stbl, entry, timescale, stream)
	case "soun":
		stream.Kind = inventory.StreamAudio
		return mp4Audio(moov, entry, stream)
	case "subt", "sbtl", "text", "clcp":
		stream.Kind = inventory.StreamSubtitle
		stream.Codec = map[string]string{"tx3g": "mov_text", "text": "mov_text", "wvtt": "webvtt", "stpp": "ttml", "c608": "eia_608"}[entry.kind]
		if stream.Codec == "" {
			return stream, false
		}
		stream.SubtitleFormat = stream.Codec
		return stream, true
	}
	return stream, true // timecode, hint and metadata tracks: an index, no facts
}

// mp4Language decodes mdhd's language as ffmpeg does: a packed ISO 639-2 code (kept even when it
// is "und"), or Macintosh code 0, English. Other Macintosh codes are left unknown.
func mp4Language(code uint16) string {
	switch {
	case code >= 0x400 && code != 0x7FFF:
		return string([]byte{byte(code>>10&0x1F) + 0x60, byte(code>>5&0x1F) + 0x60, byte(code&0x1F) + 0x60})
	case code == 0:
		return "eng"
	}
	return ""
}

const visualSampleEntryHeader = 78 // the fields before a visual sample entry's child boxes

func mp4Video(moov mp4Moov, stbl, entry mp4Box, timescale int64, stream inventory.Stream) (inventory.Stream, bool) {
	b := moov.body(entry)
	if len(b) < visualSampleEntryHeader {
		return stream, false
	}
	stream.Width = int(binary.BigEndian.Uint16(b[24:26]))
	stream.Height = int(binary.BigEndian.Uint16(b[26:28]))
	if stream.Width <= 0 || stream.Height <= 0 {
		return stream, false
	}
	kids := mp4Boxes(bytesReaderAt(moov), entry.body+visualSampleEntryHeader, entry.end)
	config := func(kind string) []byte {
		box, ok := find(kids, kind)
		if !ok {
			return nil
		}
		return moov.body(box)
	}
	var bits videoBits
	var ok bool
	switch entry.kind {
	case "avc1", "avc3":
		stream.Codec = "h264"
		bits, ok = videoConfig("h264", config("avcC"))
	case "hvc1", "hev1", "dvh1", "dvhe":
		stream.Codec = "hevc"
		bits, ok = videoConfig("hevc", config("hvcC"))
	case "av01":
		stream.Codec = "av1"
		bits, ok = videoConfig("av1", config("av1C"))
	case "vp09":
		stream.Codec = "vp9"
		bits, ok = parseVPConfig(config("vpcC"))
	case "mp4v":
		info, objectType := esdsDecoderInfo(config("esds"))
		stream.Codec = map[byte]string{0x20: "mpeg4", 0x61: "mpeg2video", 0x60: "mpeg2video", 0x6A: "mpeg1video"}[objectType]
		if stream.Codec != "" {
			bits, ok = videoConfig(stream.Codec, info)
		}
	}
	if !ok {
		return stream, false
	}
	var container colourInfo
	if colr := config("colr"); len(colr) >= 10 {
		switch string(colr[0:4]) {
		case "nclx", "nclc":
			container = colourInfo{
				primaries: int(binary.BigEndian.Uint16(colr[4:6])),
				transfer:  int(binary.BigEndian.Uint16(colr[6:8])),
				matrix:    int(binary.BigEndian.Uint16(colr[8:10])),
			}
		}
	}
	if !describeVideo(&stream, bits, container) {
		return stream, false
	}
	// A QuickTime 'fiel' box states the field layout; otherwise the bitstream must be progressive.
	if fiel := config("fiel"); len(fiel) >= 2 {
		switch {
		case fiel[0] == 1:
			if bits.interlaced {
				return stream, false
			}
		case fiel[1] == 1 || fiel[1] == 6 || fiel[1] == 9 || fiel[1] == 14:
			stream.Interlaced = true
		default:
			return stream, false
		}
	} else if bits.interlaced {
		return stream, false
	}
	frames, ticks, ok := sttsTotals(moov.body(mustChild(moov, stbl, "stts")))
	if !ok {
		return stream, false
	}
	num, den := avReduce(timescale*frames, ticks, math.MaxInt32)
	if num <= 0 || den <= 0 {
		return stream, false
	}
	stream.FrameRate = rational(num, den)
	return stream, true
}

func mustChild(moov mp4Moov, parent mp4Box, kind string) mp4Box {
	box, _ := moov.child(parent, kind)
	return box
}

// sttsTotals sums the decoding time-to-sample table: frames and media ticks.
func sttsTotals(b []byte) (frames, ticks int64, ok bool) {
	if len(b) < 8 {
		return 0, 0, false
	}
	n := int(binary.BigEndian.Uint32(b[4:8]))
	if n <= 0 || n > (len(b)-8)/8 {
		return 0, 0, false
	}
	for i := 0; i < n; i++ {
		count := int64(binary.BigEndian.Uint32(b[8+8*i:]))
		delta := int64(binary.BigEndian.Uint32(b[12+8*i:]))
		frames += count
		ticks += count * delta
		if frames < 0 || ticks < 0 {
			return 0, 0, false
		}
	}
	return frames, ticks, frames > 0 && ticks > 0
}

const audioSampleEntryHeader = 28 // the version-0 fields before an audio sample entry's children

func mp4Audio(moov mp4Moov, entry mp4Box, stream inventory.Stream) (inventory.Stream, bool) {
	b := moov.body(entry)
	if len(b) < audioSampleEntryHeader {
		return stream, false
	}
	version := binary.BigEndian.Uint16(b[8:10])
	stream.Channels = int(binary.BigEndian.Uint16(b[16:18]))
	stream.SampleRate = int(binary.BigEndian.Uint32(b[24:28]) >> 16)
	skip := int64(audioSampleEntryHeader)
	switch version {
	case 1:
		skip += 16
	case 2:
		return stream, false // QuickTime v2 audio (LPCM and friends): ffprobe reads it
	}
	kids := mp4Boxes(bytesReaderAt(moov), entry.body+skip, entry.end)
	config := func(kind string) []byte {
		box, ok := find(kids, kind)
		if !ok {
			return nil
		}
		return moov.body(box)
	}
	switch entry.kind {
	case "mp4a":
		info, objectType := esdsDecoderInfo(config("esds"))
		switch objectType {
		case 0x40, 0x66, 0x67, 0x68:
			stream.Codec = "aac"
			asc, ok := parseAudioSpecificConfig(info)
			if !ok || asc.channels == 0 {
				return stream, false
			}
			stream.Profile, stream.Channels = asc.profile, asc.channels
			if asc.sampleRate > 0 {
				stream.SampleRate = asc.sampleRate
			}
		case 0x69, 0x6B:
			stream.Codec = "mp3"
		case 0xA5:
			stream.Codec = "ac3"
		case 0xA6:
			stream.Codec = "eac3"
		default:
			return stream, false
		}
	case "ac-3":
		stream.Codec = "ac3"
		dac3 := config("dac3")
		if len(dac3) < 3 {
			return stream, false
		}
		bits := newBitReader(dac3)
		bits.u(2 + 5 + 3) // fscod, bsid, bsmod
		acmod, lfe := bits.u(3), bits.u(1)
		stream.Channels = ac3Channels[acmod] + int(lfe)
	case "ec-3":
		stream.Codec = "eac3"
		dec3 := config("dec3")
		if len(dec3) < 5 {
			return stream, false
		}
		bits := newBitReader(dec3)
		bits.u(13)
		if bits.u(3) != 0 { // more than one independent substream
			return stream, false
		}
		bits.u(2 + 5 + 1 + 1 + 3) // fscod, bsid, reserved, asvc, bsmod
		acmod, lfe := bits.u(3), bits.u(1)
		bits.u(3)
		if bits.u(4) != 0 { // dependent substreams (7.1): ffprobe counts their channels
			return stream, false
		}
		stream.Channels = ac3Channels[acmod] + int(lfe)
	case "Opus":
		stream.Codec = "opus"
		dops := config("dOps")
		if len(dops) < 2 {
			return stream, false
		}
		stream.Channels, stream.SampleRate = int(dops[1]), 48000
	case "fLaC":
		stream.Codec = "flac"
	case "alac":
		stream.Codec = "alac"
	default:
		return stream, false
	}
	if stream.Channels <= 0 || stream.SampleRate <= 0 {
		return stream, false
	}
	return stream, true
}

var ac3Channels = [8]int{2, 1, 2, 3, 3, 4, 4, 5}

// esdsDecoderInfo reads an esds body: the DecoderConfigDescriptor's objectTypeIndication and its
// DecoderSpecificInfo (an AAC AudioSpecificConfig, an MPEG-4 VOL).
func esdsDecoderInfo(b []byte) (info []byte, objectType byte) {
	if len(b) < 4 {
		return nil, 0
	}
	b = b[4:] // version and flags
	descriptor := func(b []byte) (tag byte, body, rest []byte, ok bool) {
		if len(b) < 2 {
			return 0, nil, nil, false
		}
		tag, size, i := b[0], 0, 1
		for ; i < 5 && i < len(b); i++ {
			size = size<<7 | int(b[i]&0x7F)
			if b[i]&0x80 == 0 {
				i++
				break
			}
		}
		if size < 0 || i+size > len(b) {
			return 0, nil, nil, false
		}
		return tag, b[i : i+size], b[i+size:], true
	}
	tag, es, _, ok := descriptor(b)
	if !ok || tag != 0x03 || len(es) < 3 {
		return nil, 0
	}
	flags := es[2]
	es = es[3:]
	if flags&0x80 != 0 { // streamDependenceFlag
		if len(es) < 2 {
			return nil, 0
		}
		es = es[2:]
	}
	if flags&0x40 != 0 { // URL_Flag
		if len(es) < 1 || len(es) < 1+int(es[0]) {
			return nil, 0
		}
		es = es[1+int(es[0]):]
	}
	if flags&0x20 != 0 { // OCRstreamFlag
		if len(es) < 2 {
			return nil, 0
		}
		es = es[2:]
	}
	tag, dec, _, ok := descriptor(es)
	if !ok || tag != 0x04 || len(dec) < 13 {
		return nil, 0
	}
	objectType = dec[0]
	if tag, specific, _, ok := descriptor(dec[13:]); ok && tag == 0x05 {
		info = specific
	}
	return info, objectType
}

// neroChapters reads a Nero 'chpl' box: start times in 100 ns units, then a title.
func neroChapters(b []byte, durationMillis int64) []Chapter {
	if len(b) < 5 {
		return nil
	}
	pos := 4
	if b[0] == 1 {
		pos += 4
	}
	if pos >= len(b) {
		return nil
	}
	count := int(b[pos])
	pos++
	var chapters []Chapter
	for i := 0; i < count && pos+9 <= len(b); i++ {
		start := int64(binary.BigEndian.Uint64(b[pos:]) / 10_000)
		n := int(b[pos+8])
		pos += 9
		if pos+n > len(b) {
			break
		}
		chapters = append(chapters, Chapter{StartMillis: start, EndMillis: -1, Title: string(b[pos : pos+n])})
		pos += n
	}
	for i := range chapters {
		if i+1 < len(chapters) {
			chapters[i].EndMillis = chapters[i+1].StartMillis
		} else {
			chapters[i].EndMillis = durationMillis
		}
	}
	return chapters
}

// mp4Keyframes reads the keyframe index of an MP4/MOV from the first video track's sample tables:
// sync samples (stss), their decode times (stts) and exact byte offsets (stsc, stsz, stco/co64).
// Only moov is read (it may sit at the end of the file); mdat is skipped by its size. Timestamps
// are decode times: composition offsets and edit lists are ignored, which shifts a keyframe by
// at most the reorder delay. ok is false for other containers or a track without stss.
func mp4Keyframes(r io.ReaderAt, size int64) (frames []inventory.Keyframe, ok bool, err error) {
	moov, root, found := readMoov(r, size)
	if !found {
		return nil, false, nil
	}
	for _, trak := range moov.children(root) {
		if trak.kind != "trak" {
			continue
		}
		mdia, has := moov.child(trak, "mdia")
		if !has {
			continue
		}
		hdlr, has := moov.child(mdia, "hdlr")
		if body := moov.body(hdlr); !has || len(body) < 12 || string(body[8:12]) != "vide" {
			continue
		}
		mdhd, has := moov.child(mdia, "mdhd")
		if !has {
			return nil, false, nil
		}
		timescale, _, timed := fullBoxTiming(moov.body(mdhd))
		stbl, has := moov.path(mdia, "minf", "stbl")
		if !timed || !has {
			return nil, false, nil
		}
		tables := map[string][]byte{}
		for _, b := range moov.children(stbl) {
			if body := moov.body(b); len(body) >= 4 {
				tables[b.kind] = body[4:] // skip version+flags
			}
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
	nSync, nStts, nStsc := u32(stss, 0), u32(stts, 0), u32(stsc, 0)
	nChunks := u32(chunkOffsets, 0)
	fixed, nSamples := u32(stsz, 0), u32(stsz, 1)
	entry := int64(4)
	if wide {
		entry = 8
	}
	if nSync == 0 || nChunks == 0 || 4+4*nSync > int64(len(stss)) || 4+8*nStts > int64(len(stts)) ||
		4+12*nStsc > int64(len(stsc)) || (fixed == 0 && 8+4*nSamples > int64(len(stsz))) ||
		4+entry*nChunks > int64(len(chunkOffsets)) {
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
	for c := 0; c < int(nChunks) && syncIdx < int(nSync); c++ {
		for stscIdx+1 < int(nStsc) && int(u32(stsc[4:], 3*(stscIdx+1)))-1 <= c {
			stscIdx++
		}
		perChunk := int(u32(stsc[4:], 3*stscIdx+1))
		offset := chunkOffset(c)
		for k := 0; k < perChunk && sample < int(nSamples) && syncIdx < int(nSync); k++ {
			for sttsLeft == 0 && sttsIdx < int(nStts) {
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
	if off < 0 || off >= int64(len(b)) {
		return 0, io.EOF
	}
	n := copy(p, b[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}
