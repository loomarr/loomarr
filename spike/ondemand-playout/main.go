// Command spikepack is a THROWAWAY phase-0 measurement spike for on-demand playout (#1512).
// It is its own Go module, is never imported by production code, and is not built by the repo gates.
//
// One ffmpeg per scheduled item (programme or clip) writes MPEG-TS to stdout, seeked into the item and
// trimmed to its slot. One in-process packager rebases every item onto a single channel timeline
// (video by frame count on a 90 kHz grid, audio by sample count with AAC priming dropped and at most half
// an AAC frame of drift per boundary) and writes fMP4 HLS plus one continuous MPEG-TS. It bursts, then
// back-pressures at -runahead seconds ahead of wall clock, lists segments only up to now+-listahead, and
// falls back to a pre-encoded slate when an item is not ready by air time - 2 s.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/pprof"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4/seekablebuffer"
	mp4codecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	tscodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts/codecs"
)

// House format: 1080p29.97 H.264 High, AAC-LC stereo 48 kHz.
const (
	outW, outH  = 1920, 1080
	fpsNum      = 30000
	fpsDen      = 1001
	frameDur    = 90000 * fpsDen / fpsNum // 3003 ticks
	aacFrame    = 1024
	sampleRate  = 48000
	pts0        = 2 * 90000 // channel timeline origin in PTS
	aacHalf     = aacFrame / 2
	ticksPerSec = 90000
)

type Item struct {
	Name   string  `json:"name"`
	File   string  `json:"file"`
	Seek   float64 `json:"seek"`
	Dur    float64 `json:"dur"`
	HDR    bool    `json:"hdr"`
	GainDB float64 `json:"gain_db"`  // static gain measured at "ingest"; 0 = none. No loudnorm.
	Delay  int     `json:"delay_ms"` // test hook: sleep before spawning, to exercise the slate fallback
}

var (
	flagSched     = flag.String("schedule", "", "schedule JSON ([]Item)")
	flagOut       = flag.String("out", "out", "output dir")
	flagSeg       = flag.Float64("seg", 2, "segment seconds")
	flagRunahead  = flag.Float64("runahead", 12, "back-pressure run-ahead seconds (0 = unpaced)")
	flagListahead = flag.Float64("listahead", 6, "list segments only up to now + this")
	flagHTTP      = flag.String("http", "", "serve out dir on this addr")
	flagSlate     = flag.String("slate", "", "pre-encoded house-format slate .ts")
	flagFirstSeg  = flag.Bool("first-segment-exit", false, "exit as soon as the first segment is written")
	flagLinger    = flag.Duration("linger", 0, "keep serving after the schedule ends")
	flagRC        = flag.String("rc", "-rc_mode QVBR -b:v 8M -maxrate 12M -global_quality 22", "h264_vaapi rate control args")
	flagInOpts    = flag.String("inopts", "", "extra ffmpeg input options (before -i)")
	flagCPUProf   = flag.String("cpuprofile", "", "write a CPU profile of the packager")
)

var (
	t0     = time.Now()
	evMu   sync.Mutex
	evFile *os.File
)

func ev(kind string, kv map[string]any) {
	if kv == nil {
		kv = map[string]any{}
	}
	kv["t_ms"] = float64(time.Since(t0).Microseconds()) / 1000
	kv["ev"] = kind
	b, _ := json.Marshal(kv)
	evMu.Lock()
	evFile.Write(append(b, '\n'))
	evMu.Unlock()
	fmt.Fprintln(os.Stderr, string(b))
}

func encoderArgs(it Item, g int) []string {
	fmtIn := "nv12"
	tm := ""
	if it.HDR {
		// Scale BEFORE tonemap: tone-mapping at 4K runs 0.7x on the A380.
		fmtIn, tm = "p010", ",tonemap_vaapi=format=nv12:t=bt709:m=bt709:p=bt709"
	}
	vf := fmt.Sprintf("scale_vaapi=w=%d:h=%d:force_original_aspect_ratio=decrease:force_divisible_by=2:format=%s%s,pad_vaapi=w=%d:h=%d,fps=%d/%d,"+
		"setparams=color_primaries=bt709:color_trc=bt709:colorspace=bt709:range=tv:chroma_location=left",
		outW, outH, fmtIn, tm, outW, outH, fpsNum, fpsDen)
	af := "aresample=48000,aformat=channel_layouts=stereo"
	if it.GainDB != 0 {
		af += fmt.Sprintf(",volume=%.2fdB", it.GainDB)
	}
	af += ",apad"
	args := []string{"-hide_banner", "-nostdin", "-nostats", "-loglevel", "info",
		"-hwaccel", "vaapi", "-hwaccel_device", "/dev/dri/renderD128", "-hwaccel_output_format", "vaapi"}
	if it.Seek > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", it.Seek))
	}
	args = append(args, strings.Fields(*flagInOpts)...)
	args = append(args, "-i", it.File, "-map", "0:v:0", "-map", "0:a:0",
		"-t", fmt.Sprintf("%.3f", it.Dur+0.1), // a little over; the packager trims to the exact frame count
		"-vf", vf,
		"-c:v", "h264_vaapi", "-profile:v", "high", "-level", "4.1", "-bf", "0", "-g", fmt.Sprint(g), "-sei", "0")
	args = append(args, strings.Fields(*flagRC)...)
	args = append(args, "-af", af, "-c:a", "aac", "-b:a", "160k", "-ar", "48000", "-ac", "2",
		"-f", "mpegts", "-muxdelay", "0", "-muxpreload", "0", "pipe:1")
	return args
}

type au struct {
	video bool
	pts   int64
	nalus [][]byte // video
	frame []byte   // audio
}

type source struct {
	cmd       *exec.Cmd
	aus       chan au
	err       error
	asc       *tscodecs.MPEG4Audio
	firstAU   chan struct{}
	done      chan struct{}
	spawnedAt time.Time
	stderr    bytes.Buffer
}

// startSource spawns an encoder (or opens the slate file) and demuxes its TS into a bounded channel.
// The bounded channel is the back-pressure path: when the packager stops pulling, the pipe fills and
// ffmpeg blocks on write.
func startSource(it Item, g int, slate bool) *source {
	s := &source{aus: make(chan au, 32), firstAU: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(s.done)
		defer close(s.aus)
		if it.Delay > 0 {
			time.Sleep(time.Duration(it.Delay) * time.Millisecond)
		}
		s.spawnedAt = time.Now()
		var r io.Reader
		if slate {
			f, err := os.Open(*flagSlate)
			if err != nil {
				s.err = err
				return
			}
			defer f.Close()
			r = f
		} else {
			s.cmd = exec.Command("ffmpeg", encoderArgs(it, g)...)
			errPipe, _ := s.cmd.StderrPipe()
			go func() {
				sc := bufio.NewScanner(errPipe)
				for sc.Scan() {
					l := sc.Text()
					if strings.HasPrefix(l, "Input #0") { // printed after open + probe + seek
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
			r = out
			defer func() { io.Copy(io.Discard, out); s.cmd.Wait() }()
		}
		rd := &mpegts.Reader{R: bufio.NewReaderSize(r, 1<<16)}
		if err := rd.Initialize(); err != nil {
			s.err = err
			return
		}
		once := sync.Once{}
		for _, tr := range rd.Tracks() {
			switch c := tr.Codec.(type) {
			case *tscodecs.H264:
				rd.OnDataH264(tr, func(pts, _ int64, n [][]byte) error {
					once.Do(func() { close(s.firstAU) })
					s.aus <- au{video: true, pts: pts, nalus: n}
					return nil
				})
			case *tscodecs.MPEG4Audio:
				s.asc = c
				rd.OnDataMPEG4Audio(tr, func(pts int64, frames [][]byte) error {
					for i, f := range frames {
						s.aus <- au{pts: pts + int64(i*aacFrame*90000/sampleRate), frame: f}
					}
					return nil
				})
			}
		}
		for {
			if err := rd.Read(); err != nil {
				if !errors.Is(err, io.EOF) {
					s.err = err
				}
				return
			}
		}
	}()
	return s
}

type segment struct {
	Seq      int     `json:"seq"`
	Name     string  `json:"name"`
	Start    float64 `json:"start"` // channel seconds
	Dur      float64 `json:"dur"`
	Item     string  `json:"item"`
	WrittenS float64 `json:"written_s"`
}

type packager struct {
	vNext   int64 // next video PTS on the channel timeline
	aNext   int64 // next audio sample index on the channel timeline
	segDur  int64
	segs    []segment
	segMu   sync.Mutex
	sps     []byte
	pps     []byte
	asc     *tscodecs.MPEG4Audio
	ts      *mpegts.Writer
	tsV     *mpegts.Track
	tsA     *mpegts.Track
	initOut bool

	// current segment
	curV      []*fmp4.Sample
	curVStart int64
	curA      []*fmp4.Sample
	curAStart int64
	curItem   string
	seq       int
}

func (p *packager) chanSeconds(pts int64) float64 { return float64(pts-pts0) / ticksPerSec }

// pace blocks while the channel is more than runahead seconds ahead of wall clock.
func (p *packager) pace() {
	if *flagRunahead <= 0 {
		return
	}
	for {
		ahead := p.chanSeconds(p.vNext) - time.Since(t0).Seconds()
		if ahead <= *flagRunahead {
			return
		}
		time.Sleep(time.Duration((ahead - *flagRunahead) * float64(time.Second)))
	}
}

func (p *packager) writeInit() {
	init := fmp4.Init{Tracks: []*fmp4.InitTrack{
		{ID: 1, TimeScale: 90000, Codec: &mp4codecs.H264{SPS: p.sps, PPS: p.pps}},
		{ID: 2, TimeScale: sampleRate, Codec: &mp4codecs.MPEG4Audio{Config: p.asc.Config}},
	}}
	var b seekablebuffer.Buffer
	if err := init.Marshal(&b); err != nil {
		log.Fatal(err)
	}
	os.WriteFile(filepath.Join(*flagOut, "init.mp4"), b.Bytes(), 0o644)
	p.initOut = true
}

func (p *packager) flush() {
	if len(p.curV) == 0 {
		return
	}
	part := fmp4.Part{SequenceNumber: uint32(p.seq), Tracks: []*fmp4.PartTrack{
		{ID: 1, BaseTime: uint64(p.curVStart), Samples: p.curV},
	}}
	if len(p.curA) > 0 {
		part.Tracks = append(part.Tracks, &fmp4.PartTrack{ID: 2, BaseTime: uint64(p.curAStart), Samples: p.curA})
	}
	var b seekablebuffer.Buffer
	if err := part.Marshal(&b); err != nil {
		log.Fatal(err)
	}
	name := fmt.Sprintf("seg%05d.m4s", p.seq)
	tmp := filepath.Join(*flagOut, name+".tmp")
	os.WriteFile(tmp, b.Bytes(), 0o644)
	os.Rename(tmp, filepath.Join(*flagOut, name))
	sg := segment{Seq: p.seq, Name: name, Start: p.chanSeconds(p.curVStart),
		Dur: float64(len(p.curV)*frameDur) / ticksPerSec, Item: p.curItem, WrittenS: time.Since(t0).Seconds()}
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
	p.curV, p.curA = nil, nil
}

func isIDR(n [][]byte) bool {
	for _, x := range n {
		if len(x) > 0 && h264.NALUType(x[0]&0x1f) == h264.NALUTypeIDR {
			return true
		}
	}
	return false
}

// play packages one item onto the timeline. It returns when the item's slot is filled.
func (p *packager) play(it Item, s *source, slate bool) {
	nFrames := int64(math.Round(it.Dur * fpsNum / fpsDen))
	vBase := p.vNext
	vEnd := vBase + nFrames*frameDur
	aEndTarget := (vEnd - pts0) * sampleRate / 90000 // channel sample index of this item's video end
	aDriftStart := p.aNext - (vBase-pts0)*sampleRate/90000
	var nv, na, droppedPriming, droppedTail int64
	var firstSrcV int64 = -1
	var gaps int
	spsSame, ppsSame := true, true
	p.curItem = it.Name
	p.flush() // cut at item boundary: every item starts a segment on its IDR
	p.curVStart = vBase
	firstPkt := -1.0
	audioFull := func() bool { return p.aNext+aacFrame > aEndTarget+aacHalf }
	for nv < nFrames || !audioFull() {
		a, ok := <-s.aus
		if !ok {
			if slate { // loop the slate until the slot is filled
				s = startSource(Item{}, 0, true)
				firstSrcV = -1
				continue
			}
			break
		}
		if p.ts == nil && s.asc != nil { // first AU of the channel: tracks are known
			p.asc = s.asc
			p.tsV = &mpegts.Track{Codec: &tscodecs.H264{}}
			p.tsA = &mpegts.Track{Codec: p.asc}
			f, _ := os.Create(filepath.Join(*flagOut, "channel.ts"))
			p.ts = &mpegts.Writer{W: bufio.NewWriterSize(f, 1<<16), Tracks: []*mpegts.Track{p.tsV, p.tsA}}
			p.ts.Initialize()
		}
		if a.video && nv >= nFrames {
			continue // slot is full; the encoder ran a little long on purpose
		}
		if a.video {
			if firstPkt < 0 {
				firstPkt = float64(time.Since(s.spawnedAt).Microseconds()) / 1000
				ev("first_video", map[string]any{"item": it.Name, "since_spawn_ms": firstPkt})
			}
			if firstSrcV >= 0 && a.pts-firstSrcV != (nv-int64(0))*frameDur && !slate {
				gaps++
			}
			if firstSrcV < 0 {
				firstSrcV = a.pts - nv*frameDur
			}
			var body [][]byte
			for _, x := range a.nalus {
				switch h264.NALUType(x[0] & 0x1f) {
				case h264.NALUTypeSPS:
					if p.sps == nil {
						p.sps = x
					} else if !bytes.Equal(p.sps, x) {
						spsSame = false
						ev("sps_diff", map[string]any{"item": it.Name, "first": fmt.Sprintf("%x", p.sps), "this": fmt.Sprintf("%x", x)})
					}
				case h264.NALUTypePPS:
					if p.pps == nil {
						p.pps = x
					} else if !bytes.Equal(p.pps, x) {
						ppsSame = false
					}
				case h264.NALUTypeAccessUnitDelimiter:
				default:
					body = append(body, x)
				}
			}
			idr := isIDR(a.nalus)
			pts := vBase + nv*frameDur
			if idr && len(p.curV) > 0 && pts-p.curVStart >= p.segDur-frameDur/2 {
				p.flush()
				p.curVStart = pts
			}
			if len(p.curV) == 0 {
				p.curVStart = pts
			}
			smp, _ := fmp4.NewSampleH264(0, body)
			smp.Duration = frameDur
			smp.IsNonSyncSample = !idr
			p.curV = append(p.curV, smp)
			if !p.initOut && p.sps != nil && p.asc != nil {
				p.writeInit()
			}
			if p.ts != nil {
				p.ts.WriteH264(p.tsV, pts, pts, a.nalus)
			}
			nv++
			p.vNext = pts + frameDur
			p.pace()
			continue
		}
		// audio
		if na == 0 && droppedPriming == 0 && !slate {
			droppedPriming++ // the first AAC frame carries the encoder's 1024 priming samples
			continue
		}
		if audioFull() {
			droppedTail++
			continue
		}
		if len(p.curA) == 0 {
			p.curAStart = p.aNext + pts0*sampleRate/90000 // same origin as video
		}
		p.curA = append(p.curA, &fmp4.Sample{Duration: aacFrame, Payload: a.frame})
		if p.ts != nil {
			p.ts.WriteMPEG4Audio(p.tsA, pts0+p.aNext*90000/sampleRate, [][]byte{a.frame})
		}
		p.aNext += aacFrame
		na++
	}
	// the encoder may still be producing; draining audio past our slot is not needed
	go func() {
		for range s.aus {
		}
	}()
	// Keep the channel's frame count exact even if the encoder came up short.
	if nv < nFrames {
		ev("short_item", map[string]any{"item": it.Name, "want": nFrames, "got": nv})
		p.vNext = vBase + nv*frameDur
	}
	aDriftEnd := p.aNext - (p.vNext-pts0)*sampleRate/90000
	ev("item_done", map[string]any{"item": it.Name, "slate": slate, "first_video_ms": firstPkt,
		"start_s": p.chanSeconds(vBase), "dur_s": float64(nv*frameDur) / ticksPerSec,
		"frames": nv, "audio_frames": na, "dropped_priming": droppedPriming, "dropped_tail": droppedTail,
		"src_pts_gaps": gaps, "sps_identical": spsSame, "pps_identical": ppsSame,
		"av_drift_start_ms": float64(aDriftStart) * 1000 / sampleRate, "av_drift_end_ms": float64(aDriftEnd) * 1000 / sampleRate,
		"encoder_err": errString(s.err, s)})
}

func errString(err error, s *source) string {
	msg := strings.TrimSpace(s.stderr.String())
	if err != nil {
		msg = err.Error() + " " + msg
	}
	return msg
}

func (p *packager) playlist(w io.Writer, limit float64) int {
	p.segMu.Lock()
	defer p.segMu.Unlock()
	var list []segment
	for _, s := range p.segs {
		if limit > 0 && s.Start+s.Dur > limit {
			break
		}
		list = append(list, s)
	}
	tdur := int(math.Ceil(*flagSeg))
	for _, s := range list {
		tdur = max(tdur, int(math.Ceil(s.Dur)))
	}
	fmt.Fprintf(w, "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:%d\n", tdur)
	fmt.Fprintf(w, "#EXT-X-SERVER-CONTROL:HOLD-BACK=%g\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-INDEPENDENT-SEGMENTS\n#EXT-X-MAP:URI=\"init.mp4\"\n", *flagListahead)
	for _, s := range list {
		pdt := t0.Add(time.Duration(s.Start * float64(time.Second))).UTC().Format("2006-01-02T15:04:05.000Z")
		fmt.Fprintf(w, "#EXT-X-PROGRAM-DATE-TIME:%s\n#EXTINF:%.5f,\n%s\n", pdt, s.Dur, s.Name)
	}
	if limit < 0 {
		fmt.Fprintln(w, "#EXT-X-ENDLIST")
	}
	return len(list)
}

func main() {
	flag.Parse()
	if *flagCPUProf != "" {
		f, _ := os.Create(*flagCPUProf)
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}
	os.MkdirAll(*flagOut, 0o755)
	evFile, _ = os.Create(filepath.Join(*flagOut, "events.jsonl"))
	var items []Item
	b, err := os.ReadFile(*flagSched)
	if err != nil {
		log.Fatal(err)
	}
	if err := json.Unmarshal(b, &items); err != nil {
		log.Fatal(err)
	}
	g := int(math.Round(*flagSeg * fpsNum / fpsDen))
	p := &packager{vNext: pts0, segDur: int64(g) * frameDur}
	if *flagHTTP != "" {
		mux := http.NewServeMux()
		mux.HandleFunc("/live.m3u8", func(w http.ResponseWriter, r *http.Request) {
			// Hold the first manifest until ~4 s of media is listable (bounded wait).
			deadline := time.Now().Add(10 * time.Second)
			for {
				var buf bytes.Buffer
				n := p.playlist(&buf, time.Since(t0).Seconds()+*flagListahead)
				if n*int(*flagSeg) >= 4 || time.Now().After(deadline) {
					w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
					w.Header().Set("Access-Control-Allow-Origin", "*")
					w.Header().Set("Cache-Control", "no-cache")
					w.Write(buf.Bytes())
					return
				}
				time.Sleep(50 * time.Millisecond)
			}
		})
		fs := http.FileServer(http.Dir(*flagOut))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			fs.ServeHTTP(w, r)
		})
		go http.ListenAndServe(*flagHTTP, mux)
	}
	ev("start", map[string]any{"seg": *flagSeg, "g": g, "runahead": *flagRunahead, "items": len(items)})
	// Item encoders are started back-to-back: the next spawns once the current one's slot is packaged,
	// which (with run-ahead) is ~runahead seconds before its air time.
	next := startSource(items[0], g, false)
	var srcs []*source
	for i, it := range items {
		s := next
		air := t0.Add(time.Duration(p.chanSeconds(p.vNext) * float64(time.Second)))
		wait := time.Until(air.Add(-2 * time.Second))
		slate := false
		if *flagSlate != "" && i > 0 {
			select {
			case <-s.firstAU:
			case <-s.done:
			case <-time.After(max(wait, 0)):
			}
			select {
			case <-s.firstAU:
			default: // not ready by air-2s, or the encoder died before its first frame
				{
					<-time.After(50 * time.Millisecond)
					ev("slate_fallback", map[string]any{"item": it.Name, "wait_ms": wait.Milliseconds(), "stderr": s.stderr.String(), "err": fmt.Sprint(s.err)})
					if s.cmd != nil && s.cmd.Process != nil {
						s.cmd.Process.Kill()
					}
					s, slate = startSource(Item{}, g, true), true
				}
			}
		}
		p.play(it, s, slate)
		srcs = append(srcs, s)
		if i+1 < len(items) {
			next = startSource(items[i+1], g, false)
		}
	}
	p.flush()
	if p.ts != nil {
		p.ts.W.(*bufio.Writer).Flush()
	}
	f, _ := os.Create(filepath.Join(*flagOut, "vod.m3u8"))
	p.playlist(f, -1)
	f.Close()
	sj, _ := json.MarshalIndent(p.segs, "", " ")
	os.WriteFile(filepath.Join(*flagOut, "segments.json"), sj, 0o644)
	for _, s := range srcs {
		<-s.done // so every encoder's CPU lands in our rusage
	}
	ev("done", map[string]any{"segments": len(p.segs), "channel_s": p.chanSeconds(p.vNext)})
	time.Sleep(*flagLinger)
}
