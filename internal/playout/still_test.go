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
