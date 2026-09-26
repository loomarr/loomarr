// Command splicecut is a throwaway feasibility spike for #1460: can a prepared v3 HLS segment
// (ONE moof per 2 s, access points every <=200 ms inside it) be re-fragmented in-process, copy only,
// so the server can join a channel at ANY access point with no ffmpeg child?
//
//	splicecut <dir-with-init.mp4-and-segments> <startSeconds> <outfile.mp4> [fragMs]
//
// It writes init + one fragment per fragMs (default 200) of the requested channel from the first
// video access point at/after startSeconds, and prints timing for the index and cut work.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

type sample struct {
	dur, size, flags uint32
	cts              int32
	off              int // absolute offset in the segment file
}

type track struct {
	id      uint32
	tfdt    uint64
	samples []sample
}

func be32(b []byte) uint32 { return binary.BigEndian.Uint32(b) }

// children iterates the boxes in b.
func children(b []byte, f func(typ string, body []byte, start int)) {
	for o := 0; o+8 <= len(b); {
		sz := int(be32(b[o:]))
		if sz < 8 || o+sz > len(b) {
			return
		}
		f(string(b[o+4:o+8]), b[o+8:o+sz], o)
		o += sz
	}
}

// parseSegment returns the tracks of the single moof in a segment file (data offsets resolved).
func parseSegment(seg []byte) (tracks []*track, moofStart int) {
	children(seg, func(typ string, body []byte, start int) {
		if typ != "moof" {
			return
		}
		moofStart = start
		children(body, func(t string, tb []byte, _ int) {
			if t != "traf" {
				return
			}
			tr := &track{}
			var defDur, defSize, defFlags uint32
			children(tb, func(t2 string, b2 []byte, _ int) {
				switch t2 {
				case "tfhd":
					fl := be32(b2) & 0xffffff
					tr.id = be32(b2[4:])
					p := 8
					if fl&0x1 != 0 {
						p += 8
					}
					if fl&0x2 != 0 {
						p += 4
					}
					if fl&0x8 != 0 {
						defDur = be32(b2[p:])
						p += 4
					}
					if fl&0x10 != 0 {
						defSize = be32(b2[p:])
						p += 4
					}
					if fl&0x20 != 0 {
						defFlags = be32(b2[p:])
					}
				case "tfdt":
					if b2[0] == 1 {
						tr.tfdt = binary.BigEndian.Uint64(b2[4:])
					} else {
						tr.tfdt = uint64(be32(b2[4:]))
					}
				case "trun":
					fl := be32(b2) & 0xffffff
					n := int(be32(b2[4:]))
					p := 8
					var dataOff int
					if fl&0x1 != 0 {
						dataOff = int(int32(be32(b2[p:])))
						p += 4
					}
					var first uint32
					hasFirst := fl&0x4 != 0
					if hasFirst {
						first = be32(b2[p:])
						p += 4
					}
					pos := moofStart + dataOff
					for i := 0; i < n; i++ {
						s := sample{dur: defDur, size: defSize, flags: defFlags}
						if fl&0x100 != 0 {
							s.dur = be32(b2[p:])
							p += 4
						}
						if fl&0x200 != 0 {
							s.size = be32(b2[p:])
							p += 4
						}
						if fl&0x400 != 0 {
							s.flags = be32(b2[p:])
							p += 4
						} else if i == 0 && hasFirst {
							s.flags = first
						}
						if fl&0x800 != 0 {
							s.cts = int32(be32(b2[p:]))
							p += 4
						}
						s.off = pos
						pos += int(s.size)
						tr.samples = append(tr.samples, s)
					}
				}
			})
			tracks = append(tracks, tr)
		})
	})
	return
}

const notSync = 0x10000 // sample_is_non_sync_sample

func u32(v uint32) []byte { b := make([]byte, 4); binary.BigEndian.PutUint32(b, v); return b }
func mk(typ string, parts ...[]byte) []byte {
	body := bytes.Join(parts, nil)
	return append(append(u32(uint32(8+len(body))), typ...), body...)
}

// fragment builds one moof+mdat holding samples ranges[i] of each track (video and audio ranges are
// chosen by the caller so both tracks stay time-aligned). Two passes: the moof size does not depend
// on the data offsets, so pass one learns it and pass two writes the real offsets.
func fragment(seq uint32, seg []byte, tks []*track, ranges [][2]int, tfdts []uint64) []byte {
	build := func(dataOff []uint32) (moof []byte, payload []byte) {
		var trafs [][]byte
		var mdat bytes.Buffer
		for ti, tk := range tks {
			lo, hi := ranges[ti][0], ranges[ti][1]
			trun := append(u32(0x0F01+0x1000000), u32(uint32(hi-lo))...) // v1: data-offset, dur, size, flags, cts
			trun = append(trun, u32(dataOff[ti])...)
			for _, s := range tk.samples[lo:hi] {
				trun = append(trun, u32(s.dur)...)
				trun = append(trun, u32(s.size)...)
				trun = append(trun, u32(s.flags)...)
				trun = append(trun, u32(uint32(s.cts))...)
				mdat.Write(seg[s.off : s.off+int(s.size)])
			}
			trafs = append(trafs, mk("traf", mk("tfhd", u32(0x020000), u32(tk.id)),
				mk("tfdt", u32(0x1000000), binary.BigEndian.AppendUint64(nil, tfdts[ti])), mk("trun", trun)))
		}
		return mk("moof", mk("mfhd", u32(0), u32(seq)), bytes.Join(trafs, nil)), mdat.Bytes()
	}
	dataOff := make([]uint32, len(tks))
	moof, payload := build(dataOff)
	off := uint32(len(moof) + 8)
	for ti, tk := range tks {
		dataOff[ti] = off
		for _, s := range tk.samples[ranges[ti][0]:ranges[ti][1]] {
			off += s.size
		}
	}
	moof, _ = build(dataOff)
	return append(moof, mk("mdat", payload)...)
}

func main() {
	dir, startS, outPath := os.Args[1], os.Args[2], os.Args[3]
	fragMS := 200
	if len(os.Args) > 4 {
		fragMS, _ = strconv.Atoi(os.Args[4])
	}
	start, _ := strconv.ParseFloat(startS, 64)
	init, _ := os.ReadFile(filepath.Join(dir, "init.mp4"))
	segs, _ := filepath.Glob(filepath.Join(dir, "segment-*.m4s"))
	sort.Strings(segs)
	const vTS, aTS = 15360.0, 48000.0
	var out bytes.Buffer
	out.Write(init)
	seq := uint32(1)
	var idxTime, cutTime time.Duration
	nFrags := 0
	first := true
	var t0 float64
	for _, sp := range segs {
		seg, _ := os.ReadFile(sp)
		ti := time.Now()
		tks, _ := parseSegment(seg)
		idxTime += time.Since(ti)
		v, a := tks[0], tks[1]
		vt := make([]float64, len(v.samples))
		acc := float64(v.tfdt)
		for i, s := range v.samples {
			vt[i] = acc / vTS
			acc += float64(s.dur)
		}
		if vt[len(vt)-1] < start-0.001 {
			continue
		}
		tc := time.Now()
		// video access points in this segment at/after the requested start
		var aps []int
		for i, s := range v.samples {
			if s.flags&notSync == 0 && vt[i] >= start-0.001 {
				aps = append(aps, i)
			}
		}
		if len(aps) == 0 {
			continue
		}
		if first {
			t0 = vt[aps[0]]
			first = false
		}
		// cut into ~fragMS pieces at successive access points
		lo := aps[0]
		for lo < len(v.samples) {
			hi := len(v.samples)
			for _, ap := range aps {
				if ap > lo && (vt[ap]-vt[lo])*1000 >= float64(fragMS)-1 {
					hi = ap
					break
				}
			}
			// audio: whole samples whose dts falls in [vt[lo], vt[hi])
			endT := 1e18
			if hi < len(v.samples) {
				endT = vt[hi]
			}
			alo, ahi := -1, len(a.samples)
			at := float64(a.tfdt)
			for i, s := range a.samples {
				sec := at / aTS
				if alo < 0 && sec >= vt[lo]-0.0005 {
					alo = i
				}
				if sec >= endT-0.0005 {
					ahi = i
					break
				}
				at += float64(s.dur)
			}
			if alo < 0 {
				alo = len(a.samples)
			}
			// tfdt for the new fragments
			vd := uint64(vt[lo]*vTS + 0.5)
			ad := a.tfdt
			for i := 0; i < alo; i++ {
				ad += uint64(a.samples[i].dur)
			}
			out.Write(fragment(seq, seg, tks, [][2]int{{lo, hi}, {alo, ahi}}, []uint64{vd, ad}))
			seq++
			nFrags++
			lo = hi
		}
		cutTime += time.Since(tc)
	}
	os.WriteFile(outPath, out.Bytes(), 0o644)
	fmt.Printf("first access point %.3fs (requested %.3fs); %d fragments of ~%d ms; parse %v total, cut+copy %v total, %d bytes\n",
		t0, start, nFrags, fragMS, idxTime, cutTime, out.Len())
}
