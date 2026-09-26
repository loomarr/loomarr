package mediatools

import "fmt"

// measureVideoPrefix shrinks frames before detection: blackdetect only needs luma statistics, and
// a 4K HEVC decode is the cost that matters when a whole library is measured in the background.
const measureVideoPrefix = "scale=w=320:h=-2:flags=fast_bilinear,setpts=PTS-STARTPTS,"

// DecodeMeasurementArgs is the one full-decode pass that yields the black and silent spans plus
// integrated loudness and true peak (EBU R128), with the same detector thresholds the filler
// pipeline uses. The caller runs it and passes ffmpeg's stderr to ParseDecodeMeasurement.
func DecodeMeasurementArgs(file string, hasVideo, hasAudio bool) []string {
	args := []string{"-nostdin", "-hide_banner", "-nostats", "-v", "info", "-i", file}
	if hasVideo {
		args = append(args, "-map", "0:v:0", "-vf", measureVideoPrefix+qualityBlackFilter)
	} else {
		args = append(args, "-vn")
	}
	if hasAudio {
		args = append(args, "-map", "0:a:0", "-af", "asetpts=PTS-STARTPTS,"+qualityAudioFilter+",ebur128=peak=true:framelog=quiet")
	} else {
		args = append(args, "-an")
	}
	return append(args, "-f", "null", "-")
}

// ParseDecodeMeasurement reads the black and silence spans and the loudness summary out of the
// stderr of a DecodeMeasurementArgs run. Loudness.Available is false when there was no audio.
func ParseDecodeMeasurement(stderr string, durationMs int64) (MediaQuality, ConditioningLoudness, error) {
	if durationMs <= 0 {
		return MediaQuality{}, ConditioningLoudness{}, fmt.Errorf("parse decode measurement: duration must be positive")
	}
	loudness, err := parseConditioningLoudness(stderr)
	if err != nil {
		return MediaQuality{}, ConditioningLoudness{}, err
	}
	return qualityFromDetectorOutput(stderr, durationMs), loudness, nil
}
