//go:build ffmpeg

package hls

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/playout/media"
	"github.com/loomarr/loomarr/internal/playout/packager"
)

func ffprobeBin(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("no ffprobe on PATH")
	}
	return p
}

// premiumHost skips a host that cannot produce a 4K HDR premium.
func premiumHost(t *testing.T, bin string) encode.HostProfile {
	t.Helper()
	enc := encode.EncoderNVENC
	probeArgs := []string{"-pix_fmt", "p010le", "-c:v", "hevc_nvenc", "-profile:v", "main10"}
	if os.Getenv("LOOMARR_PREMIUM_ENCODER") == "vaapi" {
		enc = encode.EncoderVAAPI
		probeArgs = []string{"-init_hw_device", "vaapi=va:" + encode.RenderNode(), "-filter_hw_device", "va",
			"-vf", "format=p010le,hwupload", "-c:v", "hevc_vaapi", "-profile:v", "main10"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	args := append([]string{"-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=s=256x144:r=25:d=0.2"}, probeArgs...)
	if out, err := exec.CommandContext(ctx, bin, append(args, "-f", "null", "-")...).CombinedOutput(); err != nil {
		t.Skipf("no working HEVC Main10 %s encoder on this host: %v\n%s", enc, err, out)
	}
	host := encode.HostFor(enc, playout.TonemapperFor(bin)(), playout.GPUFiltersFor(bin)())
	if !host.Libplacebo {
		t.Skip("this ffmpeg build has no libplacebo: a 4K HDR premium is not producible here")
	}
	return host
}

// synth renders a lavfi source to a file with the given video encode arguments, plus AAC stereo.
func synth(t *testing.T, bin, name, lavfi string, seconds int, video ...string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), name)
	args := []string{"-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", lavfi,
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-ac", "2", "-t", strconv.Itoa(seconds)}
	args = append(args, video...)
	args = append(args, "-c:a", "aac", "-shortest", out)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if b, err := exec.CommandContext(ctx, bin, args...).CombinedOutput(); err != nil {
		t.Skipf("cannot synthesize %s with this build: %v\n%s", name, err, b)
	}
	return out
}

type premiumItem struct {
	name  string
	path  string
	facts media.MediaFormat
}

// premiumClasses synthesizes the two classes this channel mixes: an SDR 1080p episode and a 4K HDR10
// film carrying its OWN mastering metadata (P3, 4000 nits), which must not survive into the channel.
func premiumClasses(t *testing.T, bin string, seconds int) (hdr10, sdrEpisode premiumItem) {
	t.Helper()
	sdr := media.MediaFormat{VideoCodec: "h264", Width: 1920, Height: 1080, FrameRate: 25, PixelFormat: "yuv420p",
		ColorTransfer: "bt709", AudioCodec: "aac", AudioChannels: 2, AudioSampleRate: 48000, Container: "matroska,webm"}
	hdr := media.MediaFormat{VideoCodec: "hevc", Width: 3840, Height: 2160, FrameRate: 25, PixelFormat: "yuv420p10le",
		ColorTransfer: "smpte2084", AudioCodec: "aac", AudioChannels: 2, AudioSampleRate: 48000, Container: "matroska,webm"}
	sdrEpisode = premiumItem{"sdr-episode", synth(t, bin, "sdr.mkv", "testsrc2=s=1920x1080:r=25", seconds,
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-color_primaries", "bt709", "-color_trc", "bt709", "-colorspace", "bt709"), sdr}
	hdr10 = premiumItem{"uhd-hdr10", synth(t, bin, "uhd-hdr.mkv", "testsrc2=s=3840x2160:r=25", seconds,
		"-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le",
		"-x265-params", "log-level=error:colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc:hdr10=1:"+
			"master-display=G(13250,34500)B(7500,3000)R(34000,16000)WP(15635,16450)L(40000000,50):max-cll=4000,1000"), hdr}
	return hdr10, sdrEpisode
}

var yavgRe = regexp.MustCompile(`lavfi\.signalstats\.YAVG=([0-9.]+)`)

func meanYAVG(t *testing.T, bin, path string) float64 {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "-hide_banner", "-loglevel", "error", "-i", path,
		"-vf", "signalstats,metadata=mode=print:file=-", "-f", "null", "-").CombinedOutput()
	if err != nil {
		t.Fatalf("signalstats %s: %v\n%s", path, err, out)
	}
	var sum float64
	m := yavgRe.FindAllStringSubmatch(string(out), -1)
	if len(m) == 0 {
		t.Fatalf("no YAVG for %s:\n%s", path, out)
	}
	for _, v := range m {
		f, _ := strconv.ParseFloat(v[1], 64)
		sum += f
	}
	return sum / float64(len(m))
}

// premiumChannelSource is a 4K HDR channel on the premium host: 12 s items alternating a PQ film
// carrying its own P3/4000-nit metadata and an SDR episode that is converted to HDR10. Items that
// long let the timeline build the run-ahead a real channel's programmes give it; at 6 s, libplacebo's
// ~2 s device start left the converted item late after a 6 s first airing.
type premiumChannelSource struct {
	epoch *atomic.Int64 // unix nanos: the schedule's origin, reset so a tune lands on an item start
	host  encode.HostProfile
	items []premiumItem
}

func (s premiumChannelSource) ItemAt(_ context.Context, _ string, at time.Time) (PackagerItem, error) {
	const itemLen = 12 * time.Second
	epoch := time.Unix(0, s.epoch.Load())
	idx := max(int(at.Sub(epoch)/itemLen), 0)
	it := s.items[idx%len(s.items)]
	return PackagerItem{Label: it.name, Input: it.path, Format: it.facts,
		Remaining: epoch.Add(time.Duration(idx+1) * itemLen).Sub(at)}, nil
}

func (premiumChannelSource) Premium(context.Context, string) playout.FormatClass {
	return playout.Format4KHDR
}

func (s premiumChannelSource) Output(_ context.Context, _ string, class playout.FormatClass, _ int) (encode.HostProfile, playout.OutputProfile) {
	base := playout.OutputProfile{Width: 1920, Height: 1080, FPS: 25, Quality: 22, TargetKbps: 8000, MaxKbps: 12000, GOPSeconds: 1, AudioKbps: 192}
	if class == playout.FormatBaseline {
		return s.host, base
	}
	out, _ := playout.PremiumOutput(class, base)
	return s.host, out
}

// TestLivePremium_PackagerServesHDR10 plays a 4K HDR channel's premium variant through the real
// Origin and PackagerHLS (#1512 G10): the master names it (VIDEO-RANGE=PQ, HEVC Main 10); playing it
// starts the premium packager, whose init carries the channel's mdcv/clli and whose every IDR
// carries the channel's static SEI and never an item's own (4000 nits); every item joins (the same
// SPS), the stream decodes without error, and the picture is not black.
func TestLivePremium_PackagerServesHDR10(t *testing.T) {
	bin, probe := ffmpegBin(t), ffprobeBin(t)
	host := premiumHost(t, bin)
	hdr10, sdrEpisode := premiumClasses(t, bin, 14)
	src := premiumChannelSource{epoch: &atomic.Int64{}, host: host, items: []premiumItem{hdr10, sdrEpisode}}
	src.epoch.Store(time.Now().UnixNano())
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	m, err := NewPackagerHLS(src, bin, filepath.Join(t.TempDir(), "hls"), time.Second, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)
	o := NewOrigin(OriginDependencies{Packager: m})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	browser, err := o.Tune(ctx, TuneRequest{ChannelID: "ch", Delivery: DeliveryHLS})
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Release()
	master := string(browser.Manifest)
	t.Logf("master before premium play:\n%s", master)
	predicted := `CODECS="hvc1.2.4.L150.90,mp4a.40.2",RESOLUTION=3840x2160,FRAME-RATE=25.000,VIDEO-RANGE=PQ` + "\n4k-hevc-hdr.m3u8\n"
	if !strings.Contains(master, predicted) {
		t.Fatalf("master does not offer the HDR10 premium:\n%s", master)
	}

	// Play the premium for 30 s of media (three items): init plus every segment, in order.
	// The premium tunes in at an item start, as a real channel's long programmes give the timeline
	// its run-ahead: after a 1 s tune-in airing, any item with a slow start (the baseline's HDR
	// tone-map, a premium's SDR conversion) misses its first slot, which is not what this measures.
	src.epoch.Store(time.Now().UnixNano())
	var stream bytes.Buffer
	seen := map[string]bool{}
	var seconds float64
	for seconds < 30 {
		pl, ok, err := o.OpenAsset(ctx, "ch", "4k-hevc-hdr.m3u8", false)
		if err != nil || !ok {
			t.Fatalf("premium playlist: ok %v err %v", ok, err)
		}
		body, _ := io.ReadAll(pl.Content)
		_ = pl.Content.Close()
		var extinf float64
		for _, line := range strings.Split(string(body), "\n") {
			uri := ""
			if u, found := strings.CutPrefix(line, `#EXT-X-MAP:URI="`); found {
				uri = strings.TrimSuffix(u, `"`)
			} else if d, found := strings.CutPrefix(line, "#EXTINF:"); found {
				extinf, _ = strconv.ParseFloat(strings.TrimSuffix(d, ","), 64)
			} else if line != "" && !strings.HasPrefix(line, "#") {
				uri = line
			}
			if uri == "" || seen[uri] {
				continue
			}
			seen[uri] = true
			a, ok, err := o.OpenAsset(ctx, "ch", uri, false)
			if err != nil || !ok {
				t.Fatalf("asset %q: ok %v err %v", uri, ok, err)
			}
			b, _ := io.ReadAll(a.Content)
			_ = a.Content.Close()
			stream.Write(b)
			if strings.HasSuffix(uri, ".m4s") {
				seconds += extinf
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	file := filepath.Join(t.TempDir(), "premium.mp4")
	if err := os.WriteFile(file, stream.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	m.mu.Lock()
	c := m.channels[packagedKey{channel: "ch", format: playout.Format4KHDR}]
	m.mu.Unlock()
	if c == nil {
		t.Fatal("playing the premium started no premium packager")
	}
	stats := c.p.Stats()
	codecs := packager.CodecsAttr(c.p.Init())
	t.Logf("premium packager: %+v; init CODECS %q; %.1f s of media", stats, codecs, seconds)
	if stats.Items < 3 || stats.Slates != 0 || stats.DecoderMismatch != 0 {
		t.Errorf("stats %+v: want every item to join the channel (one SPS), no slate", stats)
	}
	if !strings.Contains(predicted, `"`+codecs+`"`) {
		t.Errorf("the master predicted %s; the running init names %q", predicted, codecs)
	}

	initProbe, err := exec.Command(probe, "-v", "error", "-select_streams", "v:0", "-show_streams", "-show_entries",
		"stream=codec_tag_string,profile,color_transfer,color_primaries:stream_side_data", "-of", "flat", file).Output()
	if err != nil {
		t.Fatalf("ffprobe init: %v", err)
	}
	t.Logf("init: %s", initProbe)
	for _, want := range []string{`codec_tag_string="hvc1"`, "Mastering display metadata", `max_luminance="10000000/10000"`, "max_content=1000", `color_transfer="smpte2084"`} {
		if !strings.Contains(string(initProbe), want) {
			t.Errorf("the premium init lacks %s", want)
		}
	}

	frames, err := exec.Command(probe, "-v", "error", "-select_streams", "v:0", "-show_frames", "-show_entries",
		"frame=key_frame:frame_side_data=side_data_type,max_luminance,max_content", "-of", "compact", file).Output()
	if err != nil {
		t.Fatalf("ffprobe frames: %v", err)
	}
	var idr, withSEI int
	for _, line := range strings.Split(string(frames), "\n") {
		if !strings.HasPrefix(line, "frame|key_frame=1") {
			if strings.Contains(line, "max_content=4000") {
				t.Errorf("an item's own light level survived: %s", line)
			}
			continue
		}
		idr++
		if strings.Contains(line, "max_luminance=10000000/10000") && strings.Contains(line, "max_content=1000") {
			withSEI++
		} else {
			t.Errorf("IDR %d without the channel's static SEI: %s", idr, line)
		}
	}
	t.Logf("IDRs %d, with the channel SEI %d", idr, withSEI)
	if idr < 28 {
		t.Errorf("only %d IDRs in %.1f s at a 1 s GOP", idr, seconds)
	}

	if out, err := exec.Command(bin, "-hide_banner", "-v", "error", "-i", file, "-f", "null", "-").CombinedOutput(); err != nil || len(bytes.TrimSpace(out)) > 0 {
		t.Errorf("decode errors: %v\n%s", err, out)
	}
	if y := meanYAVG(t, bin, file); y < 40 {
		t.Errorf("mean YAVG %.1f: the premium picture is dark or black", y)
	} else {
		t.Logf("mean YAVG %.1f", y)
	}
}
