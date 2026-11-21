package tools

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Parsers for the split pipeline's tool output (§10 V34). All pure: the exec
// side is mediatools.go, and these are pinned against captured tool output in
// splitparse_test.go — the same fixture rule as the testkit (write parsers
// against captured truth, not remembered field names).

var (
	// [blackdetect @ 0x…] black_start:1.28 black_end:2.52 black_duration:1.24
	blackRe = regexp.MustCompile(`black_start:(-?\d+(?:\.\d+)?) black_end:(-?\d+(?:\.\d+)?)`)
	// [silencedetect @ 0x…] silence_start: 3.36
	silenceStartRe = regexp.MustCompile(`silence_start: (-?\d+(?:\.\d+)?)`)
	// [silencedetect @ 0x…] silence_end: 5.84 | silence_duration: 2.48
	silenceEndRe = regexp.MustCompile(`silence_end: (-?\d+(?:\.\d+)?)`)
	// [freezedetect @ 0x…] freeze_start: 4.2
	freezeStartRe = regexp.MustCompile(`freeze_start: (-?\d+(?:\.\d+)?)`)
	// [freezedetect @ 0x…] freeze_end: 15.8 | freeze_duration: 11.6
	freezeEndRe = regexp.MustCompile(`freeze_end: (-?\d+(?:\.\d+)?)`)
)

// boundaryGaps normalises detector output inside one seeked span and maps it back onto the file's
// absolute timeline. Closing an EOF silence at the span end is essential for chunking: the next
// span starts inside the same silence and boundaryCuts then unions the two touching intervals.
//
// The detectors run with short minimums (boundaryDetectMinSec) so that a broadcast gap of a few
// frames, or the tail of a fade, is seen at all; corroborated keeps a short gap only where the
// other detector agrees, so a pause in speech or one dark frame never becomes a cut.
func boundaryGaps(stderr string, startMs, endMs int64) ([]Interval, []Interval) {
	spanMs := endMs - startMs
	rawBlack := normaliseIntervals(parseBlackdetect(stderr), spanMs)
	rawSilence := normaliseIntervals(parseSilencedetect(stderr), spanMs)
	black := corroborated(rawBlack, rawSilence, minLoneBlackMs, spanMs)
	silence := corroborated(rawSilence, rawBlack, minLoneSilenceMs, spanMs)
	for _, gaps := range [][]Interval{black, silence} {
		for i := range gaps {
			gaps[i].StartMs += startMs
			gaps[i].EndMs += startMs
		}
	}
	return black, silence
}

const (
	// boundaryDetectMinSec is the shortest run either detector reports: one frame at 30 fps. A gap
	// between broadcast spots is often four frames, and a fade's black and silent tail is about as
	// short; at the old minimums (0.1 s black, 0.3 s silence) both went unseen (#4409, split corpus).
	boundaryDetectMinSec = "0.03"
	// minLoneBlackMs and minLoneSilenceMs are how long a run one detector alone saw must last to be
	// a gap: the minimums the detectors used before. Shorter runs are gaps only where both agree.
	minLoneBlackMs   = 100
	minLoneSilenceMs = 300
)

// corroborated keeps the runs that are long enough on their own, overlap a run of the other
// detector, or touch the span's edge (a chunk boundary can cut one short, so its length is unknown).
func corroborated(runs, other []Interval, loneMs, spanMs int64) []Interval {
	var out []Interval
	for _, r := range runs {
		keep := r.EndMs-r.StartMs >= loneMs || r.StartMs <= 0 || r.EndMs >= spanMs
		for _, o := range other {
			if keep {
				break
			}
			keep = o.StartMs < r.EndMs && r.StartMs < o.EndMs
		}
		if keep {
			out = append(out, r)
		}
	}
	return out
}

// parseBlackdetect extracts the black intervals ffmpeg reports on stderr. An
// advert break in a compilation is usually a fade to black between spots; at
// pix_th=0.20 (media/tools) these measured 81–100% of boundaries (plan §6.4).
func parseBlackdetect(stderr string) []Interval {
	var out []Interval
	for _, m := range blackRe.FindAllStringSubmatch(stderr, -1) {
		start, err1 := strconv.ParseFloat(m[1], 64)
		end, err2 := strconv.ParseFloat(m[2], 64)
		if err1 != nil || err2 != nil || end <= start {
			continue
		}
		out = append(out, Interval{StartMs: int64(start * 1000), EndMs: int64(end * 1000)})
	}
	return out
}

// parseSilencedetect pairs silence_start with the following silence_end. A
// trailing unclosed start (silence running to EOF) is closed at +∞ by the
// caller's duration clamp in boundaryCuts, so it is emitted with EndMs = start.
func parseSilencedetect(stderr string) []Interval {
	var out []Interval
	var open int64 = -1
	for _, line := range strings.Split(stderr, "\n") {
		if m := silenceStartRe.FindStringSubmatch(line); m != nil {
			if s, err := strconv.ParseFloat(m[1], 64); err == nil {
				open = int64(s * 1000)
			}
			continue
		}
		if m := silenceEndRe.FindStringSubmatch(line); m != nil && open >= 0 {
			if e, err := strconv.ParseFloat(m[1], 64); err == nil && int64(e*1000) > open {
				out = append(out, Interval{StartMs: open, EndMs: int64(e * 1000)})
			}
			open = -1
		}
	}
	if open >= 0 {
		out = append(out, Interval{StartMs: open, EndMs: open})
	}
	return out
}

// parseFreezedetect pairs freeze_start with freeze_end. Like silencedetect, ffmpeg may leave a
// span open when it reaches EOF; the quality normaliser closes it at the probed duration.
func parseFreezedetect(stderr string) []Interval {
	var out []Interval
	var open int64 = -1
	for _, line := range strings.Split(stderr, "\n") {
		if m := freezeStartRe.FindStringSubmatch(line); m != nil {
			if s, err := strconv.ParseFloat(m[1], 64); err == nil {
				open = int64(s * 1000)
			}
			continue
		}
		if m := freezeEndRe.FindStringSubmatch(line); m != nil && open >= 0 {
			if e, err := strconv.ParseFloat(m[1], 64); err == nil && int64(e*1000) > open {
				out = append(out, Interval{StartMs: open, EndMs: int64(e * 1000)})
			}
			open = -1
		}
	}
	if open >= 0 {
		out = append(out, Interval{StartMs: open, EndMs: open})
	}
	return out
}

// ffprobeChapters is `ffprobe -print_format json -show_chapters` (triage). Times
// arrive in time_base units — usually 1/1000 but NOT guaranteed, so they are
// scaled, never assumed.
type ffprobeChapters struct {
	Chapters []struct {
		TimeBase string `json:"time_base"`
		Start    int64  `json:"start"`
		End      int64  `json:"end"`
		Tags     struct {
			Title string `json:"title"`
		} `json:"tags"`
	} `json:"chapters"`
}

func parseFFprobeChapters(out []byte) ([]Chapter, error) {
	var p ffprobeChapters
	if err := json.Unmarshal(out, &p); err != nil {
		return nil, fmt.Errorf("ffprobe chapters not JSON: %w", err)
	}
	var chs []Chapter
	for _, c := range p.Chapters {
		num, den := 1, 1000 // ffprobe's usual 1/1000
		if c.TimeBase != "" {
			if _, err := fmt.Sscanf(c.TimeBase, "%d/%d", &num, &den); err != nil || den == 0 {
				return nil, fmt.Errorf("ffprobe chapter time_base %q unparsable", c.TimeBase)
			}
		}
		toMs := func(v int64) int64 { return v * int64(num) * 1000 / int64(den) }
		start, end := toMs(c.Start), toMs(c.End)
		if end <= start {
			continue
		}
		chs = append(chs, Chapter{StartMs: start, EndMs: end, Title: c.Tags.Title})
	}
	return chs, nil
}

// whisperJSON is `whisper-cli -oj`: offsets are milliseconds inside the probed
// span, text is one utterance per entry.
type whisperJSON struct {
	Transcription []struct {
		Offsets struct {
			From int64 `json:"from"`
			To   int64 `json:"to"`
		} `json:"offsets"`
		Text string `json:"text"`
	} `json:"transcription"`
}

func parseWhisperJSON(out []byte) ([]TranscriptSegment, error) {
	var w whisperJSON
	if err := json.Unmarshal(out, &w); err != nil {
		return nil, fmt.Errorf("whisper output not JSON: %w", err)
	}
	var segs []TranscriptSegment
	for _, t := range w.Transcription {
		text := strings.TrimSpace(t.Text)
		if text == "" || t.Offsets.To <= t.Offsets.From {
			continue
		}
		segs = append(segs, TranscriptSegment{StartMs: t.Offsets.From, EndMs: t.Offsets.To, Text: text})
	}
	return segs, nil
}
