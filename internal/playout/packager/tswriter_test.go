package packager

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	tscodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts/codecs"
)

// twoItemTimeline is the channel's packaged output across an item boundary: real ffmpeg fragments
// (testdata/ffmpeg-fragmented.mp4) stamped twice onto one timeline, as two items would be.
func twoItemTimeline(t *testing.T) (init []byte, segments [][]byte, frameTicks int64) {
	t.Helper()
	raw, err := os.ReadFile("testdata/ffmpeg-fragmented.mp4")
	if err != nil {
		t.Fatal(err)
	}
	const frameDur = 3000 // 30 fps on 90 kHz
	var v, a int64
	for item := range 2 {
		stream := readStream(bytes.NewReader(raw))
		first := true
		for frag := range stream.frags {
			// Stamped as Packager.forward does: an item's first fragment is rewritten (priming
			// frame dropped, first durations evened), the rest patched in place.
			base := map[uint32]uint64{videoTrack: uint64(v), audioTrack: uint64(a)}
			out, c, err := rewriteFragment(frag, uint32(len(segments)), base, first, frameDur,
				map[uint32]int64{videoTrack: 1 << 20, audioTrack: 1 << 20})
			if !first {
				out = frag
				c, err = patchFragment(frag, uint32(len(segments)), base)
			}
			if err != nil {
				t.Fatal(err)
			}
			first = false
			segments = append(segments, out)
			v += c[videoTrack] * frameDur
			a += c[audioTrack] * aacFrame
		}
		if item == 0 {
			init = stream.init
		}
	}
	return init, segments, frameDur
}

// The TS sink (#1512 phase 2b) turns the channel's one fMP4 timeline into one MPEG-TS stream: a
// media server tuned across an item boundary sees no continuity-counter jump, no timestamp gap, and
// an IDR it can start decoding at (parameter sets in band), exactly as within an item.
func TestTSWriterIsContinuousAcrossAnItemBoundary(t *testing.T) {
	init, segments, frameTicks := twoItemTimeline(t)
	var out bytes.Buffer
	w, err := NewTSWriter(&out, init)
	if err != nil {
		t.Fatal(err)
	}
	for _, seg := range segments {
		if err := w.WriteSegment(seg); err != nil {
			t.Fatal(err)
		}
	}
	ts := out.Bytes()
	if len(ts) == 0 || len(ts)%188 != 0 {
		t.Fatalf("output is %d bytes, not whole TS packets", len(ts))
	}

	// Continuity counters: +1 mod 16 on every payload-carrying packet of a PID.
	last := map[uint16]int{}
	for off := 0; off < len(ts); off += 188 {
		p := ts[off : off+188]
		if p[0] != 0x47 {
			t.Fatalf("packet %d: no sync byte", off/188)
		}
		pid := uint16(p[1]&0x1f)<<8 | uint16(p[2])
		if p[3]&0x10 == 0 {
			continue // adaptation field only: the counter does not advance
		}
		cc := int(p[3] & 0x0f)
		if prev, ok := last[pid]; ok && cc != (prev+1)%16 {
			t.Fatalf("PID %d: continuity counter %d after %d at packet %d", pid, cc, prev, off/188)
		}
		last[pid] = cc
	}

	// Timestamps and decodability, read back as a media server would.
	r := &mpegts.Reader{R: bytes.NewReader(ts)}
	if err := r.Initialize(); err != nil {
		t.Fatal(err)
	}
	var dts []int64
	idrs, idrWithParams, audio := 0, 0, 0
	for _, track := range r.Tracks() {
		switch track.Codec.(type) {
		case *tscodecs.H264:
			r.OnDataH264(track, func(_ int64, d int64, au [][]byte) error {
				dts = append(dts, d)
				if !h264.IsRandomAccess(au) {
					return nil
				}
				idrs++
				has := map[h264.NALUType]bool{}
				for _, n := range au {
					has[h264.NALUType(n[0]&0x1f)] = true
				}
				if has[h264.NALUTypeSPS] && has[h264.NALUTypePPS] {
					idrWithParams++
				}
				return nil
			})
		case *tscodecs.MPEG4Audio:
			r.OnDataMPEG4Audio(track, func(int64, [][]byte) error { audio++; return nil })
		}
	}
	r.OnDecodeError(func(err error) { t.Errorf("decode error: %v", err) })
	for {
		if err := r.Read(); err != nil {
			if !errors.Is(err, io.EOF) {
				t.Fatal(err)
			}
			break
		}
	}
	if len(dts) < 2 || audio == 0 {
		t.Fatalf("read back %d video access units, %d audio PES", len(dts), audio)
	}
	for i := 1; i < len(dts); i++ {
		if dts[i]-dts[i-1] != frameTicks {
			t.Fatalf("video DTS step %d at access unit %d, want %d", dts[i]-dts[i-1], i, frameTicks)
		}
	}
	if idrs < 2 || idrWithParams != idrs {
		t.Fatalf("%d of %d IDRs carry SPS/PPS in band, want every one (and one per item at least)", idrWithParams, idrs)
	}
}
