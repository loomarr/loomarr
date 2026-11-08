package scan

import (
	"bufio"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// externalTrack is a subtitle or audio file beside a media file, as its name describes it
// ("Film (2001).en.forced.srt"): never opened (decision 61).
type externalTrack struct {
	Ref             string `json:"ref"`
	Kind            string `json:"kind"`
	Codec           string `json:"codec,omitempty"`
	Language        string `json:"language,omitempty"`
	Default         bool   `json:"default,omitempty"`
	Forced          bool   `json:"forced,omitempty"`
	HearingImpaired bool   `json:"hearingImpaired,omitempty"`
	Commentary      bool   `json:"commentary,omitempty"`
}

// externalTracks is the file's external tracks, by path. The words between the media file's name
// and the extension say the language (the first a language code names) and the flags: "forced",
// "default", "sdh", "cc" or "hi" (for the deaf and hard of hearing), and "commentary".
func (f File) externalTracks() []externalTrack {
	stem := strings.ToLower(strings.TrimSuffix(filepath.Base(f.Path), filepath.Ext(f.Path)))
	var out []externalTrack
	for path := range f.Sidecars {
		kind := externalTrackKind(path)
		if kind == "" {
			continue
		}
		ext := filepath.Ext(path)
		t := externalTrack{Ref: path, Kind: kind, Codec: externalCodecs[strings.ToLower(ext)][1]}
		base := strings.ToLower(strings.TrimSuffix(filepath.Base(path), ext))
		for _, word := range strings.Split(strings.TrimPrefix(base, stem), ".") {
			switch word {
			case "":
			case "forced":
				t.Forced = true
			case "default":
				t.Default = true
			case "sdh", "cc", "hi":
				t.HearingImpaired = true
			case "commentary":
				t.Commentary = true
			default:
				if t.Language == "" {
					if code, name := languageLabel(word); name != "" {
						t.Language = code
					}
				}
			}
		}
		out = append(out, t)
	}
	slices.SortFunc(out, func(a, b externalTrack) int { return strings.Compare(a.Ref, b.Ref) })
	return out
}

// marker is one span of an .edl file, in milliseconds; Kind names its action.
type marker struct {
	StartMs int64  `json:"startMs"`
	EndMs   int64  `json:"endMs"`
	Kind    string `json:"kind"`
}

// edlActions names an .edl line's action, as MPlayer and Kodi read it; a line without one is a cut.
var edlActions = map[string]string{"0": "cut", "1": "mute", "2": "scene", "3": "commercial"}

// maxEDLBytes bounds an .edl read: a real one is a few lines.
const maxEDLBytes = 64 << 10

// readEDL reads an .edl file's spans: "start end [action]" a line, in seconds. A line that does
// not parse, or a span that ends before it starts, is skipped; a file that cannot be read has none.
func readEDL(path string) []marker {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	var out []marker
	lines := bufio.NewScanner(io.LimitReader(f, maxEDLBytes))
	for lines.Scan() {
		fields := strings.Fields(lines.Text())
		if len(fields) < 2 {
			continue
		}
		start, errStart := strconv.ParseFloat(fields[0], 64)
		end, errEnd := strconv.ParseFloat(fields[1], 64)
		if errStart != nil || errEnd != nil || start < 0 || end < start || math.IsInf(end, 0) {
			continue
		}
		kind := "cut"
		if len(fields) > 2 {
			if kind = edlActions[fields[2]]; kind == "" {
				continue
			}
		}
		out = append(out, marker{StartMs: int64(math.Round(start * 1000)), EndMs: int64(math.Round(end * 1000)), Kind: kind})
	}
	return out
}
