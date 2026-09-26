//go:build ffmpeg

package playout

import (
	"context"
	"io"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// AN HDR PROGRAM MUST COME OUT AS A PICTURE on every tone-mapper the host can run (#1516).
//
// tonemap_vaapi on the household Arc exits 0 at normal speed with every frame at Y=16: black. Speed,
// start time and exit status all passed on it, twice. So this test asserts the PICTURE: signalstats
// luma of the encoded output, on a real PQ source, for each family this host has and each tone-mapper
// the production ladder (ProgramSpec.DemoteTonemap) would try on it.
//
// A tone-mapper whose runtime is missing (no OpenCL ICD, no Vulkan device) fails to start, which the
// live chain answers by demoting; the test logs it and moves down the ladder. Set
// PLAYOUT_TEST_TONEMAPPERS (e.g. "vaapi/opencl,nvenc/opencl,nvenc/libplacebo") on a GPU host to make
// the named ones mandatory, so a missing runtime cannot turn the run into a vacuous green.
func TestLive_HDRTonemapProducesAPicture(t *testing.T) {
	bin := ffmpegBin(t)
	if !TonemapperFor(bin)() {
		t.Skip("this ffmpeg build has no zscale/tonemap")
	}
	src := makePQSource(t, bin)
	facts := MediaFormat{VideoCodec: "hevc", Width: 1280, Height: 720, FrameRate: 25, PixelFormat: "yuv420p10le",
		ColorTransfer: "smpte2084", Container: "mpegts"}
	ran := map[string]bool{}

	// Software: the CPU chain, which is also every GPU family's last resort after the GPU downscale.
	t.Run("software/cpu", func(t *testing.T) {
		out := OutputProfile{Width: 1280, Height: 720, FPS: 25, Quality: 22, TargetKbps: 3600, MaxKbps: 5300, GOPSeconds: 1, AudioKbps: 128}
		pipe, err := Build(HostProfile{Family: FamilySoftware, CPUTonemap: true, SoftwareHDR: true}, facts, out)
		if err != nil {
			t.Fatal(err)
		}
		o := t.TempDir() + "/o.ts"
		if errText := encodeTo(t, bin, replaceOutput(pipe.ItemArgs(src, 0, 50, 25, 0), o)); errText != "" {
			t.Fatalf("software tone-map failed: %s", errText)
		}
		assertPicture(t, bin, o)
		ran["software/cpu"] = !t.Failed()
	})

	for _, enc := range []Encoder{EncoderVAAPI, EncoderNVENC} {
		if c := trialEncodeObserved(context.Background(), bin, enc, DefaultProfile(), 1, nil); !c.Works {
			t.Logf("%s: not usable on this host, skipped (%s)", enc, firstLine(c.Err))
			continue
		}
		p := DefaultProfile()
		p.Encoder = enc
		spec := ProgramSpec{Profile: p, Input: src, Limit: 2 * time.Second, Source: facts,
			Tonemap: true, GPUTonemap: GPUFiltersFor(bin)()}
		for {
			pipe, err := spec.Pipeline()
			if err != nil {
				t.Fatalf("%s: %v", enc, err)
			}
			name := string(pipe.Family) + "/" + tonemapperIn(pipe.VideoFilter)
			t.Run(name, func(t *testing.T) {
				out := t.TempDir() + "/o.ts"
				if errText := encodeTo(t, bin, replaceOutput(ProgramArgs(spec), out)); errText != "" {
					// The live chain demotes on this (no output); a missing runtime is not a picture defect.
					t.Logf("%s did not start (the ladder demotes): %s", name, errText)
					return
				}
				assertPicture(t, bin, out)
				ran[name] = !t.Failed()
			})
			if !spec.DemoteTonemap() {
				break
			}
		}
	}

	for _, want := range strings.FieldsFunc(os.Getenv("PLAYOUT_TEST_TONEMAPPERS"), func(r rune) bool { return r == ',' }) {
		if !ran[strings.TrimSpace(want)] {
			t.Errorf("PLAYOUT_TEST_TONEMAPPERS requires %s, which did not produce output here (ran: %v)", want, keys(ran))
		}
	}
	t.Logf("tone-mappers that produced a picture: %v", keys(ran))
}

// tonemapperIn names the tone-mapper a built graph uses.
func tonemapperIn(vf string) string {
	for _, f := range []string{"tonemap_vaapi", "tonemap_opencl", "libplacebo"} {
		if strings.Contains(vf, f+"=") {
			return strings.TrimPrefix(f, "tonemap_")
		}
	}
	return "cpu"
}

// makePQSource is a REAL HDR10 signal, not SDR pixels tagged PQ: testsrc2 converted into PQ/BT.2020
// with SDR white at BT.2408's 203 nits (10-bit code ~565), HEVC Main10, with the mastering-display
// and content-light metadata real films carry. Without that metadata tonemap_vaapi writes nothing
// (which the ladder rescues) instead of the black picture #1516 aired. Measured: a correct Hable
// tone-map lands at YAVG 104-152, YMAX >= 155 on every tone-mapper (CPU, OpenCL, libplacebo).
func makePQSource(t *testing.T, bin string) string {
	t.Helper()
	out := t.TempDir() + "/pq.ts"
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=1280x720:rate=25:duration=2",
		"-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=48000",
		"-vf", "zscale=tin=bt709:min=bt709:pin=bt709:rin=tv:t=smpte2084:p=bt2020:m=bt2020nc:r=tv:npl=203,format=yuv420p10le",
		"-c:v", "libx265", "-x265-params", "log-level=error:colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc:"+
			"hdr10=1:master-display=G(13250,34500)B(7500,3000)R(34000,16000)WP(15635,16450)L(10000000,50):max-cll=1000,400",
		"-color_primaries", "bt2020", "-color_trc", "smpte2084", "-colorspace", "bt2020nc",
		"-c:a", "aac", "-t", "2", "-f", "mpegts", out)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot synthesize a PQ source with this build (zscale/libx265?): %v\n%s", err, b)
	}
	// The fixture must really be bright PQ, or a black output could pass the bounds below.
	if avg, ymax := lumaStats(t, bin, out); ymax < 500 {
		t.Fatalf("fixture is not a bright PQ signal (YAVG %.0f, YMAX %d on 10-bit); the test would be vacuous", avg, ymax)
	}
	return out
}

// encodeTo runs one encode to completion; it returns ffmpeg's error, or "" on success.
func encodeTo(t *testing.T, bin string, args []string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	proc, err := Start(ctx, bin, args, nil, nil)
	if err != nil {
		return err.Error()
	}
	go func() { _, _ = io.Copy(io.Discard, proc.Stdout) }()
	if err := proc.Wait(); err != nil {
		return err.Error() + ": " + proc.LastError()
	}
	return ""
}

// Picture bounds for the 8-bit tone-mapped output. Black is YAVG = YMAX = 16. The measured correct
// range is YAVG 104-152 and a per-frame YMAX of at least 155 (makePQSource), so the bounds sit well
// outside both a black and a blown-out picture without pinning one implementation's curve.
const (
	pictureMinYMAX = 100
	pictureMinYAVG = 60
	pictureMaxYAVG = 200
)

func assertPicture(t *testing.T, bin, path string) {
	t.Helper()
	avg, ymax := lumaStats(t, bin, path)
	t.Logf("YAVG mean %.1f, lowest per-frame YMAX %d", avg, ymax)
	if ymax < pictureMinYMAX || avg < pictureMinYAVG || avg > pictureMaxYAVG {
		t.Errorf("tone-mapped output is not a picture: YAVG mean %.1f (want %d-%d), lowest YMAX %d (want >= %d); "+
			"black is 16/16 (#1516)", avg, pictureMinYAVG, pictureMaxYAVG, ymax, pictureMinYMAX)
	}
}

// lumaStats is signalstats over every video frame: the mean YAVG and the LOWEST per-frame YMAX, so one
// black frame anywhere fails.
func lumaStats(t *testing.T, bin, path string) (float64, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, bin, "-hide_banner", "-nostats", "-i", path, "-map", "0:v:0",
		"-vf", "signalstats,metadata=print:file=-", "-f", "null", "-").Output()
	if err != nil {
		t.Fatalf("signalstats: %v", err)
	}
	var sum float64
	frames, lowest := 0, -1
	for line := range strings.Lines(string(b)) {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		switch {
		case !ok:
		case k == "lavfi.signalstats.YAVG":
			f, _ := strconv.ParseFloat(v, 64)
			sum += f
			frames++
		case k == "lavfi.signalstats.YMAX":
			n, _ := strconv.Atoi(v)
			if lowest < 0 || n < lowest {
				lowest = n
			}
		}
	}
	if frames == 0 {
		t.Fatalf("signalstats measured no frames in %s", path)
	}
	return sum / float64(frames), lowest
}

func keys(m map[string]bool) []string {
	var k []string
	for s := range m {
		k = append(k, s)
	}
	slices.Sort(k)
	return k
}
