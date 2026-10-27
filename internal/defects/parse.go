package defects

import (
	"bufio"
	"maps"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Each summary is the line its filter logs once when the graph closes.
var (
	blockMeanLine = regexp.MustCompile(`\[Parsed_blockdetect_\d+ @ [^\]]+\] block mean: (\S+)`)
	blurMeanLine  = regexp.MustCompile(`\[Parsed_blurdetect_\d+ @ [^\]]+\] blur mean: (\S+)`)
	// idet logs its counters twice, at configuration (all zero) and at close; the last line wins.
	idetLine = regexp.MustCompile(`\[Parsed_idet_\d+ @ [^\]]+\] Multi frame detection: TFF:\s*(\d+) BFF:\s*(\d+) Progressive:\s*(\d+)`)
	ssimLine = regexp.MustCompile(`\[Parsed_ssim_(\d+) @ [^\]]+\] SSIM Y:(\S+)`)
	// astats measures overall only (measure_perchannel=none), so each key appears once.
	astatsLine = regexp.MustCompile(`\[Parsed_astats_\d+ @ [^\]]+\] (Peak level dB|Flat factor|Peak count|Noise floor dB): (\S+)`)
)

// ActiveArea is the rectangle of a frame that holds picture, as cropdetect found it.
type ActiveArea struct{ Width, Height, X, Y int }

// ParseActiveArea reads the active-area pass's per-frame cropdetect values. cropdetect with
// reset=0 widens its rectangle as the clip goes on, so the last frame's values are the union of
// every frame's picture. A clip with no picture found, or a rectangle that does not fit, is
// measured whole.
func ParseActiveArea(metadata string, width, height int) ActiveArea {
	var a ActiveArea
	sc := bufio.NewScanner(strings.NewReader(metadata))
	for sc.Scan() {
		key, value, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		switch key {
		case "lavfi.cropdetect.w":
			a.Width = atoi(value)
		case "lavfi.cropdetect.h":
			a.Height = atoi(value)
		case "lavfi.cropdetect.x":
			a.X = atoi(value)
		case "lavfi.cropdetect.y":
			a.Y = atoi(value)
		}
	}
	// minActive keeps a nearly black clip from being measured through a keyhole.
	const minActive = 32
	if a.Width < minActive || a.Height < minActive || a.X < 0 || a.Y < 0 || a.X+a.Width > width || a.Y+a.Height > height {
		return ActiveArea{Width: width, Height: height}
	}
	return a
}

// Parse reads one MeasureDefects pass: the filters' closing summaries from ffmpeg's log, and the
// per-frame values the metadata filter wrote. width and height are the decoded frame size; active
// is the picture inside it that the pass measured.
func Parse(log, metadata string, width, height int, active ActiveArea, hasAudio bool) Readings {
	r := Readings{Version: MeasurementVersion, Width: width, Height: height, HasAudio: hasAudio,
		ActiveWidth: active.Width, ActiveHeight: active.Height}
	r.BlockMean = lastFloat(blockMeanLine, log)
	r.BlurMean = lastFloat(blurMeanLine, log)
	// Two ssim filters, the half-size branch first in the graph and so first in filter order.
	ssim := map[int]float64{}
	for _, m := range ssimLine.FindAllStringSubmatch(log, -1) {
		ssim[atoi(m[1])] = finite(m[2])
	}
	if order := slices.Sorted(maps.Keys(ssim)); len(order) == 2 {
		r.UpscaleSSIM, r.UpscaleSSIM4 = ssim[order[0]], ssim[order[1]]
	}
	if m := idetLine.FindAllStringSubmatch(log, -1); len(m) > 0 {
		last := m[len(m)-1]
		tff, bff, prog := atoi(last[1]), atoi(last[2]), atoi(last[3])
		// ⚠ The lead of one field order over the other, not the interlaced share. Woven fields
		// keep one order frame after frame (a combed clip reads TFF 100, BFF 0). Fine detail in
		// fast motion fools idet too, but evenly both ways: a progressive test pattern read
		// TFF 40, BFF 38, which as a share is 78% "interlaced".
		if decided := tff + bff + prog; decided > 0 {
			r.InterlacedShare = math.Abs(float64(tff-bff)) / float64(decided)
		}
	}
	if hasAudio {
		for _, m := range astatsLine.FindAllStringSubmatch(log, -1) {
			v := finite(m[2])
			switch m[1] {
			case "Peak level dB":
				r.PeakDB = v
			case "Flat factor":
				r.FlatFactor = v
			case "Peak count":
				r.PeakCount = v
			case "Noise floor dB":
				r.NoiseFloorDB = v
			}
		}
	}

	var tout, vrep float64
	sc := bufio.NewScanner(strings.NewReader(metadata))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "frame:") {
			r.Frames++
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "lavfi.signalstats.TOUT":
			tout += finite(value)
		case "lavfi.signalstats.VREP":
			vrep += finite(value)
		}
	}
	if r.Frames > 0 {
		r.OutlierShare = tout / float64(r.Frames)
		r.RepeatedLineShare = vrep / float64(r.Frames)
	}
	return r
}

func lastFloat(re *regexp.Regexp, log string) float64 {
	m := re.FindAllStringSubmatch(log, -1)
	if len(m) == 0 {
		return 0
	}
	return finite(m[len(m)-1][1])
}

// finite parses a detector value. ffmpeg prints "-inf" for the level of a silent track and "nan"
// for a statistic with no samples; neither survives JSON, so both read as the floor of the scale.
func finite(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	switch {
	case err != nil || math.IsNaN(v):
		return 0
	case math.IsInf(v, -1):
		return -200
	case math.IsInf(v, 1):
		return 200
	}
	return v
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
