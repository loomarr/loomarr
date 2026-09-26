package mediameasure

import (
	"context"
	"regexp"
	"strconv"
	"time"
)

// The active picture (#1512 phase 1d): the part of the coded frame that is picture, with baked-in
// letterbox or pillarbox bars excluded, so the channel watermark anchors to the picture's corner
// instead of sitting in a black bar.
//
// Sampled, never a whole-file decode: cropdetect runs on a dozen frames at each of a few points
// spread through the programme, and the boxes are united. One dark scene alone would crop into the
// picture; the union across scenes is the widest area any of them lit.

// ActiveSampling is how many points, and frames at each, the measurement decodes.
type ActiveSampling struct {
	Points, Frames int
}

// DefaultActiveSampling decodes 5 × 12 frames.
func DefaultActiveSampling() ActiveSampling { return ActiveSampling{Points: 5, Frames: 12} }

// Box is a rectangle in source pixels.
type Box struct{ X, Y, W, H int }

var cropLine = regexp.MustCompile(`crop=(\d+):(\d+):(\d+):(\d+)`)

// ActivePicture measures the active picture of a width×height video, or ok=false when no sample
// produced a box (no video, every sample failed, or every sampled frame was black).
func (t Tools) ActivePicture(ctx context.Context, path string, durationMs int64, width, height int, s ActiveSampling) (Box, bool) {
	if durationMs <= 0 || width <= 0 || height <= 0 || s.Points <= 0 {
		return Box{}, false
	}
	var union Box
	found := false
	for i := 0; i < s.Points; i++ {
		at := time.Duration((float64(i)+1)/float64(s.Points+1)*float64(durationMs)) * time.Millisecond
		_, stderr, err := t.Run(ctx, t.FFmpeg, "-hide_banner", "-nostdin", "-ss", strconv.FormatFloat(at.Seconds(), 'f', 3, 64),
			"-i", path, "-map", "0:v:0", "-frames:v", strconv.Itoa(s.Frames),
			"-vf", "cropdetect=limit=24:round=2:reset=0", "-f", "null", "-")
		if ctx.Err() != nil {
			return Box{}, false
		}
		if err != nil {
			continue
		}
		b, ok := lastCrop(string(stderr), width, height)
		if !ok {
			continue
		}
		if !found {
			union, found = b, true
			continue
		}
		x0, y0 := min(union.X, b.X), min(union.Y, b.Y)
		x1, y1 := max(union.X+union.W, b.X+b.W), max(union.Y+union.H, b.Y+b.H)
		union = Box{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
	}
	return union, found
}

// lastCrop is cropdetect's final box for a sample: its running answer after every frame. A box
// that is empty or outside the frame (cropdetect reports a negative-size box on an all-black
// sample) is no answer.
func lastCrop(stderr string, width, height int) (Box, bool) {
	m := cropLine.FindAllStringSubmatch(stderr, -1)
	if len(m) == 0 {
		return Box{}, false
	}
	last := m[len(m)-1]
	v := make([]int, 4)
	for i := range v {
		v[i], _ = strconv.Atoi(last[i+1])
	}
	b := Box{W: v[0], H: v[1], X: v[2], Y: v[3]}
	if b.W <= 0 || b.H <= 0 || b.X+b.W > width || b.Y+b.H > height {
		return Box{}, false
	}
	return b, true
}
