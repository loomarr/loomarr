package scan

import (
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

// The labels a track is stored with, computed once from its raw header values (#2151 decision 44)
// so every reader shows and filters on the same words.

// resolutionLabel names a picture's resolution by its height, or by its width where the picture
// is wider than 16:9: a 1920×800 scope film is 1080p, a 3840×1600 one 4K. Heights round to the
// nearest standard, since encoders pad (1088) and crop (718).
func resolutionLabel(width, height int) string {
	h := max(height, width*9/16)
	switch {
	case h <= 0:
		return ""
	case h >= 2000:
		return "4K"
	case h >= 1000:
		return "1080p"
	case h >= 700:
		return "720p"
	case h >= 460:
		return "480p"
	case h >= 340:
		return "360p"
	default:
		return "240p"
	}
}

// hdrLabel names a picture's dynamic range: "Dolby Vision" for any Dolby Vision profile, whatever
// its base layer, then from its transfer "HDR10" for PQ, "HLG", "HDR" for one the header marks HDR
// another way, "" for SDR.
func hdrLabel(transfer string, hdr bool, dolbyVision int) string {
	if dolbyVision > 0 {
		return "Dolby Vision"
	}
	switch strings.ToLower(transfer) {
	case "smpte2084":
		return "HDR10"
	case "arib-std-b67":
		return "HLG"
	}
	if hdr {
		return "HDR"
	}
	return ""
}

// audioFormats names the audio codecs a household recognises; any other is its codec name,
// upper-cased.
var audioFormats = map[string]string{
	"aac": "AAC", "ac3": "Dolby Digital", "eac3": "Dolby Digital Plus", "truehd": "Dolby TrueHD",
	"dts": "DTS", "flac": "FLAC", "opus": "Opus", "mp3": "MP3", "mp2": "MP2", "vorbis": "Vorbis",
	"alac": "ALAC",
}

// audioFormatLabel names an audio track's format; a DTS track's profile says which DTS it is.
func audioFormatLabel(codec, profile string) string {
	codec = strings.ToLower(codec)
	if codec == "" {
		return ""
	}
	if codec == "dts" && profile != "" {
		switch strings.ToLower(profile) {
		case "dts-hd ma":
			return "DTS-HD MA"
		case "dts-hd hra":
			return "DTS-HD HRA"
		case "dts:x", "dts-hd ma + dts:x":
			return "DTS:X"
		}
	}
	if strings.HasPrefix(codec, "pcm_") {
		return "PCM"
	}
	if name, ok := audioFormats[codec]; ok {
		return name
	}
	return strings.ToUpper(codec)
}

// languageLabel is a track's language as an ISO 639-1 code where one exists ("eng" is "en"),
// and its English name. A code the file gives that names no language is kept as it is, unnamed;
// "und" and "" are no language.
func languageLabel(code string) (string, string) {
	code = strings.TrimSpace(code)
	if code == "" || strings.EqualFold(code, "und") {
		return "", ""
	}
	tag, err := language.Parse(code)
	if err != nil {
		return code, ""
	}
	base, conf := tag.Base()
	if conf == language.No {
		return code, ""
	}
	return base.String(), display.English.Languages().Name(base)
}
