package playout

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeAirings is the schedule as the still sees it: whatever airs on the channel now.
type fakeAirings struct {
	mu     sync.Mutex
	airing StillAiring
	ok     bool
}

func (f *fakeAirings) set(a StillAiring) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.airing, f.ok = a, true
}

func (f *fakeAirings) resolve(context.Context, string) (StillAiring, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.airing, f.ok, nil
}

// airingOf is an airing whose source resolves to input, counting each resolution in resolved.
func airingOf(key, input string, offset time.Duration, at time.Time, resolved *atomic.Int32) StillAiring {
	return StillAiring{Key: key, Offset: offset, At: at, Source: func(context.Context) (StillSource, bool, error) {
		if resolved != nil {
			resolved.Add(1)
		}
		return StillSource{Input: input}, true, nil
	}}
}

// sourceCalls records every source decode and echoes what it was asked for.
type sourceCalls struct{ n atomic.Int32 }

func (c *sourceCalls) extract(_ context.Context, source StillSource, offset time.Duration) ([]byte, error) {
	c.n.Add(1)
	return fmt.Appendf(nil, "jpeg:%s@%s", source.Input, offset), nil
}

func TestOriginStill_ColdChannelDecodesOneFrameFromTheSourcePerAiring(t *testing.T) {
	var resolved atomic.Int32
	airings := &fakeAirings{}
	var calls sourceCalls
	o := NewOrigin(OriginDependencies{StillAiring: airings.resolve, SourceStill: calls.extract})

	for i := range 5 {
		// The viewer keeps surfing back while the same programme airs; "now" moves on.
		airings.set(airingOf("airing-1", "/media/film.mkv", time.Duration(73+i)*time.Second, time.Unix(1000, 0), &resolved))
		got, ok, err := o.Still(context.Background(), "ch1", PlanBaseline)
		if err != nil || !ok || string(got.JPEG) != "jpeg:/media/film.mkv@1m13s" {
			t.Fatalf("Still = %q ok=%v err=%v, want the frame at the first request's offset", got.JPEG, ok, err)
		}
		if !got.At.Equal(time.Unix(1000, 0)) {
			t.Fatalf("At = %v, want the airing's start", got.At)
		}
	}
	if calls.n.Load() != 1 || resolved.Load() != 1 {
		t.Fatalf("decoded %d times and resolved the source %d times for one airing, want 1 and 1 (cached per airing)",
			calls.n.Load(), resolved.Load())
	}

	airings.set(airingOf("airing-2", "/media/episode.mkv", 5*time.Second, time.Unix(7000, 0), nil))
	got, ok, _ := o.Still(context.Background(), "ch1", PlanBaseline)
	if !ok || string(got.JPEG) != "jpeg:/media/episode.mkv@5s" {
		t.Fatalf("after the next airing began got %q ok=%v, want the new programme's frame", got.JPEG, ok)
	}
	if calls.n.Load() != 2 {
		t.Fatalf("decoded %d times across two airings, want 2", calls.n.Load())
	}
}

func TestOriginStill_WarmChannelShowsItsNewestSegmentNotTheSource(t *testing.T) {
	airings := &fakeAirings{}
	airings.set(airingOf("airing-1", "/media/film.mkv", 0, time.Unix(1000, 0), nil))
	var source sourceCalls
	var segments atomic.Int32
	o := NewOrigin(OriginDependencies{StillAiring: airings.resolve, SourceStill: source.extract, Still: countingExtractor(&segments)})
	o.stillSources = append([]stillSource{&fakeStillSource{ok: true, seg: segment("seg-9", "live", time.Unix(1100, 0))}}, o.stillSources...)

	got, ok, err := o.Still(context.Background(), "ch1", PlanBaseline)
	if err != nil || !ok || string(got.JPEG) != "jpeg:live" {
		t.Fatalf("got %q ok=%v err=%v, want the live segment's frame", got.JPEG, ok, err)
	}
	if source.n.Load() != 0 {
		t.Fatal("a warm channel decoded its source file")
	}
}

func TestOriginStill_NothingAiringIsAMiss(t *testing.T) {
	var calls sourceCalls
	o := NewOrigin(OriginDependencies{StillAiring: (&fakeAirings{}).resolve, SourceStill: calls.extract})

	if _, ok, err := o.Still(context.Background(), "ch1", PlanBaseline); ok || err != nil {
		t.Fatalf("ok=%v err=%v, want a clean miss", ok, err)
	}
	if calls.n.Load() != 0 {
		t.Fatal("decoded with nothing airing")
	}
}

// A surf tunes one channel while the warmer prefetches both neighbours' stills: three decodes at
// once. Past the decode bound a source still waits for a slot instead of missing, because a cold
// channel has no older frame to stand in and a miss leaves the viewer a blank card.
func TestOriginStill_SourceStillsWaitForADecodeSlot(t *testing.T) {
	release := make(chan struct{})
	var running, peak atomic.Int32
	extract := func(_ context.Context, source StillSource, _ time.Duration) ([]byte, error) {
		now := running.Add(1)
		for p := peak.Load(); now > p && !peak.CompareAndSwap(p, now); p = peak.Load() {
		}
		<-release
		running.Add(-1)
		return []byte("jpeg:" + source.Input), nil
	}
	resolve := func(_ context.Context, channelID string) (StillAiring, bool, error) {
		return airingOf("airing-"+channelID, channelID+".mkv", 0, time.Unix(1, 0), nil), true, nil
	}
	o := NewOrigin(OriginDependencies{StillAiring: resolve, SourceStill: extract})

	channels := []string{"ch1", "ch2", "ch3"}
	results := make([]string, len(channels))
	var wg sync.WaitGroup
	for i, id := range channels {
		wg.Go(func() {
			got, ok, err := o.Still(context.Background(), id, PlanBaseline)
			if err != nil || !ok {
				results[i] = fmt.Sprintf("miss ok=%v err=%v", ok, err)
				return
			}
			results[i] = string(got.JPEG)
		})
	}
	time.Sleep(50 * time.Millisecond) // let all three reach the bound
	close(release)
	wg.Wait()

	for i, id := range channels {
		if want := "jpeg:" + id + ".mkv"; results[i] != want {
			t.Errorf("%s: got %q, want %q", id, results[i], want)
		}
	}
	if peak.Load() > stillDecodeLimit {
		t.Fatalf("%d decodes ran at once, want at most %d", peak.Load(), stillDecodeLimit)
	}
}

func TestFFmpegSourceStill_DecodesThePictureAtTheOffset(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	// A black first half and a bright second half with 2 s GOPs: a frame from the wrong place, or
	// no seek at all, comes out black.
	file := filepath.Join(t.TempDir(), "source.mkv")
	build := exec.Command(ffmpeg, "-v", "error",
		"-f", "lavfi", "-i", "color=c=black:size=1920x1080:rate=24:duration=6",
		"-f", "lavfi", "-i", "testsrc2=size=1920x1080:rate=24:duration=6",
		"-filter_complex", "[0:v][1:v]concat=n=2:v=1[v]", "-map", "[v]",
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "48", "-pix_fmt", "yuv420p", file)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the fixture: %v: %s", err, out)
	}

	jpg, err := FFmpegSourceStill(ffmpeg, nil)(context.Background(), StillSource{Input: file}, 9*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(jpg))
	if err != nil {
		t.Fatalf("output is not a JPEG: %v", err)
	}
	if img.Bounds().Dx() != stillWidth {
		t.Fatalf("width %d, want %d (downscaled from 1920)", img.Bounds().Dx(), stillWidth)
	}
	if luma := meanLuma(img); luma < 60 {
		t.Fatalf("mean luma %.0f: the still is black, so it is not the picture at 9 s", luma)
	}

	if _, err := FFmpegSourceStill(ffmpeg, nil)(context.Background(), StillSource{Input: filepath.Join(t.TempDir(), "missing.mkv")}, 0); err == nil {
		t.Fatal("an unreadable source must be an error (a miss), not an empty still")
	}
}

func meanLuma(img image.Image) float64 {
	ycc, ok := img.(*image.YCbCr)
	if !ok {
		return 0
	}
	var sum float64
	for _, y := range ycc.Y {
		sum += float64(y)
	}
	return sum / float64(len(ycc.Y))
}
