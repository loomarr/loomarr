//go:build ffmpeg

package playout

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// rotatingSource airs short items rotating through generated sources, so a channel crosses an item
// boundary every itemLen.
type rotatingSource struct {
	epoch   time.Time
	itemLen time.Duration
	paths   []string
	formats []MediaFormat
	out     OutputProfile
}

func (s rotatingSource) ItemAt(_ context.Context, _ string, at time.Time) (PackagerItem, error) {
	idx := int(at.Sub(s.epoch) / s.itemLen)
	end := s.epoch.Add(time.Duration(idx+1) * s.itemLen)
	return PackagerItem{Label: filepath.Base(s.paths[idx%len(s.paths)]), Input: s.paths[idx%len(s.paths)],
		Format: s.formats[idx%len(s.formats)], Seek: time.Duration(idx%3) * time.Second, Remaining: end.Sub(at)}, nil
}

func (s rotatingSource) Output(context.Context, string, FormatClass, int) (HostProfile, OutputProfile) {
	return HostFor(EncoderSoftware, true, GPUFilters{}), s.out
}

// TestPackagerOriginServesHLSAndTSAcrossItemBoundaries runs a channel packager behind the real
// Origin (#1512 phase 2b): the browser's master playlist, its variant and every asset it names, and
// a media-server tuner's MPEG-TS, read from the same packager across six item boundaries. The TS
// must have unbroken continuity counters, every video and audio timestamp one frame apart, and no
// decode errors.
func TestPackagerOriginServesHLSAndTSAcrossItemBoundaries(t *testing.T) {
	dir := t.TempDir()
	src := rotatingSource{epoch: time.Now(), itemLen: 2 * time.Second,
		out: OutputProfile{Width: 640, Height: 360, FPS: 30, Quality: 28, TargetKbps: 800, MaxKbps: 1200, GOPSeconds: 1, AudioKbps: 128}}
	for i, s := range []struct {
		lavfi     string
		w, h, fps int
	}{{"testsrc2=s=1280x720:r=25", 1280, 720, 25}, {"smptebars=s=720x480:r=30", 720, 480, 30}, {"mandelbrot=s=854x480:r=50", 854, 480, 50}} {
		p := filepath.Join(dir, "src"+string(rune('a'+i))+".mkv")
		ffmpegRun(t, "-f", "lavfi", "-i", s.lavfi, "-f", "lavfi", "-i", "sine=f=440:r=44100",
			"-t", "5", "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac",
			"-metadata:s:a", "title=Surround Mix", p) // library sources title their streams
		src.paths = append(src.paths, p)
		src.formats = append(src.formats, MediaFormat{VideoCodec: "h264", Width: s.w, Height: s.h, FrameRate: float64(s.fps),
			PixelFormat: "yuv420p", AudioCodec: "aac", AudioChannels: 1, AudioSampleRate: 44100, Container: "matroska,webm"})
	}
	m, err := NewPackagerHLS(src, "ffmpeg", filepath.Join(dir, "hls"), time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)
	on := func() bool { return true }
	o := newOrigin(nil, switchedSessions{packaged: m, usePackager: on}, switchedHLS{packaged: m, usePackager: on})
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// The tuner joins first, so its stream starts at the channel's first segment.
	tuner, err := o.Tune(ctx, TuneRequest{ChannelID: "ch", Plan: PlanBaseline, Delivery: DeliveryMPEGTS})
	if err != nil {
		t.Fatal(err)
	}
	defer tuner.Release()

	browser, err := o.Tune(ctx, TuneRequest{ChannelID: "ch", Plan: PlanBaseline, Delivery: DeliveryHLS})
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Release()
	master := string(browser.Manifest)
	if !strings.Contains(master, `CODECS="avc1.`) || !strings.Contains(master, "RESOLUTION=640x360") ||
		!strings.Contains(master, "\n1080p-h264-sdr.m3u8\n") {
		t.Fatalf("master playlist:\n%s", master)
	}
	variant, ok, err := o.OpenAsset(ctx, "ch", PlanBaseline, "1080p-h264-sdr.m3u8")
	if err != nil || !ok || !variant.Playlist {
		t.Fatalf("variant: ok %v playlist %v err %v", ok, variant.Playlist, err)
	}
	body, _ := io.ReadAll(variant.Content)
	var uris []string
	for _, line := range strings.Split(string(body), "\n") {
		if u, found := strings.CutPrefix(line, `#EXT-X-MAP:URI="`); found {
			uris = append(uris, strings.TrimSuffix(u, `"`))
		} else if line != "" && !strings.HasPrefix(line, "#") {
			uris = append(uris, line)
		}
	}
	if len(uris) < 2 {
		t.Fatalf("variant lists no init and segment:\n%s", body)
	}
	for _, u := range uris {
		a, ok, err := o.OpenAsset(ctx, "ch", PlanBaseline, u)
		if err != nil || !ok {
			t.Fatalf("asset %q named by the variant: ok %v err %v", u, ok, err)
		}
		_ = a.Content.Close()
	}

	// Six boundaries at 2 s items: 13 s of the tuner's stream.
	var ts bytes.Buffer
	for ts.Len() == 0 || tsDuration(t, dir, ts.Bytes()) < 13*time.Second {
		b, err := tuner.Stream.Next(ctx)
		if err != nil {
			t.Fatalf("tuner stream after %d bytes: %v", ts.Len(), err)
		}
		ts.Write(b)
	}
	m.mu.Lock()
	c := m.channels[packagedKey{channel: "ch", format: FormatBaseline}]
	m.mu.Unlock()
	if s := c.p.Stats(); s.Items < 7 || s.Slates != 0 {
		t.Fatalf("stats %+v: want 7 items back to back, no slate", s)
	}
	last := map[int]int{}
	for off := 0; off+188 <= ts.Len(); off += 188 {
		p := ts.Bytes()[off : off+188]
		if p[0] != 0x47 {
			t.Fatalf("packet %d: no sync byte", off/188)
		}
		pid := int(p[1]&0x1f)<<8 | int(p[2])
		if pid == 0x1fff || p[3]&0x10 == 0 {
			continue
		}
		cc := int(p[3] & 0x0f)
		if prev, ok := last[pid]; ok && cc != (prev+1)%16 {
			t.Fatalf("PID %d: continuity counter %d after %d at packet %d", pid, cc, prev, off/188)
		}
		last[pid] = cc
	}
	path := filepath.Join(dir, "tuner.ts")
	if err := os.WriteFile(path, ts.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	for stream, want := range map[string]int{"v:0": 3000, "a:0": 1920} { // 90 kHz: 1/30 s, 1024/48000 s
		for i, d := range packetDeltas(t, path, stream) {
			if d != want {
				t.Fatalf("%s packet %d delta %d, want %d (a gap or overlap at an item boundary)", stream, i+1, d, want)
			}
		}
	}
	if o, err := exec.Command("ffmpeg", "-v", "error", "-i", path, "-f", "null", "-").CombinedOutput(); err != nil || len(o) > 0 {
		t.Fatalf("decode errors: %v\n%s", err, o)
	}
}

// tsDuration is the span of a transport stream's video timestamps.
func tsDuration(t *testing.T, dir string, ts []byte) time.Duration {
	t.Helper()
	path := filepath.Join(dir, "partial.ts")
	if err := os.WriteFile(path, ts, 0o600); err != nil {
		t.Fatal(err)
	}
	var sum int
	for _, d := range packetDeltas(t, path, "v:0") {
		sum += d
	}
	return time.Duration(sum) * time.Second / 90000
}
