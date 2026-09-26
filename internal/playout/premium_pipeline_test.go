package playout

import (
	"errors"
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

func premiumOutput(t *testing.T, class FormatClass) OutputProfile {
	t.Helper()
	out, ok := PremiumOutput(class, testOutput)
	if !ok {
		t.Fatalf("no output for %s", class)
	}
	return out
}

// TestBuild_PremiumGolden pins each family × {4K SDR, 4K HDR passthrough, SDR→HDR10}.
func TestBuild_PremiumGolden(t *testing.T) {
	hosts := testHosts()
	cases := []struct {
		name  string
		class FormatClass
		src   string
	}{
		{"4k-sdr", Format4KSDR, "hevc10-4k-sdr"},
		{"4k-hdr", Format4KHDR, "hevc-4k-hdr-dv"},
		{"sdr-to-hdr", Format4KHDR, "h264-1080p-sdr-25"},
	}
	for _, hostName := range []string{"vaapi-intel", "nvenc-opencl", "videotoolbox", "software", "generic-qsv"} {
		for _, tc := range cases {
			name := hostName + "__" + tc.name
			t.Run(name, func(t *testing.T) {
				checkGolden(t, filepath.Join("premium", name), buildGolden(hosts[hostName], premiumSources()[tc.src], premiumOutput(t, tc.class)))
			})
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
	for _, hostName := range []string{"software", "software-hdrcapable", "generic-qsv"} {
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
