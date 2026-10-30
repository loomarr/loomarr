package playout

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/diagnostics"
)

// Soft subtitles travel BESIDE the stream, not in it (#2088 step 7). A channel's master playlist is
// read once per tune, but subtitle languages change programme by programme, so an HLS subtitle
// rendition could only ever describe the programme a viewer tuned into. Instead the player asks
// for the cues of the programme at its playhead (GET /v1/channels/{id}/subtitles), which the
// server extracts from the source's text track here. It is never on the broadcast path, and the
// encode is untouched.

// SubtitleCue is one cue in SOURCE time: how far into the file it shows and hides.
type SubtitleCue struct {
	Start, End time.Duration
	// Text is the WebVTT cue payload: lines joined by "\n", with WebVTT's inline <i>/<b> tags kept.
	Text string
}

// SubtitleAiring is one airing in a window of the channel's wall clock: its text subtitle tracks,
// and the chosen track's cues. A break or a programme without text subtitles has no tracks, which
// tells the player its choices end at Stop.
type SubtitleAiring struct {
	Start, Stop time.Time
	Tracks      []Track
	Cues        []SubtitleWallCue
}

// SubtitleWallCue is a cue placed on the channel's wall clock.
type SubtitleWallCue struct {
	Start, End time.Time
	Text       string
}

// SubtitlePick is the viewer's choice: the track with this language and title, else the first with
// this language.
type SubtitlePick struct{ Language, Title string }

// PickSubtitle returns the index (among tracks) a pick selects, or -1.
func PickSubtitle(tracks []Track, pick SubtitlePick) int {
	for i, t := range tracks {
		if t.Language == pick.Language && t.Title == pick.Title {
			return i
		}
	}
	for i, t := range tracks {
		if t.Language == pick.Language {
			return i
		}
	}
	return -1
}

// SubtitleChunk is the source span one extraction covers, and the unit the caller caches. Five
// minutes keeps a seek and one extraction cheap, where reading a film's whole text track would
// read the whole file.
const SubtitleChunk = 5 * time.Minute

// IsTextSubtitle reports whether ffmpeg converts this subtitle codec to WebVTT as text. Image
// formats (PGS, VOBSUB, DVB) are not offered as soft subtitles; burning them in is step 8.
func IsTextSubtitle(codec string) bool {
	switch codec {
	case "subrip", "srt", "ass", "ssa", "mov_text", "webvtt", "text":
		return true
	}
	return false
}

// SubtitleExtractor returns the cues of subtitle track N (per type, `0:s:N`) that START in
// [from, to) of the source.
type SubtitleExtractor func(ctx context.Context, input string, track int, from, to time.Duration) ([]SubtitleCue, error)

// FFmpegSubtitlesWith extracts with ffmpeg: an input seek to `from` (so a chunk deep in a film does
// not read the film up to it), the one text track converted to WebVTT on stdout, and -copyts so
// cue times stay in source time.
func FFmpegSubtitlesWith(ffmpegPath string, observer *diagnostics.ProcessManager) SubtitleExtractor {
	return func(ctx context.Context, input string, track int, from, to time.Duration) ([]SubtitleCue, error) {
		if track < 0 || to <= from {
			return nil, fmt.Errorf("extract subtitles: bad span track=%d %v..%v", track, from, to)
		}
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		args := []string{"-v", "error", "-nostdin", "-copyts", "-ss", seconds(from), "-to", seconds(to), "-i", input,
			"-map", "0:s:" + strconv.Itoa(track), "-c:s", "webvtt", "-f", "webvtt", "pipe:1"}
		cmd := exec.CommandContext(ctx, ffmpegPath, args...)
		var out probeOutputLimit
		var stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &stderr
		spec, _ := diagnostics.ProcessSpecFromContext(ctx)
		spec.Purpose, spec.Executable, spec.Args, spec.Target = "subtitle_extract", ffmpegPath, args, "subtitles"
		run := observer.Begin(spec)
		err := cmd.Run()
		if run != nil {
			if stderr.Len() > 0 {
				run.RecordOutput(strings.TrimSpace(stderr.String()))
			}
			run.Finish(diagnostics.ProcessResult{Err: err, Cancelled: ctx.Err() != nil})
		}
		if err != nil {
			return nil, fmt.Errorf("extract subtitles: %w: %s", err, strings.TrimSpace(stderr.String()))
		}
		cues, err := ParseWebVTT(out.Bytes())
		if err != nil {
			return nil, err
		}
		// The input seek lands on a keyframe before `from`, so the head can carry the previous
		// chunk's cues: keep each cue in exactly one chunk, by its start.
		kept := cues[:0]
		for _, c := range cues {
			if c.Start >= from && c.Start < to {
				kept = append(kept, c)
			}
		}
		return kept, nil
	}
}

// ParseWebVTT reads ffmpeg's WebVTT output into cues. Header, NOTE, STYLE and REGION blocks are
// skipped; a cue's optional identifier line is dropped.
func ParseWebVTT(b []byte) ([]SubtitleCue, error) {
	var cues []SubtitleCue
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var block []string
	flush := func() error {
		defer func() { block = block[:0] }()
		for i, line := range block {
			if !strings.Contains(line, "-->") {
				continue
			}
			start, end, err := cueTiming(line)
			if err != nil {
				return err
			}
			if text := strings.Join(block[i+1:], "\n"); text != "" {
				cues = append(cues, SubtitleCue{Start: start, End: end, Text: text})
			}
			return nil
		}
		return nil
	}
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			if err := flush(); err != nil {
				return nil, err
			}
			continue
		}
		block = append(block, line)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return cues, nil
}

// cueTiming parses "00:01:02.500 --> 00:01:04.000 align:start": hours are optional in WebVTT.
func cueTiming(line string) (time.Duration, time.Duration, error) {
	parts := strings.SplitN(line, "-->", 2)
	start, err := vttTime(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, err
	}
	endField := strings.Fields(parts[1])
	if len(endField) == 0 {
		return 0, 0, errors.New("webvtt: cue timing has no end")
	}
	end, err := vttTime(endField[0])
	if err != nil {
		return 0, 0, err
	}
	return start, end, nil
}

func vttTime(s string) (time.Duration, error) {
	clock, frac, ok := strings.Cut(s, ".")
	if !ok || len(frac) != 3 {
		return 0, fmt.Errorf("webvtt: bad timestamp %q", s)
	}
	fields := strings.Split(clock, ":")
	if len(fields) < 2 || len(fields) > 3 {
		return 0, fmt.Errorf("webvtt: bad timestamp %q", s)
	}
	var d time.Duration
	for _, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("webvtt: bad timestamp %q", s)
		}
		d = d*60 + time.Duration(n)
	}
	ms, err := strconv.Atoi(frac)
	if err != nil {
		return 0, fmt.Errorf("webvtt: bad timestamp %q", s)
	}
	return d*time.Second + time.Duration(ms)*time.Millisecond, nil
}
