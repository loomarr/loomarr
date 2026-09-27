package playout

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

var updatePipelineGolden = flag.Bool("update-pipeline", false, "rewrite testdata/pipeline goldens")

// Host profiles as data: the certified families plus each declared fallback.
func testHosts() map[string]HostProfile {
	arc := HostProfile{Family: FamilyVAAPI, RenderNode: "/dev/dri/renderD128", DecodeCodecs: vaapiDecodes, TonemapOpenCL: true, Libplacebo: true, CPUTonemap: true}
	// AMD (no OpenCL-VAAPI interop) and an Intel host missing the OpenCL runtime demote to this.
	amd := arc
	amd.TonemapOpenCL = false
	vaCPU := amd
	vaCPU.Libplacebo = false
	nv := HostProfile{Family: FamilyNVENC, DecodeCodecs: cudaDecodes, TonemapOpenCL: true, Libplacebo: true, CPUTonemap: true}
	nvPlacebo := nv
	nvPlacebo.TonemapOpenCL = false
	nvCPU := nvPlacebo
	nvCPU.Libplacebo = false
	sw := HostProfile{Family: FamilySoftware, CPUTonemap: true}
	return map[string]HostProfile{
		"vaapi-intel": arc, "vaapi-amd": amd, "vaapi-cputonemap": vaCPU,
		"nvenc-opencl": nv, "nvenc-libplacebo": nvPlacebo, "nvenc-cputonemap": nvCPU,
		"software":     sw,
		"videotoolbox": {Family: FamilyVideoToolbox, DecodeCodecs: vtDecodes, Libplacebo: true, CPUTonemap: true},
		"generic-qsv":  {Family: FamilyGeneric, Encoder: EncoderQSV, DecodeCodecs: anyCodec, CPUTonemap: true},
	}
}

// Content classes, as Loomarr's inventory measures them.
func testSources() map[string]MediaFormat {
	h264 := func(fps float64) MediaFormat {
		return MediaFormat{VideoCodec: "h264", Width: 1920, Height: 1080, FrameRate: fps, PixelFormat: "yuv420p",
			AudioCodec: "eac3", AudioChannels: 6, AudioSampleRate: 48000, Container: "matroska,webm"}
	}
	return map[string]MediaFormat{
		"h264-1080p-sdr-23.976": h264(24000.0 / 1001),
		"h264-1080p-sdr-25":     h264(25),
		"h264-1080p-sdr-29.97":  h264(30000.0 / 1001),
		"hevc10-1080p": {VideoCodec: "hevc", Width: 1920, Height: 1080, FrameRate: 25, PixelFormat: "yuv420p10le",
			AudioCodec: "eac3", AudioChannels: 6, AudioSampleRate: 48000, Container: "matroska,webm"},
		"hevc-4k-hdr-dv": {VideoCodec: "hevc", Width: 3840, Height: 2160, FrameRate: 24000.0 / 1001, PixelFormat: "yuv420p10le",
			ColorTransfer: "smpte2084", AudioCodec: "truehd", AudioChannels: 8, AudioSampleRate: 48000, Container: "matroska,webm"},
		"mpeg2-480i": {VideoCodec: "mpeg2video", Width: 720, Height: 480, FrameRate: 30000.0 / 1001, PixelFormat: "yuv420p",
			Interlaced: true, AudioCodec: "ac3", AudioChannels: 2, AudioSampleRate: 48000, Container: "mpegts"},
		"filler-clip": {VideoCodec: "h264", Width: 1280, Height: 720, FrameRate: 30, PixelFormat: "yuv420p",
			AudioCodec: "aac", AudioChannels: 2, AudioSampleRate: 44100, Container: "mov,mp4,m4a,3gp,3g2,mj2"},
	}
}

var testOutput = OutputProfile{Width: 1920, Height: 1080, FPS: 25, Quality: 22, TargetKbps: 8000, MaxKbps: 12000, GOPSeconds: 1, AudioKbps: 160}

// TestBuild_Golden pins the exact argv for every family × content class. Regenerate with
// `go test ./internal/playout -run TestBuild_Golden -update-pipeline` and review the diff.
func TestBuild_Golden(t *testing.T) {
	for hostName, host := range testHosts() {
		for srcName, src := range testSources() {
			name := hostName + "__" + srcName
			t.Run(name, func(t *testing.T) { checkGolden(t, name, buildGolden(host, src, testOutput)) })
		}
	}
}

// buildGolden is one item's argv plus its declared fallbacks and missing facts, or the refusal.
func buildGolden(host HostProfile, src MediaFormat, out OutputProfile) string {
	p, err := Build(host, src, out)
	if err != nil {
		return "REFUSED: " + err.Error() + "\n"
	}
	got := strings.Join(p.ItemArgs("/media/source.mkv", 90*time.Second, 750, out.FPS, 1), "\n") + "\n"
	if len(p.Fallbacks) > 0 {
		got += "# fallbacks: " + strings.Join(p.Fallbacks, "; ") + "\n"
	}
	if len(p.MissingFacts) > 0 {
		got += "# missing facts: " + strings.Join(p.MissingFacts, ", ") + "\n"
	}
	return got
}

// checkGolden compares got with testdata/pipeline/<name>.golden, or rewrites it with -update-pipeline.
func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "pipeline", name+".golden")
	if *updatePipelineGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden (run with -update-pipeline): %v", err)
	}
	if got != string(want) {
		t.Errorf("argv drifted from %s\n--- got\n%s--- want\n%s", path, got, want)
	}
}

// gpuFilter reports whether a filter runs on GPU frames; hwAgnostic filters run on either.
func gpuFilter(name string) bool {
	return strings.HasSuffix(name, "_vaapi") || strings.HasSuffix(name, "_cuda") ||
		strings.HasSuffix(name, "_opencl") || strings.HasSuffix(name, "_vt") || strings.HasSuffix(name, "_videotoolbox")
}

// hwAgnostic filters touch only timestamps or frame metadata, never pixels, so they run on GPU
// frames (sidedata on CUDA frames: TestSlateSampleDescriptionMatchesEveryItem/nvenc).
func hwAgnostic(name string) bool {
	return name == "fps" || name == "setsar" || name == "setparams" || name == "sidedata"
}

// TestBuild_NoCPUFilterOnAGPUPathExceptItsDeclaredFallback walks each GPU family's graph tracking
// where the frames are. A CPU filter may only run on downloaded (or CPU-decoded) frames, and every
// such stretch must be declared in Pipeline.Fallbacks. The encoder must receive GPU frames.
func TestBuild_NoCPUFilterOnAGPUPathExceptItsDeclaredFallback(t *testing.T) {
	for hostName, host := range testHosts() {
		if host.Family != FamilyVAAPI && host.Family != FamilyNVENC {
			continue
		}
		for srcName, src := range testSources() {
			p, err := Build(host, src, testOutput)
			if err != nil {
				t.Fatalf("%s/%s: a GPU family with a CPU tone-mapper must not refuse: %v", hostName, srcName, err)
			}
			checkGPUResidency(t, hostName+"/"+srcName, p)
		}
	}
}

// checkGPUResidency walks a GPU family's graph tracking where the frames are.
func checkGPUResidency(t *testing.T, label string, p Pipeline) {
	t.Helper()
	onGPU := slices.Contains(p.PreInput, "-hwaccel_output_format")
	if !onGPU && len(p.Fallbacks) == 0 {
		t.Errorf("%s: CPU decode without a declared decode fallback", label)
	}
	cpuStretch := !onGPU
	for _, f := range strings.Split(p.VideoFilter, ",") {
		name, _, _ := strings.Cut(f, "=")
		switch {
		case name == "hwupload" || name == "hwupload_cuda":
			onGPU = true
		case name == "hwdownload":
			onGPU, cpuStretch = false, true
		case name == "hwmap":
			// VAAPI <-> OpenCL surface mapping: GPU frames in and out, zero-copy.
			if !onGPU {
				t.Errorf("%s: hwmap fed CPU frames in %q", label, p.VideoFilter)
			}
		case name == "libplacebo":
			// Its own Vulkan device: CPU frames in and out.
			if onGPU {
				t.Errorf("%s: libplacebo fed GPU frames in %q", label, p.VideoFilter)
			}
		case gpuFilter(name):
			if !onGPU {
				t.Errorf("%s: GPU filter %s on CPU frames in %q", label, name, p.VideoFilter)
			}
		case hwAgnostic(name):
		default:
			if onGPU {
				t.Errorf("%s: CPU filter %q on GPU frames in %q", label, f, p.VideoFilter)
			}
		}
	}
	if cpuStretch && len(p.Fallbacks) == 0 {
		t.Errorf("%s: frames left the GPU without a declared fallback: %q", label, p.VideoFilter)
	}
	if !onGPU {
		t.Errorf("%s: the encoder must receive GPU frames: %q", label, p.VideoFilter)
	}
}

// TestBuild_FullGPUForCertifiedSources: the classes the certified GPUs decode never leave the GPU
// unless the host lacks a GPU tone-mapper.
func TestBuild_FullGPUForCertifiedSources(t *testing.T) {
	hosts := testHosts()
	for _, hostName := range []string{"vaapi-intel", "nvenc-opencl"} {
		for _, srcName := range []string{"h264-1080p-sdr-25", "hevc10-1080p", "hevc-4k-hdr-dv", "filler-clip", "mpeg2-480i"} {
			p, err := Build(hosts[hostName], testSources()[srcName], testOutput)
			if err != nil {
				t.Fatal(err)
			}
			for _, fb := range p.Fallbacks {
				if strings.HasPrefix(fb, "decode") || (hostName == "vaapi-intel" && strings.HasPrefix(fb, "tonemap")) {
					t.Errorf("%s/%s: unexpected fallback %q", hostName, srcName, fb)
				}
			}
		}
	}
}

// TestBuild_ScaleBeforeToneMap: tone-mapping at 4K runs 0.7x on the A380 and 2.6x after the scale.
func TestBuild_ScaleBeforeToneMap(t *testing.T) {
	src := testSources()["hevc-4k-hdr-dv"]
	for hostName, host := range testHosts() {
		p, err := Build(host, src, testOutput)
		if errors.Is(err, ErrRefused) {
			continue
		}
		scale, tonemap := -1, -1
		for i, f := range strings.Split(p.VideoFilter, ",") {
			name, _, _ := strings.Cut(f, "=")
			if scale < 0 && strings.HasPrefix(name, "scale") {
				scale = i
			}
			if tonemap < 0 && (strings.HasPrefix(name, "tonemap") || name == "libplacebo" || f == "zscale=t=linear:npl=100") {
				tonemap = i
			}
		}
		if scale < 0 || tonemap < 0 || scale > tonemap {
			t.Errorf("%s: want a scale before the tone-map, got %q", hostName, p.VideoFilter)
		}
	}
}

// TestBuild_UniformOutput: within a family every class gets the same encoder arguments, and every
// graph ends in the channel geometry, cadence and BT.709 labels, so no boundary changes the SPS.
func TestBuild_UniformOutput(t *testing.T) {
	for hostName, host := range testHosts() {
		var encode []string
		for srcName, src := range testSources() {
			p, err := Build(host, src, testOutput)
			if err != nil {
				continue
			}
			if encode == nil {
				encode = p.VideoEncode
			} else if !slices.Equal(encode, p.VideoEncode) {
				t.Errorf("%s/%s: encoder args differ between classes: %q vs %q", hostName, srcName, p.VideoEncode, encode)
			}
			// The generic family's own upload step follows; every other graph ends here.
			end := p.VideoFilter
			if host.Family == FamilyGeneric {
				end, _, _ = strings.Cut(end, conformHDRMetadata)
				end += conformHDRMetadata
			}
			if !strings.HasSuffix(end, ",fps=25,"+conformSAR+","+conformColour+","+conformHDRMetadata) {
				t.Errorf("%s/%s: graph must end fps=25 + setsar + setparams + HDR metadata drop: %q", hostName, srcName, p.VideoFilter)
			}
			if !strings.Contains(p.VideoFilter, "1920") {
				t.Errorf("%s/%s: graph never pads to 1920x1080: %q", hostName, srcName, p.VideoFilter)
			}
			if !slices.Equal(p.AudioEncode, []string{"-c:a", "aac", "-profile:a", "aac_low", "-b:a", "160k", "-ac", "2", "-ar", "48000"}) {
				t.Errorf("%s/%s: audio must be AAC-LC stereo 48 kHz: %q", hostName, srcName, p.AudioEncode)
			}
		}
	}
}

// TestBuild_MinimalProbeOnlyWithFacts: minimal probing without Loomarr's facts cannot find a
// packet-discovered container's streams; the item falls back to ffmpeg's own probe.
func TestBuild_MinimalProbeOnlyWithFacts(t *testing.T) {
	host := testHosts()["vaapi-intel"]
	for srcName, src := range testSources() {
		p, _ := Build(host, src, testOutput)
		minimal := slices.Contains(p.PreInput, "-probesize")
		if minimal == (len(p.MissingFacts) > 0) {
			t.Errorf("%s: minimal=%v with missing facts %q", srcName, minimal, p.MissingFacts)
		}
	}
	p, _ := Build(host, MediaFormat{}, testOutput)
	if slices.Contains(p.PreInput, "-probesize") {
		t.Error("an unmeasured source must keep ffmpeg's probe")
	}
}

// TestBuild_IntelHDRUsesOpenCLNeverTonemapVAAPI (#1516): tonemap_vaapi outputs a black picture on
// the household Arc (Y=16 on every frame, normal speed, exit 0). No host profile, source or build
// may ever emit it; Intel HDR maps the scaled VAAPI surface into OpenCL and back, zero-copy.
func TestBuild_IntelHDRUsesOpenCLNeverTonemapVAAPI(t *testing.T) {
	for _, gpu := range []GPUFilters{{}, {TonemapOpenCL: true}, {Libplacebo: true}, {TonemapOpenCL: true, Libplacebo: true}} {
		for _, enc := range h264Engines {
			for srcName, src := range testSources() {
				p, err := Build(HostFor(enc, true, gpu), src, testOutput)
				if err == nil && strings.Contains(p.VideoFilter, "tonemap_vaapi") {
					t.Errorf("%s %+v %s: tonemap_vaapi (black on the Arc, #1516): %q", enc, gpu, srcName, p.VideoFilter)
				}
			}
		}
	}
	p, err := Build(HostFor(EncoderVAAPI, true, GPUFilters{TonemapOpenCL: true, Libplacebo: true}), testSources()["hevc-4k-hdr-dv"], testOutput)
	if err != nil {
		t.Fatal(err)
	}
	want := "scale_vaapi=w=1920:h=1080:force_original_aspect_ratio=decrease:force_divisible_by=2:format=p010," +
		"hwmap=derive_device=opencl,tonemap_opencl=tonemap=hable:desat=0:t=bt709:m=bt709:p=bt709:r=tv:format=nv12," +
		"hwmap=derive_device=vaapi:reverse=1,pad_vaapi=w=1920:h=1080,"
	if !strings.HasPrefix(p.VideoFilter, want) {
		t.Errorf("Intel HDR graph:\n got %q\nwant prefix %q", p.VideoFilter, want)
	}
	if len(p.Fallbacks) != 0 {
		t.Errorf("the zero-copy OpenCL path declares no fallback, got %q", p.Fallbacks)
	}
}

// TestBuild_OneToneCurveEverywhere (G11): for every curve on every host, the graph has exactly one
// tone-mapper and it runs that curve in its own spelling; the CPU tone-mapper, which lacks the three
// libplacebo curves, substitutes Mobius and declares it.
func TestBuild_OneToneCurveEverywhere(t *testing.T) {
	src := testSources()["hevc-4k-hdr-dv"]
	for _, c := range ToneCurves {
		out := testOutput
		out.ToneCurve = c
		for hostName, host := range testHosts() {
			p, err := Build(host, src, out)
			if errors.Is(err, ErrRefused) {
				continue
			}
			var got []string
			for _, f := range strings.Split(p.VideoFilter, ",") {
				name, opts, _ := strings.Cut(f, "=")
				for _, opt := range strings.Split(opts, ":") {
					if k, v, _ := strings.Cut(opt, "="); k == "tonemap" || k == "tonemapping" {
						got = append(got, name+"="+v)
					}
				}
			}
			cpu, exact := c.cpu()
			want := map[string]string{
				TonemapperOpenCL:     "tonemap_opencl=" + c.openCL(),
				TonemapperLibplacebo: "libplacebo=" + c.placebo(),
				TonemapperCPU:        "tonemap=" + string(cpu),
			}[p.Tonemapper]
			if len(got) != 1 || got[0] != want {
				t.Errorf("%s/%s: tone curves %q, want exactly [%s]: %q", c, hostName, got, want, p.VideoFilter)
			}
			declared := slices.Contains(p.Fallbacks, "tonemap: the CPU tone-mapper has no "+string(c)+"; mobius instead")
			if substituted := p.Tonemapper == TonemapperCPU && !exact; declared != substituted {
				t.Errorf("%s/%s: CPU substitute declared=%v, substituted=%v: %q", c, hostName, declared, substituted, p.Fallbacks)
			}
		}
	}
}

// TestToneCurve_SpellingsExistInEachFilter: each curve name the builder emits is one the filter
// accepts, per `ffmpeg -h filter=<name>` on the pinned n8.1 build. A name a filter lacks fails the
// spawn ("Invalid argument"): the ladder rescues it, but every program pays a failed start. That is
// how bt2390 on tonemap_opencl was caught (live test, dev GeForce).
func TestToneCurve_SpellingsExistInEachFilter(t *testing.T) {
	cpuAndOpenCL := []string{"none", "linear", "gamma", "clip", "reinhard", "hable", "mobius"}
	placebo := []string{"auto", "clip", "st2094-40", "st2094-10", "bt.2390", "bt.2446a", "spline", "reinhard", "mobius", "hable", "gamma", "linear"}
	for _, c := range ToneCurves {
		if o := c.openCL(); o != "" && !slices.Contains(cpuAndOpenCL, o) {
			t.Errorf("%s: tonemap_opencl has no %q", c, o)
		}
		if !slices.Contains(placebo, c.placebo()) {
			t.Errorf("%s: libplacebo has no %q", c, c.placebo())
		}
		if cpu, _ := c.cpu(); !slices.Contains(cpuAndOpenCL, string(cpu)) {
			t.Errorf("%s: tonemap has no %q", c, cpu)
		}
		if ParseToneCurve(string(c)) != c {
			t.Errorf("%s does not round-trip", c)
		}
	}
	if ParseToneCurve("") != DefaultToneCurve || ParseToneCurve("nonsense") != DefaultToneCurve {
		t.Error("an empty or unknown curve must be the default")
	}
}

// TestBuild_GoldenToneCurves pins the HDR graph of every host for every non-default curve.
func TestBuild_GoldenToneCurves(t *testing.T) {
	for _, c := range ToneCurves[1:] {
		out := testOutput
		out.ToneCurve = c
		for hostName, host := range testHosts() {
			t.Run(string(c)+"__"+hostName, func(t *testing.T) {
				checkGolden(t, "curve-"+string(c)+"__"+hostName, buildGolden(host, testSources()["hevc-4k-hdr-dv"], out))
			})
		}
	}
}

// TestDemoteTonemap_VAAPIFallsToCPU: an Intel host without the OpenCL runtime (or AMD) on a build
// without libplacebo fails device derivation before any output; the ladder then tone-maps on the CPU
// after the GPU downscale.
func TestDemoteTonemap_VAAPIFallsToCPU(t *testing.T) {
	spec := ProgramSpec{Profile: Profile{Width: 1920, Height: 1080, Framerate: 25, Encoder: EncoderVAAPI},
		Source: testSources()["hevc-4k-hdr-dv"], Tonemap: true, GPUTonemap: GPUFilters{TonemapOpenCL: true}}
	if p, _ := spec.Pipeline(); !strings.Contains(p.VideoFilter, "tonemap_opencl") {
		t.Fatalf("first attempt must be OpenCL: %q", p.VideoFilter)
	}
	if !spec.DemoteTonemap() {
		t.Fatal("OpenCL must demote")
	}
	p, err := spec.Pipeline()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.VideoFilter, "scale_vaapi=w=1920:h=1080:force_original_aspect_ratio=decrease:force_divisible_by=2:format=p010,hwdownload,format=p010le,"+hdrToSDRChain) ||
		!slices.Equal(p.Fallbacks, []string{"tonemap: no GPU tone-mapper runs hable on this host"}) {
		t.Errorf("demoted VAAPI must CPU tone-map after the GPU downscale, declared: %q %q", p.VideoFilter, p.Fallbacks)
	}
	if spec.DemoteTonemap() {
		t.Error("the CPU tone-map is the end of the ladder")
	}
}

// TestDemoteTonemap_OrderPerCurve: each family walks the curve's preferred GPU tone-mapper, the other
// one that has the curve, then the CPU. Hable is the maintainer order (#1512).
func TestDemoteTonemap_OrderPerCurve(t *testing.T) {
	want := map[ToneCurve][]string{
		ToneCurveHable:    {TonemapperOpenCL, TonemapperLibplacebo, TonemapperCPU},
		ToneCurveMobius:   {TonemapperOpenCL, TonemapperLibplacebo, TonemapperCPU},
		ToneCurveReinhard: {TonemapperOpenCL, TonemapperLibplacebo, TonemapperCPU},
		ToneCurveBT2390:   {TonemapperLibplacebo, TonemapperCPU},
		ToneCurveBT2446a:  {TonemapperLibplacebo, TonemapperCPU},
		ToneCurveSpline:   {TonemapperLibplacebo, TonemapperCPU},
	}
	for _, enc := range []Encoder{EncoderVAAPI, EncoderNVENC} {
		for _, c := range ToneCurves {
			spec := ProgramSpec{Profile: Profile{Width: 1920, Height: 1080, Framerate: 25, Encoder: enc},
				Source: testSources()["hevc-4k-hdr-dv"], Tonemap: true, ToneCurve: c,
				GPUTonemap: GPUFilters{TonemapOpenCL: true, Libplacebo: true}}
			var steps []string
			for {
				p, err := spec.Pipeline()
				if err != nil {
					t.Fatal(err)
				}
				steps = append(steps, p.Tonemapper)
				if !spec.DemoteTonemap() {
					break
				}
			}
			if !slices.Equal(steps, want[c]) {
				t.Errorf("%s/%s: tone-map order %q, want %q", enc, c, steps, want[c])
			}
		}
	}
	sdr := ProgramSpec{Profile: Profile{Encoder: EncoderVAAPI, Width: 1920, Height: 1080, Framerate: 25},
		Source: testSources()["h264-1080p-sdr-25"], GPUTonemap: GPUFilters{TonemapOpenCL: true}}
	if sdr.DemoteTonemap() {
		t.Error("an SDR source has no tone-mapper to demote")
	}
}

// TestChannelOutput_RateScalesWithRungPixels: q22 on every rung, but 8M/12M is the 1080p budget.
// A lower rung gets the target and cap scaled by its pixel count (to 100 kbit/s, with a floor),
// and keeps its geometry, cadence and audio.
func TestChannelOutput_RateScalesWithRungPixels(t *testing.T) {
	want := map[[2]int][2]int{
		{1920, 1080}: {8000, 12000},
		{1280, 720}:  {3600, 5300},
		{854, 480}:   {1600, 2400},
		{640, 360}:   {1000, 1500}, // floor
	}
	for geom, rate := range want {
		p := Profile{Width: geom[0], Height: geom[1], Framerate: 25, AudioBitrate: 96, VideoBitrate: 1200, Encoder: EncoderNVENC}
		out := ChannelOutput(p)
		if out.TargetKbps != rate[0] || out.MaxKbps != rate[1] || out.Quality != 22 {
			t.Errorf("%dx%d: rate q%d %d/%d, want q22 %d/%d", geom[0], geom[1], out.Quality, out.TargetKbps, out.MaxKbps, rate[0], rate[1])
		}
		if out.Width != geom[0] || out.Height != geom[1] || out.FPS != 25 || out.AudioKbps != 96 {
			t.Errorf("%dx%d: rung geometry/cadence/audio changed: %+v", geom[0], geom[1], out)
		}
	}
	// Every shipped rung lands on the scaled budget, and each family's encoder carries it.
	for tier, l := range ladders {
		for _, r := range l {
			out := ChannelOutput(Profile{Width: r.width, Height: r.height, Framerate: r.framerate, AudioBitrate: r.audioBitrate, Encoder: EncoderNVENC})
			if _, ok := want[[2]int{r.width, r.height}]; !ok {
				t.Errorf("%s: rung %dx%d has no expected budget", tier, r.width, r.height)
			}
			for hostName, host := range testHosts() {
				p, err := Build(host, testSources()["h264-1080p-sdr-25"], out)
				if err != nil {
					t.Fatalf("%s: %v", hostName, err)
				}
				if !slices.Contains(p.VideoEncode, strconv.Itoa(out.MaxKbps)+"k") {
					t.Errorf("%s/%s %dx%d: encoder lacks the scaled cap %dk: %q", tier, hostName, r.width, r.height, out.MaxKbps, p.VideoEncode)
				}
			}
		}
	}
}
