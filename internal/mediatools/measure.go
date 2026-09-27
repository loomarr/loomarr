package mediatools

import (
	"math"
	"regexp"
	"strconv"
)

// Windowed measurement helpers (beta.8 G7). Loomarr never decodes a whole file to measure it: it
// seeks (through the container index) to a short window and reads only that. Each helper returns
// ffmpeg arguments; the caller runs them and hands ffmpeg's stderr to the matching parser. Spans
// the detectors report are relative to the window start.

func windowArgs(file string, startMs, lenMs int64) []string {
	return []string{"-nostdin", "-hide_banner", "-nostats", "-v", "info",
		"-ss", msToSeconds(startMs), "-t", msToSeconds(lenMs), "-i", file}
}

// LoudnessWindowArgs reads lenMs of the first audio stream from startMs and measures EBU R128
// integrated loudness and true peak. Video is never demuxed into a decoder.
func LoudnessWindowArgs(file string, startMs, lenMs int64) []string {
	return append(windowArgs(file, startMs, lenMs), "-vn", "-map", "0:a:0",
		"-af", "asetpts=PTS-STARTPTS,ebur128=peak=true:framelog=quiet", "-f", "null", "-")
}

// SilenceWindowArgs reads lenMs of audio from startMs and reports silent spans with the same
// threshold the filler detectors use.
func SilenceWindowArgs(file string, startMs, lenMs int64) []string {
	return append(windowArgs(file, startMs, lenMs), "-vn", "-map", "0:a:0",
		"-af", "asetpts=PTS-STARTPTS,"+qualityAudioFilter, "-f", "null", "-")
}

// BlackWindowArgs decodes lenMs of the first video stream from startMs, downscaled, and reports
// black spans with the same threshold the filler detectors use. It is meant for a second or two
// around a silence point, not for scanning.
func BlackWindowArgs(file string, startMs, lenMs int64) []string {
	return append(windowArgs(file, startMs, lenMs), "-an", "-map", "0:v:0",
		"-vf", measureVideoPrefix+qualityBlackFilter, "-f", "null", "-")
}

// measureVideoPrefix shrinks frames before detection: blackdetect only needs luma statistics.
const measureVideoPrefix = "scale=w=160:h=-2:flags=fast_bilinear,setpts=PTS-STARTPTS,"

// ParseWindowSpans returns the black and silent spans a window run reported, clamped to lenMs.
func ParseWindowSpans(stderr string, lenMs int64) (black, silence []Interval) {
	q := qualityFromDetectorOutput(stderr, lenMs)
	return q.Black, q.Silence
}

// ParseLoudnessSummary reads the EBU R128 summary a LoudnessWindowArgs run printed.
func ParseLoudnessSummary(stderr string) (ConditioningLoudness, error) {
	return parseConditioningLoudness(stderr)
}

// ChapterFadeArgs decodes the one second centred on a chapter mark: each video frame's mean luma
// (signalstats YAVG on 8-bit limited-range YUV, so black reads about 16 whatever the source bit depth, printed
// to stdout) and the audio's mean level (volumedetect, on stderr). One short read tells whether
// the mark sits in a fade: black on both sides and near-silent.
func ChapterFadeArgs(file string, atMs int64) []string {
	start := max(0, atMs-500)
	return append(windowArgs(file, start, 1000), "-map", "0:v:0", "-map", "0:a:0",
		"-vf", measureVideoPrefix+"format=yuv420p,signalstats,metadata=print:key=lavfi.signalstats.YAVG:file=-",
		"-af", "volumedetect", "-f", "null", "-")
}

var (
	yavgLine       = regexp.MustCompile(`lavfi\.signalstats\.YAVG=([0-9.]+)`)
	meanVolumeLine = regexp.MustCompile(`mean_volume:\s*(-?[0-9.]+|-inf) dB`)
)

// ParseChapterFade reads a ChapterFadeArgs run: the first and last frame's YAVG (half a second
// before and after the mark) and the audio's mean volume in dBFS. ok is false when either the
// frames or the audio level are missing, so an unreadable mark is never taken for a fade.
func ParseChapterFade(stdout, stderr string) (firstY, lastY, meanDB float64, ok bool) {
	frames := yavgLine.FindAllStringSubmatch(stdout, -1)
	level := meanVolumeLine.FindStringSubmatch(stderr)
	if len(frames) == 0 || level == nil {
		return 0, 0, 0, false
	}
	firstY, err1 := strconv.ParseFloat(frames[0][1], 64)
	lastY, err2 := strconv.ParseFloat(frames[len(frames)-1][1], 64)
	if err1 != nil || err2 != nil {
		return 0, 0, 0, false
	}
	meanDB = math.Inf(-1)
	if level[1] != "-inf" {
		if meanDB, err1 = strconv.ParseFloat(level[1], 64); err1 != nil {
			return 0, 0, 0, false
		}
	}
	return firstY, lastY, meanDB, true
}
