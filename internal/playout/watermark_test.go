package playout

import (
	"strings"
	"testing"
)

// testBug is the approved default look at 1080p: 6% of the frame height (65 px, rounded to even),
// margins 5% of width and height.
func testBug() *Watermark {
	return &Watermark{Straight: "/data/watermarks/ch1-abc.png",
		Width: 144, Height: 64, Corner: CornerTopRight, MarginX: 96, MarginY: 54}
}

func overlayHost(h HostProfile) HostProfile {
	h.Overlay = true
	return h
}

// THE BUG ANCHORS TO THE ACTIVE PICTURE, not the frame (maintainer, 2026-09-26): a 2.39:1 film
// with its letterbox baked into a 16:9 frame must not wear the bug in the black bar.
func TestWatermarkPlacement_AnchorsToTheActivePicture(t *testing.T) {
	out := testOutput
	cases := []struct {
		name   string
		src    MediaFormat
		corner Corner
		x, y   int
	}{
		// Full-frame 16:9: the frame corner, minus the margin.
		{"16:9 top-right", MediaFormat{Width: 1920, Height: 1080}, CornerTopRight, 1920 - 96 - 144, 54},
		{"16:9 bottom-left", MediaFormat{Width: 1920, Height: 1080}, CornerBottomLeft, 96, 1080 - 54 - 64},
		// 4:3 source, pillarboxed by the pad: the picture is 1440 wide at x=240.
		{"4:3 pillarbox", MediaFormat{Width: 1440, Height: 1080}, CornerTopRight, 240 + 1440 - 96 - 144, 54},
		// 2.39:1 letterbox baked into a 4K frame: 3840x1608 active at y=276, i.e. 804 lines at y=138.
		{"baked letterbox", MediaFormat{Width: 3840, Height: 2160, Active: Rect{X: 0, Y: 276, W: 3840, H: 1608}},
			CornerTopRight, 1920 - 96 - 144, 138 + 54},
		{"baked letterbox bottom", MediaFormat{Width: 3840, Height: 2160, Active: Rect{X: 0, Y: 276, W: 3840, H: 1608}},
			CornerBottomRight, 1920 - 96 - 144, 138 + 804 - 54 - 64},
		// Unknown geometry: the frame.
		{"unknown geometry", MediaFormat{}, CornerTopLeft, 96, 54},
	}
	for _, c := range cases {
		wm := testBug()
		wm.Corner = c.corner
		x, y := wm.position(c.src, out)
		if x != c.x || y != c.y {
			t.Errorf("%s: bug at (%d,%d), want (%d,%d)", c.name, x, y, c.x, c.y)
		}
		if x%2 != 0 || y%2 != 0 {
			t.Errorf("%s: odd position (%d,%d) misaligns 4:2:0 chroma", c.name, x, y)
		}
	}
}

// A picture too small for the bug and its margins keeps the bug inside the frame.
func TestWatermarkPlacement_StaysInsideTheFrame(t *testing.T) {
	wm := testBug()
	for _, corner := range []Corner{CornerTopLeft, CornerTopRight, CornerBottomLeft, CornerBottomRight} {
		wm.Corner = corner
		x, y := wm.position(MediaFormat{Width: 1920, Height: 1080, Active: Rect{X: 900, Y: 500, W: 100, H: 60}}, testOutput)
		if x < 0 || y < 0 || x+wm.Width > testOutput.Width || y+wm.Height > testOutput.Height {
			t.Errorf("%s: bug at (%d,%d) leaves the frame", corner, x, y)
		}
	}
}

// Each GPU family overlays on the GPU: the bug is decoded once, uploaded once, and blended by the
// family's overlay filter. NVENC honours the spike's findings: overlay_cuda blends alpha only onto a
// yuv420p main with a yuva420p bug, and emits the decoder's aligned 1088-line surface, so a
// passthrough=0 scale_cuda restores the output geometry (and with it an SPS identical to bug-off).
func TestBuildItem_WatermarkIsAGPUOverlay(t *testing.T) {
	for hostName, host := range testHosts() {
		if host.Family != FamilyVAAPI && host.Family != FamilyNVENC {
			continue
		}
		for srcName, src := range testSources() {
			p, err := BuildItem(overlayHost(host), src, testOutput, testBug())
			if err != nil {
				t.Fatalf("%s/%s: %v", hostName, srcName, err)
			}
			if !p.Watermark {
				t.Errorf("%s/%s: no watermark; fallbacks %v", hostName, srcName, p.Fallbacks)
				continue
			}
			g := p.VideoFilter
			switch host.Family {
			case FamilyNVENC:
				wantAll(t, hostName+"/"+srcName, g,
					"movie=filename=/data/watermarks/ch1-abc.png,format=yuva420p,hwupload_cuda[wm]",
					"[main][wm]overlay_cuda=x=", "scale_cuda=w=1920:h=1080:format=yuv420p:passthrough=0,fps=25,")
				main, _, _ := strings.Cut(g, "[main]")
				if !strings.Contains(main, "format=yuv420p") || strings.HasSuffix(strings.Split(main, "pad_cuda")[0], "format=nv12,") {
					t.Errorf("%s/%s: overlay_cuda needs a yuv420p main: %q", hostName, srcName, main)
				}
			case FamilyVAAPI:
				wantAll(t, hostName+"/"+srcName, g,
					// Straight alpha (on the Arc a premultiplied bug blended as 129 where 65% white is 178),
					// mapped into limited range: the Arc carried RGB 255 into Y 255 (#1541,
					// scripts/watermark-overlay-matrix.sh).
					"movie=filename=/data/watermarks/ch1-abc.png,format=bgra,"+limitedRGB+",hwupload[wm]", "[main][wm]overlay_vaapi=x=")
			}
			if n := strings.Count(g, "movie="); n != 1 {
				t.Errorf("%s/%s: the bug must be read once, got %d movie sources", hostName, srcName, n)
			}
		}
	}
}

// NEVER ON THE CPU (maintainer): a family without a GPU overlay, or a host whose overlay failed the
// self-check, airs the programme without the bug and says why. It never refuses the programme.
func TestBuildItem_WatermarkNeverOnTheCPU(t *testing.T) {
	src := testSources()["h264-1080p-sdr-25"]
	hosts := testHosts()
	for _, name := range []string{"software", "generic-qsv", "nvenc-opencl", "vaapi-intel"} {
		host := hosts[name]
		if name == "software" || name == "generic-qsv" {
			host.Overlay = true // even a claimed overlay never puts the bug on the CPU
		}
		p, err := BuildItem(host, src, testOutput, testBug())
		if err != nil {
			t.Fatalf("%s: a missing overlay must not refuse the programme: %v", name, err)
		}
		if p.Watermark || strings.Contains(p.VideoFilter, "movie=") || strings.Contains(p.VideoFilter, "overlay") {
			t.Errorf("%s: watermark drawn without a certified GPU overlay: %q", name, p.VideoFilter)
		}
		if !hasFallback(p, "watermark") {
			t.Errorf("%s: the disabled watermark is not declared: %v", name, p.Fallbacks)
		}
	}
}

// Breaks are bug-off: Build (no watermark) is byte-for-byte the pipeline it was, so the SPS of a
// break and a programme can only differ through the overlay stage itself (asserted live).
func TestBuildItem_NilWatermarkIsBuild(t *testing.T) {
	for hostName, host := range testHosts() {
		for srcName, src := range testSources() {
			want := buildGolden(overlayHost(host), src, testOutput)
			p, err := BuildItem(overlayHost(host), src, testOutput, nil)
			if err == nil {
				got := strings.Join(p.ItemArgs("/media/source.mkv", 0, 0, 25, 0), " ")
				want2, _ := Build(overlayHost(host), src, testOutput)
				if strings.Join(want2.ItemArgs("/media/source.mkv", 0, 0, 25, 0), " ") != got || p.Watermark || hasFallback(p, "watermark") {
					t.Errorf("%s/%s: a nil watermark changed the pipeline", hostName, srcName)
				}
			} else if !strings.HasPrefix(want, "REFUSED") {
				t.Errorf("%s/%s: %v", hostName, srcName, err)
			}
		}
	}
}

// A path the filter graph would have to escape is refused as a watermark, never spliced raw.
func TestBuildItem_UnsafeWatermarkPathIsNotSpliced(t *testing.T) {
	wm := testBug()
	wm.Straight = "/data/wm/a:b,c[x].png"
	p, err := BuildItem(overlayHost(testHosts()["nvenc-opencl"]), testSources()["h264-1080p-sdr-25"], testOutput, wm)
	if err != nil {
		t.Fatal(err)
	}
	if p.Watermark || strings.Contains(p.VideoFilter, "a:b") || !hasFallback(p, "watermark") {
		t.Errorf("unsafe path spliced into the graph: %q %v", p.VideoFilter, p.Fallbacks)
	}
}

// pad_vaapi defaults to x=0:y=0: without explicit centring a pillarboxed title sits at the left edge
// and a letterboxed one at the top, and the bug's active-picture anchor would be wrong.
func TestBuild_VAAPIPadCentres(t *testing.T) {
	p, err := Build(testHosts()["vaapi-intel"], testSources()["mpeg2-480i"], testOutput)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.VideoFilter, "pad_vaapi=w=1920:h=1080:x=(ow-iw)/2:y=(oh-ih)/2") {
		t.Errorf("pad_vaapi not centred: %q", p.VideoFilter)
	}
}

func wantAll(t *testing.T, name, graph string, parts ...string) {
	t.Helper()
	for _, part := range parts {
		if !strings.Contains(graph, part) {
			t.Errorf("%s: graph lacks %q:\n%s", name, part, graph)
		}
	}
}

func hasFallback(p Pipeline, stage string) bool {
	for _, f := range p.Fallbacks {
		if strings.HasPrefix(f, stage+":") {
			return true
		}
	}
	return false
}
