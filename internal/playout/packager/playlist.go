package packager

import (
	"bytes"
	"fmt"
	"math"
	"time"
)

// InitName is the channel init segment's file name, referenced by EXT-X-MAP.
const InitName = "init.mp4"

func segmentName(seq uint32) string { return fmt.Sprintf("seg%08d.m4s", seq) }

type segment struct {
	seq        uint32
	name       string
	start, dur int64 // channel media ticks, 90 kHz
}

// window is the DVR window of published segments, oldest first.
type window struct{ segs []segment }

// push appends a segment and returns the segments that fell out of the window (ending at or
// before floor).
func (w *window) push(s segment, floor int64) []segment {
	w.segs = append(w.segs, s)
	i := 0
	for i < len(w.segs) && w.segs[i].start+w.segs[i].dur <= floor {
		i++
	}
	gone := append([]segment(nil), w.segs[:i]...)
	w.segs = w.segs[i:]
	return gone
}

// listable is the prefix of the window whose segments end at or before the listing edge.
func (w *window) listable(edge int64) []segment {
	n := 0
	for n < len(w.segs) && w.segs[n].start+w.segs[n].dur <= edge {
		n++
	}
	return w.segs[:n]
}

func (w *window) listableTicks(edge int64) int64 {
	var t int64
	for _, s := range w.listable(edge) {
		t += s.dur
	}
	return t
}

type playlistView struct {
	edge     int64
	epoch    time.Time
	holdBack time.Duration
	prefix   string // Config.URIPrefix
}

// render writes the live media playlist. It is the one place a delta update (EXT-X-SKIP with
// CAN-SKIP-UNTIL, #1512 phase 0b) would be added: render the tail after a skip boundary.
func (w *window) render(v playlistView) []byte {
	segs := w.listable(v.edge)
	target := int64(1)
	for _, s := range segs {
		target = max(target, int64(math.Ceil(float64(s.dur)/videoRate)))
	}
	var b bytes.Buffer
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-INDEPENDENT-SEGMENTS\n")
	fmt.Fprintf(&b, "#EXT-X-TARGETDURATION:%d\n", target)
	fmt.Fprintf(&b, "#EXT-X-SERVER-CONTROL:HOLD-BACK=%.1f\n", v.holdBack.Seconds())
	var first uint32
	if len(segs) > 0 {
		first = segs[0].seq
	}
	fmt.Fprintf(&b, "#EXT-X-MEDIA-SEQUENCE:%d\n", first)
	fmt.Fprintf(&b, "#EXT-X-MAP:URI=%q\n", v.prefix+InitName)
	for i, s := range segs {
		if i == 0 {
			at := v.epoch.Add(ticks(s.start)).UTC().Format("2006-01-02T15:04:05.000Z07:00")
			fmt.Fprintf(&b, "#EXT-X-PROGRAM-DATE-TIME:%s\n", at)
		}
		fmt.Fprintf(&b, "#EXTINF:%.5f,\n%s%s\n", float64(s.dur)/videoRate, v.prefix, s.name)
	}
	return b.Bytes()
}
