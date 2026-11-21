package scan

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/loomarr/loomarr/internal/library/metadataprovider/localart"
)

// isNFO reports whether name is an NFO, Kodi's metadata sidecar.
func isNFO(name string) bool { return strings.EqualFold(filepath.Ext(name), ".nfo") }

// isTrickplay reports whether name is a trickplay file: a Roku BIF of seek thumbnails, which Emby
// writes beside the video as "<name>.bif" (decision 61).
func isTrickplay(name string) bool { return strings.EqualFold(filepath.Ext(name), ".bif") }

// isEDL reports whether name is an edit decision list: the spans a player skips or mutes, or marks
// as scenes or commercial breaks (decision 61).
func isEDL(name string) bool { return strings.EqualFold(filepath.Ext(name), ".edl") }

// externalCodecs are the external track files the walk notes, by extension: each one's kind and
// codec as ffprobe names it ("" where the extension does not say).
var externalCodecs = map[string][2]string{
	".srt": {"subtitle", "subrip"}, ".ass": {"subtitle", "ass"}, ".ssa": {"subtitle", "ssa"},
	".vtt": {"subtitle", "webvtt"}, ".sup": {"subtitle", "hdmv_pgs_subtitle"}, ".sub": {"subtitle", "dvd_subtitle"},
	".mka": {"audio", ""}, ".ac3": {"audio", "ac3"}, ".eac3": {"audio", "eac3"}, ".dts": {"audio", "dts"},
	".aac": {"audio", "aac"}, ".flac": {"audio", "flac"},
}

// externalTrackKind is "subtitle" or "audio" for an external track file, "" for anything else.
func externalTrackKind(name string) string {
	return externalCodecs[strings.ToLower(filepath.Ext(name))][0]
}

// edl is the file's .edl, "" for none.
func (f File) edl() string {
	for path := range f.Sidecars {
		if isEDL(path) {
			return path
		}
	}
	return ""
}

// trickplay is the file's trickplay files, by path, in order.
func (f File) trickplay() []string {
	var out []string
	for path := range f.Sidecars {
		if isTrickplay(path) {
			out = append(out, path)
		}
	}
	slices.Sort(out)
	return out
}

// hasNFO reports whether the file has an NFO to read.
func (f File) hasNFO() bool {
	for path := range f.Sidecars {
		if isNFO(path) {
			return true
		}
	}
	return false
}

// art is the file's artwork sidecars, by name.
func (f File) art() []string {
	var names []string
	for path := range f.Sidecars {
		if localart.IsImage(path) {
			names = append(names, filepath.Base(path))
		}
	}
	return names
}
