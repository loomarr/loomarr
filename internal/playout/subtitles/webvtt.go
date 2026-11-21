package subtitles

import (
	"bufio"
	"bytes"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Cue is one subtitle on the media's own clock: shown from Start until End.
type Cue struct {
	Start, End time.Duration
	// Text is plain, with its line breaks kept: styling tags are removed, so every client draws it
	// the same way.
	Text string
}

var cueTag = regexp.MustCompile(`<[^>]*>`)

// ParseWebVTT reads the cues of a WebVTT document, in order. ffmpeg writes every text subtitle
// format it reads as WebVTT, so this is the one format a track is parsed from. A block without a
// timing line (the header, a NOTE, a STYLE) is skipped, as is a cue with no text.
func ParseWebVTT(doc []byte) ([]Cue, error) {
	doc = bytes.TrimPrefix(doc, []byte("\xEF\xBB\xBF"))
	if !bytes.HasPrefix(doc, []byte("WEBVTT")) {
		return nil, fmt.Errorf("webvtt: no WEBVTT header")
	}
	cues := []Cue{}
	scanner := bufio.NewScanner(bytes.NewReader(doc))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
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
			text := cueText(block[i+1:])
			if text != "" && end > start {
				cues = append(cues, Cue{Start: start, End: end, Text: text})
			}
			return nil
		}
		return nil
	}
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			if err := flush(); err != nil {
				return nil, err
			}
			continue
		}
		block = append(block, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("webvtt: %w", err)
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return cues, nil
}

// cueTiming reads "00:01:02.500 --> 00:01:04.000 align:start" (hours optional).
func cueTiming(line string) (time.Duration, time.Duration, error) {
	from, rest, _ := strings.Cut(line, "-->")
	to := strings.Fields(rest)
	if len(to) == 0 {
		return 0, 0, fmt.Errorf("webvtt: no end time in %q", line)
	}
	start, err := timestamp(strings.TrimSpace(from))
	if err != nil {
		return 0, 0, err
	}
	end, err := timestamp(to[0])
	if err != nil {
		return 0, 0, err
	}
	return start, end, nil
}

// timestamp reads "hh:mm:ss.ttt" or "mm:ss.ttt".
func timestamp(s string) (time.Duration, error) {
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("webvtt: bad timestamp %q", s)
	}
	secs, millis, ok := strings.Cut(parts[len(parts)-1], ".")
	if !ok || len(millis) != 3 {
		return 0, fmt.Errorf("webvtt: bad timestamp %q", s)
	}
	var total time.Duration
	for i, unit := range []string{secs, millis} {
		n, err := strconv.Atoi(unit)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("webvtt: bad timestamp %q", s)
		}
		total += time.Duration(n) * []time.Duration{time.Second, time.Millisecond}[i]
	}
	for i, unit := range parts[:len(parts)-1] {
		n, err := strconv.Atoi(unit)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("webvtt: bad timestamp %q", s)
		}
		scale := time.Minute
		if len(parts) == 3 && i == 0 {
			scale = time.Hour
		}
		total += time.Duration(n) * scale
	}
	return total, nil
}

// cueText is a cue's payload as plain text: tags removed, entities decoded, blank lines dropped.
func cueText(lines []string) string {
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(html.UnescapeString(cueTag.ReplaceAllString(line, "")))
		if line != "" {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}
