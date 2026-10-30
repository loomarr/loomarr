//go:build ffmpeg

package playout

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playout/packager"
)

// The alternate audio end to end (#2088 decision 6): a source with an English 440 Hz track and a
// Spanish 880 Hz one, the default English and the alternate Spanish, through ffmpeg's real item
// command and the real packager. A player's two fetches, init and segments of the variant and of
// <class>-alt, must each decode to their own tone, and the variant to no second audio track.
func TestLive_PackagerCarriesTheAlternateAudio(t *testing.T) {
	bin := ffmpegBin(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	dir := t.TempDir()
	src := filepath.Join(dir, "programme.mkv")
	if b, err := exec.CommandContext(ctx, bin, "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=640x360:rate=25:duration=4",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=4",
		"-f", "lavfi", "-i", "sine=frequency=880:sample_rate=48000:duration=4",
		"-map", "0", "-map", "1", "-map", "2", "-c:v", "libx264", "-preset", "ultrafast", "-g", "25",
		"-c:a", "aac", "-metadata:s:a:0", "language=eng", "-metadata:s:a:1", "language=spa", src).CombinedOutput(); err != nil {
		t.Fatalf("source: %v: %s", err, b)
	}

	host := HostFor(EncoderSoftware, true, GPUFilters{})
	out, _ := FormatOutput(FormatBaseline, DefaultProfile())
	format := MediaFormat{VideoCodec: "h264", Width: 640, Height: 360, FrameRate: 25, PixelFormat: "yuv420p",
		AudioCodec: "aac", AudioChannels: 1, AudioSampleRate: 48000, Container: "matroska,webm"}
	pick := PickAudioTracks([]AudioTrack{{Language: "eng"}, {Language: "spa"}}, "eng")
	if pick != (AudioPick{Default: 0, Alternate: 1}) {
		t.Fatalf("pick %+v", pick)
	}
	encode := func(slot packager.Slot) ([]byte, error) {
		it := PackagerItem{Label: "prog", Input: src, Remaining: 4 * time.Second, Format: format,
			AudioTrack: pick.Default, AltAudioTrack: pick.Alternate}
		_, args, err := packagerItemArgs(host, out, it, nil, nil, slot, itemFault{})
		if err != nil {
			return nil, err
		}
		var stderr bytes.Buffer
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Stderr = &stderr
		b, err := cmd.Output()
		if err != nil {
			return nil, errors.Join(err, errors.New(stderr.String()))
		}
		return b, nil
	}
	// The slate is the same command's output, so it matches the item's init as production's does.
	slateBytes, err := encode(packager.Slot{Frames: 25, AudioFrames: 47})
	if err != nil {
		t.Fatal(err)
	}
	slate, err := packager.NewSlate(slateBytes)
	if err != nil {
		t.Fatal(err)
	}
	asked := false
	sched := func(ctx context.Context, _ time.Time) (packager.Item, error) {
		if asked {
			<-ctx.Done()
			return packager.Item{}, ctx.Err()
		}
		asked = true
		return packager.Item{Label: "prog", Duration: 3 * time.Second, Open: func(_ context.Context, slot packager.Slot) (io.ReadCloser, error) {
			b, err := encode(slot)
			return io.NopCloser(bytes.NewReader(b)), err
		}}, nil
	}
	pkgDir := filepath.Join(dir, "pkg")
	if err := os.Mkdir(pkgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	p, err := packager.New(packager.Config{FPS: out.FPS, Dir: pkgDir, URIPrefix: "baseline-", RunAhead: time.Hour,
		ListAhead: time.Hour, FirstManifest: 3 * time.Second, FirstItemWait: time.Minute},
		sched, func(context.Context) (*packager.Slate, error) { return slate, nil })
	if err != nil {
		t.Fatal(err)
	}
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- p.Run(runCtx) }()
	if err := p.AwaitPlaylist(ctx); err != nil {
		t.Fatal(err)
	}
	stop()
	<-done
	if !p.HasAlt() {
		t.Fatal("the real encoder's init carries no track 3")
	}

	// What a player fetches: each playlist's map, then its segments, in order.
	rendition := func(playlist []byte) string {
		var buf bytes.Buffer
		for _, line := range strings.Split(string(playlist), "\n") {
			name := ""
			switch {
			case strings.HasPrefix(line, "#EXT-X-MAP:URI="):
				name = strings.Trim(strings.TrimPrefix(line, "#EXT-X-MAP:URI="), `"`)
			case line != "" && !strings.HasPrefix(line, "#"):
				name = line
			}
			if name == "" {
				continue
			}
			b, err := os.ReadFile(filepath.Join(pkgDir, strings.TrimPrefix(name, "baseline-")))
			if err != nil {
				t.Fatal(err)
			}
			buf.Write(b)
		}
		f := filepath.Join(dir, "rendition.mp4")
		f = strings.TrimSuffix(f, ".mp4") + "-" + time.Now().Format("150405.000000") + ".mp4"
		if err := os.WriteFile(f, buf.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
		return f
	}
	main, alt := rendition(p.Playlist()), rendition(p.AltPlaylist())
	for _, c := range []struct {
		name, file string
		hz         float64
		streams    string
	}{
		{"variant (default English)", main, 440, "video\naudio"},
		{"alternate (Spanish)", alt, 880, "audio"},
	} {
		probe, err := exec.CommandContext(ctx, strings.TrimSuffix(bin, "ffmpeg")+"ffprobe", "-v", "error",
			"-show_entries", "stream=codec_type", "-of", "csv=p=0", c.file).Output()
		if err != nil {
			t.Fatalf("%s: probe: %v", c.name, err)
		}
		if got := strings.TrimSpace(string(probe)); got != c.streams {
			t.Errorf("%s: streams %q, want %q", c.name, got, c.streams)
		}
		hz, secs := toneOf(t, ctx, bin, c.file)
		t.Logf("%s: %.0f Hz over %.2f s", c.name, hz, secs)
		if hz < c.hz*0.97 || hz > c.hz*1.03 {
			t.Errorf("%s: %.0f Hz, want %.0f", c.name, hz, c.hz)
		}
		if secs < 2.9 {
			t.Errorf("%s: %.2f s of audio, want the 3 s slot", c.name, secs)
		}
	}
}

// toneOf decodes a file's audio to mono and measures its tone by zero crossings, skipping the
// first and last 0.1 s (AAC's edges).
func toneOf(t *testing.T, ctx context.Context, bin, file string) (hz, secs float64) {
	t.Helper()
	pcm, err := exec.CommandContext(ctx, bin, "-v", "error", "-i", file, "-map", "0:a:0", "-ac", "1", "-ar", "48000",
		"-f", "s16le", "pipe:1").Output()
	if err != nil {
		t.Fatalf("%s: decode: %v", file, err)
	}
	n := len(pcm) / 2
	edge := 4800
	if n < 3*edge {
		t.Fatalf("%s: %d samples", file, n)
	}
	crossings, prev := 0, int16(binary.LittleEndian.Uint16(pcm[2*edge:]))
	for i := edge + 1; i < n-edge; i++ {
		s := int16(binary.LittleEndian.Uint16(pcm[2*i:]))
		if (prev < 0) != (s < 0) {
			crossings++
		}
		prev = s
	}
	return float64(crossings) / 2 / (float64(n-2*edge) / 48000), float64(n) / 48000
}
