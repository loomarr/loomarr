package mediatools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sort"
)

const (
	qualityVideoFilters = "blackdetect=d=0.1:pix_th=0.20,freezedetect=n=-60dB:d=2"
	qualityAudioFilter  = "silencedetect=n=-35dB:d=0.3"
)

// InspectQuality measures black, content-silent and frozen spans in one decode. It exists for
// mezzanines produced before quality facts rode the transcode pass; new ingest gets the same facts
// from Transcode without paying for a second decode.
func InspectQuality(ctx context.Context, ffmpegPath, file string, durationMs int64, hadAudio bool) (MediaQuality, error) {
	args := []string{"-nostdin", "-hide_banner", "-nostats", "-v", "info", "-i", file,
		"-vf", qualityVideoFilters}
	if hadAudio {
		args = append(args, "-af", qualityAudioFilter)
	} else {
		args = append(args, "-an")
	}
	args = append(args, "-f", "null", "-")

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, FFmpegOr(ffmpegPath), args...)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return MediaQuality{}, fmt.Errorf("inspect media quality %s: %w: %s", file, err, stderr.String())
	}
	return qualityFromDetectorOutput(stderr.String(), durationMs), nil
}

func qualityFromDetectorOutput(stderr string, durationMs int64) MediaQuality {
	return MediaQuality{
		DurationMs: durationMs,
		Black:      normaliseIntervals(parseBlackdetect(stderr), durationMs),
		Silence:    normaliseIntervals(parseSilencedetect(stderr), durationMs),
		Freeze:     normaliseIntervals(parseFreezedetect(stderr), durationMs),
	}
}

// normaliseIntervals closes EOF spans, clamps detector rounding to the measured duration, and
// unions overlaps. Coverage is derived from this shape, so it must never exceed 100% merely
// because ffmpeg emitted adjacent or overlapping events.
func normaliseIntervals(in []Interval, durationMs int64) []Interval {
	if durationMs <= 0 || len(in) == 0 {
		return nil
	}
	clean := make([]Interval, 0, len(in))
	for _, span := range in {
		start, end := span.StartMs, span.EndMs
		if start < 0 {
			start = 0
		}
		if start >= durationMs {
			continue
		}
		if end <= start || end > durationMs {
			end = durationMs
		}
		if end > start {
			clean = append(clean, Interval{StartMs: start, EndMs: end})
		}
	}
	if len(clean) == 0 {
		return nil
	}
	sort.Slice(clean, func(i, j int) bool {
		if clean[i].StartMs == clean[j].StartMs {
			return clean[i].EndMs < clean[j].EndMs
		}
		return clean[i].StartMs < clean[j].StartMs
	})
	out := []Interval{clean[0]}
	for _, span := range clean[1:] {
		last := &out[len(out)-1]
		if span.StartMs <= last.EndMs {
			if span.EndMs > last.EndMs {
				last.EndMs = span.EndMs
			}
			continue
		}
		out = append(out, span)
	}
	return out
}
