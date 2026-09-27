package packager

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4/seekablebuffer"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
)

const testFPS = 30

// A real 64x64 baseline SPS/PPS (libx264), so mediacommon's init marshal accepts it.
var (
	testSPS = []byte{0x67, 0x42, 0xc0, 0x0a, 0xd9, 0x04, 0x26, 0xc0, 0x44, 0x00, 0x00, 0x03, 0x00, 0x04, 0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x48, 0x99, 0x20}
	testPPS = []byte{0x68, 0xcb, 0x83, 0xcb, 0x20}
)

func encodeInit(t testing.TB, sps []byte) []byte {
	t.Helper()
	init := fmp4.Init{Tracks: []*fmp4.InitTrack{
		{ID: videoTrack, TimeScale: videoRate, Codec: &codecs.H264{SPS: sps, PPS: testPPS}},
		{ID: audioTrack, TimeScale: audioRate, Codec: &codecs.MPEG4Audio{Config: mpeg4audio.AudioSpecificConfig{
			Type: mpeg4audio.ObjectTypeAACLC, SampleRate: audioRate, ChannelConfig: 2,
		}}},
	}}
	var w seekablebuffer.Buffer
	if err := init.Marshal(&w); err != nil {
		t.Fatal(err)
	}
	return w.Bytes()
}

// synth imitates ffmpeg's fMP4 output for one slot: timestamps from an arbitrary base (the
// packager must restamp), uneven first video durations, an AAC priming frame, one fragment per
// 1 s GOP, and over-production past the slot (ffmpeg's -frames is not exact; the caller over-asks).
type synth struct {
	label       string
	sps         []byte
	frames      int64 // total video frames produced; 0 = the slot's plus a margin
	extraAudio  int64
	zeroFrames  bool
	blockBefore <-chan struct{} // hold the first fragment until closed (or ctx ends)
	gate        chan struct{}   // if set, each fragment waits for one receive
	wrote       *atomic.Int64   // counts fragments written
	// audioJitter stamps AAC frames ±16 around 1024 samples (a Matroska-timed DTS source, live).
	audioJitter bool
}

func (s synth) encode(t testing.TB, ctx context.Context, slot Slot) io.ReadCloser {
	pr, pw := io.Pipe()
	sps := s.sps
	if sps == nil {
		sps = testSPS
	}
	frames := s.frames
	if frames == 0 {
		frames = slot.Frames + 3
	}
	go func() {
		defer func() { _ = pw.Close() }()
		if _, err := pw.Write(encodeInit(t, sps)); err != nil || s.zeroFrames {
			return
		}
		if s.blockBefore != nil {
			select {
			case <-s.blockBefore:
			case <-ctx.Done():
				return
			}
		}
		const d = videoRate / testFPS
		vBase, aBase := uint64(777777), uint64(55555)
		var a int64
		for f := int64(0); f < frames; f += testFPS {
			if s.gate != nil {
				select {
				case <-s.gate:
				case <-ctx.Done():
					return
				}
			}
			var vs, as []*fmp4.Sample
			for i := f; i < min(f+testFPS, frames); i++ {
				dur := uint32(d)
				switch i {
				case 0:
					dur = d + 1920
				case 1:
					dur = d - 1920
				}
				vs = append(vs, &fmp4.Sample{Duration: dur, IsNonSyncSample: i != f, Payload: []byte(fmt.Sprintf("%s/v%d", s.label, i))})
			}
			owed := (min(f+testFPS, frames)*d*audioRate/videoRate + aacFrame/2) / aacFrame
			owed++ // ffmpeg leads with one AAC priming frame
			if f+testFPS >= frames {
				owed += s.extraAudio
			}
			for ; a < owed; a++ {
				dur := uint32(aacFrame)
				if s.audioJitter {
					dur = uint32(aacFrame - 16 + 32*(a%2))
				}
				as = append(as, &fmp4.Sample{Duration: dur, Payload: []byte(fmt.Sprintf("%s/a%d", s.label, a))})
			}
			part := fmp4.Part{SequenceNumber: uint32(f), Tracks: []*fmp4.PartTrack{
				{ID: videoTrack, BaseTime: vBase + uint64(f*d), Samples: vs},
				{ID: audioTrack, BaseTime: aBase, Samples: as},
			}}
			for _, smp := range as {
				aBase += uint64(smp.Duration)
			}
			var w seekablebuffer.Buffer
			if err := part.Marshal(&w); err != nil {
				pw.CloseWithError(err)
				return
			}
			if s.wrote != nil {
				s.wrote.Add(1)
			}
			if _, err := pw.Write(w.Bytes()); err != nil {
				return
			}
		}
	}()
	go func() { <-ctx.Done(); pr.CloseWithError(ctx.Err()) }()
	return pr
}

func testSlate(t testing.TB) *Slate {
	t.Helper()
	pr := synth{label: "slate", frames: testFPS}.encode(t, context.Background(), Slot{Frames: testFPS})
	b, err := io.ReadAll(pr)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSlate(b)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

type plan struct {
	label string
	dur   time.Duration
	enc   synth
}

type harness struct {
	t      *testing.T
	p      *Packager
	dir    string
	opened sync.Map // label -> Slot
	cancel map[string]*atomic.Bool
}

// run packages plans in order (then slate-less idling: the schedule blocks) until every plan
// aired, with a clock anchored to real time plus skew.
func runPlans(t *testing.T, cfg Config, plans []plan) *harness {
	t.Helper()
	h := &harness{t: t, dir: t.TempDir(), cancel: map[string]*atomic.Bool{}}
	cfg.FPS, cfg.Dir = testFPS, h.dir
	if cfg.RunAhead == 0 {
		cfg.RunAhead = time.Hour // no pacing unless the test asks for it
	}
	for _, pl := range plans {
		h.cancel[pl.label] = &atomic.Bool{}
	}
	var i atomic.Int32
	finished := make(chan struct{})
	sched := func(ctx context.Context, _ time.Time) (Item, error) {
		n := int(i.Add(1)) - 1
		if n >= len(plans) {
			close(finished)
			<-ctx.Done()
			return Item{}, ctx.Err()
		}
		pl := plans[n]
		return Item{Label: pl.label, Duration: pl.dur, Open: func(ictx context.Context, slot Slot) (io.ReadCloser, error) {
			h.opened.Store(pl.label, slot)
			context.AfterFunc(ictx, func() { h.cancel[pl.label].Store(true) })
			return pl.enc.encode(t, ictx, slot), nil
		}}, nil
	}
	p, err := New(cfg, sched, ReadySlate(testSlate(t)))
	if err != nil {
		t.Fatal(err)
	}
	h.p = p
	start(t, p)
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("plans did not finish airing")
	}
	return h
}

// closeNotifier records that the packager closed an item's reader.
type closeNotifier struct {
	io.ReadCloser
	closed *atomic.Bool
}

func (c closeNotifier) Close() error {
	c.closed.Store(true)
	return c.ReadCloser.Close()
}

// An item reports the frames it put on the timeline (Item.Delivered) before its reader is closed:
// the whole slot when it fills it, fewer when its encoder ends early, and nothing when the channel
// stops mid-item. The encoder's cost sample divides by this, never by the slot (#1512).
func TestAirItemReportsTheFramesItDelivered(t *testing.T) {
	type report struct {
		frames      int64
		closedFirst bool
	}
	var mu sync.Mutex
	got := map[string]report{}
	gate := make(chan struct{})
	items := []plan{
		{"full", time.Second, synth{label: "full"}},
		{"short", 2 * time.Second, synth{label: "short", frames: 20}},
		{"stopped", time.Hour, synth{label: "stopped", gate: gate}},
	}
	var i atomic.Int32
	reached := make(chan struct{})
	sched := func(ctx context.Context, _ time.Time) (Item, error) {
		n := int(i.Add(1)) - 1
		if n >= len(items) {
			<-ctx.Done()
			return Item{}, ctx.Err()
		}
		pl := items[n]
		closed := &atomic.Bool{}
		return Item{Label: pl.label, Duration: pl.dur,
			Open: func(ictx context.Context, slot Slot) (io.ReadCloser, error) {
				if pl.label == "stopped" {
					close(reached)
				}
				return closeNotifier{ReadCloser: pl.enc.encode(t, ictx, slot), closed: closed}, nil
			},
			Delivered: func(frames int64) {
				mu.Lock()
				defer mu.Unlock()
				got[pl.label] = report{frames: frames, closedFirst: closed.Load()}
			},
		}, nil
	}
	p, err := New(Config{FPS: testFPS, Dir: t.TempDir(), RunAhead: time.Hour}, sched, ReadySlate(testSlate(t)))
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = p.Run(ctx) }()
	select {
	case <-reached:
	case <-time.After(10 * time.Second):
		stop()
		t.Fatal("the third item never opened")
	}
	// The stopped item is on the timeline before its channel stops: the pipe hands fragments over one
	// at a time, so synth taking its 4th gate means airItem has taken fragment 2, and so forwarded 1.
	for range 4 {
		gate <- struct{}{}
	}
	if p.Stats().Items != 3 {
		t.Fatalf("items aired = %d, want 3 before the stop", p.Stats().Items)
	}
	stop()
	<-done

	mu.Lock()
	defer mu.Unlock()
	want := map[string]report{"full": {frames: testFPS}, "short": {frames: 20}}
	if !maps.Equal(got, want) {
		t.Fatalf("delivered reports = %+v, want %+v (before each reader closed; none for the stopped channel's item)", got, want)
	}
}

type track struct {
	base    uint64
	samples []*fmp4.Sample
}

// segments parses every published segment in order.
func (h *harness) segments() (video, audio []track, seqs []uint32) {
	h.t.Helper()
	h.p.mu.Lock()
	segs := append([]segment(nil), h.p.window.segs...)
	h.p.mu.Unlock()
	for _, s := range segs {
		b, err := os.ReadFile(filepath.Join(h.dir, s.name))
		if err != nil {
			h.t.Fatal(err)
		}
		var parts fmp4.Parts
		if err := parts.Unmarshal(b); err != nil {
			h.t.Fatalf("%s: %v", s.name, err)
		}
		for _, part := range parts {
			seqs = append(seqs, part.SequenceNumber)
			for _, tr := range part.Tracks {
				if tr.ID == videoTrack {
					video = append(video, track{tr.BaseTime, tr.Samples})
				} else {
					audio = append(audio, track{tr.BaseTime, tr.Samples})
				}
			}
		}
	}
	return video, audio, seqs
}

// assertGapless checks the channel timeline: every video sample one frame long, every track
// fragment starting where the previous ended, from zero; sequence numbers strictly increasing.
func assertGapless(t *testing.T, h *harness) (payloads []string, audioFrames int) {
	video, audio, seqs := h.segments()
	t.Helper()
	var v, a uint64
	for i, tr := range video {
		if tr.base != v {
			t.Fatalf("video fragment %d tfdt %d, want %d (gap %d ticks)", i, tr.base, v, int64(tr.base)-int64(v))
		}
		for _, s := range tr.samples {
			if s.Duration != videoRate/testFPS || s.PTSOffset != 0 {
				t.Fatalf("video fragment %d sample duration %d offset %d", i, s.Duration, s.PTSOffset)
			}
			payloads = append(payloads, string(s.Payload))
			v += uint64(s.Duration)
		}
	}
	var vEnd uint64
	for i, tr := range audio {
		if tr.base != a {
			t.Fatalf("audio fragment %d tfdt %d, want %d", i, tr.base, a)
		}
		a += uint64(len(tr.samples)) * aacFrame
		// Every fragment boundary, not only the channel end, stays within one AAC frame of video.
		vEnd = video[i].base + uint64(len(video[i].samples))*videoRate/testFPS
		if diff := int64(a) - int64(vEnd)*audioRate/videoRate; diff > aacFrame || diff < -aacFrame {
			t.Fatalf("fragment %d: audio ends %d samples from video", i, diff)
		}
		audioFrames += len(tr.samples)
		for _, s := range tr.samples {
			if strings.HasSuffix(string(s.Payload), "/a0") && !strings.HasPrefix(string(s.Payload), "slate") {
				t.Fatalf("AAC priming frame %q was forwarded", s.Payload)
			}
		}
	}
	for i := 1; i < len(seqs); i++ {
		if seqs[i] != seqs[i-1]+1 {
			t.Fatalf("mfhd sequence %d follows %d", seqs[i], seqs[i-1])
		}
	}
	// Audio ends within half an AAC frame of video.
	if diff := int64(a) - int64(v)*audioRate/videoRate; diff > aacFrame/2 || diff < -aacFrame/2 {
		t.Fatalf("audio ends %d samples from video", diff)
	}
	return payloads, audioFrames
}

func countPrefix(payloads []string, prefix string) int {
	n := 0
	for _, p := range payloads {
		if strings.HasPrefix(p, prefix+"/") {
			n++
		}
	}
	return n
}

func TestStitchesItemsOntoOneGaplessTimeline(t *testing.T) {
	var wrote atomic.Int64
	h := runPlans(t, Config{}, []plan{
		{"prog", 2500 * time.Millisecond, synth{label: "prog"}},
		{"ad1", 1200 * time.Millisecond, synth{label: "ad1", extraAudio: 3}},
		{"ad2", 3 * time.Second, synth{label: "ad2", frames: 1 << 20, wrote: &wrote}}, // an encoder that never stops by itself
	})
	payloads, _ := assertGapless(t, h)
	// The encoder is stopped as soon as its slot fills: 3 fragments, plus what the pipe and the
	// reader hold, not the million frames it would go on producing.
	if n := wrote.Load(); n > 8 {
		t.Errorf("the endless encoder wrote %d fragments after a 3-fragment slot", n)
	}
	for label, want := range map[string]int{"prog": 75, "ad1": 36, "ad2": 90} {
		if got := countPrefix(payloads, label); got != want {
			t.Errorf("%s: %d frames on the timeline, want exactly %d", label, got, want)
		}
	}
	if s := h.p.Stats(); s.Items != 3 || s.Slates != 0 || s.Trimmed == 0 {
		t.Errorf("stats %+v: want 3 items, no slate, trimmed over-production", s)
	}
	// The slot handed to each encoder is its place on the timeline.
	if v, _ := h.opened.Load("ad1"); v.(Slot).Offset != 2500*time.Millisecond || v.(Slot).Frames != 36 {
		t.Errorf("ad1 slot %+v", v)
	}
	// The encoder is stopped as soon as its slot fills (one encoder per channel at a time).
	for label, c := range h.cancel {
		if !c.Load() {
			t.Errorf("%s encoder was not stopped", label)
		}
	}
}

func TestItemEndingShortDoesNotGap(t *testing.T) {
	// ad1 produces 20 of its 36 frames; the next item is placed at the real end.
	h := runPlans(t, Config{}, []plan{
		{"ad1", 1200 * time.Millisecond, synth{label: "ad1", frames: 20}},
		{"ad2", time.Second, synth{label: "ad2"}},
	})
	payloads, _ := assertGapless(t, h)
	if countPrefix(payloads, "ad1") != 20 || countPrefix(payloads, "ad2") != 30 {
		t.Fatalf("frames: ad1 %d ad2 %d", countPrefix(payloads, "ad1"), countPrefix(payloads, "ad2"))
	}
	if v, _ := h.opened.Load("ad2"); v.(Slot).Offset != 20*time.Second/testFPS {
		t.Errorf("ad2 placed at %v, want the short item's real end", v.(Slot).Offset)
	}
}

func TestLateItemFillsItsSlotWithSlate(t *testing.T) {
	never := make(chan struct{})
	h := runPlans(t, Config{SlateLead: 1500 * time.Millisecond}, []plan{
		{"prog", 2 * time.Second, synth{label: "prog"}},
		{"late", 2 * time.Second, synth{label: "late", blockBefore: never}},
		{"next", time.Second, synth{label: "next"}},
	})
	payloads, _ := assertGapless(t, h)
	if countPrefix(payloads, "late") != 0 || countPrefix(payloads, "slate") != 60 || countPrefix(payloads, "next") != 30 {
		t.Fatalf("late %d slate %d next %d", countPrefix(payloads, "late"), countPrefix(payloads, "slate"), countPrefix(payloads, "next"))
	}
	if s := h.p.Stats(); s.Late != 1 || s.Slates != 1 {
		t.Errorf("stats %+v", s)
	}
	if !h.cancel["late"].Load() {
		t.Error("the late encoder was not stopped")
	}
}

func TestZeroFrameEOFIsNotReady(t *testing.T) {
	h := runPlans(t, Config{}, []plan{
		{"prog", time.Second, synth{label: "prog"}},
		{"pastend", 1500 * time.Millisecond, synth{label: "pastend", zeroFrames: true}},
		{"next", time.Second, synth{label: "next"}},
	})
	payloads, _ := assertGapless(t, h)
	if countPrefix(payloads, "slate") != 45 || countPrefix(payloads, "next") != 30 {
		t.Fatalf("slate %d next %d", countPrefix(payloads, "slate"), countPrefix(payloads, "next"))
	}
	if s := h.p.Stats(); s.ZeroFrame != 1 {
		t.Errorf("stats %+v", s)
	}
}

func TestDecoderMismatchIsSlated(t *testing.T) {
	other := append([]byte(nil), testSPS...)
	other[3] = 0x1e // a different level: a different stsd
	h := runPlans(t, Config{}, []plan{
		{"prog", time.Second, synth{label: "prog"}},
		{"odd", time.Second, synth{label: "odd", sps: other}},
	})
	payloads, _ := assertGapless(t, h)
	if countPrefix(payloads, "odd") != 0 || countPrefix(payloads, "slate") != 30 {
		t.Fatalf("odd %d slate %d", countPrefix(payloads, "odd"), countPrefix(payloads, "slate"))
	}
	if s := h.p.Stats(); s.DecoderMismatch != 1 {
		t.Errorf("stats %+v", s)
	}
}

func TestLongSlateKeepsAudioInSync(t *testing.T) {
	h := runPlans(t, Config{SlateRetry: 2 * time.Minute}, []plan{
		{"pastend", 95 * time.Second, synth{label: "pastend", zeroFrames: true}},
	})
	payloads, _ := assertGapless(t, h) // asserts audio within half a frame of video
	if countPrefix(payloads, "slate") != 95*testFPS {
		t.Fatalf("slate frames %d", countPrefix(payloads, "slate"))
	}
}

// clock is real time plus an adjustable skew.
type clock struct{ skew atomic.Int64 }

func (c *clock) now() time.Time          { return time.Now().Add(time.Duration(c.skew.Load())) }
func (c *clock) advance(d time.Duration) { c.skew.Add(int64(d)) }
func (h *harness) listed(t *testing.T) []string {
	t.Helper()
	var names []string
	for _, l := range strings.Split(string(h.p.Playlist()), "\n") {
		if strings.HasSuffix(l, ".m4s") {
			names = append(names, l)
		}
	}
	return names
}

func TestListingGateAndFirstManifest(t *testing.T) {
	c := &clock{}
	h := runPlans(t, Config{Now: c.now}, []plan{{"prog", 20 * time.Second, synth{label: "prog"}}})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := h.p.AwaitPlaylist(ctx); err != nil {
		t.Fatal(err)
	}
	pl := string(h.p.Playlist())
	for _, want := range []string{"#EXT-X-SERVER-CONTROL:HOLD-BACK=6.0", `#EXT-X-MAP:URI="init.mp4"`, "#EXT-X-MEDIA-SEQUENCE:0", "#EXT-X-PROGRAM-DATE-TIME:"} {
		if !strings.Contains(pl, want) {
			t.Errorf("playlist lacks %q:\n%s", want, pl)
		}
	}
	// 20 s are packaged (no pacing), but only what ends by now + 6 s is listed.
	if n := len(h.listed(t)); n < 6 || n > 7 {
		t.Fatalf("listed %d segments at tune-in, want the 6 s gate", n)
	}
	c.advance(5 * time.Second)
	if n := len(h.listed(t)); n < 11 || n > 12 {
		t.Fatalf("listed %d segments 5 s later", n)
	}
}

func TestFirstManifestIsHeldUntilFourSeconds(t *testing.T) {
	gate := make(chan struct{})
	dir := t.TempDir()
	sched := func(ctx context.Context, _ time.Time) (Item, error) {
		return Item{Label: "prog", Duration: time.Minute, Open: func(ictx context.Context, slot Slot) (io.ReadCloser, error) {
			return synth{label: "prog", gate: gate}.encode(t, ictx, slot), nil
		}}, nil
	}
	p, err := New(Config{FPS: testFPS, Dir: dir, RunAhead: time.Hour}, sched, ReadySlate(testSlate(t)))
	if err != nil {
		t.Fatal(err)
	}
	ctx := start(t, p)
	for i := range 4 {
		wctx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
		err := p.AwaitPlaylist(wctx)
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("after %d s of media: AwaitPlaylist = %v, want held", i, err)
		}
		gate <- struct{}{}
	}
	wctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := p.AwaitPlaylist(wctx); err != nil {
		t.Fatalf("with 4 s of media: %v", err)
	}
}

func TestRunAheadBackPressure(t *testing.T) {
	dir := t.TempDir()
	var produced atomic.Int64
	sched := func(ctx context.Context, _ time.Time) (Item, error) {
		return Item{Label: "prog", Duration: time.Hour, Open: func(ictx context.Context, slot Slot) (io.ReadCloser, error) {
			return &countingReader{r: synth{label: "prog"}.encode(t, ictx, slot), n: &produced}, nil
		}}, nil
	}
	p, err := New(Config{FPS: testFPS, Dir: dir, RunAhead: 2 * time.Second}, sched, ReadySlate(testSlate(t)))
	if err != nil {
		t.Fatal(err)
	}
	start(t, p)
	time.Sleep(400 * time.Millisecond)
	p.mu.Lock()
	lead := ticks(p.v) - time.Since(p.epoch)
	p.mu.Unlock()
	// The pipe and reader buffer a little, but the timeline must not race ahead of 2 s + one segment.
	if lead > 3*time.Second+100*time.Millisecond || lead < time.Second {
		t.Fatalf("timeline leads the wall clock by %v with a 2 s run-ahead", lead)
	}
}

type countingReader struct {
	r io.ReadCloser
	n *atomic.Int64
}

func (c *countingReader) Read(b []byte) (int, error) {
	n, err := c.r.Read(b)
	c.n.Add(int64(n))
	return n, err
}
func (c *countingReader) Close() error { return c.r.Close() }

func TestDVRWindowPrunesSegments(t *testing.T) {
	h := runPlans(t, Config{DVR: 3 * time.Second}, []plan{{"prog", 10 * time.Second, synth{label: "prog"}}})
	h.p.mu.Lock()
	n := len(h.p.window.segs)
	h.p.mu.Unlock()
	files, _ := filepath.Glob(filepath.Join(h.dir, "seg*.m4s"))
	if n > 4 || len(files) != n {
		t.Fatalf("window %d segments, %d files on disk; want ≤ 4 and equal", n, len(files))
	}
}

func TestPatchFragmentRestampsInPlace(t *testing.T) {
	r := synth{label: "x", frames: testFPS}.encode(t, context.Background(), Slot{})
	stream := readStream(r)
	frag := <-stream.frags
	c, err := patchFragment(frag, 42, map[uint32]uint64{videoTrack: 9000, audioTrack: 4800})
	if err != nil {
		t.Fatal(err)
	}
	if c[videoTrack] != testFPS {
		t.Fatalf("counts %v", c)
	}
	var parts fmp4.Parts
	if err := parts.Unmarshal(frag); err != nil {
		t.Fatal(err)
	}
	if parts[0].SequenceNumber != 42 || parts[0].Tracks[0].BaseTime != 9000 || parts[0].Tracks[1].BaseTime != 4800 {
		t.Fatalf("seq %d bases %d/%d", parts[0].SequenceNumber, parts[0].Tracks[0].BaseTime, parts[0].Tracks[1].BaseTime)
	}
	if _, err := patchFragment([]byte("\x00\x00\x00\x08mdat"), 0, nil); err == nil {
		t.Fatal("a fragment not led by moof was accepted")
	}
	if _, _, err := readBox(bufio.NewReader(bytes.NewReader([]byte("\xff\xff\xff\xffmoof")))); !errors.Is(err, errBadBox) {
		t.Fatalf("oversized box: %v", err)
	}
}

// start runs p until the test ends and waits for Run to return before the test's TempDir goes.
func start(t *testing.T, p *Packager) context.Context {
	t.Helper()
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = p.Run(ctx) }()
	t.Cleanup(func() { stop(); <-done })
	return ctx
}

// testdata/ffmpeg-fragmented.mp4 is real ffmpeg output (-movflags
// empty_moov+default_base_moof+frag_keyframe, 33 frames of libx264 + AAC). ffmpeg flags the IDR
// only through trun's first_sample_flags over a non-sync tfhd default, which mediacommon's
// unmarshal ignores: every re-marshalled item start would go out labelled non-sync.
func TestRewriteKeepsFFmpegIDRSync(t *testing.T) {
	raw, err := os.ReadFile("testdata/ffmpeg-fragmented.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewSlate(raw); err != nil {
		t.Fatalf("slate from real ffmpeg output: %v", err)
	}
	stream := readStream(bytes.NewReader(raw))
	frag := <-stream.frags
	out, c, err := rewriteFragment(frag, 7, map[uint32]uint64{videoTrack: 0, audioTrack: 0}, true, 3000,
		map[uint32]int64{videoTrack: 1 << 20, audioTrack: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	var parts fmp4.Parts
	if err := parts.Unmarshal(out); err != nil {
		t.Fatal(err)
	}
	v := parts[0].Tracks[0]
	if c[videoTrack] != 30 || v.Samples[0].IsNonSyncSample || !v.Samples[1].IsNonSyncSample {
		t.Fatalf("re-marshalled first fragment: %d frames, sample 0 non-sync %v, sample 1 non-sync %v",
			c[videoTrack], v.Samples[0].IsNonSyncSample, v.Samples[1].IsNonSyncSample)
	}
}

// A programme whose encoder is late is slated for one retry, not its whole slot: the schedule is
// asked again and the programme rejoins in progress (live, a 3 s slow start blanked a 2.5 h film).
func TestLateLongProgrammeRejoinsAfterOneRetry(t *testing.T) {
	never := make(chan struct{})
	h := runPlans(t, Config{SlateRetry: 2 * time.Second, SlateLead: 1500 * time.Millisecond}, []plan{
		{"prog", time.Second, synth{label: "prog"}},
		{"film", 2 * time.Hour, synth{label: "film", blockBefore: never}},
		{"film-rejoined", 3 * time.Second, synth{label: "rejoined"}},
	})
	payloads, _ := assertGapless(t, h)
	if countPrefix(payloads, "slate") != 60 || countPrefix(payloads, "rejoined") != 90 {
		t.Fatalf("slate %d rejoined %d: a late programme must slate one retry, then rejoin",
			countPrefix(payloads, "slate"), countPrefix(payloads, "rejoined"))
	}
	if v, _ := h.opened.Load("film-rejoined"); v.(Slot).Offset != 3*time.Second {
		t.Errorf("rejoined at %v, want the retry's end (3 s)", v.(Slot).Offset)
	}
}

func testSlateWith(t testing.TB, sps []byte) *Slate {
	t.Helper()
	b, err := io.ReadAll(synth{label: "slate", sps: sps, frames: testFPS}.encode(t, context.Background(), Slot{Frames: testFPS}))
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSlate(b)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// A channel that slates at tune-in (nothing airs, or the first item is late) must not publish the
// slate's decoder configuration as the channel's: only a real item defines the init, and the first
// manifest waits for it (live, a slate init became the channel's and every item after it was
// refused as a mismatch).
func TestFirstManifestWaitsForARealItem(t *testing.T) {
	sched := func(ctx context.Context, _ time.Time) (Item, error) { return Item{}, errors.New("nothing airs") }
	p, err := New(Config{FPS: testFPS, Dir: t.TempDir(), RunAhead: time.Hour}, sched, ReadySlate(testSlate(t)))
	if err != nil {
		t.Fatal(err)
	}
	ctx := start(t, p)
	wctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	if err := p.AwaitPlaylist(wctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("AwaitPlaylist with only slate = %v, want held", err)
	}
	if p.Stats().Slates == 0 {
		t.Fatal("the packager never slated")
	}
	if p.Init() != nil {
		t.Fatal("the slate defined the channel init")
	}
}

func TestSlateNeverDefinesTheChannelInit(t *testing.T) {
	slateSPS := append([]byte(nil), testSPS...)
	slateSPS[3] = 0x1e
	late := make(chan struct{})
	var calls atomic.Int32
	sched := func(ctx context.Context, _ time.Time) (Item, error) {
		enc := synth{label: "prog"}
		if calls.Add(1) == 1 {
			enc.blockBefore = late // the tune-in item misses FirstItemWait: slate first
		}
		return Item{Label: "prog", Duration: time.Minute, Open: func(ictx context.Context, slot Slot) (io.ReadCloser, error) {
			return enc.encode(t, ictx, slot), nil
		}}, nil
	}
	p, err := New(Config{FPS: testFPS, Dir: t.TempDir(), RunAhead: time.Hour, FirstItemWait: 50 * time.Millisecond,
		SlateRetry: time.Second}, sched, ReadySlate(testSlateWith(t, slateSPS)))
	if err != nil {
		t.Fatal(err)
	}
	ctx := start(t, p)
	wctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := p.AwaitPlaylist(wctx); err != nil {
		t.Fatalf("AwaitPlaylist: %v", err)
	}
	if !SameDecoderConfig(p.Init(), encodeInit(t, testSPS)) {
		t.Fatal("the channel init is not the first real item's")
	}
	if s := p.Stats(); s.Items == 0 || s.Slates == 0 {
		t.Fatalf("stats %+v: want a tune-in slate, then the item on air", s)
	}
}

// With the slate barred from the init, a tune-in slate only delays the first manifest: a first item
// that is slow to produce (a cold HDR tone-map took over 3 s live) is waited for, not slated,
// re-resolved and re-spawned.
func TestSlowTuneInItemIsWaitedFor(t *testing.T) {
	slow := make(chan struct{})
	time.AfterFunc(3500*time.Millisecond, func() { close(slow) })
	var opens atomic.Int32
	sched := func(ctx context.Context, _ time.Time) (Item, error) {
		return Item{Label: "prog", Duration: time.Minute, Open: func(ictx context.Context, slot Slot) (io.ReadCloser, error) {
			opens.Add(1)
			return synth{label: "prog", blockBefore: slow}.encode(t, ictx, slot), nil
		}}, nil
	}
	p, err := New(Config{FPS: testFPS, Dir: t.TempDir(), RunAhead: time.Hour}, sched, ReadySlate(testSlate(t)))
	if err != nil {
		t.Fatal(err)
	}
	ctx := start(t, p)
	wctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	if err := p.AwaitPlaylist(wctx); err != nil {
		t.Fatal(err)
	}
	if s := p.Stats(); s.Slates != 0 || s.Late != 0 || opens.Load() != 1 {
		t.Fatalf("stats %+v, opens %d: want the tune-in item waited for, once", s, opens.Load())
	}
}

// The slate is fetched only when a slot needs it (#1512 G2): a first item that produces airs while
// the slate is still encoding, and the packager never waits on it.
func TestFirstItemAirsWhileTheSlateIsEncoding(t *testing.T) {
	sched := func(ctx context.Context, _ time.Time) (Item, error) {
		return Item{Label: "prog", Duration: time.Minute, Open: func(ictx context.Context, slot Slot) (io.ReadCloser, error) {
			return synth{label: "prog"}.encode(t, ictx, slot), nil
		}}, nil
	}
	encoding := func(ctx context.Context) (*Slate, error) { <-ctx.Done(); return nil, ctx.Err() }
	p, err := New(Config{FPS: testFPS, Dir: t.TempDir(), RunAhead: time.Hour}, sched, encoding)
	if err != nil {
		t.Fatal(err)
	}
	ctx := start(t, p)
	wctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := p.AwaitPlaylist(wctx); err != nil {
		t.Fatalf("AwaitPlaylist while the slate encodes: %v", err)
	}
	if s := p.Stats(); s.Slates != 0 || s.Items == 0 {
		t.Fatalf("stats %+v: want the item on air and no slate", s)
	}
}

// Every AAC frame is 1024 samples, and the packager's audio timeline counts them so (tfdt steps by
// aacFrame). Live, an encoder fed a Matroska-timed DTS source stamped its frames 1008..1080 apart:
// forwarded as-is, each fragment's sample durations disagreed with its own tfdt grid (ffprobe:
// ~20 of 47 audio DTS deltas per segment were not 1024).
func TestAudioSamplesAreOneAACFrameEach(t *testing.T) {
	h := runPlans(t, Config{}, []plan{
		{"dts", 2500 * time.Millisecond, synth{label: "dts", audioJitter: true}},
		{"next", time.Second, synth{label: "next", audioJitter: true}},
	})
	assertGapless(t, h)
	_, audio, _ := h.segments()
	for i, tr := range audio {
		for j, s := range tr.samples {
			if s.Duration != aacFrame {
				t.Fatalf("audio fragment %d sample %d lasts %d, want one AAC frame (%d)", i, j, s.Duration, aacFrame)
			}
		}
	}
}

// An airing's rounded slot can end up to half a frame before the schedule's boundary; the lookup at
// the timeline's end then finds the same airing with no whole frame left. Live (boundary harness,
// 60 s items), that aired one frame of slate, in its own 848-byte segment, at every boundary. The
// packager asks for what airs at the airing's end instead.
func TestSubFrameRemainderRollsIntoTheNextItem(t *testing.T) {
	t0 := time.Unix(1_000_000, 0)
	boundary := t0.Add(1010 * time.Millisecond) // 25.25 frames: the first slot rounds to 25
	sched := func(ctx context.Context, at time.Time) (Item, error) {
		label, d := "b", time.Hour
		if at.Before(boundary) {
			label, d = "a", boundary.Sub(at)
		}
		return Item{Label: label, Duration: d, Open: func(ictx context.Context, slot Slot) (io.ReadCloser, error) {
			return synth{label: label}.encode(t, ictx, slot), nil
		}}, nil
	}
	p, err := New(Config{FPS: testFPS, Dir: t.TempDir(), RunAhead: time.Hour, Now: func() time.Time { return t0 }},
		sched, ReadySlate(testSlate(t)))
	if err != nil {
		t.Fatal(err)
	}
	start(t, p)
	deadline := time.Now().Add(3 * time.Second)
	for p.Stats().Items < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if s := p.Stats(); s.Items < 2 || s.Slates != 0 {
		t.Fatalf("stats %+v: want both items aired back to back, no slate frame between them", s)
	}
}

// A tune-in that lands in an airing's last half frame skips it without airing anything, so the next
// item is still the tune-in item: it gets FirstItemWait, not the lead a later item needs.
func TestTuneInOnASubFrameRemainderKeepsTheTuneInWait(t *testing.T) {
	t0 := time.Now()
	boundary := t0.Add(10 * time.Millisecond) // under half a frame at testFPS
	slow := make(chan struct{})
	time.AfterFunc(300*time.Millisecond, func() { close(slow) })
	sched := func(ctx context.Context, at time.Time) (Item, error) {
		if at.Before(boundary) {
			return Item{Label: "a", Duration: boundary.Sub(at), Open: func(ictx context.Context, slot Slot) (io.ReadCloser, error) {
				return synth{label: "a"}.encode(t, ictx, slot), nil
			}}, nil
		}
		return Item{Label: "b", Duration: time.Hour, Open: func(ictx context.Context, slot Slot) (io.ReadCloser, error) {
			return synth{label: "b", blockBefore: slow}.encode(t, ictx, slot), nil
		}}, nil
	}
	p, err := New(Config{FPS: testFPS, Dir: t.TempDir(), RunAhead: time.Hour, Now: func() time.Time { return t0 }},
		sched, ReadySlate(testSlate(t)))
	if err != nil {
		t.Fatal(err)
	}
	ctx := start(t, p)
	wctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	if err := p.AwaitPlaylist(wctx); err != nil {
		t.Fatal(err)
	}
	if s := p.Stats(); s.Slates != 0 || s.Late != 0 {
		t.Fatalf("stats %+v: want the slow item after the skipped remainder waited for, not slated", s)
	}
}

// A channel's formats share one flat asset namespace (#1512 phase 2b: one packager per format), so
// each packager's playlist names its files with its format's prefix; the files keep bare names.
func TestPlaylistURIsCarryThePrefix(t *testing.T) {
	h := runPlans(t, Config{URIPrefix: "1080p-h264-sdr-", ListAhead: time.Hour}, []plan{
		{"prog", 2 * time.Second, synth{label: "prog"}},
	})
	pl := string(h.p.Playlist())
	if !strings.Contains(pl, `#EXT-X-MAP:URI="1080p-h264-sdr-init.mp4"`) {
		t.Fatalf("init URI not prefixed:\n%s", pl)
	}
	segs := 0
	for _, line := range strings.Split(pl, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, "1080p-h264-sdr-seg") {
			t.Fatalf("segment URI %q not prefixed", line)
		}
		if _, err := os.Stat(filepath.Join(h.dir, strings.TrimPrefix(line, "1080p-h264-sdr-"))); err != nil {
			t.Fatalf("segment file: %v", err)
		}
		segs++
	}
	if segs == 0 {
		t.Fatalf("no segments listed:\n%s", pl)
	}
}

func TestCodecsAttrReadsTheInit(t *testing.T) {
	want := fmt.Sprintf("avc1.%02x%02x%02x,mp4a.40.2", testSPS[1], testSPS[2], testSPS[3])
	if got := CodecsAttr(encodeInit(t, testSPS)); got != want {
		t.Fatalf("CodecsAttr = %q, want %q", got, want)
	}
	if got := CodecsAttr([]byte("not an init")); got != "" {
		t.Fatalf("CodecsAttr of junk = %q, want empty", got)
	}
}

// A premium variant's CODECS is what a web player checks with MediaSource.isTypeSupported before it
// picks the variant (#1512 G10), so it must be the init's own: the sample entry's 4CC and the SPS's
// profile, compatibility flags, tier, level and constraint bytes (ISO/IEC 14496-15 E.3). The fixture
// is libx265 Main 10 at level 1 (ffprobe: hvc1, Main 10, level 30); its own hvcC box carries
// compatibility 0x20000000 (flag 2 only, so 4 reversed) and constraint bytes 90 00 00 00 00 00
// (progressive, frame-only).
func TestCodecsAttrNamesAnHEVCInit(t *testing.T) {
	init, _ := hevcFixture(t)
	if got, want := CodecsAttr(init), "hvc1.2.4.L30.90,mp4a.40.2"; got != want {
		t.Fatalf("CodecsAttr(HEVC Main 10 init) = %q, want %q", got, want)
	}
	// The HDR10 boxes the packager adds to the served init leave its CODECS alone.
	boxed, err := testHDR10.init(init)
	if err != nil {
		t.Fatal(err)
	}
	if got := CodecsAttr(boxed); got != CodecsAttr(init) {
		t.Fatalf("CodecsAttr(HDR10 init) = %q, want the raw init's %q", got, CodecsAttr(init))
	}
}
