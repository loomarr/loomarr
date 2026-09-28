package playout

import (
	"math"
	"strconv"
)

// ffmpeg argument construction for internal playout (§9.1).
//
// Args are built as a []string, never a shelled-out command line — a filter graph
// contains commas, colons and quotes, and handing that to a shell is how you get an
// injection or an unexplainable parse error.
//
// Shape borrowed from Tunarr/ErsatzTV (prior-art notes in git history before #1572):
// options are appended in POSITIONAL buckets, because ffmpeg is order-sensitive in ways
// that are easy to get wrong — an input option after `-i` applies to the *next* input,
// and a filter before its input is a parse error.
//
//	global → per-input (opts then -i) → filter → per-output → output target

// Encoder is a video encoder ffmpeg can use. Not an exhaustive list of what ffmpeg
// supports — only what playout offers, which is H.264 in software or via one of the
// eight hardware families below (§15 `playout.encoder`).
type Encoder string

// The families ErsatzTV maintains pipelines for, which is the breadth real deployments
// need: NVIDIA, Intel, AMD on both OSes, Apple Silicon, and ARM SBCs. Availability is never
// assumed from this list — `Detect` asks the local ffmpeg build and then tries each one.
const (
	EncoderSoftware     Encoder = "libx264"           // always available
	EncoderNVENC        Encoder = "h264_nvenc"        // NVIDIA
	EncoderQSV          Encoder = "h264_qsv"          // Intel Quick Sync
	EncoderVAAPI        Encoder = "h264_vaapi"        // Intel AND AMD on Linux
	EncoderAMF          Encoder = "h264_amf"          // AMD on Windows
	EncoderVideoToolbox Encoder = "h264_videotoolbox" // Apple Silicon / Intel Macs
	EncoderRKMPP        Encoder = "h264_rkmpp"        // Rockchip SBCs
	EncoderV4L2M2M      Encoder = "h264_v4l2m2m"      // Raspberry Pi, V4L2 stateful encoders
	EncoderVulkan       Encoder = "h264_vulkan"       // cross-vendor, newer
)

// HEVC encoder variants (§9.1 V49). When an HEVC-capable client's session (EncodePlan hevc8/10)
// must TRANSCODE a non-HEVC program (a VP9/h264/mpeg2 commercial), it transcodes to HEVC — not h264 —
// so the fMP4 stream the browser plays stays uniformly one codec. (fMP4/MSE binds ONE decoder from
// its init segment and cannot survive a mid-stream codec change; mixing h264 into an HEVC fMP4 is a
// black frame.) Each is the hevc sibling of the h264 encoder above, on the same hardware engine, so
// it shares that family's preset vocabulary — see hevcVariant and the family-keyed videoEncodeArgs.
// ⚠ TYPED `Encoder`, not bare strings. They were untyped constants until 2026-08-09, and that is
// the mechanical reason nine encoders silently skipped every `switch enc Encoder` in capability.go:
// an untyped constant satisfies `Encoder` where one is expected, so nothing ever failed to compile
// — it just never matched a case either. Typing them does not by itself fix a missing case, but it
// makes the omission the kind of thing a reader and a linter can see.
const (
	EncoderSoftwareHEVC Encoder = "libx265"
	EncoderNVENCHEVC    Encoder = "hevc_nvenc"
	EncoderQSVHEVC      Encoder = "hevc_qsv"
	EncoderVAAPIHEVC    Encoder = "hevc_vaapi"
	EncoderAMFHEVC      Encoder = "hevc_amf"
	EncoderVTHEVC       Encoder = "hevc_videotoolbox"
	EncoderRKMPPHEVC    Encoder = "hevc_rkmpp"
	EncoderV4L2M2MHEVC  Encoder = "hevc_v4l2m2m"
	EncoderVulkanHEVC   Encoder = "hevc_vulkan"
)

// h264Engines is every encoder that NAMES a hardware engine — the canonical member of each
// engine's h264/HEVC pair. It is the iteration source for engineOf below and, deliberately, the
// same list encoderPreference draws from.
var h264Engines = []Encoder{
	EncoderSoftware, EncoderNVENC, EncoderQSV, EncoderVAAPI, EncoderAMF,
	EncoderVideoToolbox, EncoderRKMPP, EncoderV4L2M2M, EncoderVulkan,
}

// engineOf normalizes an encoder to the h264 constant naming its HARDWARE ENGINE, so a switch over
// engines matches an HEVC variant exactly as it matches its h264 sibling.
//
// ⚠ **Why this exists, and why it is DERIVED rather than written out.** Three functions in
// capability.go — deviceInitArgs, hardwareUploadFilter, hardwareDecodeArgs — key on the raw encoder
// value, and each listed only the h264 constants. So every hevc_* encoder fell to `default` and got
// `deviceInit=[] hwdec=[] upload=""`: no `-vaapi_device`, no `-init_hw_device` for QSV/Vulkan, no
// `-hwaccel cuda` for NVENC. The consequence was not a clean failure but a WORSE one — the
// hardware encode produced nothing, the ladder fell through to libx264, and for an HEVC fMP4
// session that mid-stream codec change is the black frame the plan exists to prevent.
//
// familyOf is NOT the right tool here and the distinction matters: it collapses VAAPI, Vulkan,
// VideoToolbox, RKMPP and V4L2M2M into `familyOther` because they share a *preset vocabulary*.
// These three functions need them kept APART, because they differ in exactly the thing being
// selected — the device, the upload filter, the decode flag.
//
// The map is built from hevcVariant rather than hand-written, so the pair list has one home. A
// tenth engine added there is normalized here with no second edit — which is the property the
// original three switches lacked.
var engineByEncoder = func() map[Encoder]Encoder {
	m := make(map[Encoder]Encoder, len(h264Engines)*2)
	for _, h := range h264Engines {
		m[h] = h
		m[hevcVariant(h)] = h
	}
	return m
}()

func engineOf(e Encoder) Encoder {
	if base, ok := engineByEncoder[e]; ok {
		return base
	}
	return e
}

// hevcVariant maps an h264 encoder to its HEVC sibling on the same hardware engine (§9.1 V49). The
// caller uses this when an hevc-plan session must transcode a non-HEVC program to keep the fMP4
// stream uniform. Returns the input unchanged if it has no known HEVC sibling — a safe degrade to
// h264 (the program still plays; only the fMP4-uniformity optimisation is lost for that encoder).
// IsSoftwareEncoder reports whether enc is either supported software codec. Treating libx265 as a
// hardware encoder would incorrectly acquire a GPU slot and attempt VRAM reclamation.
func IsSoftwareEncoder(enc Encoder) bool {
	return enc == EncoderSoftware || enc == EncoderSoftwareHEVC
}

func hevcVariant(h264 Encoder) Encoder {
	switch h264 {
	case EncoderSoftware:
		return EncoderSoftwareHEVC
	case EncoderNVENC:
		return EncoderNVENCHEVC
	case EncoderQSV:
		return EncoderQSVHEVC
	case EncoderVAAPI:
		return EncoderVAAPIHEVC
	case EncoderAMF:
		return EncoderAMFHEVC
	case EncoderVideoToolbox:
		return EncoderVTHEVC
	case EncoderRKMPP:
		return EncoderRKMPPHEVC
	case EncoderV4L2M2M:
		return EncoderV4L2M2MHEVC
	case EncoderVulkan:
		return EncoderVulkanHEVC
	default:
		return h264
	}
}

// Profile is the operator-facing output target: a ladder rung's size, frame rate and bitrates on
// one encoder. ChannelOutput turns it into the OutputProfile the live pipeline (Build) encodes to.
type Profile struct {
	Width     int
	Height    int
	Framerate int
	// VideoBitrate in kbit/s. 0 = let the encoder choose (CRF-ish for software).
	VideoBitrate int
	Encoder      Encoder
	// AudioBitrate in kbit/s; audio is always AAC stereo 48kHz. Fixed deliberately:
	// a varying audio layout across programs breaks `-c copy` exactly like video does,
	// and ErsatzTV's comment about ac3 downmix reinit failures is the warning
	// (prior-art §5).
	AudioBitrate int
}

// DefaultProfile is a conservative 720p/25 H.264 target. Chosen for compatibility over
// quality: this is what a media server will remux to whatever the client wants, so the
// job here is to be universally decodable, not to look best.
func DefaultProfile() Profile {
	return Profile{
		Width: 1280, Height: 720, Framerate: 25,
		VideoBitrate: 4000, Encoder: EncoderSoftware, AudioBitrate: 128,
	}
}

// MaxGainDB bounds a static per-item gain. Clips are normalised to the target at ingest, so a
// real correction is a fraction of a dB; anything past this is a bad measurement, and applying it
// would either clip (boost) or mute (cut) a clip on the strength of a number nobody checked.
const MaxGainDB = 6.0

// StaticGainDB is the constant gain that moves a clip measured at measuredLUFS to targetLUFS,
// clamped to ±MaxGainDB.
func StaticGainDB(targetLUFS, measuredLUFS float64) float64 {
	return math.Max(-MaxGainDB, math.Min(MaxGainDB, targetLUFS-measuredLUFS))
}

// FillerGain is the static loudness gain an airing gets (#1512 G6): FILLER ONLY, from the loudness
// measured at ingest against the live `filler.target_lufs`. A library title is never adjusted
// (advert loudness would flatten a film's dynamic range) and gets 0 dB with no note. A filler clip
// that cannot be corrected airs at 0 dB, and note says why, for the caller to log.
//
// ⚠ `a.Source` is the discriminator: set for a resolved filler clip, empty for a library title.
// An empty target means no gain.
func FillerGain(a Airing, targetLUFS string) (gainDB float64, note string) {
	if a.Source == "" || targetLUFS == "" {
		return 0, ""
	}
	target, err := strconv.ParseFloat(targetLUFS, 64)
	switch {
	case err != nil:
		return 0, "filler.target_lufs is not a number"
	case a.MeasuredLUFS == nil:
		return 0, "filler clip has no ingest loudness measurement"
	}
	return StaticGainDB(target, *a.MeasuredLUFS), ""
}

// gainFilter is the constant-gain audio filter for a filler clip.
//
// ⚠ **A static `volume`, never `loudnorm`.** Single-pass `loudnorm` re-estimates its gain from the
// first samples of every clip, so each break opened with an audible swell (#1512 G6) and the
// filter cost a real-time analysis on the live path. The clip was already measured at ingest
// (`DerivativeQC.Loudness`); the correction is one number, applied identically from the first
// sample. It never rewrites the file on disk.
func gainFilter(gainDB float64) string {
	return "volume=" + strconv.FormatFloat(gainDB, 'f', -1, 64) + "dB"
}
