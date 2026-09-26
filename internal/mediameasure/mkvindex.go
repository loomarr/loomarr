package mediameasure

import (
	"encoding/binary"
	"errors"
	"io"

	"github.com/loomarr/loomarr/internal/inventory"
)

// Matroska element IDs (with their length-marker bits, as they appear on disk).
const (
	idEBML           = 0x1A45DFA3
	idSegment        = 0x18538067
	idSeekHead       = 0x114D9B74
	idSeek           = 0x4DBB
	idSeekID         = 0x53AB
	idSeekPosition   = 0x53AC
	idInfo           = 0x1549A966
	idTimecodeScale  = 0x2AD7B1
	idTracks         = 0x1654AE6B
	idTrackEntry     = 0xAE
	idTrackNumber    = 0xD7
	idTrackType      = 0x83
	idCues           = 0x1C53BB6B
	idCuePoint       = 0xBB
	idCueTime        = 0xB3
	idCueTrackPos    = 0xB7
	idCueTrack       = 0xF7
	idCueClusterPos  = 0xF1
	mkvHeadWindow    = 1 << 20 // the SeekHead, Info and Tracks live in the first megabyte
	mkvMaxCuesBytes  = 64 << 20
	mkvMaxKeyframes  = 2_000_000
	defaultTimescale = 1_000_000 // ns per timecode tick
)

// ErrMatroskaIndex reports a Matroska file whose index is present but does not parse.
var ErrMatroskaIndex = errors.New("mediameasure: matroska index does not parse")

// matroskaKeyframes reads the keyframe index of a Matroska/WebM file from its Cues element: a
// few reads at the head (SeekHead, Info, Tracks) and one at the Cues position, which many files
// keep at the END of the file. It never scans clusters, so a 50 GB remux costs kilobytes over a
// network mount. ok is false when the file is not Matroska or has no Cues for a video track;
// the caller then decides whether a scan is affordable. Offsets are the cluster positions (the
// seekable unit), not packet positions.
func matroskaKeyframes(r io.ReaderAt, size int64) (frames []inventory.Keyframe, ok bool, err error) {
	head := make([]byte, min(size, mkvHeadWindow))
	if n, rerr := r.ReadAt(head, 0); n < 4 || (rerr != nil && rerr != io.EOF) {
		return nil, false, nil
	}
	head = head[:min(int64(len(head)), size)]
	id, _, n := readElementHeader(head)
	if n == 0 || id != idEBML {
		return nil, false, nil
	}
	// Skip the EBML header, then find the Segment.
	_, l, n := readElementHeader(head)
	pos := int64(n) + l
	if pos >= int64(len(head)) {
		return nil, false, nil
	}
	id, _, n = readElementHeader(head[pos:])
	if n == 0 || id != idSegment {
		return nil, false, nil
	}
	segStart := pos + int64(n)

	var cuesPos int64 = -1
	timescale := int64(defaultTimescale)
	videoTrack := int64(-1)
	// Walk the Segment's top-level children inside the head window (SeekHead, Info, Tracks come
	// before the first Cluster; anything past the window is not needed).
	for p := segStart; p < int64(len(head)); {
		cid, clen, cn := readElementHeader(head[p:])
		if cn == 0 {
			break
		}
		body := p + int64(cn)
		end := body + clen
		if clen < 0 || end > int64(len(head)) {
			end = int64(len(head))
		}
		switch cid {
		case idSeekHead:
			forEach(head[body:end], func(cid uint64, b []byte) {
				if cid != idSeek {
					return
				}
				var target uint64
				var at int64 = -1
				forEach(b, func(cid uint64, v []byte) {
					switch cid {
					case idSeekID:
						target = beUint(v)
					case idSeekPosition:
						at = int64(beUint(v))
					}
				})
				if target == idCues && at >= 0 {
					cuesPos = segStart + at
				}
			})
		case idInfo:
			forEach(head[body:end], func(cid uint64, b []byte) {
				if cid == idTimecodeScale {
					if v := int64(beUint(b)); v > 0 {
						timescale = v
					}
				}
			})
		case idTracks:
			forEach(head[body:end], func(cid uint64, b []byte) {
				if cid != idTrackEntry || videoTrack >= 0 {
					return
				}
				var number, kind int64 = -1, -1
				forEach(b, func(cid uint64, v []byte) {
					switch cid {
					case idTrackNumber:
						number = int64(beUint(v))
					case idTrackType:
						kind = int64(beUint(v))
					}
				})
				if kind == 1 {
					videoTrack = number
				}
			})
		case idCues:
			cuesPos = p
		case 0x1F43B675: // Cluster: past the head metadata
			p = int64(len(head))
			continue
		}
		if clen < 0 {
			break
		}
		p = end
	}
	if cuesPos < 0 || videoTrack < 0 {
		return nil, false, nil
	}
	hdr := make([]byte, 16)
	if n, _ := r.ReadAt(hdr, cuesPos); n < 4 {
		return nil, false, nil
	}
	id, clen, hn := readElementHeader(hdr)
	if hn == 0 || id != idCues || clen < 0 || clen > mkvMaxCuesBytes || cuesPos+int64(hn)+clen > size {
		return nil, false, nil
	}
	body := make([]byte, clen)
	if _, rerr := r.ReadAt(body, cuesPos+int64(hn)); rerr != nil && rerr != io.EOF {
		return nil, false, nil
	}
	forEach(body, func(cid uint64, b []byte) {
		if cid != idCuePoint || len(frames) >= mkvMaxKeyframes {
			return
		}
		var timeTicks, cluster int64 = -1, -1
		onVideo := false
		forEach(b, func(cid uint64, v []byte) {
			switch cid {
			case idCueTime:
				timeTicks = int64(beUint(v))
			case idCueTrackPos:
				var track int64 = -1
				var pos int64 = -1
				forEach(v, func(cid uint64, x []byte) {
					switch cid {
					case idCueTrack:
						track = int64(beUint(x))
					case idCueClusterPos:
						pos = int64(beUint(x))
					}
				})
				if track == videoTrack && !onVideo {
					onVideo, cluster = true, pos
				}
			}
		})
		if onVideo && timeTicks >= 0 && cluster >= 0 {
			frames = append(frames, inventory.Keyframe{
				PTSMs: timeTicks * timescale / 1_000_000, Offset: segStart + cluster,
			})
		}
	})
	if len(frames) == 0 {
		return nil, false, nil
	}
	for i := 1; i < len(frames); i++ {
		if frames[i].PTSMs < frames[i-1].PTSMs {
			return nil, false, ErrMatroskaIndex
		}
	}
	return frames, true, nil
}

// readElementHeader decodes an EBML element ID and data size. length is -1 for unknown size.
// n is the header size in bytes, 0 when the buffer does not hold a whole header.
func readElementHeader(b []byte) (id uint64, length int64, n int) {
	if len(b) == 0 {
		return 0, 0, 0
	}
	idLen := 1
	for mask := byte(0x80); idLen <= 4 && b[0]&mask == 0; mask >>= 1 {
		idLen++
	}
	if idLen > 4 || len(b) < idLen+1 {
		return 0, 0, 0
	}
	for _, c := range b[:idLen] {
		id = id<<8 | uint64(c)
	}
	sizeLen := 1
	for mask := byte(0x80); sizeLen <= 8 && b[idLen]&mask == 0; mask >>= 1 {
		sizeLen++
	}
	if sizeLen > 8 || len(b) < idLen+sizeLen {
		return 0, 0, 0
	}
	value := uint64(b[idLen] & (0xFF >> sizeLen))
	allOnes := value == uint64(0xFF>>sizeLen)
	for _, c := range b[idLen+1 : idLen+sizeLen] {
		value = value<<8 | uint64(c)
		allOnes = allOnes && c == 0xFF
	}
	if allOnes {
		return id, -1, idLen + sizeLen
	}
	return id, int64(value), idLen + sizeLen
}

// forEach calls fn for each child element in buf, stopping at the first truncated one.
func forEach(buf []byte, fn func(id uint64, body []byte)) {
	for len(buf) > 0 {
		id, l, n := readElementHeader(buf)
		if n == 0 || l < 0 || int64(n)+l > int64(len(buf)) {
			return
		}
		fn(id, buf[n:int64(n)+l])
		buf = buf[int64(n)+l:]
	}
}

func beUint(b []byte) uint64 {
	var buf [8]byte
	if len(b) > 8 {
		b = b[len(b)-8:]
	}
	copy(buf[8-len(b):], b)
	return binary.BigEndian.Uint64(buf[:])
}
