//go:build ffmpeg

package playout

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// THE VAAPI BLEND KERNEL AGAINST THE CPU OVERLAY (#1613), on any OpenCL device (NVIDIA's ICD runs the
// same kernel as the Arc's). It runs the BUILDER's graph: its bug chains, its kernel file and its
// re-timing, verbatim. Only the VAAPI plumbing is swapped for frames uploaded from system memory,
// because a host without VAAPI cannot map them (the Arc run proves the mapping). The reference is
// ffmpeg's CPU overlay of the same bug, converted the same way, onto the same frames.
//
// The main is timed like a real source that trips program_opencl: a 1001/24000 time base (the
// kernel's output keeps input 0's time base but carries framesync's pts) and a first frame before
// the bug's pts (framesync drops those, before=EXT_STOP). Output frames and pts must equal the input.
//
// PLAYOUT_TEST_OPENCL=1 makes a missing OpenCL device a failure instead of a skip.
func TestLive_BlendKernelMatchesTheCPUOverlay(t *testing.T) {
	bin := ffmpegBin(t)
	ctx := context.Background()
	dir := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second) // a graph that never ends fails, not hangs
		defer cancel()
		out, err := exec.CommandContext(ctx, bin, append([]string{"-hide_banner", "-nostdin", "-nostats", "-y"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("ffmpeg %s: %v\n%s", strings.Join(args, " "), err, lastLines(string(out), 8))
		}
		return string(out)
	}
	probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(probeCtx, bin, "-hide_banner", "-v", "error", "-init_hw_device", "opencl=cl",
		"-f", "lavfi", "-i", "nullsrc=s=16x16", "-frames:v", "1", "-f", "null", "-").CombinedOutput(); err != nil {
		if os.Getenv("PLAYOUT_TEST_OPENCL") != "" {
			t.Fatalf("no OpenCL device: %v: %s", err, out)
		}
		t.Skipf("no OpenCL device: %s", firstLine(string(out)))
	}

	const w, h, frames = 1920, 1080, 30
	// A coloured bug with an alpha ramp and a transparent margin, like a rendered logo's edge.
	bugPNG := filepath.Join(dir, "bug.png")
	run("-v", "error", "-f", "lavfi", "-i", "testsrc2=s=200x80,format=rgba,geq=r='r(X,Y)':g='g(X,Y)':b='b(X,Y)':a='if(lt(X,20),0,min(255,X*1.4))'",
		"-frames:v", "1", bugPNG)
	mainRaw := filepath.Join(dir, "main.nv12")
	run("-v", "error", "-f", "lavfi", "-i", fmt.Sprintf("testsrc2=s=%dx%d:rate=24000/1001", w, h), "-frames:v", fmt.Sprint(frames),
		"-pix_fmt", "nv12", "-f", "rawvideo", mainRaw)
	input := []string{"-f", "rawvideo", "-pix_fmt", "nv12", "-s", fmt.Sprintf("%dx%d", w, h), "-framerate", "24000/1001",
		"-itsoffset", "-0.0834", "-i", mainRaw}

	kernel, err := WriteBlendKernel(dir)
	if err != nil {
		t.Fatal(err)
	}
	wm := &Watermark{Straight: bugPNG, Kernel: kernel, Width: 200, Height: 80, Corner: CornerTopRight, MarginX: 48, MarginY: 32}
	src := MediaFormat{VideoCodec: "h264", Width: w, Height: h, FrameRate: 23.976, PixelFormat: "yuv420p", Container: "matroska,webm"}
	p, err := BuildItem(overlayHost(testHosts()["vaapi-intel"]), src, testOutput, wm)
	if err != nil || !p.Watermark {
		t.Fatalf("no kernel graph: %v %v", err, p.Fallbacks)
	}
	bench := kernelBench(t, p.VideoFilter)
	x, y := wm.position(src, testOutput)

	newRaw, refRaw := filepath.Join(dir, "new.nv12"), filepath.Join(dir, "ref.nv12")
	newLog := run(append(append([]string{"-init_hw_device", "opencl=cl", "-filter_hw_device", "cl"}, input...),
		"-filter_complex", bench+",showinfo", "-f", "rawvideo", newRaw)...)
	// The CPU overlay also leaves main frames before its bug's first pts bare, so its bug starts early too.
	refLog := run(append(input, "-filter_complex",
		"[0:v]"+conformColour+"[m];movie=filename="+bugPNG+",format=rgba,scale=out_color_matrix=bt709:out_range=tv,"+conformColour+
			",format=yuva420p,setpts="+bugPTS+"[b];"+fmt.Sprintf("[m][b]overlay=x=%d:y=%d:format=yuv420,format=nv12,showinfo", x, y),
		"-f", "rawvideo", refRaw)...)

	// The kernel's pts are in AV_TIME_BASE (µs), the reference's in the input's 1001/24000.
	got, want := showinfoPTS(newLog), showinfoPTS(refLog)
	same := len(want) == frames && len(got) == len(want)
	for i := 0; same && i < len(got); i++ {
		same = math.Abs(got[i]-want[i]) <= 1e-6
	}
	if !same {
		t.Fatalf("the kernel's frames are not the input's:\n got %d %v\nwant %d %v", len(got), got, len(want), want)
	}
	nv, err := os.ReadFile(newRaw)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := os.ReadFile(refRaw)
	if err != nil {
		t.Fatal(err)
	}
	for _, pl := range comparePlanes(ref, nv, w, h, Rect{X: x, Y: y, W: 200, H: 80}) {
		t.Logf("%s", pl)
		if pl.outsideMax != 0 {
			t.Errorf("%s: the programme changed outside the bug", pl.name)
		}
		// The CPU overlay approximates its chroma alpha on the bug's last row and column (it has no
		// 2x2 block there); the kernel uses the true 2x2 mean, so only the interior is held to ±3.
		if pl.interiorMax > 3 || math.Abs(pl.refMean-pl.newMean) > 1 {
			t.Errorf("%s: the kernel's blend is off the CPU overlay's", pl.name)
		}
	}
}

// kernelBench swaps the builder's VAAPI plumbing around the kernel for uploads from system memory:
// the main enters at the labels the builder puts on it, the bug chains upload to the -filter_hw_device
// OpenCL device, and the kernel's output is downloaded instead of mapped back and encoded.
func kernelBench(t *testing.T, graph string) string {
	t.Helper()
	swap := func(s, old, repl string, n int) string {
		t.Helper()
		if c := strings.Count(s, old); c != n {
			t.Fatalf("the kernel graph has %d of %q, want %d:\n%s", c, old, n, graph)
		}
		return strings.ReplaceAll(s, old, repl)
	}
	main, rest, ok := strings.Cut(graph, "[main];")
	if !ok {
		t.Fatalf("no main chain: %s", graph)
	}
	_, labelled, ok := strings.Cut(main, "pad_vaapi=")
	labelled = labelled[strings.Index(labelled, ",")+1:]
	if !ok || !strings.HasPrefix(labelled, conformColour) {
		t.Fatalf("the main is not labelled after the pad: %s", main)
	}
	main = "[0:v]" + swap(labelled, "hwmap=derive_device=opencl", "hwupload", 1)
	rest = swap(rest, "hwupload=derive_device=opencl,", "hwupload,", 2)
	blend, _, ok := strings.Cut(rest, ",hwmap=derive_device=vaapi:reverse=1")
	if !ok {
		t.Fatalf("the kernel's output is not mapped back to VAAPI: %s", rest)
	}
	return main + "[main];" + blend + ",hwdownload,format=nv12"
}

var showinfoLine = regexp.MustCompile(`Parsed_showinfo.* pts_time:(\S+)`)

func showinfoPTS(log string) []float64 {
	var pts []float64
	for _, m := range showinfoLine.FindAllStringSubmatch(log, -1) {
		v, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			v = math.NaN()
		}
		pts = append(pts, v)
	}
	return pts
}

type planeDiff struct {
	name                    string
	interiorMax, outsideMax int
	refMean, newMean        float64
}

func (d planeDiff) String() string {
	return fmt.Sprintf("%s: bug interior max |Δ| %d, bug mean ref %.2f new %.2f, outside max |Δ| %d",
		d.name, d.interiorMax, d.refMean, d.newMean, d.outsideMax)
}

// comparePlanes compares two raw NV12 streams plane by plane: inside the bug (its interior, one
// chroma sample in from each edge) and outside it.
func comparePlanes(ref, nv []byte, w, h int, bug Rect) []planeDiff {
	size := w * h * 3 / 2
	if len(ref) != len(nv) || len(ref)%size != 0 {
		return []planeDiff{{name: fmt.Sprintf("sizes differ: %d vs %d bytes", len(ref), len(nv)), outsideMax: 1}}
	}
	diffs := []planeDiff{{name: "Y"}, {name: "U"}, {name: "V"}}
	var n [3]float64
	for f := 0; f < len(ref); f += size {
		for i := 0; i < size; i++ {
			var pl, x, y, s int
			if i < w*h {
				x, y, s = i%w, i/w, 1
			} else {
				c := i - w*h
				pl, x, y, s = 1+c%2, (c%w)/2, c/w, 2
			}
			bx, by, bw, bh := bug.X/s, bug.Y/s, bug.W/s, bug.H/s
			d := int(ref[f+i]) - int(nv[f+i])
			if d < 0 {
				d = -d
			}
			p := &diffs[pl]
			switch {
			case x < bx || x >= bx+bw || y < by || y >= by+bh:
				p.outsideMax = max(p.outsideMax, d)
				continue
			case x > bx && x < bx+bw-1 && y > by && y < by+bh-1:
				p.interiorMax = max(p.interiorMax, d)
			}
			p.refMean += float64(ref[f+i])
			p.newMean += float64(nv[f+i])
			n[pl]++
		}
	}
	for i := range diffs {
		diffs[i].refMean /= n[i]
		diffs[i].newMean /= n[i]
	}
	return diffs
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.Join(lines[max(0, len(lines)-n):], "\n")
}
