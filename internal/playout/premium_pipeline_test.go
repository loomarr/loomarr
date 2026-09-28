package playout

import (
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Premium sources: a native 4K SDR film (HEVC Main10 BT.709, like the spike's S4) and an HLG
// broadcast recording, alongside the baseline classes.
func premiumSources() map[string]MediaFormat {
	src := testSources()
	src["hevc10-4k-sdr"] = MediaFormat{VideoCodec: "hevc", Width: 3840, Height: 2160, FrameRate: 24000.0 / 1001,
		PixelFormat: "yuv420p10le", ColorTransfer: "bt709", AudioCodec: "eac3", AudioChannels: 6, AudioSampleRate: 48000,
		Container: "matroska,webm"}
	src["hevc10-1080p-hlg"] = MediaFormat{VideoCodec: "hevc", Width: 1920, Height: 1080, FrameRate: 25,
		PixelFormat: "yuv420p10le", ColorTransfer: "arib-std-b67", AudioCodec: "aac", AudioChannels: 2, AudioSampleRate: 48000,
		Container: "matroska,webm"}
	return src
}

// scopeSources are pictures shorter than the 16:9 output, so every family must letterbox them
// (#1673): a 2.40:1 4K HDR10 film and a 2.40:1 1080p SDR episode.
func scopeSources() map[string]MediaFormat {
	src := premiumSources()
	hdr := src["hevc-4k-hdr-dv"]
	hdr.Height = 1600
	sdr := src["h264-1080p-sdr-25"]
	sdr.Height = 800
	return map[string]MediaFormat{"hevc-4k-hdr-scope": hdr, "h264-scope-sdr": sdr}
}

func premiumOutput(t *testing.T, class FormatClass) OutputProfile {
	t.Helper()
	out, ok := PremiumOutput(class, testOutput)
	if !ok {
		t.Fatalf("no output for %s", class)
	}
	return out
}

// TestBuild_PremiumGolden pins each family × {4K SDR, 4K HDR passthrough, SDR→HDR10}, full-frame
// and letterboxed.
func TestBuild_PremiumGolden(t *testing.T) {
	hosts := testHosts()
	sources := premiumSources()
	maps.Copy(sources, scopeSources())
	cases := []struct {
		name  string
		class FormatClass
		src   string
	}{
		{"4k-sdr", Format4KSDR, "hevc10-4k-sdr"},
		{"4k-hdr", Format4KHDR, "hevc-4k-hdr-dv"},
		{"sdr-to-hdr", Format4KHDR, "h264-1080p-sdr-25"},
		{"4k-hdr-scope", Format4KHDR, "hevc-4k-hdr-scope"},
		{"sdr-scope-to-hdr", Format4KHDR, "h264-scope-sdr"},
	}
	for _, hostName := range []string{"vaapi-intel", "nvenc-opencl", "videotoolbox", "software", "generic-qsv"} {
		for _, tc := range cases {
			name := hostName + "__" + tc.name
			t.Run(name, func(t *testing.T) {
				checkGolden(t, filepath.Join("premium", name), buildGolden(hosts[hostName], sources[tc.src], premiumOutput(t, tc.class)))
			})
		}
	}
}

// TestBuild_TenBitLetterboxPadsInOpenCL (#1673): pad_vaapi writes Y=U=V=0 into a P010 frame's bars
// whatever colour it is given (green on screen; measured on the household Arc), while its NV12 bars
// are black. So a PQ letterbox on VAAPI is padded by pad_opencl on the surface mapped from VAAPI,
// the tone-map's zero-copy route, and a host that cannot map to OpenCL refuses it rather than air
// green bars or pad on the CPU. An SDR item is boxed by the libplacebo conversion it already takes,
// as on NVENC, and a 10-bit picture that fills the frame is not padded at all.
func TestBuild_TenBitLetterboxPadsInOpenCL(t *testing.T) {
	hosts := testHosts()
	hdr := premiumOutput(t, Format4KHDR)
	scope := scopeSources()

	const openCLPad = ",hwmap=derive_device=opencl,pad_opencl=w=3840:h=2160:x=(ow-iw)/2:y=(oh-ih)/2:color=black," +
		"hwmap=derive_device=vaapi:reverse=1,fps="
	p, err := Build(hosts["vaapi-intel"], scope["hevc-4k-hdr-scope"], hdr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.VideoFilter, openCLPad) || strings.Contains(p.VideoFilter, "pad_vaapi") {
		t.Errorf("PQ scope: want the letterbox from pad_opencl on the mapped surface, never pad_vaapi on P010: %q", p.VideoFilter)
	}
	checkGPUResidency(t, "vaapi-intel/hevc-4k-hdr-scope", p)
	if _, err := Build(hosts["vaapi-amd"], scope["hevc-4k-hdr-scope"], hdr); !errors.Is(err, ErrRefused) {
		t.Errorf("vaapi-amd PQ scope: no OpenCL mapping, so the letterbox must be refused, got %v", err)
	}

	for _, hostName := range []string{"vaapi-intel", "vaapi-amd"} {
		p, err := Build(hosts[hostName], scope["h264-scope-sdr"], hdr)
		if err != nil {
			t.Fatalf("%s SDR scope: %v", hostName, err)
		}
		if !strings.Contains(p.VideoFilter, ":pos_y=140:pos_w=1920:pos_h=800:fillcolor=black,hwupload,scale_vaapi=w=3840:h=2160:format=p010,fps=") ||
			strings.Contains(p.VideoFilter, "pad_") {
			t.Errorf("%s SDR scope: want the letterbox boxed by the libplacebo conversion, no pad: %q", hostName, p.VideoFilter)
		}
	}

	for _, srcName := range []string{"hevc-4k-hdr-dv", "h264-1080p-sdr-25"} {
		for _, hostName := range []string{"vaapi-intel", "vaapi-amd"} {
			p, err := Build(hosts[hostName], premiumSources()[srcName], hdr)
			if err != nil {
				t.Fatalf("%s/%s: a 16:9 picture needs no letterbox, so no OpenCL: %v", hostName, srcName, err)
			}
			if strings.Contains(p.VideoFilter, "pad_") {
				t.Errorf("%s/%s: the picture fills the frame, nothing to pad: %q", hostName, srcName, p.VideoFilter)
			}
		}
	}
	// The 8-bit graphs keep pad_vaapi: its NV12 bars are black (the 4K SDR premium, the tone-map).
	for _, tc := range []struct {
		label string
		src   string
		out   OutputProfile
	}{
		{"4K SDR premium", "h264-scope-sdr", premiumOutput(t, Format4KSDR)},
		{"1080p tone-map", "hevc-4k-hdr-scope", testOutput},
	} {
		p, err := Build(hosts["vaapi-intel"], scopeSources()[tc.src], tc.out)
		if err != nil {
			t.Fatalf("%s: %v", tc.label, err)
		}
		if want := fmt.Sprintf("pad_vaapi=w=%d:h=%d:x=(ow-iw)/2:y=(oh-ih)/2", tc.out.Width, tc.out.Height); !strings.Contains(p.VideoFilter, want) {
			t.Errorf("%s: want %s: %q", tc.label, want, p.VideoFilter)
		}
	}
}

// Apple's HLS authoring spec requires HEVC in fMP4 to be tagged hvc1 (parameter sets in the sample
// entry); ffmpeg's mp4 muxer writes hev1 unless told. Every family's HEVC encode is tagged, so the
// premium items and the premium slate share one sample entry type; H.264 is never tagged.
func TestBuild_HEVCIsTaggedHvc1(t *testing.T) {
	src := premiumSources()["h264-1080p-sdr-25"]
	for _, hostName := range []string{"vaapi-intel", "nvenc-opencl", "videotoolbox", "software", "generic-qsv"} {
		host := testHosts()[hostName]
		for _, class := range []FormatClass{Format4KSDR, Format4KHDR} {
			p, err := Build(host, src, premiumOutput(t, class))
			if err != nil {
				continue // a family that refuses this premium has no encode to tag
			}
			if i := slices.Index(p.VideoEncode, "-tag:v"); i < 0 || i+1 >= len(p.VideoEncode) || p.VideoEncode[i+1] != "hvc1" {
				t.Errorf("%s %s: video encode %v is not tagged hvc1", hostName, class, p.VideoEncode)
			}
		}
		if p, err := Build(host, src, testOutput); err == nil && slices.Contains(p.VideoEncode, "-tag:v") {
			t.Errorf("%s baseline: H.264 encode is tagged: %v", hostName, p.VideoEncode)
		}
	}
}

// TestBuild_PremiumUniformOutput: within a family and premium class every source gets the same
// encoder arguments and ends in the same cadence, side-data strip and colour labels, so no seam
// changes the VPS/SPS/PPS or the HDR metadata (the live test checks the bytes).
func TestBuild_PremiumUniformOutput(t *testing.T) {
	for _, hostName := range []string{"vaapi-intel", "vaapi-amd", "nvenc-opencl", "nvenc-libplacebo", "videotoolbox"} {
		host := testHosts()[hostName]
		for _, class := range []FormatClass{Format4KSDR, Format4KHDR} {
			out := premiumOutput(t, class)
			wantEnd, wantProfile := ",fps=25,"+conformSAR+","+stripSideData+","+conformColour, "main"
			if out.HDR {
				wantEnd, wantProfile = ",fps=25,"+conformSAR+","+stripSideData+","+conformHDR10, "main10"
			}
			var encode []string
			built := 0
			for srcName, src := range premiumSources() {
				label := hostName + "/" + string(class) + "/" + srcName
				p, err := Build(host, src, out)
				if err != nil {
					if !out.HDR && src.HDR() && errors.Is(err, ErrRefused) {
						continue // an SDR premium never carries HDR items
					}
					t.Fatalf("%s: %v", label, err)
				}
				built++
				if encode == nil {
					encode = p.VideoEncode
				} else if !slices.Equal(encode, p.VideoEncode) {
					t.Errorf("%s: encoder args differ between sources: %q vs %q", label, p.VideoEncode, encode)
				}
				if i := slices.Index(p.VideoEncode, "-profile:v"); i < 0 || p.VideoEncode[i+1] != wantProfile {
					t.Errorf("%s: want HEVC profile %s: %q", label, wantProfile, p.VideoEncode)
				}
				if !strings.HasSuffix(p.VideoFilter, wantEnd) {
					t.Errorf("%s: graph must end %q: %q", label, wantEnd, p.VideoFilter)
				}
				if !strings.Contains(p.VideoFilter, "3840") {
					t.Errorf("%s: graph never pads to 3840x2160: %q", label, p.VideoFilter)
				}
				if host.Family != FamilyVideoToolbox {
					checkGPUResidency(t, label, p)
				}
			}
			if built < 4 {
				t.Errorf("%s/%s: only %d sources built", hostName, class, built)
			}
		}
	}
}

// TestBuild_SDRToHDR10: SDR and HLG items on a 4K HDR channel go through libplacebo at source size,
// then the GPU upscales. Never the Intel VPP conversion (~2,600-nit white, spike 0b), never an
// inverse tone-map, and a PQ item never goes through the conversion.
func TestBuild_SDRToHDR10(t *testing.T) {
	out := premiumOutput(t, Format4KHDR)
	for _, hostName := range []string{"vaapi-intel", "nvenc-opencl"} {
		host := testHosts()[hostName]
		for srcName, src := range premiumSources() {
			p, err := Build(host, src, out)
			if err != nil {
				t.Fatalf("%s/%s: %v", hostName, srcName, err)
			}
			label := hostName + "/" + srcName
			filters := strings.Split(p.VideoFilter, ",")
			convert := slices.IndexFunc(filters, func(f string) bool { return strings.HasPrefix(f, sdrToHDR10) })
			upscale := -1
			for i, f := range filters {
				if strings.HasPrefix(f, "scale_vaapi=") || strings.HasPrefix(f, "scale_cuda=") {
					upscale = i
				}
			}
			switch {
			case src.PQ() && convert >= 0:
				t.Errorf("%s: a PQ item must pass through, not convert: %q", label, p.VideoFilter)
			case !src.PQ() && (convert < 0 || upscale < convert):
				t.Errorf("%s: want libplacebo at source size, then the GPU scale: %q", label, p.VideoFilter)
			}
			if strings.Contains(p.VideoFilter, "out_color_transfer") || strings.Contains(p.VideoFilter, "inverse_tonemapping") ||
				strings.Contains(p.VideoFilter, "tonemap_vaapi") {
				t.Errorf("%s: forbidden SDR→HDR path in %q", label, p.VideoFilter)
			}
		}
	}
	// pad_cuda takes 8-bit frames only: an NVENC HDR10 graph never pads on the GPU, and a letterbox
	// the source needs is a declared CPU fallback.
	nv := testHosts()["nvenc-opencl"]
	for srcName, src := range premiumSources() {
		p, _ := Build(nv, src, out)
		if strings.Contains(p.VideoFilter, "pad_cuda") {
			t.Errorf("nvenc/%s: pad_cuda on 10-bit frames: %q", srcName, p.VideoFilter)
		}
	}
	scope := premiumSources()["hevc-4k-hdr-dv"]
	scope.Height = 1600
	if p, _ := Build(nv, scope, out); !strings.Contains(p.VideoFilter, "pad=3840:2160") || !slices.ContainsFunc(p.Fallbacks, func(f string) bool {
		return strings.HasPrefix(f, "pad:")
	}) {
		t.Errorf("a 3840x1600 HDR10 film needs the declared CPU letterbox: %q %q", p.VideoFilter, p.Fallbacks)
	}

	noPlacebo := testHosts()["nvenc-cputonemap"]
	if _, err := Build(noPlacebo, premiumSources()["h264-1080p-sdr-25"], out); !errors.Is(err, ErrRefused) {
		t.Errorf("an SDR item on an HDR10 channel without libplacebo must be refused, got %v", err)
	}
}

// TestBuild_PremiumRefusals: software and generic hosts never produce a premium; a 4K SDR premium
// never tone-maps an HDR item at 4K (0.69x on NVIDIA, spike 0b).
func TestBuild_PremiumRefusals(t *testing.T) {
	hosts := testHosts()
	sdr := premiumSources()["h264-1080p-sdr-25"]
	for _, hostName := range []string{"software", "generic-qsv"} {
		for _, class := range []FormatClass{Format4KSDR, Format4KHDR} {
			if _, err := Build(hosts[hostName], sdr, premiumOutput(t, class)); !errors.Is(err, ErrRefused) {
				t.Errorf("%s/%s: want ErrRefused, got %v", hostName, class, err)
			}
		}
	}
	if _, err := Build(hosts["nvenc-opencl"], premiumSources()["hevc-4k-hdr-dv"], premiumOutput(t, Format4KSDR)); !errors.Is(err, ErrRefused) {
		t.Errorf("4K SDR premium with an HDR item: want ErrRefused, got %v", err)
	}
}
