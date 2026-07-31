package filler

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
	blackRe = regexp.MustCompile(`black_start:(\d+(?:\.\d+)?) black_end:(\d+(?:\.\d+)?)`)
	// [silencedetect @ 0x…] silence_start: 3.36
	silenceStartRe = regexp.MustCompile(`silence_start: (\d+(?:\.\d+)?)`)
	// [silencedetect @ 0x…] silence_end: 5.84 | silence_duration: 2.48
	silenceEndRe = regexp.MustCompile(`silence_end: (\d+(?:\.\d+)?)`)
)

// parseBlackdetect extracts the black intervals ffmpeg reports on stderr. An
// advert break in a compilation is usually a fade to black between spots; at
// pix_th=0.20 (mediatools) these measured 81–100% of boundaries (plan §6.4).
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
