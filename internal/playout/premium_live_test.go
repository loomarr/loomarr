//go:build ffmpeg

package playout

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Live premium-format tests (#1512 G10). They drive Build's own argv through the real encoder and
// then check the PICTURE and the BITSTREAM with a software decode, never the argv. On a shared
// machine run them under the GPU lock:
//
//	flock /tmp/loomarr-gpu.lock go test -tags ffmpeg ./internal/playout -run TestLivePremium -v

// premiumHost is the NVENC host profile this build and GPU give, or a skip.
func premiumHost(t *testing.T, bin string) HostProfile {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	probe := exec.CommandContext(ctx, bin, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=s=256x144:r=25:d=0.2",
		"-pix_fmt", "p010le", "-c:v", "hevc_nvenc", "-profile:v", "main10", "-f", "null", "-")
	if out, err := probe.CombinedOutput(); err != nil {
		t.Skipf("no working hevc_nvenc on this host: %v\n%s", err, out)
	}
	host := HostFor(EncoderNVENC, TonemapperFor(bin)(), GPUFiltersFor(bin)())
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
// metadata (P3, 4000 nits), which must not survive into the channel.
func premiumItems(t *testing.T, bin string) map[string]premiumItem {
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
		"sdr-episode": {"sdr-episode", synth(t, bin, "sdr.mkv", "testsrc2=s=1920x1080:r=25", 2, h264...), sdr},
		"sdr-white":   {"sdr-white", synth(t, bin, "white.mkv", "color=white:s=1920x1080:r=25", 2, h264...), sdr},
		"uhd-sdr": {"uhd-sdr", synth(t, bin, "uhd-sdr.mkv", "testsrc2=s=3840x2160:r=25", 2,
			"-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le",
			"-x265-params", "log-level=error:colorprim=bt709:transfer=bt709:colormatrix=bt709"), uhdSDR},
		"uhd-hdr10": {"uhd-hdr10", synth(t, bin, "uhd-hdr.mkv", "testsrc2=s=3840x2160:r=25", 2,
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
	p, err := Build(host, it.facts, out)
	if err != nil {
		t.Fatalf("%s: Build: %v", it.name, err)
	}
	dst := filepath.Join(t.TempDir(), it.name+".ts")
	args := replaceOutput(p.ItemArgs(it.path, 0, seconds*out.FPS, out.FPS, 0), "-y", dst)
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
	bin := ffmpegBin(t)
	host := premiumHost(t, bin)
	probeFormat := FFprobeFormatNextTo(bin)
	for _, class := range []FormatClass{Format4KHDR, Format4KSDR, FormatBaseline} {
		base := ChannelOutput(Profile{Width: 1920, Height: 1080, Framerate: 24, AudioBitrate: 192, Encoder: EncoderNVENC})
		out, ok := PremiumOutput(class, base)
		if !ok {
			out = base
		}
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
			path, stats := encodeItem(t, bin, host, premiumItem{name, input, facts}, out, seconds)
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
			t.Logf("%-15s %-12s src %dx%d %s: %.2fx, %.3f cores at 1x, YAVG %.1f, param sets %s",
				class, name, facts.Width, facts.Height, facts.ColorTransfer, stats.speed, stats.cpuAt1x, yavg, same)
		}
	}
}
