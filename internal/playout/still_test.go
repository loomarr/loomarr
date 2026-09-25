package playout

import (
	"bytes"
	"context"
	"errors"
	"image/jpeg"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeStillSource serves whatever "newest segment" the test sets, like a live edge that moves.
type fakeStillSource struct {
	seg stillSegment
	ok  bool
	err error
}

func (f *fakeStillSource) newestStillSegment(context.Context, string, EncodePlan) (stillSegment, bool, error) {
	return f.seg, f.ok, f.err
}

func segment(key, body string, at time.Time) stillSegment {
	return stillSegment{
		key:   key,
		at:    at,
		parts: []func() (io.ReadCloser, error){func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(body)), nil }},
	}
}

// countingExtractor records each decode and echoes the media it was handed, so a test can tell
// WHICH segment a still came from.
func countingExtractor(calls *atomic.Int32) StillExtractor {
	return func(_ context.Context, media io.Reader) ([]byte, error) {
		calls.Add(1)
		b, err := io.ReadAll(media)
		return []byte("jpeg:" + string(b)), err
	}
}

func TestOriginStill_DecodesOncePerSegmentAndTracksTheLiveEdge(t *testing.T) {
	var calls atomic.Int32
	src := &fakeStillSource{ok: true, seg: segment("seg-1", "one", time.Unix(100, 0))}
	o := newOrigin(nil, nil, nil)
	o.stillSources = []stillSource{src}
	o.stillExtractor = countingExtractor(&calls)

	for range 5 { // five requests inside one segment interval
		got, ok, err := o.Still(context.Background(), "ch1", PlanBaseline)
		if err != nil || !ok || string(got.JPEG) != "jpeg:one" {
			t.Fatalf("Still = %q ok=%v err=%v, want jpeg:one", got.JPEG, ok, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("decoded %d times for one segment, want 1 (a request must never decode)", calls.Load())
	}

	// The live edge moves: the next request must show the NEW segment, not the cached one.
	src.seg = segment("seg-2", "two", time.Unix(104, 0))
	got, _, _ := o.Still(context.Background(), "ch1", PlanBaseline)
	if string(got.JPEG) != "jpeg:two" || !got.At.Equal(time.Unix(104, 0)) {
		t.Fatalf("after the edge moved got %q at %v, want jpeg:two at 104", got.JPEG, got.At)
	}
	if calls.Load() != 2 {
		t.Fatalf("decoded %d times across two segments, want 2", calls.Load())
	}
}

func TestOriginStill_NoSegmentIsAMissNotAnError(t *testing.T) {
	var calls atomic.Int32
	o := newOrigin(nil, nil, nil)
	o.stillSources = []stillSource{&fakeStillSource{ok: false}}
	o.stillExtractor = countingExtractor(&calls)

	if _, ok, err := o.Still(context.Background(), "ch1", PlanBaseline); ok || err != nil {
		t.Fatalf("ok=%v err=%v, want a clean miss", ok, err)
	}
	if calls.Load() != 0 {
		t.Fatal("decoded with no segment")
	}
}

func TestOriginStill_FallsThroughToTheNextSource(t *testing.T) {
	var calls atomic.Int32
	o := newOrigin(nil, nil, nil)
	o.stillSources = []stillSource{
		&fakeStillSource{err: errors.New("prepared broke")},
		&fakeStillSource{ok: true, seg: segment("live-1", "live", time.Unix(1, 0))},
	}
	o.stillExtractor = countingExtractor(&calls)

	got, ok, err := o.Still(context.Background(), "ch1", PlanBaseline)
	if err != nil || !ok || string(got.JPEG) != "jpeg:live" {
		t.Fatalf("got %q ok=%v err=%v, want the live source's frame", got.JPEG, ok, err)
	}
}

func TestOriginStill_NoExtractorMeansNoStills(t *testing.T) {
	o := newOrigin(nil, nil, nil)
	o.stillSources = []stillSource{&fakeStillSource{ok: true, seg: segment("s", "x", time.Now())}}
	if _, ok, err := o.Still(context.Background(), "ch1", PlanBaseline); ok || err != nil {
		t.Fatalf("ok=%v err=%v, want a clean miss when ffmpeg is not wired", ok, err)
	}
}

func TestSegmentAtEdge_PicksTheSegmentHoldingTheOffset(t *testing.T) {
	media := preparedMedia{segments: []preparedSegment{
		{duration: 4 * time.Second, uri: "a"}, {duration: 4 * time.Second, uri: "b"}, {duration: 4 * time.Second, uri: "c"},
	}}
	for offset, want := range map[time.Duration]string{0: "a", 3999 * time.Millisecond: "a", 4 * time.Second: "b", 11 * time.Second: "c"} {
		media.airing.Offset = offset
		if got, ok := segmentAtEdge(media); !ok || got.uri != want {
			t.Errorf("offset %v → %q ok=%v, want %q", offset, got.uri, ok, want)
		}
	}
	media.airing.Offset = 12 * time.Second
	if _, ok := segmentAtEdge(media); ok {
		t.Error("an offset past the media must be a miss, not the last segment")
	}
}

func TestHLSStill_NewestCompletedSegmentPlusInit(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("master.m3u8", "#EXTM3U\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXTINF:4.0,\nseg-1.m4s\n#EXTINF:4.0,\nseg-2.m4s\n")
	write("init.mp4", "INIT")
	write("seg-1.m4s", "ONE")
	write("seg-2.m4s", "TWO")
	write("seg-3.m4s", "PARTIAL") // being written: not in the playlist yet, must never be picked
	m := &HLSManager{remuxes: map[remuxKey]*hlsRemux{
		{channel: "ch1", plan: PlanBaseline}: {dir: dir, playlist: filepath.Join(dir, "master.m3u8")},
	}}

	seg, ok, err := m.newestStillSegment(context.Background(), "ch1", PlanBaseline)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	var got strings.Builder
	for _, open := range seg.parts {
		rc, err := open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		got.Write(b)
	}
	if got.String() != "INITTWO" {
		t.Fatalf("stream = %q, want INITTWO (init + newest COMPLETED segment)", got.String())
	}
	if _, ok, _ := m.newestStillSegment(context.Background(), "other", PlanBaseline); ok {
		t.Fatal("a channel with no remux must be a miss")
	}
}

// The production extractor against a real segment: one H.264 MPEG-TS in, one downscaled JPEG out.
// Skipped without ffmpeg; CI's playout job has it.
func TestFFmpegStill_DecodesARealSegment(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	segment, err := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc=size=1920x1080:rate=25:duration=1",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-f", "mpegts", "pipe:1").Output()
	if err != nil || len(segment) == 0 {
		t.Fatalf("building the fixture segment: %v", err)
	}

	jpg, err := FFmpegStill(ffmpeg)(context.Background(), bytes.NewReader(segment))
	if err != nil {
		t.Fatal(err)
	}
	if len(jpg) < 4 || jpg[0] != 0xff || jpg[1] != 0xd8 {
		t.Fatalf("output is not a JPEG: % x…", jpg[:min(len(jpg), 4)])
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(jpg))
	if err != nil || cfg.Width != stillWidth {
		t.Fatalf("decoded %+v err=%v, want width %d (downscaled from 1920)", cfg, err, stillWidth)
	}

	if _, err := FFmpegStill(ffmpeg)(context.Background(), strings.NewReader("not media")); err == nil {
		t.Fatal("garbage input must be an error (a miss), not an empty still")
	}
}

// gatedExtractor blocks every decode until release is closed, and tracks how many run at once.
type gatedExtractor struct {
	release chan struct{}
	started chan struct{}
	calls   atomic.Int32
	running atomic.Int32
	peak    atomic.Int32
}

func newGatedExtractor() *gatedExtractor {
	return &gatedExtractor{release: make(chan struct{}), started: make(chan struct{}, 64)}
}

func (g *gatedExtractor) extract(_ context.Context, media io.Reader) ([]byte, error) {
	g.calls.Add(1)
	now := g.running.Add(1)
	for {
		peak := g.peak.Load()
		if now <= peak || g.peak.CompareAndSwap(peak, now) {
			break
		}
	}
	g.started <- struct{}{}
	<-g.release
	g.running.Add(-1)
	b, _ := io.ReadAll(media)
	return []byte("jpeg:" + string(b)), nil
}

func TestOriginStill_ConcurrentMissesShareOneDecode(t *testing.T) {
	gate := newGatedExtractor()
	o := newOrigin(nil, nil, nil)
	o.stillSources = []stillSource{&fakeStillSource{ok: true, seg: segment("seg-1", "one", time.Unix(100, 0))}}
	o.stillExtractor = gate.extract

	const requests = 8
	var wg sync.WaitGroup
	results := make([]string, requests)
	for i := range requests {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, ok, err := o.Still(context.Background(), "ch1", PlanBaseline)
			if err != nil || !ok {
				t.Errorf("request %d: ok=%v err=%v", i, ok, err)
			}
			results[i] = string(got.JPEG)
		}()
	}
	<-gate.started // the one decode is running; the rest must be waiting on it, not spawning
	time.Sleep(50 * time.Millisecond)
	close(gate.release)
	wg.Wait()

	if gate.calls.Load() != 1 {
		t.Fatalf("%d concurrent misses ran %d decodes, want 1 (single-flight per channel+segment)", requests, gate.calls.Load())
	}
	for i, r := range results {
		if r != "jpeg:one" {
			t.Fatalf("request %d got %q", i, r)
		}
	}
}

func TestOriginStill_DecodeBoundNeverQueuesAPileOfFFmpeg(t *testing.T) {
	gate := newGatedExtractor()
	now := time.Unix(1_000, 0)
	o := newOrigin(nil, nil, nil)
	o.stillExtractor = gate.extract
	o.stillClock = func() time.Time { return now }

	// A previous frame for ch-old exists from a moment ago; ch-cold has never had one.
	o.stills.entries = map[string]stillEntry{
		"ch-old":     {key: "old-seg", still: Still{JPEG: []byte("previous"), At: now.Add(-6 * time.Second)}},
		"ch-ancient": {key: "old-seg", still: Still{JPEG: []byte("ancient"), At: now.Add(-30 * time.Second)}},
	}
	sources := map[string]*fakeStillSource{}
	for _, id := range []string{"ch1", "ch2", "ch-old", "ch-cold", "ch-ancient"} {
		sources[id] = &fakeStillSource{ok: true, seg: segment(id+"-new", id, now)}
	}
	o.stillSources = []stillSource{stillSourceFunc(func(_ context.Context, id string, _ EncodePlan) (stillSegment, bool, error) {
		s := sources[id]
		return s.seg, s.ok, nil
	})}

	// Two decodes take both slots and hold them.
	var wg sync.WaitGroup
	for _, id := range []string{"ch1", "ch2"} {
		wg.Add(1)
		go func() { defer wg.Done(); _, _, _ = o.Still(context.Background(), id, PlanBaseline) }()
	}
	<-gate.started
	<-gate.started

	// Past the bound: recent previous frame is served, otherwise a clean miss — and no new ffmpeg.
	got, ok, err := stillWithin(t, o, "ch-old")
	if err != nil || !ok || string(got.JPEG) != "previous" {
		t.Fatalf("recent previous frame: got %q ok=%v err=%v, want the previous frame", got.JPEG, ok, err)
	}
	if _, ok, err := stillWithin(t, o, "ch-ancient"); ok || err != nil {
		t.Fatalf("stale previous frame: ok=%v err=%v, want a clean miss (too old to stand in)", ok, err)
	}
	if _, ok, err := stillWithin(t, o, "ch-cold"); ok || err != nil {
		t.Fatalf("no previous frame: ok=%v err=%v, want a clean miss", ok, err)
	}
	if gate.calls.Load() != 2 || gate.peak.Load() != 2 {
		t.Fatalf("decodes=%d peak=%d, want exactly the 2 allowed", gate.calls.Load(), gate.peak.Load())
	}

	close(gate.release)
	wg.Wait()
}

// stillSourceFunc adapts a function to stillSource, for tests that need per-channel segments.
type stillSourceFunc func(context.Context, string, EncodePlan) (stillSegment, bool, error)

func (f stillSourceFunc) newestStillSegment(ctx context.Context, id string, plan EncodePlan) (stillSegment, bool, error) {
	return f(ctx, id, plan)
}

// stillWithin asks for a still and fails the test if the call blocks: past the decode bound a
// request must answer at once, never wait behind (or start) another ffmpeg.
func stillWithin(t *testing.T, o *Origin, id string) (Still, bool, error) {
	t.Helper()
	type answer struct {
		still Still
		ok    bool
		err   error
	}
	done := make(chan answer, 1)
	go func() {
		s, ok, err := o.Still(context.Background(), id, PlanBaseline)
		done <- answer{s, ok, err}
	}()
	select {
	case a := <-done:
		return a.still, a.ok, a.err
	case <-time.After(time.Second):
		t.Fatalf("Still(%s) blocked: a decode past the bound must not start or queue", id)
		return Still{}, false, nil
	}
}
