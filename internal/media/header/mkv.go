package header

import (
	"io"
	"strings"

	"github.com/loomarr/loomarr/internal/library/inventory"
)

// Matroska element IDs (with their length-marker bits, as they appear on disk).
const (
	idEBML            = 0x1A45DFA3
	idDocType         = 0x4282
	idSegment         = 0x18538067
	idSeekHead        = 0x114D9B74
	idSeek            = 0x4DBB
	idSeekID          = 0x53AB
	idSeekPosition    = 0x53AC
	idInfo            = 0x1549A966
	idTimecodeScale   = 0x2AD7B1
	idDuration        = 0x4489
	idTracks          = 0x1654AE6B
	idTrackEntry      = 0xAE
	idTrackNumber     = 0xD7
	idTrackType       = 0x83
	idFlagDefault     = 0x88
	idFlagForced      = 0x55AA
	idFlagHearing     = 0x55AB
	idFlagVisual      = 0x55AC
	idFlagCommentary  = 0x55AF
	idBlockAddMapping = 0x41E4
	idBlockAddIDType  = 0x41E7
	idBlockAddIDExtra = 0x41ED
	idDefaultDuration = 0x23E383
	idName            = 0x536E
	idLanguage        = 0x22B59C
	idLanguageBCP47   = 0x22B59D
	idCodecID         = 0x86
	idCodecPrivate    = 0x63A2
	idVideo           = 0xE0
	idPixelWidth      = 0xB0
	idPixelHeight     = 0xBA
	idFlagInterlaced  = 0x9A
	idFieldOrder      = 0x9D
	idColour          = 0x55B0
	idMatrixCoeffs    = 0x55B1
	idTransferChar    = 0x55BA
	idPrimaries       = 0x55BB
	idAudio           = 0xE1
	idSamplingFreq    = 0xB5
	idOutputSampFreq  = 0x78B5
	idChannels        = 0x9F
	idBitDepth        = 0x6264
	idChapters        = 0x1043A770
	idEditionEntry    = 0x45B9
	idChapterAtom     = 0xB6
	idChapterStart    = 0x91
	idChapterEnd      = 0x92
	idChapterDisplay  = 0x80
	idChapString      = 0x85
	idCues            = 0x1C53BB6B
	idCuePoint        = 0xBB
	idCueTime         = 0xB3
	idCueTrackPos     = 0xB7
	idCueTrack        = 0xF7
	idCueClusterPos   = 0xF1
	idCluster         = 0x1F43B675

	// headWindow is the first read of every file: the SeekHead, Info and Tracks of nearly every
	// Matroska file fit in it (#2151 spike: one 64 KiB read at p50).
	headWindow       = 64 << 10
	mkvMaxElement    = 16 << 20 // Info, Tracks or Chapters larger than this is not a header
	mkvMaxCuesBytes  = 64 << 20
	mkvMaxKeyframes  = 2_000_000
	defaultTimescale = 1_000_000 // ns per timecode tick
)

// mkvHead is the Segment's metadata, gathered from the head window and, where the SeekHead points
// past it, from one read per element.
type mkvHead struct {
	segStart int64
	info     []byte
	tracks   []byte
	chapters []byte
	cuesPos  int64 // -1 when the file has no Cues
}

// readMatroskaHead locates and reads the Segment's Info, Tracks and Chapters, and the Cues
// position. ok is false when the file is not Matroska/WebM or its Info or Tracks cannot be found.
func readMatroskaHead(r io.ReaderAt, size int64) (mkvHead, bool) {
	head := make([]byte, min(size, headWindow))
	n, err := r.ReadAt(head, 0)
	if n < 4 || (err != nil && err != io.EOF) {
		return mkvHead{}, false
	}
	head = head[:n]
	id, l, hn := readElementHeader(head)
	if hn == 0 || id != idEBML || l < 0 || l > int64(len(head)-hn) {
		return mkvHead{}, false
	}
	docType := ""
	forEach(head[hn:int64(hn)+l], func(cid uint64, b []byte) {
		if cid == idDocType {
			docType = string(b)
		}
	})
	if docType != "matroska" && docType != "webm" {
		return mkvHead{}, false
	}
	pos := int64(hn) + l
	if pos >= int64(len(head)) {
		return mkvHead{}, false
	}
	id, _, hn = readElementHeader(head[pos:])
	if hn == 0 || id != idSegment {
		return mkvHead{}, false
	}
	h := mkvHead{segStart: pos + int64(hn), cuesPos: -1}
	seeks := map[uint64]int64{}

	// element returns an element's body: from the head window when it fits, otherwise one read.
	element := func(body, length int64) []byte {
		if length < 0 || length > mkvMaxElement || body+length > size {
			return nil
		}
		if body+length <= int64(len(head)) {
			return head[body : body+length]
		}
		buf := make([]byte, length)
		if got, rerr := r.ReadAt(buf, body); int64(got) < length || (rerr != nil && rerr != io.EOF) {
			return nil
		}
		return buf
	}
	take := func(cid uint64, body []byte) {
		switch cid {
		case idSeekHead:
			forEach(body, func(cid uint64, b []byte) {
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
						if p := beUint(v); p < uint64(size) {
							at = int64(p)
						}
					}
				})
				if at >= 0 {
					if _, seen := seeks[target]; !seen {
						seeks[target] = h.segStart + at
					}
				}
			})
		case idInfo:
			h.info = body
		case idTracks:
			h.tracks = body
		case idChapters:
			h.chapters = body
		}
	}

	// Walk the Segment's top-level children inside the head window; metadata precedes the first
	// Cluster.
	for p := h.segStart; p < int64(len(head)); {
		cid, clen, cn := readElementHeader(head[p:])
		if cn == 0 || cid == idCluster {
			break
		}
		if cid == idCues {
			h.cuesPos = p
		}
		if clen < 0 {
			break
		}
		// Only the elements read here cost a read when they run past the window; Void,
		// Attachments and Tags are skipped by their size.
		if cid == idSeekHead || cid == idInfo || cid == idTracks || cid == idChapters {
			if body := element(p+int64(cn), clen); body != nil {
				take(cid, body)
			}
		}
		p += int64(cn) + clen
	}

	// Follow the SeekHead to anything the window did not hold, including a second SeekHead.
	at := func(target uint64) []byte {
		p, ok := seeks[target]
		if !ok {
			return nil
		}
		var hdr [16]byte
		got, _ := r.ReadAt(hdr[:], p)
		cid, clen, cn := readElementHeader(hdr[:got])
		if cn == 0 || cid != target {
			return nil
		}
		return element(p+int64(cn), clen)
	}
	if p, ok := seeks[idSeekHead]; ok && p >= int64(len(head)) {
		if body := at(idSeekHead); body != nil {
			take(idSeekHead, body)
		}
	}
	if h.info == nil {
		h.info = at(idInfo)
	}
	if h.tracks == nil {
		h.tracks = at(idTracks)
	}
	if h.chapters == nil {
		h.chapters = at(idChapters)
	}
	if p, ok := seeks[idCues]; ok && h.cuesPos < 0 {
		h.cuesPos = p
	}
	if h.info == nil || h.tracks == nil {
		return mkvHead{}, false
	}
	return h, true
}

// readMatroska returns the header facts of a Matroska/WebM file. ok is false when the file is not
// Matroska or any fact playout relies on is not determined by the header.
func readMatroska(r io.ReaderAt, size int64) (Header, bool) {
	h, ok := readMatroskaHead(r, size)
	if !ok {
		return Header{}, false
	}
	timescale := int64(defaultTimescale)
	duration := -1.0
	forEach(h.info, func(cid uint64, b []byte) {
		switch cid {
		case idTimecodeScale:
			if v := int64(beUint(b)); v > 0 {
				timescale = v
			}
		case idDuration:
			duration = beFloat(b)
		}
	})
	if duration <= 0 {
		return Header{}, false // a live-written file without a Duration: ffprobe estimates it
	}
	// ffmpeg's matroskadec: duration × TimecodeScale, in microseconds, then milliseconds.
	micros := int64(duration * float64(timescale) / 1000)
	out := Header{
		Container:      "matroska,webm",
		DurationMillis: micros / 1000,
		Bitrate:        bitrateOf(size, micros),
	}

	index := 0
	complete := true
	forEach(h.tracks, func(cid uint64, b []byte) {
		if cid != idTrackEntry || !complete {
			return
		}
		stream, known := matroskaTrack(b)
		if !known {
			complete = false
			return
		}
		stream.Index = index
		index++
		if stream.Kind != "" {
			out.Streams = append(out.Streams, stream)
		}
	})
	if !complete || index == 0 {
		return Header{}, false
	}
	out.Chapters = matroskaChapters(h.chapters)
	return out, true
}

// matroskaTrack reads one TrackEntry. Kind is empty for a track ffprobe lists as neither video,
// audio nor subtitle (it still takes a stream index). known is false when the codec is one the
// header cannot describe completely.
func matroskaTrack(b []byte) (stream inventory.Stream, known bool) {
	var kind, flagInterlaced, fieldOrder uint64
	var defaultDuration int64
	var codecID, language, bcp47 string
	var private []byte
	var colour colourInfo
	var sampling, outputSampling float64
	var bitDepth int
	stream.Disposition.Default = true // FlagDefault defaults to 1
	stream.Channels = 1               // Channels defaults to 1
	forEach(b, func(cid uint64, v []byte) {
		switch cid {
		case idTrackType:
			kind = beUint(v)
		case idFlagDefault:
			stream.Disposition.Default = beUint(v) != 0
		case idFlagForced:
			stream.Disposition.Forced = beUint(v) != 0
		case idFlagHearing:
			stream.Disposition.HearingImpaired = beUint(v) != 0
		case idFlagVisual:
			stream.Disposition.VisualImpaired = beUint(v) != 0
		case idFlagCommentary:
			stream.Disposition.Comment = beUint(v) != 0
		case idBlockAddMapping:
			var fourcc uint64
			var record []byte
			forEach(v, func(cid uint64, x []byte) {
				switch cid {
				case idBlockAddIDType:
					fourcc = beUint(x)
				case idBlockAddIDExtra:
					record = x
				}
			})
			switch fourcc {
			case 0x64766343, 0x64767643, 0x64767743: // dvcC, dvvC, dvwC
				stream.DolbyVision = dolbyVisionProfile(record)
			}
		case idDefaultDuration:
			defaultDuration = int64(beUint(v))
		case idName:
			stream.Title = string(v)
		case idLanguage:
			language = string(v)
		case idLanguageBCP47:
			bcp47 = string(v)
		case idCodecID:
			codecID = string(v)
		case idCodecPrivate:
			private = v
		case idVideo:
			forEach(v, func(cid uint64, x []byte) {
				switch cid {
				case idPixelWidth:
					stream.Width = int(beUint(x))
				case idPixelHeight:
					stream.Height = int(beUint(x))
				case idFlagInterlaced:
					flagInterlaced = beUint(x)
				case idFieldOrder:
					fieldOrder = beUint(x)
				case idColour:
					forEach(x, func(cid uint64, c []byte) {
						switch cid {
						case idMatrixCoeffs:
							colour.matrix = int(beUint(c))
						case idTransferChar:
							colour.transfer = int(beUint(c))
						case idPrimaries:
							colour.primaries = int(beUint(c))
						}
					})
				}
			})
		case idAudio:
			forEach(v, func(cid uint64, x []byte) {
				switch cid {
				case idSamplingFreq:
					sampling = beFloat(x)
				case idOutputSampFreq:
					outputSampling = beFloat(x)
				case idChannels:
					stream.Channels = int(beUint(x))
				case idBitDepth:
					bitDepth = int(beUint(x))
				}
			})
		}
	})
	// ffmpeg reads Language (default "eng") and drops "und".
	switch {
	case language != "":
	case bcp47 != "":
		language = bcp47
	default:
		language = "eng"
	}
	if language != "und" {
		stream.Language = language
	}

	switch kind {
	case 1:
		stream.Kind = inventory.StreamVideo
		stream.Channels = 0
		stream.Codec = matroskaVideoCodec(codecID, private)
		if stream.Codec == "" || stream.Width <= 0 || stream.Height <= 0 {
			return stream, false
		}
		if defaultDuration <= 0 {
			return stream, false // ffprobe counts frames for the rate; the header cannot
		}
		num, den := avReduce(1_000_000_000, defaultDuration, 30000)
		stream.FrameRate = rational(num, den)
		var bits videoBits
		var ok bool
		if stream.Codec == "vp9" {
			bits, ok = matroskaVP9(private)
		} else {
			bits, ok = videoConfig(stream.Codec, private)
		}
		if !ok || !describeVideo(&stream, bits, colour) {
			return stream, false
		}
		// The container's interlace flag: 1 is interlaced, and then the field order must be known.
		switch flagInterlaced {
		case 1:
			if fieldOrder != 1 && fieldOrder != 6 && fieldOrder != 9 && fieldOrder != 14 {
				return stream, false
			}
			stream.Interlaced = true
		case 2:
			if bits.interlaced {
				return stream, false
			}
		default:
			if bits.interlaced {
				return stream, false
			}
		}
	case 2:
		stream.Kind = inventory.StreamAudio
		stream.Codec = matroskaAudioCodec(codecID, bitDepth)
		if stream.Codec == "" {
			return stream, false
		}
		rate := sampling
		if outputSampling > 0 {
			rate = outputSampling
		}
		if stream.Codec == "aac" {
			if asc, ok := parseAudioSpecificConfig(private); ok {
				stream.Profile = asc.profile
				if asc.sampleRate > 0 {
					rate = float64(asc.sampleRate)
				}
				if asc.channels > 0 {
					stream.Channels = asc.channels
				}
			}
		}
		if rate <= 0 || stream.Channels <= 0 {
			return stream, false
		}
		stream.SampleRate = int(rate)
	case 0x11:
		stream.Kind = inventory.StreamSubtitle
		stream.Channels = 0
		stream.Codec = matroskaSubtitleCodec(codecID)
		if stream.Codec == "" {
			return stream, false
		}
		stream.SubtitleFormat = stream.Codec
	default:
		stream.Channels = 0
		stream.Kind = ""
	}
	return stream, true
}

// matroskaChapters reads the first edition's top-level chapters.
func matroskaChapters(body []byte) []Chapter {
	var chapters []Chapter
	edition := false
	forEach(body, func(cid uint64, b []byte) {
		if cid != idEditionEntry || edition {
			return
		}
		edition = true
		forEach(b, func(cid uint64, atom []byte) {
			if cid != idChapterAtom {
				return
			}
			c := Chapter{EndMillis: -1}
			forEach(atom, func(cid uint64, v []byte) {
				switch cid {
				case idChapterStart:
					c.StartMillis = int64(beUint(v) / 1_000_000)
				case idChapterEnd:
					c.EndMillis = int64(beUint(v) / 1_000_000)
				case idChapterDisplay:
					forEach(v, func(cid uint64, s []byte) {
						if cid == idChapString && c.Title == "" {
							c.Title = string(s)
						}
					})
				}
			})
			chapters = append(chapters, c)
		})
	})
	return chapters
}

var matroskaVideoCodecs = map[string]string{
	"V_MPEG4/ISO/AVC":  "h264",
	"V_MPEGH/ISO/HEVC": "hevc",
	"V_AV1":            "av1",
	"V_VP8":            "vp8",
	"V_VP9":            "vp9",
	"V_MPEG2":          "mpeg2video",
	"V_MPEG1":          "mpeg1video",
	"V_MPEG4/ISO/ASP":  "mpeg4",
	"V_MPEG4/ISO/SP":   "mpeg4",
	"V_MPEG4/ISO/AP":   "mpeg4",
	"V_THEORA":         "theora",
	"V_PRORES":         "prores",
	"V_MJPEG":          "mjpeg",
	"V_FFV1":           "ffv1",
}

func matroskaVideoCodec(codecID string, private []byte) string {
	if codec, ok := matroskaVideoCodecs[codecID]; ok {
		return codec
	}
	if codecID == "V_MS/VFW/FOURCC" && len(private) >= 20 {
		// BITMAPINFOHEADER: biCompression at offset 16.
		switch strings.ToUpper(string(private[16:20])) {
		case "XVID", "DIVX", "DX50", "FMP4", "MP4V":
			return "mpeg4"
		case "H264", "AVC1", "X264":
			return "h264"
		case "WVC1":
			return "vc1"
		case "MPG2":
			return "mpeg2video"
		}
	}
	return ""
}

func matroskaAudioCodec(codecID string, bitDepth int) string {
	switch {
	case strings.HasPrefix(codecID, "A_AAC"):
		return "aac"
	case codecID == "A_AC3", codecID == "A_AC3/BSID9", codecID == "A_AC3/BSID10":
		return "ac3"
	case codecID == "A_EAC3":
		return "eac3"
	case codecID == "A_DTS", codecID == "A_DTS/EXPRESS", codecID == "A_DTS/LOSSLESS":
		return "dts"
	case codecID == "A_TRUEHD":
		return "truehd"
	case codecID == "A_FLAC":
		return "flac"
	case codecID == "A_OPUS":
		return "opus"
	case codecID == "A_VORBIS":
		return "vorbis"
	case codecID == "A_MPEG/L3":
		return "mp3"
	case codecID == "A_MPEG/L2":
		return "mp2"
	case codecID == "A_ALAC":
		return "alac"
	case codecID == "A_PCM/INT/LIT":
		switch bitDepth {
		case 8:
			return "pcm_u8"
		case 16:
			return "pcm_s16le"
		case 24:
			return "pcm_s24le"
		case 32:
			return "pcm_s32le"
		}
	case codecID == "A_PCM/INT/BIG":
		switch bitDepth {
		case 16:
			return "pcm_s16be"
		case 24:
			return "pcm_s24be"
		case 32:
			return "pcm_s32be"
		}
	case codecID == "A_PCM/FLOAT/IEEE":
		switch bitDepth {
		case 32:
			return "pcm_f32le"
		case 64:
			return "pcm_f64le"
		}
	}
	return ""
}

var matroskaSubtitleCodecs = map[string]string{
	"S_TEXT/UTF8":   "subrip",
	"S_TEXT/ASCII":  "subrip",
	"S_TEXT/SSA":    "ass",
	"S_TEXT/ASS":    "ass",
	"S_SSA":         "ass",
	"S_ASS":         "ass",
	"S_TEXT/WEBVTT": "webvtt",
	"S_HDMV/PGS":    "hdmv_pgs_subtitle",
	"S_HDMV/TEXTST": "hdmv_text_subtitle",
	"S_VOBSUB":      "dvd_subtitle",
	"S_DVBSUB":      "dvb_subtitle",
	"S_ARIBSUB":     "arib_caption",
}

func matroskaSubtitleCodec(codecID string) string { return matroskaSubtitleCodecs[codecID] }

// matroskaKeyframes reads the keyframe index of a Matroska/WebM file from its Cues element: the
// head reads above and one read at the Cues position, which many files keep at the END of the file.
// It never scans clusters, so a 50 GB remux costs kilobytes over a network mount. ok is false when
// the file is not Matroska or has no Cues for a video track. Offsets are cluster positions (the
// seekable unit), not packet positions.
func matroskaKeyframes(r io.ReaderAt, size int64) (frames []inventory.Keyframe, ok bool, err error) {
	h, found := readMatroskaHead(r, size)
	if !found || h.cuesPos < 0 {
		return nil, false, nil
	}
	timescale := int64(defaultTimescale)
	forEach(h.info, func(cid uint64, b []byte) {
		if cid == idTimecodeScale {
			if v := int64(beUint(b)); v > 0 {
				timescale = v
			}
		}
	})
	videoTrack := int64(-1)
	forEach(h.tracks, func(cid uint64, b []byte) {
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
	if videoTrack < 0 {
		return nil, false, nil
	}
	hdr := make([]byte, 16)
	if n, _ := r.ReadAt(hdr, h.cuesPos); n < 4 {
		return nil, false, nil
	}
	id, clen, hn := readElementHeader(hdr)
	if hn == 0 || id != idCues || clen < 0 || clen > mkvMaxCuesBytes || h.cuesPos+int64(hn)+clen > size {
		return nil, false, nil
	}
	body := make([]byte, clen)
	if _, rerr := r.ReadAt(body, h.cuesPos+int64(hn)); rerr != nil && rerr != io.EOF {
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
				PTSMs: timeTicks * timescale / 1_000_000, Offset: h.segStart + cluster,
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
