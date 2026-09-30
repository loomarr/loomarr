//go:build ffmpeg

package playout

import (
	"bytes"
	"context"
	"fmt"
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

	"github.com/loomarr/loomarr/internal/playout/packager"
)

// Live premium-format tests (#1512 G10). They drive Build's own argv through the real encoder and
// then check the PICTURE and the BITSTREAM with a software decode, never the argv. On a shared
// machine run them under the GPU lock:
//
//	flock /tmp/loomarr-gpu.lock go test -tags ffmpeg ./internal/playout -run TestLivePremium -v

// premiumHost is the host profile this build and GPU give for LOOMARR_PREMIUM_ENCODER: "nvenc"
// (the default) or "vaapi" (Intel/AMD; the render node is PLAYOUT_RENDER_NODE, as in production),
// or a skip when that encoder cannot produce HEVC Main10 here.
func premiumHost(t *testing.T, bin string) HostProfile {
	t.Helper()
	enc := EncoderNVENC
	probeArgs := []string{"-pix_fmt", "p010le", "-c:v", "hevc_nvenc", "-profile:v", "main10"}
	if os.Getenv("LOOMARR_PREMIUM_ENCODER") == "vaapi" {
		enc = EncoderVAAPI
		probeArgs = []string{"-init_hw_device", "vaapi=va:" + renderNode(), "-filter_hw_device", "va",
			"-vf", "format=p010le,hwupload", "-c:v", "hevc_vaapi", "-profile:v", "main10"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	args := append([]string{"-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=s=256x144:r=25:d=0.2"}, probeArgs...)
	if out, err := exec.CommandContext(ctx, bin, append(args, "-f", "null", "-")...).CombinedOutput(); err != nil {
		t.Skipf("no working HEVC Main10 %s encoder on this host: %v\n%s", enc, err, out)
	}
	host := HostFor(enc, TonemapperFor(bin)(), GPUFiltersFor(bin)())
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
	facts MediaFormat
}

// premiumItems synthesizes the classes a 4K channel mixes: an SDR 1080p episode, a flat white SDR
// card (the BT.2408 reference), a 4K SDR film, and a 4K HDR10 film carrying its OWN mastering
// metadata (P3, 4000 nits), which must not survive into the channel. Each is 2 s long.
func premiumItems(t *testing.T, bin string) map[string]premiumItem {
	t.Helper()
	return premiumItemsOf(t, bin, 2)
}

// premiumItemsOf is premiumItems at a given length.
func premiumItemsOf(t *testing.T, bin string, seconds int) map[string]premiumItem {
	t.Helper()
	h264 := []string{"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-color_primaries", "bt709", "-color_trc", "bt709", "-colorspace", "bt709"}
	sdr := MediaFormat{VideoCodec: "h264", Width: 1920, Height: 1080, FrameRate: 25, PixelFormat: "yuv420p",
		ColorTransfer: "bt709", AudioCodec: "aac", AudioChannels: 2, AudioSampleRate: 48000, Container: "matroska,webm"}
	hdr := MediaFormat{VideoCodec: "hevc", Width: 3840, Height: 2160, FrameRate: 25, PixelFormat: "yuv420p10le",
		ColorTransfer: "smpte2084", AudioCodec: "aac", AudioChannels: 2, AudioSampleRate: 48000, Container: "matroska,webm"}
	uhdSDR := hdr
	uhdSDR.ColorTransfer = "bt709"
	return map[string]premiumItem{
		"sdr-episode": {"sdr-episode", synth(t, bin, "sdr.mkv", "testsrc2=s=1920x1080:r=25", seconds, h264...), sdr},
		"sdr-white":   {"sdr-white", synth(t, bin, "white.mkv", "color=white:s=1920x1080:r=25", seconds, h264...), sdr},
		"uhd-sdr": {"uhd-sdr", synth(t, bin, "uhd-sdr.mkv", "testsrc2=s=3840x2160:r=25", seconds,
			"-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le",
			"-x265-params", "log-level=error:colorprim=bt709:transfer=bt709:colormatrix=bt709"), uhdSDR},
		"uhd-hdr10": {"uhd-hdr10", synth(t, bin, "uhd-hdr.mkv", "testsrc2=s=3840x2160:r=25", seconds,
			"-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le",
			"-x265-params", "log-level=error:colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc:hdr10=1:"+
				"master-display=G(13250,34500)B(7500,3000)R(34000,16000)WP(15635,16450)L(40000000,50):max-cll=4000,1000"), hdr},
	}
}

type encodeStats struct {
	speed, cpuAt1x float64
}

// encodeItem runs one item's Build argv for seconds of content into an MPEG-TS file.
func encodeItem(t *testing.T, bin string, host HostProfile, it premiumItem, out OutputProfile, seconds int) (string, encodeStats) {
	t.Helper()
	return encodeItemAt(t, bin, host, it, out, 0, seconds)
}

func encodeItemAt(t *testing.T, bin string, host HostProfile, it premiumItem, out OutputProfile, seek time.Duration, seconds int) (string, encodeStats) {
	t.Helper()
	p, err := Build(host, it.facts, out)
	if err != nil {
		t.Fatalf("%s: Build: %v", it.name, err)
	}
	if len(p.Fallbacks) > 0 {
		t.Logf("%s: fallbacks %q", it.name, p.Fallbacks)
	}
	dst := filepath.Join(t.TempDir(), it.name+".ts")
	args := replaceOutput(p.ItemArgs(it.path, seek, seconds*out.FPS, out.FPS, 0), "-y", dst)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	start := time.Now()
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s: encode: %v\n%s\nargv: %q", it.name, err, b, args)
	}
	wall := time.Since(start).Seconds()
	cpu := (cmd.ProcessState.UserTime() + cmd.ProcessState.SystemTime()).Seconds()
	return dst, encodeStats{speed: float64(seconds) / wall, cpuAt1x: cpu / float64(seconds)}
}

var yavgRe = regexp.MustCompile(`lavfi\.signalstats\.YAVG=([0-9.]+)`)

// meanYAVG decodes a file in SOFTWARE and averages signalstats' YAVG over every frame, in the
// file's own bit depth (0-1023 for 10-bit).
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

// videoKbps is the output's video stream bitrate: the sum of its packets over the content length.
func videoKbps(t *testing.T, probe, path string, seconds int) int {
	t.Helper()
	out, err := exec.Command(probe, "-v", "error", "-select_streams", "v:0", "-show_entries", "packet=size",
		"-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatalf("ffprobe packets %s: %v", path, err)
	}
	var bytes int
	for _, line := range strings.Fields(string(out)) {
		n, _ := strconv.Atoi(strings.TrimSuffix(line, ","))
		bytes += n
	}
	return bytes * 8 / seconds / 1000
}

var vmafRe = regexp.MustCompile(`VMAF score: ([0-9.]+)`)

// vmafAgainstSource scores the output with libvmaf against the same stretch of the source, brought
// to the output's cadence, fitted size and bit depth with a bicubic software scale.
func vmafAgainstSource(t *testing.T, bin, output, source string, seek time.Duration, seconds int, out OutputProfile) float64 {
	t.Helper()
	pix := "yuv420p"
	if out.HDR {
		pix = "yuv420p10le"
	}
	ref := fmt.Sprintf("[1:v]fps=%d,scale=w=%d:h=%d:force_original_aspect_ratio=decrease:force_divisible_by=2:flags=bicubic,"+
		"pad=%d:%d:-1:-1,format=%s,setpts=PTS-STARTPTS[ref]", out.FPS, out.Width, out.Height, out.Width, out.Height, pix)
	graph := fmt.Sprintf("[0:v]format=%s,setpts=PTS-STARTPTS[dist];%s;[dist][ref]libvmaf=n_threads=4:shortest=1", pix, ref)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	b, err := exec.CommandContext(ctx, bin, "-hide_banner", "-nostats", "-i", output,
		"-ss", seconds64(seek), "-t", strconv.Itoa(seconds), "-i", source,
		"-filter_complex", graph, "-f", "null", "-").CombinedOutput()
	if err != nil {
		t.Fatalf("libvmaf: %v\n%s", err, b)
	}
	m := vmafRe.FindSubmatch(b)
	if m == nil {
		t.Fatalf("no VMAF score in:\n%s", b)
	}
	v, _ := strconv.ParseFloat(string(m[1]), 64)
	return v
}

func seconds64(d time.Duration) string { return strconv.FormatFloat(d.Seconds(), 'f', 3, 64) }

// annexB extracts a file's HEVC elementary stream (Annex B).
func annexB(t *testing.T, bin, path string, bsf ...string) []byte {
	t.Helper()
	args := []string{"-hide_banner", "-loglevel", "error", "-i", path, "-map", "0:v:0", "-c:v", "copy"}
	args = append(args, bsf...)
	args = append(args, "-f", "hevc", "-")
	out, err := exec.Command(bin, args...).Output()
	if err != nil {
		t.Fatalf("extract %s: %v", path, err)
	}
	return out
}

// nalUnits splits an Annex B stream at its start codes (each unit keeps its 4-byte start code).
func nalUnits(b []byte) [][]byte {
	var starts []int
	for i := 0; i+3 <= len(b); i++ {
		if b[i] == 0 && b[i+1] == 0 && b[i+2] == 1 {
			s := i
			if s > 0 && b[s-1] == 0 {
				s--
			}
			starts = append(starts, s)
			i += 2
		}
	}
	var units [][]byte
	for k, s := range starts {
		end := len(b)
		if k+1 < len(starts) {
			end = starts[k+1]
		}
		units = append(units, b[s:end])
	}
	return units
}

func nalType(unit []byte) int {
	i := bytes.Index(unit, []byte{0, 0, 1})
	return int(unit[i+3]>>1) & 0x3f
}

// parameterSets is every VPS/SPS/PPS NAL unit of the stream, in order and deduplicated.
func parameterSets(t *testing.T, bin, path string) [][]byte {
	t.Helper()
	var sets [][]byte
	for _, u := range nalUnits(annexB(t, bin, path)) {
		if typ := nalType(u); typ >= 32 && typ <= 34 {
			u = bytes.TrimLeft(u, "\x00")
			seen := false
			for _, s := range sets {
				seen = seen || bytes.Equal(s, u)
			}
			if !seen {
				sets = append(sets, u)
			}
		}
	}
	return sets
}

func sameSets(a, b [][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

// frameSideData is ffprobe's side data list for the first video frame.
func frameSideData(t *testing.T, probe, path string, format ...string) string {
	t.Helper()
	args := append(format, "-v", "error", "-select_streams", "v:0", "-read_intervals", "%+#1",
		"-show_frames", "-show_entries", "frame=side_data_list", "-of", "flat", path)
	out, err := exec.Command(probe, args...).Output()
	if err != nil {
		t.Fatalf("ffprobe side data %s: %v", path, err)
	}
	return string(out)
}

// TestLivePremium_4KHDRChannel: every item of a 4K HDR channel comes out as a real HDR10 picture
// with byte-identical parameter sets and no per-item HDR SEI; the SDR reference white lands on
// BT.2408's 203 nits (10-bit PQ code 573).
func TestLivePremium_4KHDRChannel(t *testing.T) {
	bin, probe := ffmpegBin(t), ffprobeBin(t)
	host := premiumHost(t, bin)
	out, _ := PremiumOutput(Format4KHDR, OutputProfile{FPS: 25, Quality: 22, GOPSeconds: 1, AudioKbps: 192})
	items := premiumItems(t, bin)

	var first [][]byte
	var firstName string
	for _, name := range []string{"sdr-episode", "uhd-hdr10", "sdr-white", "uhd-sdr"} {
		it := items[name]
		path, stats := encodeItem(t, bin, host, it, out, 2)
		if got := probeColor(t, probe, path); got != "yuv420p10le,tv,bt2020nc,smpte2084,bt2020" {
			t.Errorf("%s: output is %q, want HDR10 (yuv420p10le, tv, bt2020nc, smpte2084, bt2020)", name, got)
		}
		yavg := meanYAVG(t, bin, path)
		t.Logf("%s: YAVG %.1f (10-bit), %.2fx, %.2f cores at 1x", name, yavg, stats.speed, stats.cpuAt1x)
		switch {
		case name == "sdr-white":
			if yavg < 565 || yavg > 581 {
				t.Errorf("SDR white converted to PQ code %.1f, want 573±8 (BT.2408 203 nits)", yavg)
			}
		case yavg < 100:
			t.Errorf("%s: picture is black (YAVG %.1f)", name, yavg)
		}
		if sd := frameSideData(t, probe, path); strings.Contains(sd, "Mastering display") || strings.Contains(sd, "Content light level") {
			t.Errorf("%s: per-item HDR metadata leaked into the channel:\n%s", name, sd)
		}
		sets := parameterSets(t, bin, path)
		if len(sets) != 3 {
			t.Fatalf("%s: want one VPS, SPS and PPS, got %d distinct parameter sets", name, len(sets))
		}
		if first == nil {
			first, firstName = sets, name
		} else if !sameSets(first, sets) {
			t.Errorf("%s: VPS/SPS/PPS differ from %s's: a seam would re-init the decoder\n%x\nvs\n%x", name, firstName, sets, first)
		}
	}
}

// TestLivePremium_4KSDRChannel: 4K SDR items and upscaled 1080p SDR items share one parameter set.
func TestLivePremium_4KSDRChannel(t *testing.T) {
	bin, probe := ffmpegBin(t), ffprobeBin(t)
	host := premiumHost(t, bin)
	out, _ := PremiumOutput(Format4KSDR, OutputProfile{FPS: 25, Quality: 22, GOPSeconds: 1, AudioKbps: 192})
	items := premiumItems(t, bin)
	var first [][]byte
	for _, name := range []string{"sdr-episode", "uhd-sdr"} {
		path, stats := encodeItem(t, bin, host, items[name], out, 2)
		if got := probeColor(t, probe, path); got != "yuv420p,tv,bt709,bt709,bt709" {
			t.Errorf("%s: output is %q, want 8-bit BT.709", name, got)
		}
		yavg := meanYAVG(t, bin, path)
		t.Logf("%s: YAVG %.1f, %.2fx, %.2f cores at 1x", name, yavg, stats.speed, stats.cpuAt1x)
		if yavg < 25 {
			t.Errorf("%s: picture is black (YAVG %.1f)", name, yavg)
		}
		sets := parameterSets(t, bin, path)
		if first == nil {
			first = sets
		} else if !sameSets(first, sets) {
			t.Errorf("%s: VPS/SPS/PPS differ between items", name)
		}
	}
}

// TestLivePremium_StaticSEIDecodes: the channel's static SEI, inserted before an IDR's first slice
// as the packager will, reads back as the maintainer's HDR10 values.
func TestLivePremium_StaticSEIDecodes(t *testing.T) {
	bin, probe := ffmpegBin(t), ffprobeBin(t)
	host := premiumHost(t, bin)
	out, _ := PremiumOutput(Format4KHDR, OutputProfile{FPS: 25, Quality: 22, GOPSeconds: 1, AudioKbps: 192})
	path, _ := encodeItem(t, bin, host, premiumItems(t, bin)["uhd-hdr10"], out, 1)

	var withSEI []byte
	inserted := false
	for _, u := range nalUnits(annexB(t, bin, path)) {
		if !inserted && nalType(u) < 32 {
			withSEI, inserted = append(withSEI, ChannelHDR10.SEI()...), true
		}
		withSEI = append(withSEI, u...)
	}
	file := filepath.Join(t.TempDir(), "with-sei.hevc")
	if err := os.WriteFile(file, withSEI, 0o644); err != nil {
		t.Fatal(err)
	}
	sd := frameSideData(t, probe, file, "-f", "hevc")
	for _, want := range []string{
		`red_x="35400/50000"`, `green_y="39850/50000"`, `blue_x="6550/50000"`, `white_point_x="15635/50000"`,
		`min_luminance="1/10000"`, `max_luminance="10000000/10000"`, `max_content=1000`, `max_average=400`,
	} {
		if !strings.Contains(sd, want) {
			t.Errorf("static SEI: %s missing from the decoded side data:\n%s", want, sd)
		}
	}
}

// TestLivePremium_Measure encodes real media through the builder for the PR's numbers. Set
// LOOMARR_PREMIUM_HDR and LOOMARR_PREMIUM_SDR to a real 4K HDR film and an SDR episode (paths or
// URLs); LOOMARR_PREMIUM_SECONDS overrides the 20 s. Skipped otherwise.
func TestLivePremium_Measure(t *testing.T) {
	hdrSrc, sdrSrc := os.Getenv("LOOMARR_PREMIUM_HDR"), os.Getenv("LOOMARR_PREMIUM_SDR")
	if hdrSrc == "" || sdrSrc == "" {
		t.Skip("LOOMARR_PREMIUM_HDR / LOOMARR_PREMIUM_SDR not set")
	}
	seconds := 20
	if s, err := strconv.Atoi(os.Getenv("LOOMARR_PREMIUM_SECONDS")); err == nil && s > 0 {
		seconds = s
	}
	// LOOMARR_PREMIUM_SEEK (seconds) skips studio logos so the sample is representative picture.
	seek, _ := strconv.Atoi(os.Getenv("LOOMARR_PREMIUM_SEEK"))
	bin, probe := ffmpegBin(t), ffprobeBin(t)
	host := premiumHost(t, bin)
	probeFormat := FFprobeFormatNextTo(bin)
	for _, class := range []FormatClass{Format4KHDR, Format4KSDR} {
		base := ChannelOutput(Profile{Width: 1920, Height: 1080, Framerate: 24, AudioBitrate: 192, Encoder: EncoderNVENC})
		out, _ := PremiumOutput(class, base)
		var first [][]byte
		for _, input := range []string{hdrSrc, sdrSrc} {
			facts, err := probeFormat(context.Background(), input)
			if err != nil {
				t.Fatalf("probe: %v", err)
			}
			if class == Format4KSDR && facts.HDR() {
				continue // derivation never pairs these
			}
			name := "hdr-film"
			if input == sdrSrc {
				name = "sdr-episode"
			}
			path, stats := encodeItemAt(t, bin, host, premiumItem{name, input, facts}, out, time.Duration(seek)*time.Second, seconds)
			yavg := meanYAVG(t, bin, path)
			sets := parameterSets(t, bin, path)
			same := "first"
			if first == nil {
				first = sets
			} else if sameSets(first, sets) {
				same = "identical"
			} else {
				same = "DIFFER"
				t.Errorf("%s/%s: parameter sets differ", class, name)
			}
			kbps := videoKbps(t, probe, path, seconds)
			vmaf := "n/a (SDR source, HDR10 output)"
			if facts.PQ() == out.HDR { // same transfer: VMAF compares like with like
				vmaf = strconv.FormatFloat(vmafAgainstSource(t, bin, path, input, time.Duration(seek)*time.Second, seconds, out), 'f', 2, 64)
			}
			t.Logf("%-15s %-12s src %dx%d %s: %.2fx, %.3f cores at 1x, YAVG %.1f, video %d kbit/s, VMAF %s, param sets %s",
				class, name, facts.Width, facts.Height, facts.ColorTransfer, stats.speed, stats.cpuAt1x, yavg, kbps, vmaf, same)
		}
	}
}

// premiumChannelSource is a 4K HDR channel on the premium host: 12 s items alternating a PQ film
// carrying its own P3/4000-nit metadata and an SDR episode that is converted to HDR10. Items that
// long let the timeline build the run-ahead a real channel's programmes give it; at 6 s, libplacebo's
// ~2 s device start left the converted item late after a 6 s first airing.
type premiumChannelSource struct {
	epoch *atomic.Int64 // unix nanos: the schedule's origin, reset so a tune lands on an item start
	host  HostProfile
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

func (premiumChannelSource) Premium(context.Context, string) FormatClass { return Format4KHDR }

func (s premiumChannelSource) Output(_ context.Context, _ string, class FormatClass, _ int) (HostProfile, OutputProfile) {
	base := OutputProfile{Width: 1920, Height: 1080, FPS: 25, Quality: 22, TargetKbps: 8000, MaxKbps: 12000, GOPSeconds: 1, AudioKbps: 192}
	if class == FormatBaseline {
		return s.host, base
	}
	out, _ := PremiumOutput(class, base)
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
	items := premiumItemsOf(t, bin, 14)
	src := premiumChannelSource{epoch: &atomic.Int64{}, host: host, items: []premiumItem{items["uhd-hdr10"], items["sdr-episode"]}}
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

	browser, err := o.Tune(ctx, TuneRequest{ChannelID: "ch", Plan: PlanBaseline, Delivery: DeliveryHLS})
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
	var media float64
	for media < 30 {
		pl, ok, err := o.OpenAsset(ctx, "ch", PlanBaseline, "4k-hevc-hdr.m3u8", false)
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
			a, ok, err := o.OpenAsset(ctx, "ch", PlanBaseline, uri, false)
			if err != nil || !ok {
				t.Fatalf("asset %q: ok %v err %v", uri, ok, err)
			}
			b, _ := io.ReadAll(a.Content)
			_ = a.Content.Close()
			stream.Write(b)
			if strings.HasSuffix(uri, ".m4s") {
				media += extinf
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	file := filepath.Join(t.TempDir(), "premium.mp4")
	if err := os.WriteFile(file, stream.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	m.mu.Lock()
	c := m.channels[packagedKey{channel: "ch", format: Format4KHDR}]
	m.mu.Unlock()
	if c == nil {
		t.Fatal("playing the premium started no premium packager")
	}
	stats := c.p.Stats()
	codecs := packager.CodecsAttr(c.p.Init())
	t.Logf("premium packager: %+v; init CODECS %q; %.1f s of media", stats, codecs, media)
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
		t.Errorf("only %d IDRs in %.1f s at a 1 s GOP", idr, media)
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
