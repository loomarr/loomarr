package playout

import (
	"strconv"

	"github.com/loomarr/loomarr/internal/encode"
	"github.com/loomarr/loomarr/internal/loudness"
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
//
// The encoder vocabulary (encode.Encoder, its engines and HEVC siblings) lives in internal/encode,
// shared with the filler pipeline (#2121).

// Profile is the operator-facing output target: a ladder rung's size, frame rate and bitrates on
// one encoder. ChannelOutput turns it into the OutputProfile the live pipeline (Build) encodes to.
type Profile struct {
	Width     int
	Height    int
	Framerate int
	// VideoBitrate in kbit/s. 0 = let the encoder choose (CRF-ish for software).
	VideoBitrate int
	Encoder      encode.Encoder
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
		VideoBitrate: 4000, Encoder: encode.EncoderSoftware, AudioBitrate: 128,
	}
}

// FillerGain is the static loudness gain an airing gets (#1512 G6): FILLER ONLY, from the loudness
// and true peak measured at ingest against the live `filler.target_lufs`, by the one correction
// rule in `loudness` (#2032). Files are never levelled on disk, so this is the whole correction;
// the old ±6 dB symmetric clamp assumed ingest had already normalised and left a -12 LUFS clip
// airing 5 dB hot. A library title is never adjusted
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
	gain, _ := loudness.Correction(target, *a.MeasuredLUFS, a.MeasuredTruePeak)
	return gain, ""
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
