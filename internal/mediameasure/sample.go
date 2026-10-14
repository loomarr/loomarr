package mediameasure

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/inventory"
	"github.com/loomarr/loomarr/internal/mediatools"
)

// Sampling bounds how much of a file measurement may read. Loomarr never decodes a whole file:
// loudness comes from a dozen short audio windows, breaks from the container's chapters or from
// a few targeted windows, and each is capped by a byte budget scaled to the file's bitrate so a
// 100 Mbps remux is never read at length just because it is long.
type Sampling struct {
	LoudnessWindows int           // windows spread through the file
	LoudnessWindow  time.Duration // full window length for a light file
	BreakEvery      time.Duration // a break is wanted about this often
	BreakHalfWindow time.Duration // search this far either side of each due point
	BudgetBytes     int64         // per file, for loudness and for the break search each
}

// DefaultSampling is about twelve 20 s loudness windows and a break search every quarter hour,
// each within 256 MiB of reads. The search window is inventory.BreakSearchHalfWindow, the same
// value mid-roll placement accepts candidates within.
func DefaultSampling() Sampling {
	return Sampling{LoudnessWindows: 12, LoudnessWindow: 20 * time.Second,
		BreakEvery: 15 * time.Minute, BreakHalfWindow: inventory.BreakSearchHalfWindow, BudgetBytes: 256 << 20}
}

const (
	minSampleWindow = time.Second
	// minBreakWindow is the shortest audio window in which silence means anything.
	minBreakWindow = 20 * time.Second
	blackConfirm   = time.Second
)

// sampleWindow shrinks a window so n of them stay inside budgetBytes at bytesPerSecond.
func sampleWindow(full time.Duration, n int, budgetBytes, bytesPerSecond int64) time.Duration {
	if n <= 0 || bytesPerSecond <= 0 {
		return full
	}
	fit := time.Duration(float64(budgetBytes) / float64(bytesPerSecond) / float64(n) * float64(time.Second))
	return max(minSampleWindow, min(full, fit))
}

func bytesPerSecond(sizeBytes, durationMs int64) int64 {
	if durationMs <= 0 {
		return 0
	}
	return sizeBytes * 1000 / durationMs
}

// SampleLoudness estimates integrated loudness and true peak from windows spread through the
// file, audio only. Window energies are averaged, ignoring near-silent windows, which is enough
// for a static gain.
func (t Tools) SampleLoudness(ctx context.Context, path string, durationMs, sizeBytes int64, s Sampling) (lufs, peak *float64, err error) {
	if durationMs <= 0 || s.LoudnessWindows <= 0 {
		return nil, nil, nil
	}
	window := sampleWindow(s.LoudnessWindow, s.LoudnessWindows, s.BudgetBytes, bytesPerSecond(sizeBytes, durationMs))
	winMs := window.Milliseconds()
	var power float64
	var used int
	var maxPeak *float64
	for i := 0; i < s.LoudnessWindows; i++ {
		// Window centres at (i+0.5)/N of the runtime, clamped inside the file.
		start := int64((float64(i)+0.5)/float64(s.LoudnessWindows)*float64(durationMs)) - winMs/2
		start = max(0, min(start, durationMs-winMs))
		_, stderr, runErr := t.Run(ctx, t.FFmpeg, mediatools.LoudnessWindowArgs(path, start, winMs)...)
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		if runErr != nil {
			continue
		}
		l, parseErr := mediatools.ParseLoudnessSummary(string(stderr))
		if parseErr != nil || !l.Available || l.IntegratedLUFS < -60 {
			continue
		}
		power += math.Pow(10, l.IntegratedLUFS/10)
		used++
		if l.TruePeak.State == mediatools.TruePeakFinite && (maxPeak == nil || l.TruePeak.DBTP > *maxPeak) {
			p := l.TruePeak.DBTP
			maxPeak = &p
		}
	}
	if used == 0 {
		return nil, nil, nil
	}
	estimate := 10 * math.Log10(power/float64(used))
	return &estimate, maxPeak, nil
}

const (
	// chapterBlackYAVG is the darkest a frame half a second either side of a chapter mark may be
	// and still count as black (8-bit limited range, where black is 16). Measured on remuxes: act
	// breaks read 16.0–20.6 on both sides; scene-selection chapters read 31.4 and brighter.
	chapterBlackYAVG = 24
	// chapterLoudDB rejects a mark over loud programme audio (mean over the second). It is a loose
	// guard, not the detectors' -35 dB silence floor: real act-break fades measured -28 to -37 dB
	// because a drama's act-out sting plays across the black, while a scene chapter in a loud
	// scene measured -22.4 dB. The picture is what separates a fade from a scene cut.
	chapterLoudDB = -25
	// chapterCheckSeconds is what one check is assumed to read: the second itself plus the seek to
	// the preceding keyframe.
	chapterCheckSeconds = 2
)

// ChapterBreaks turns the container's chapter marks into break candidates, but only marks that sit
// in a fade. A chapter mark is metadata, not a measurement: on a Blu-ray remux most are
// scene-selection points in the middle of the picture, and cutting there would break mid-scene.
// Each mark away from the edges gets one cheap check (ChapterFadeArgs: one second around it) and
// is kept only when chapterIsFade says so. Checks stop at the break-search byte budget, spread
// evenly over the marks. The caller runs the targeted fade search when no mark survives.
func (t Tools) ChapterBreaks(ctx context.Context, path string, durationMs, sizeBytes int64, keyframes []inventory.Keyframe, s Sampling) ([]inventory.Break, error) {
	stdout, stderr, err := t.Run(ctx, t.FFprobe, "-v", "error", "-show_entries", "chapter=start_time", "-of", "csv=p=0", path)
	if err != nil {
		return nil, fmt.Errorf("chapters: %w: %s", err, tail(stderr))
	}
	var marks []int64
	for _, line := range strings.Split(string(stdout), "\n") {
		seconds, perr := strconv.ParseFloat(strings.TrimSpace(line), 64)
		if perr != nil || math.IsNaN(seconds) {
			continue
		}
		at := int64(math.Round(seconds * 1000))
		if at < edgeMarginMs || at > durationMs-edgeMarginMs {
			continue
		}
		marks = append(marks, at)
	}
	stride := 1
	if perCheck := bytesPerSecond(sizeBytes, durationMs) * chapterCheckSeconds; perCheck > 0 && s.BudgetBytes > 0 {
		if affordable := max(1, int(s.BudgetBytes/perCheck)); affordable < len(marks) {
			stride = (len(marks) + affordable - 1) / affordable
		}
	}
	var out []inventory.Break
	for i := 0; i < len(marks); i += stride {
		at := marks[i]
		vOut, aOut, err := t.Run(ctx, t.FFmpeg, mediatools.ChapterFadeArgs(path, at)...)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			continue // an unreadable mark is not a fade
		}
		firstY, lastY, meanDB, ok := mediatools.ParseChapterFade(string(vOut), string(aOut))
		if !ok || !chapterIsFade(firstY, lastY, meanDB) {
			continue
		}
		// Verified black across the whole checked second: that is the measured overlap.
		out = append(out, inventory.Break{AtMs: at, KeyframeMs: keyframeAtOrAfter(keyframes, at), Source: "chapter",
			OverlapMs: fullConfidenceMs, Confidence: 1})
	}
	return out, nil
}

// chapterIsFade reports whether a chapter mark sits in a fade: black half a second before and after
// it, with no loud programme audio across it.
func chapterIsFade(firstY, lastY, meanDB float64) bool {
	return firstY <= chapterBlackYAVG && lastY <= chapterBlackYAVG && meanDB <= chapterLoudDB
}

// TargetedBreaks finds fades only where a break is due: for each due point (about every
// BreakEvery of runtime) it listens for silence in a window around it, then decodes a second of
// video at each silence to confirm black. Nothing else is read. When the byte budget cannot pay
// for a window long enough to mean anything, the file gets no fade breaks.
func (t Tools) TargetedBreaks(ctx context.Context, path string, durationMs, sizeBytes int64, keyframes []inventory.Keyframe, s Sampling) ([]inventory.Break, error) {
	every := s.BreakEvery.Milliseconds()
	if every <= 0 || durationMs <= 0 {
		return nil, nil
	}
	var due []int64
	for at := every; at < durationMs-edgeMarginMs; at += every {
		due = append(due, at)
	}
	if len(due) == 0 {
		return nil, nil
	}
	half := sampleWindow(s.BreakHalfWindow*2, len(due), s.BudgetBytes, bytesPerSecond(sizeBytes, durationMs)) / 2
	if 2*half < min(2*s.BreakHalfWindow, minBreakWindow) {
		// The byte budget shrank the window below what is worth listening to.
		return nil, nil
	}
	halfMs := half.Milliseconds()
	var out []inventory.Break
	for _, centre := range due {
		start := max(0, centre-halfMs)
		length := min(2*halfMs, durationMs-start)
		_, stderr, err := t.Run(ctx, t.FFmpeg, mediatools.SilenceWindowArgs(path, start, length)...)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			continue
		}
		_, silence := mediatools.ParseWindowSpans(string(stderr), length)
		for _, span := range longestFirst(silence, 3) {
			mid := start + (span.StartMs+span.EndMs)/2
			from := max(0, mid-blackConfirm.Milliseconds()/2)
			_, vErr, err := t.Run(ctx, t.FFmpeg, mediatools.BlackWindowArgs(path, from, blackConfirm.Milliseconds())...)
			if err != nil {
				continue
			}
			black, _ := mediatools.ParseWindowSpans(string(vErr), blackConfirm.Milliseconds())
			abs := make([]mediatools.Interval, 0, len(black))
			for _, b := range black {
				abs = append(abs, mediatools.Interval{StartMs: from + b.StartMs, EndMs: from + b.EndMs})
			}
			absSilence := mediatools.Interval{StartMs: start + span.StartMs, EndMs: start + span.EndMs}
			for _, b := range BreakCandidates(abs, []mediatools.Interval{absSilence}, keyframes, durationMs) {
				b.Source = "fade"
				out = append(out, b)
			}
		}
	}
	return out, nil
}

func longestFirst(spans []mediatools.Interval, n int) []mediatools.Interval {
	out := append([]mediatools.Interval(nil), spans...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].EndMs-out[j].StartMs > out[j-1].EndMs-out[j-1].StartMs; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out[:min(n, len(out))]
}
