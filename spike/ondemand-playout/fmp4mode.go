package main

// -mode fmp4: ffmpeg muxes fragmented MP4 itself (one fragment per GOP = one HLS segment) with the
// item's channel offset applied; Go only forwards fragments, patching mfhd.sequence_number and each
// traf's tfdt in place onto the channel timeline. No TS demux, no per-sample repacking. The one
// exception is each item's first fragment, which goes through mediacommon's Parts to drop the AAC
// priming frame. Frame/sample counts are exact because the timeline is precomputed from the schedule
// and passed to ffmpeg as -frames:v / -frames:a.

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4/seekablebuffer"
)

type slot struct {
	vBase, aBase    int64 // channel video PTS (90 kHz) and audio sample index
	nFrames, nAudio int64
}

// timeline precomputes every item's slot; the TS path derives the same numbers incrementally.
func timeline(items []Item) []slot {
	var out []slot
	v, a := int64(pts0), int64(0)
	for _, it := range items {
		n := int64(math.Round(it.Dur * fpsNum / fpsDen))
		aEnd := (v + n*frameDur - pts0) * sampleRate / 90000
		na := (aEnd + aacHalf - a) / aacFrame
		out = append(out, slot{v, a, n, na})
		v += n * frameDur
		a += na * aacFrame
	}
	return out
}

type frag struct {
	b     []byte // moof+mdat
	first bool
}

type fsource struct {
	init      []byte
	frags     chan frag
	done      chan struct{}
	err       error
	spawnedAt time.Time
	stderr    bytes.Buffer
	cmd       *exec.Cmd
}

func fmp4Args(it Item, g int, sl slot) []string {
	a := encoderArgs(it, g)
	a = a[:len(a)-7]                                                                              // drop "-f mpegts -muxdelay 0 -muxpreload 0 pipe:1"
	return append(a, "-frames:v", fmt.Sprint(sl.nFrames+3), "-frames:a", fmt.Sprint(sl.nAudio+4), // margin; Go trims to the slot
		"-video_track_timescale", "90000", "-output_ts_offset", fmt.Sprintf("%.6f", float64(sl.vBase-pts0)/ticksPerSec),
		"-f", "mp4", "-movflags", "empty_moov+default_base_moof+frag_keyframe", "pipe:1")
}

func readBox(r *bufio.Reader) (typ string, b []byte, err error) {
	var h [8]byte
	if _, err = io.ReadFull(r, h[:]); err != nil {
		return
	}
	size := uint64(binary.BigEndian.Uint32(h[:4]))
	typ = string(h[4:8])
	hdr := h[:]
	if size == 1 {
		var ls [8]byte
		if _, err = io.ReadFull(r, ls[:]); err != nil {
			return
		}
		size = binary.BigEndian.Uint64(ls[:])
		hdr = append(hdr[:8:8], ls[:]...)
	}
	b = make([]byte, size)
	copy(b, hdr)
	_, err = io.ReadFull(r, b[len(hdr):])
	return
}

func startFSource(it Item, g int, sl slot) *fsource {
	s := &fsource{frags: make(chan frag, 4), done: make(chan struct{})}
	go func() {
		defer close(s.done)
		defer close(s.frags)
		s.spawnedAt = time.Now()
		s.cmd = exec.Command("ffmpeg", fmp4Args(it, g, sl)...)
		errPipe, _ := s.cmd.StderrPipe()
		go func() {
			sc := bufio.NewScanner(errPipe)
			for sc.Scan() {
				l := sc.Text()
				if strings.HasPrefix(l, "Input #0") {
					ev("probe_done", map[string]any{"item": it.Name, "since_spawn_ms": float64(time.Since(s.spawnedAt).Microseconds()) / 1000})
				}
				if !strings.HasPrefix(l, " ") {
					s.stderr.WriteString(l + "\n")
				}
			}
		}()
		out, _ := s.cmd.StdoutPipe()
		if err := s.cmd.Start(); err != nil {
			s.err = err
			return
		}
		defer func() { io.Copy(io.Discard, out); s.cmd.Wait() }()
		r := bufio.NewReaderSize(out, 1<<16)
		var moof []byte
		first := true
		for {
			typ, b, err := readBox(r)
			if err != nil {
				if !errors.Is(err, io.EOF) {
					s.err = err
				}
				return
			}
			switch typ {
			case "ftyp", "moov":
				s.init = append(s.init, b...)
				if typ == "moov" { // written once the encoder has opened: splits probe from first-GOP encode
					ev("header", map[string]any{"item": it.Name, "since_spawn_ms": float64(time.Since(s.spawnedAt).Microseconds()) / 1000})
				}
			case "moof":
				moof = b
			case "mdat":
				s.frags <- frag{b: append(moof, b...), first: first}
				first = false
			}
		}
	}()
	return s
}

// children calls fn for each child box of the container box body b[off:end].
func children(b []byte, off int, fn func(typ string, start, size int)) {
	for off+8 <= len(b) {
		sz := int(binary.BigEndian.Uint32(b[off:]))
		if sz < 8 || off+sz > len(b) {
			return
		}
		fn(string(b[off+4:off+8]), off, sz)
		off += sz
	}
}

// patch rewrites mfhd.sequence_number and tfdt in place; it returns the sample count per track id.
func patch(b []byte, seq uint32, base map[uint32]uint64) map[uint32]int64 {
	counts := map[uint32]int64{}
	moofSize := int(binary.BigEndian.Uint32(b))
	children(b[:moofSize], 8, func(typ string, s, sz int) {
		switch typ {
		case "mfhd":
			binary.BigEndian.PutUint32(b[s+12:], seq)
		case "traf":
			var id uint32
			children(b[:s+sz], s+8, func(t string, cs, _ int) {
				switch t {
				case "tfhd":
					id = binary.BigEndian.Uint32(b[cs+12:])
				case "tfdt":
					if b[cs+8] == 1 {
						binary.BigEndian.PutUint64(b[cs+12:], base[id])
					} else {
						binary.BigEndian.PutUint32(b[cs+12:], uint32(base[id]))
					}
				case "trun":
					counts[id] += int64(binary.BigEndian.Uint32(b[cs+12:]))
				}
			})
		}
	})
	return counts
}

// rewrite re-marshals a fragment through mediacommon's Parts. It is used only where samples must change:
// an item's first fragment (drop the AAC priming frame, even out ffmpeg's start-shifted durations) and the
// fragment where the slot fills (ffmpeg's -frames:v/-frames:a are not exact, measured 1198/1199 video and
// ±2 audio, so ffmpeg over-produces and Go trims to the slot: at most left[id] samples per track).
func rewrite(b []byte, seq uint32, base map[uint32]uint64, first bool, left map[uint32]int64) ([]byte, map[uint32]int64, error) {
	var ps fmp4.Parts
	if err := ps.Unmarshal(b); err != nil {
		return nil, nil, err
	}
	counts := map[uint32]int64{}
	for _, pt := range ps {
		pt.SequenceNumber = seq
		for _, tr := range pt.Tracks {
			id := uint32(tr.ID)
			if first && id == 2 && len(tr.Samples) > 0 {
				tr.Samples = tr.Samples[1:]
			}
			if first && id == 1 {
				for _, s := range tr.Samples {
					s.Duration, s.PTSOffset = frameDur, 0
				}
			}
			if n := left[id] - counts[id]; int64(len(tr.Samples)) > n {
				tr.Samples = tr.Samples[:max(n, 0)]
			}
			tr.BaseTime = base[id] + uint64(counts[id])*map[uint32]uint64{1: frameDur, 2: aacFrame}[id]
			counts[id] += int64(len(tr.Samples))
		}
	}
	var w seekablebuffer.Buffer
	if err := ps.Marshal(&w); err != nil {
		return nil, nil, err
	}
	return w.Bytes(), counts, nil
}

// stsd returns the bytes of the first stsd box in each trak, to check encoders agree.
func stsds(init []byte) [][]byte {
	var out [][]byte
	var walk func(off, end int)
	walk = func(off, end int) {
		children(init[:end], off, func(t string, s, sz int) {
			switch t {
			case "moov", "trak", "mdia", "minf", "stbl":
				walk(s+8, s+sz)
			case "stsd":
				out = append(out, init[s:s+sz])
			}
		})
	}
	walk(0, len(init))
	return out
}

func runFMP4(items []Item, g int, p *packager) {
	tl := timeline(items)
	var initRef [][]byte
	var srcs []*fsource
	next := startFSource(items[0], g, tl[0])
	for i, it := range items {
		s, sl := next, tl[i]
		var nv, na int64
		firstMs := -1.0
		for f := range s.frags {
			if firstMs < 0 {
				firstMs = float64(time.Since(s.spawnedAt).Microseconds()) / 1000
				ev("first_video", map[string]any{"item": it.Name, "since_spawn_ms": firstMs})
				if initRef == nil {
					initRef = stsds(s.init)
					os.WriteFile(filepath.Join(*flagOut, "init.mp4"), s.init, 0o644)
				} else if got := stsds(s.init); len(got) != len(initRef) || !bytes.Equal(got[0], initRef[0]) || !bytes.Equal(got[1], initRef[1]) {
					ev("stsd_diff", map[string]any{"item": it.Name})
				}
			}
			base := map[uint32]uint64{1: uint64(sl.vBase + nv*frameDur), 2: uint64(sl.aBase + na*aacFrame + pts0*sampleRate/90000)}
			left := map[uint32]int64{1: sl.nFrames - nv, 2: sl.nAudio - na}
			if left[1] <= 0 {
				continue // slot full: discard the encoder's margin (any audio still owed is logged in item_done)
			}
			b, c := f.b, patch(f.b, uint32(p.seq), base)
			if f.first || c[1] > left[1] || c[2]-map[bool]int64{true: 1}[f.first] > left[2] {
				var err error
				if b, c, err = rewrite(f.b, uint32(p.seq), base, f.first, left); err != nil {
					ev("fatal", map[string]any{"err": err.Error()})
					os.Exit(1)
				}
			}
			name := fmt.Sprintf("seg%05d.m4s", p.seq)
			os.WriteFile(filepath.Join(*flagOut, name), b, 0o644)
			sg := segment{Seq: p.seq, Name: name, Start: p.chanSeconds(int64(base[1])), Dur: float64(c[1]*frameDur) / ticksPerSec,
				Item: it.Name, WrittenS: time.Since(t0).Seconds()}
			p.segMu.Lock()
			p.segs = append(p.segs, sg)
			p.segMu.Unlock()
			if p.seq == 0 {
				ev("first_segment", map[string]any{"ms": float64(time.Since(t0).Microseconds()) / 1000, "dur": sg.Dur})
				if *flagFirstSeg {
					os.Exit(0)
				}
			}
			p.seq++
			nv += c[1]
			na += c[2]
			p.vNext = sl.vBase + nv*frameDur
			p.pace()
		}
		ev("item_done", map[string]any{"item": it.Name, "first_video_ms": firstMs, "frames": nv, "want_frames": sl.nFrames,
			"audio_frames": na, "want_audio": sl.nAudio, "start_s": p.chanSeconds(sl.vBase), "encoder_err": strings.TrimSpace(s.stderr.String()) + fmt.Sprint(s.err)})
		srcs = append(srcs, s)
		if i+1 < len(items) {
			next = startFSource(items[i+1], g, tl[i+1])
		}
	}
	for _, s := range srcs {
		<-s.done
	}
}
