package mediatools

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
