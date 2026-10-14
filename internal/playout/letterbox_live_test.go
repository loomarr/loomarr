//go:build ffmpeg

package playout

import (
	"context"
	"encoding/binary"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"testing"
	"time"
)

// Live letterbox tests (#1673). pad_vaapi writes Y=U=V=0 into a P010 frame's bars whatever colour
// it is given, which a player shows as green; the 8-bit NV12 path pads black. A source whose
// letterbox is baked into a full-height frame never pads, so these sources are SHORTER than the
// output. Mean luma (YAVG) did not notice the green bars, so every pixel of every bar row is
// checked on all three planes, and the picture between the bars is scored against a software
// reference at the position the builder must put it. On a shared machine run them under the GPU
// lock:
//
//	flock /tmp/loomarr-gpu.lock go test -tags ffmpeg ./internal/playout -run TestLiveLetterbox -v
//
// LOOMARR_PREMIUM_ENCODER=vaapi picks the Intel/AMD family, as for TestLivePremium.

// barMeanTolerance is how far any frame's bar mean may sit from black on each plane, and
// barPixelTolerance how far any one bar pixel may, in 8-bit code values (×4 at 10-bit). Flat bars
// encode within ±1 of black on the mean but a 10-bit pixel strays by up to 5 (NVENC, measured);
// #1673's zeros miss by 64 (Y) and 512 (U, V).
const barMeanTolerance, barPixelTolerance = 4, 4

// minPicturePSNR bounds how far the padded output's picture region may drift from the same graph
// encoded unpadded: two encodes of one picture, not a moved or recoloured one.
const minPicturePSNR = 35

var planeStatRe = regexp.MustCompile(`lavfi\.signalstats\.([YUV])(MIN|MAX|AVG)=([0-9.]+)`)

// planeRange is, over every frame, the lowest and highest code value of each plane (Y, U, V) and
// the lowest and highest frame mean.
type planeRange struct{ min, max, avgMin, avgMax [3]float64 }

// cropRange decodes path in SOFTWARE and reports the Y/U/V range inside one crop rectangle.
func cropRange(t *testing.T, bin, path string, w, h, x, y int) planeRange {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	vf := fmt.Sprintf("crop=w=%d:h=%d:x=%d:y=%d,signalstats,metadata=mode=print:file=-", w, h, x, y)
	out, err := exec.CommandContext(ctx, bin, "-hide_banner", "-loglevel", "error", "-i", path,
		"-vf", vf, "-f", "null", "-").CombinedOutput()
	if err != nil {
		t.Fatalf("signalstats %s: %v\n%s", path, err, out)
	}
	const far = 1 << 16
	r := planeRange{min: [3]float64{far, far, far}, avgMin: [3]float64{far, far, far}}
	m := planeStatRe.FindAllStringSubmatch(string(out), -1)
	if len(m) == 0 {
		t.Fatalf("no plane stats for %s:\n%s", path, out)
	}
	for _, v := range m {
		p := map[string]int{"Y": 0, "U": 1, "V": 2}[v[1]]
		f, _ := strconv.ParseFloat(v[3], 64)
		switch v[2] {
		case "MIN":
			r.min[p] = min(r.min[p], f)
		case "MAX":
			r.max[p] = max(r.max[p], f)
		default:
			r.avgMin[p], r.avgMax[p] = min(r.avgMin[p], f), max(r.avgMax[p], f)
		}
	}
	return r
}

// rowFrame is the decoded frame whose rows rowMeans reads.
const rowFrame = 12

// rowMeans decodes frame rowFrame of path in SOFTWARE and returns the mean of every row of each
// plane (Y: h rows, U and V: h/2), area-scaled to two columns so the chroma keeps one.
func rowMeans(t *testing.T, bin, path string, h int, tenBit bool) [3][]float64 {
	t.Helper()
	pix, size := "yuv420p", 1
	if tenBit {
		pix, size = "yuv420p10le", 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	vf := fmt.Sprintf("select=eq(n\\,%d),scale=w=2:h=%d:flags=area,format=%s", rowFrame, h, pix)
	out, err := exec.CommandContext(ctx, bin, "-hide_banner", "-loglevel", "error", "-i", path,
		"-vf", vf, "-frames:v", "1", "-f", "rawvideo", "-").Output()
	if err != nil {
		t.Fatalf("row means %s: %v", path, err)
	}
	if want := (2*h + h) * size; len(out) != want { // Y 2×h, U and V 1×h/2 each
		t.Fatalf("row means %s: %d bytes, want %d", path, len(out), want)
	}
	sample := func(i int) float64 {
		if size == 2 {
			return float64(binary.LittleEndian.Uint16(out[2*i:]))
		}
		return float64(out[i])
	}
	var m [3][]float64
	for r := range h {
		m[0] = append(m[0], (sample(2*r)+sample(2*r+1))/2)
	}
	for p := 1; p <= 2; p++ {
		for r := range h / 2 {
			m[p] = append(m[p], sample(2*h+(p-1)*h/2+r))
		}
	}
	return m
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

var psnrRe = regexp.MustCompile(`PSNR .*average:([0-9.]+|inf)`)

// pictureScore is the PSNR of the output's picture region (the crop) against a reference encode of
// the same size.
func pictureScore(t *testing.T, bin, output, crop, reference string) float64 {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	graph := fmt.Sprintf("[0:v]%s,setpts=PTS-STARTPTS[o];[1:v]setpts=PTS-STARTPTS[r];[o][r]psnr=shortest=1", crop)
	out, err := exec.CommandContext(ctx, bin, "-hide_banner", "-nostats", "-i", output, "-i", reference,
		"-filter_complex", graph, "-f", "null", "-").CombinedOutput()
	if err != nil {
		t.Fatalf("psnr: %v\n%s", err, out)
	}
	m := psnrRe.FindSubmatch(out)
	if m == nil {
		t.Fatalf("no PSNR in:\n%s", out)
	}
	if string(m[1]) == "inf" {
		return 100
	}
	v, _ := strconv.ParseFloat(string(m[1]), 64)
	return v
}

// TestLiveLetterbox_BarsAreBlack encodes a scope source (shorter than the output) through each
// graph that pads, then asserts the bars are black on Y, U and V and the picture sits between them
// unchanged. The 10-bit cases are #1673's (the 4K HDR premium and the SDR item converted to it);
// the 8-bit ones (4K SDR premium, the tone-mapped 1080p channel) pin that the fix left them alone.
func TestLiveLetterbox_BarsAreBlack(t *testing.T) {
	bin, probe := ffmpegBin(t), ffprobeBin(t)
	host := premiumHost(t, bin)
	const seconds = 1
	hdrScope := premiumItem{"hdr10-scope", synth(t, bin, "hdr10-scope.mkv", "testsrc2=s=3840x1600:r=25", seconds,
		"-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le",
		"-x265-params", "log-level=error:colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc:hdr10=1:"+
			"master-display=G(13250,34500)B(7500,3000)R(34000,16000)WP(15635,16450)L(10000000,50):max-cll=1000,400"),
		MediaFormat{VideoCodec: "hevc", Width: 3840, Height: 1600, FrameRate: 25, PixelFormat: "yuv420p10le",
			ColorTransfer: "smpte2084", AudioCodec: "aac", AudioChannels: 2, AudioSampleRate: 48000, Container: "matroska,webm"}}
	sdrScope := premiumItem{"sdr-scope", synth(t, bin, "sdr-scope.mkv", "testsrc2=s=1920x800:r=25", seconds,
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-color_primaries", "bt709", "-color_trc", "bt709", "-colorspace", "bt709"),
		MediaFormat{VideoCodec: "h264", Width: 1920, Height: 800, FrameRate: 25, PixelFormat: "yuv420p",
			ColorTransfer: "bt709", AudioCodec: "aac", AudioChannels: 2, AudioSampleRate: 48000, Container: "matroska,webm"}}

	base := OutputProfile{FPS: 25, Quality: 22, GOPSeconds: 1, AudioKbps: 192}
	uhdHDR, _ := PremiumOutput(Format4KHDR, base)
	uhdSDR, _ := PremiumOutput(Format4KSDR, base)
	hd := OutputProfile{Width: 1920, Height: 1080, FPS: 25, Quality: 22, TargetKbps: 6000, MaxKbps: 9000, GOPSeconds: 1, AudioKbps: 128}
	cases := []struct {
		name string
		item premiumItem
		out  OutputProfile
		// fitted is the picture's height in the output.
		fitted int
	}{
		{"4k-hdr", hdrScope, uhdHDR, 1600},
		{"sdr-to-hdr", sdrScope, uhdHDR, 1600},
		{"4k-sdr", sdrScope, uhdSDR, 1600},
		{"1080p-tonemap", hdrScope, hd, 800},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path, stats := encodeItem(t, bin, host, tc.item, tc.out, seconds)
			// The reference is the same graph on an output exactly the picture's size: nothing to pad.
			refOut := tc.out
			refOut.Height = tc.fitted
			ref, _ := encodeItem(t, bin, host, tc.item, refOut, seconds)
			black, scale := [3]float64{16, 128, 128}, 1.0
			if tc.out.HDR {
				black, scale = [3]float64{64, 512, 512}, 4
			}
			w, h := tc.out.Width, tc.out.Height
			bar := (h - tc.fitted) / 2
			// Only whole 64-row blocks of bar: the encoder's block holding the picture edge may ring.
			top := bar / 64 * 64
			bottomY := (bar + tc.fitted + 63) / 64 * 64
			for _, r := range []struct {
				name string
				y, h int
			}{{"top", 0, top}, {"bottom", bottomY, h - bottomY}} {
				got := cropRange(t, bin, path, w, r.h, 0, r.y)
				for p, plane := range []string{"Y", "U", "V"} {
					t.Logf("%s bar rows %d-%d %s: pixels %.0f-%.0f, frame means %.1f-%.1f (black %.0f)", r.name, r.y, r.y+r.h-1,
						plane, got.min[p], got.max[p], got.avgMin[p], got.avgMax[p], black[p])
					if d := max(black[p]-got.avgMin[p], got.avgMax[p]-black[p]); d > barMeanTolerance {
						t.Errorf("%s bar: %s frame means %.1f-%.1f, want %.0f±%d (black)", r.name, plane, got.avgMin[p], got.avgMax[p], black[p], barMeanTolerance)
					}
					if d := max(black[p]-got.min[p], got.max[p]-black[p]); d > barPixelTolerance*scale {
						t.Errorf("%s bar: %s pixels span %.0f-%.0f, want %.0f±%.0f (black)", r.name, plane, got.min[p], got.max[p], black[p], barPixelTolerance*scale)
					}
				}
			}
			// Every bar row, including the ones beside the picture that the blocks above leave out:
			// an upscale after the box blends the edge row into the bar (#1673, Y 142 on the Arc).
			rows := rowMeans(t, bin, path, h, tc.out.HDR)
			off := 0
			for p, plane := range []string{"Y", "U", "V"} {
				step := 1 // luma rows per row of this plane
				if p > 0 {
					step = 2
				}
				for r, m := range rows[p] {
					if y := r * step; (y < bar || y >= bar+tc.fitted) && abs(m-black[p]) > barPixelTolerance*scale {
						t.Errorf("%s row %d (output row %d) mean %.1f, want %.0f±%.0f (black)", plane, r, y, m, black[p], barPixelTolerance*scale)
						off++
					}
				}
			}
			t.Logf("rows: %d bar rows off black on frame %d", off, rowFrame)
			crop := fmt.Sprintf("crop=w=%d:h=%d:x=0:y=%d", w, tc.fitted, bar)
			score := pictureScore(t, bin, path, crop, ref)
			t.Logf("picture rows %d-%d: PSNR %.1f dB against the unpadded graph; %.2fx; output %s",
				bar, bar+tc.fitted-1, score, stats.speed, probeColor(t, probe, path))
			if score < minPicturePSNR {
				t.Errorf("picture region: PSNR %.1f dB against the unpadded graph, want ≥ %d (moved, scaled or recoloured by the pad)", score, minPicturePSNR)
			}
		})
	}
}
