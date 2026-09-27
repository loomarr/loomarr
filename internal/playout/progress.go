package playout

import (
	"bufio"
	"io"
	"strconv"
	"strings"
)

// Progress is one sample of ffmpeg's structured progress output.
type Progress struct {
	// Frame is the encoded frame count; monotonic while healthy.
	Frame int64
	// Speed is the realtime multiple. Below ~1.0 sustained means the encoder cannot keep
	// up and the channel will stutter — the single most useful health signal there is.
	Speed float64
	// OutTimeMS is how much output has been produced, in milliseconds.
	OutTimeMS int64
}

// ReadProgress parses ffmpeg's `-progress` stream and calls onProgress once per block.
//
// ⚠ **Exported so the filler transcode stage shares this parser rather than growing a second
// copy** (§10 V51b). It is the same key set and the same block-boundary rule, and the two would
// drift the moment ffmpeg changed a field name — `out_time_ms` already reports MICROSECONDS
// despite its name, which is exactly the sort of fact that gets fixed in one copy.
//
// It takes ownership of r and closes it. A nil onProgress still DRAINS the pipe: a full pipe
// blocks ffmpeg's writes and stalls the encode.
func ReadProgress(r io.ReadCloser, onProgress func(Progress)) {
	defer func() { _ = r.Close() }()
	if onProgress == nil {
		_, _ = io.Copy(io.Discard, r)
		return
	}
	var cur Progress
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		if consumeProgressLine(strings.TrimSpace(sc.Text()), &cur) {
			onProgress(cur)
		}
	}
}

// consumeProgressLine parses one ffmpeg progress-protocol line into cur and reports whether it
// closed a block (progress=continue|end), so a consumer sees whole samples, never half-updated ones.
// A malformed value leaves cur unchanged.
func consumeProgressLine(line string, cur *Progress) (complete bool) {
	k, v, ok := strings.Cut(line, "=")
	if !ok {
		return false
	}
	v = strings.TrimSpace(v)
	switch k {
	case "frame":
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			cur.Frame = n
		}
	case "out_time_ms":
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			cur.OutTimeMS = n / 1000 // ffmpeg reports microseconds despite the name
		}
	case "speed":
		if f, err := strconv.ParseFloat(strings.TrimSuffix(v, "x"), 64); err == nil && strings.HasSuffix(v, "x") {
			cur.Speed = f
		}
	case "progress":
		return v == "continue" || v == "end"
	}
	return false
}
