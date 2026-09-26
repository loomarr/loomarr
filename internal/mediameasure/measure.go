// Package mediameasure is Loomarr's own measurement of a media source (beta.8 G7): the keyframe
// index, loudness and natural break candidates that playout, the packager and the scheduler need,
// measured once per source revision with Loomarr's ffprobe/ffmpeg so nothing is asked of the media
// server, or re-probed, at airtime.
package mediameasure

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/loomarr/loomarr/internal/inventory"
	"github.com/loomarr/loomarr/internal/mediatools"
)

// Runner runs one external tool to completion and returns its stdout and stderr. It is the seam
// the job runs every ffprobe/ffmpeg through, so the composition root can supply the low-priority
// background runner filler uses and tests can supply a fake.
type Runner func(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)

// Tools are the binaries and the runner one measurement uses.
type Tools struct {
	FFmpeg, FFprobe string
	Run             Runner
}

// DefaultTools resolves tools from PATH (or the given paths) and runs them with os/exec.
func DefaultTools(ffmpeg, ffprobe string) Tools {
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	if ffprobe == "" {
		ffprobe = "ffprobe"
	}
	return Tools{FFmpeg: ffmpeg, FFprobe: ffprobe, Run: execRunner}
}

// execRunner runs the tool at background priority. It is the interim equivalent of filler's bgexec
// runner (#1514); the composition root can swap it by setting Tools.Run.
func execRunner(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // operator-resolved media tools
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := lowPriority(cmd)
	if err == nil {
		err = cmd.Wait()
	}
	return stdout.Bytes(), stderr.Bytes(), err
}

// scanKeyframes indexes every video sync packet with ffprobe. It demuxes the WHOLE file, so it is
// only used on files small enough that reading them all is cheap.
func (t Tools) scanKeyframes(ctx context.Context, path string) ([]inventory.Keyframe, error) {
	stdout, stderr, err := t.Run(ctx, t.FFprobe, "-v", "error", "-select_streams", "v:0",
		"-show_entries", "packet=pts_time,pos,flags", "-of", "csv=p=0", path)
	if err != nil {
		return nil, fmt.Errorf("keyframe index: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	return parseKeyframes(string(stdout)), nil
}

func parseKeyframes(csv string) []inventory.Keyframe {
	var frames []inventory.Keyframe
	for _, line := range strings.Split(csv, "\n") {
		fields := strings.Split(strings.TrimSpace(line), ",")
		if len(fields) < 3 || !strings.Contains(fields[2], "K") {
			continue
		}
		pts, err := strconv.ParseFloat(fields[0], 64)
		if err != nil || math.IsNaN(pts) || pts < 0 {
			continue
		}
		offset := int64(-1)
		if pos, err := strconv.ParseInt(fields[1], 10, 64); err == nil && pos >= 0 {
			offset = pos
		}
		frames = append(frames, inventory.Keyframe{PTSMs: int64(math.Round(pts * 1000)), Offset: offset})
	}
	sort.SliceStable(frames, func(i, j int) bool { return frames[i].PTSMs < frames[j].PTSMs })
	return frames
}

// Decode runs the one full-decode pass: black and silent spans plus EBU R128 integrated loudness
// and true peak. durationMs bounds the returned spans.
func (t Tools) Decode(ctx context.Context, path string, durationMs int64, hasVideo, hasAudio bool) (mediatools.MediaQuality, mediatools.ConditioningLoudness, error) {
	args := append([]string{"-threads", "2", "-filter_threads", "2"}, mediatools.DecodeMeasurementArgs(path, hasVideo, hasAudio)...)
	_, stderr, err := t.Run(ctx, t.FFmpeg, args...)
	if err != nil {
		return mediatools.MediaQuality{}, mediatools.ConditioningLoudness{}, fmt.Errorf("decode measurement: %w: %s", err, tail(stderr))
	}
	return mediatools.ParseDecodeMeasurement(string(stderr), durationMs)
}

func tail(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 400 {
		s = s[len(s)-400:]
	}
	return s
}

const (
	// minCoincidenceMs is the shortest black+silence overlap worth reporting.
	minCoincidenceMs = 100
	// fullConfidenceMs is the overlap at which a coincidence is a certain scene fade.
	fullConfidenceMs = 1000
	// edgeMarginMs drops fades touching the opening or closing seconds: they are the programme's
	// own fade-in/out, not a place to put a break.
	edgeMarginMs = 2000
)

// BreakCandidates finds the natural breaks: spans where black video and silent audio coincide, so
// a break never lands mid-scene. Either detector alone is not enough (a dark scene, a quiet scene).
// keyframes may be empty; KeyframeMs is then 0.
func BreakCandidates(black, silence []mediatools.Interval, keyframes []inventory.Keyframe, durationMs int64) []inventory.Break {
	var out []inventory.Break
	for _, b := range black {
		for _, s := range silence {
			start, end := max(b.StartMs, s.StartMs), min(b.EndMs, s.EndMs)
			overlap := end - start
			if overlap < minCoincidenceMs || start < edgeMarginMs || end > durationMs-edgeMarginMs {
				continue
			}
			at := start + overlap/2
			out = append(out, inventory.Break{
				AtMs: at, KeyframeMs: keyframeAtOrAfter(keyframes, at), OverlapMs: overlap,
				Confidence: math.Min(1, float64(overlap)/fullConfidenceMs),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AtMs < out[j].AtMs })
	return out
}

func keyframeAtOrAfter(frames []inventory.Keyframe, atMs int64) int64 {
	i := sort.Search(len(frames), func(i int) bool { return frames[i].PTSMs >= atMs })
	if i == len(frames) {
		return 0
	}
	return frames[i].PTSMs
}

// scanLimitBytes is the largest file that may be scanned packet by packet when the container has
// no index Loomarr can read. Bigger files (remuxes over a network mount) get no keyframe index
// rather than a full read.
const scanLimitBytes = 1 << 30

// Keyframes returns the video keyframe index, read from the container's own index (Matroska Cues)
// so a large file over a network mount costs kilobytes. A file whose index Loomarr cannot read is
// scanned only up to scanLimitBytes; above that the result is empty and no bytes are wasted.
func (t Tools) Keyframes(ctx context.Context, path string) ([]inventory.Keyframe, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("keyframe index: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("keyframe index: %w", err)
	}
	frames, ok, err := matroskaKeyframes(f, info.Size())
	_ = f.Close()
	if err != nil {
		return nil, err
	}
	if ok {
		return frames, nil
	}
	if info.Size() > scanLimitBytes {
		return nil, nil
	}
	return t.scanKeyframes(ctx, path)
}
