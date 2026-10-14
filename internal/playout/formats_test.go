package playout

import "testing"

// TestDeriveChannelFormats: top resolution and dynamic range are derived INDEPENDENTLY from the
// lineup's inventory facts (maintainer rule, #1512 G10). The baseline always exists.
func TestDeriveChannelFormats(t *testing.T) {
	sdr1080 := MediaFormat{VideoCodec: "h264", Width: 1920, Height: 1080}
	hdr1080 := MediaFormat{VideoCodec: "hevc", Width: 1920, Height: 1080, ColorTransfer: "smpte2084"}
	hlg1080 := MediaFormat{VideoCodec: "hevc", Width: 1920, Height: 1080, ColorTransfer: "arib-std-b67"}
	sdr4K := MediaFormat{VideoCodec: "hevc", Width: 3840, Height: 2160, ColorTransfer: "bt709"}
	hdr4K := MediaFormat{VideoCodec: "hevc", Width: 3840, Height: 2160, ColorTransfer: "smpte2084"}
	scope4K := MediaFormat{VideoCodec: "hevc", Width: 3840, Height: 1600} // 2.40:1 UHD film
	qhd := MediaFormat{VideoCodec: "h264", Width: 2560, Height: 1440}
	unmeasured := MediaFormat{}

	cases := []struct {
		name    string
		items   []MediaFormat
		premium FormatClass
	}{
		{"empty lineup: baseline only", nil, ""},
		{"no inventory facts: baseline only", []MediaFormat{unmeasured, unmeasured}, ""},
		{"all 1080p SDR: baseline only", []MediaFormat{sdr1080, sdr1080}, ""},
		{"1080p HDR without 4K: baseline only", []MediaFormat{hdr1080, sdr1080}, ""},
		{"QHD is not 4K", []MediaFormat{qhd}, ""},
		{"4K SDR items, none HDR: 4K SDR", []MediaFormat{sdr1080, sdr4K}, Format4KSDR},
		{"scope 4K (3840x1600) counts as 4K", []MediaFormat{scope4K}, Format4KSDR},
		{"4K HDR item: 4K HDR", []MediaFormat{sdr1080, hdr4K}, Format4KHDR},
		{"1080p HDR plus 4K SDR: 4K HDR (independent axes)", []MediaFormat{hdr1080, sdr4K}, Format4KHDR},
		{"HLG counts as HDR", []MediaFormat{hlg1080, sdr4K}, Format4KHDR},
		{"unmeasured items do not block a measured 4K HDR", []MediaFormat{unmeasured, hdr4K}, Format4KHDR},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DeriveChannelFormats(tc.items)
			if got.Baseline != FormatBaseline {
				t.Errorf("baseline = %q, want %q (it always exists)", got.Baseline, FormatBaseline)
			}
			if got.Premium != tc.premium {
				t.Errorf("premium = %q, want %q", got.Premium, tc.premium)
			}
		})
	}
}

// TestChannelFormats_OnHost: software-only hosts never produce a premium format, and neither does a
// host that cannot run the premium's stages (maintainer rules, spike 0b decisions 3 and 5).
func TestChannelFormats_OnHost(t *testing.T) {
	hosts := testHosts()
	nvNoPlacebo := hosts["nvenc-cputonemap"]
	cases := []struct {
		name    string
		host    HostProfile
		formats ChannelFormats
		premium FormatClass
	}{
		{"software drops 4K SDR", hosts["software"], ChannelFormats{FormatBaseline, Format4KSDR}, ""},
		{"software drops 4K HDR", hosts["software"], ChannelFormats{FormatBaseline, Format4KHDR}, ""},
		{"generic drops premium (unverified CPU filters at 4K)", hosts["generic-qsv"], ChannelFormats{FormatBaseline, Format4KSDR}, ""},
		{"NVENC keeps 4K HDR", hosts["nvenc-libplacebo"], ChannelFormats{FormatBaseline, Format4KHDR}, Format4KHDR},
		{"4K HDR needs libplacebo for its SDR items", nvNoPlacebo, ChannelFormats{FormatBaseline, Format4KHDR}, ""},
		{"4K SDR needs no libplacebo", nvNoPlacebo, ChannelFormats{FormatBaseline, Format4KSDR}, Format4KSDR},
		{"VAAPI keeps 4K SDR", hosts["vaapi-intel"], ChannelFormats{FormatBaseline, Format4KSDR}, Format4KSDR},
		{"baseline-only stays baseline-only", hosts["nvenc-libplacebo"], ChannelFormats{Baseline: FormatBaseline}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, why := tc.formats.OnHost(tc.host)
			if got.Baseline != FormatBaseline {
				t.Errorf("baseline = %q, want %q", got.Baseline, FormatBaseline)
			}
			if got.Premium != tc.premium {
				t.Errorf("premium = %q, want %q", got.Premium, tc.premium)
			}
			if dropped := tc.formats.Premium != "" && got.Premium == ""; dropped != (why != "") {
				t.Errorf("dropped=%v but reason %q: a dropped premium must say why, a kept one must not", dropped, why)
			}
		})
	}
}

// TestBuild_CostClass: the budget charges a converted SDR item on an HDR10 channel as its own class
// (maintainer, #1512), a PQ item as 4k-hevc-hdr, and the baseline as itself.
func TestBuild_CostClass(t *testing.T) {
	host := testHosts()["nvenc-opencl"]
	sdr, pq, hlg := testSources()["h264-1080p-sdr-25"], testSources()["hevc-4k-hdr-dv"], premiumSources()["hevc10-1080p-hlg"]
	hdrOut, _ := PremiumOutput(Format4KHDR, testOutput)
	sdrOut, _ := PremiumOutput(Format4KSDR, testOutput)
	for _, tc := range []struct {
		name string
		src  MediaFormat
		out  OutputProfile
		want FormatClass
	}{
		{"baseline", sdr, testOutput, FormatBaseline},
		{"HDR baseline item still baseline", pq, testOutput, FormatBaseline},
		{"4K SDR premium", sdr, sdrOut, Format4KSDR},
		{"PQ item on 4K HDR", pq, hdrOut, Format4KHDR},
		{"SDR item on 4K HDR", sdr, hdrOut, CostHDRConvert},
		{"HLG item on 4K HDR", hlg, hdrOut, CostHDRConvert},
	} {
		p, err := Build(host, tc.src, tc.out)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if p.CostClass != tc.want {
			t.Errorf("%s: cost class %q, want %q", tc.name, p.CostClass, tc.want)
		}
	}
	if CostHDRConvert != "4k-hevc-hdr-convert" {
		t.Errorf("the budget measures the class by name: %q", CostHDRConvert)
	}
}

// TestPremiumOutput: the premium is HEVC at 3840x2160 on the channel's cadence, Main10 PQ for HDR,
// with the spike's q22 16M/24M rate control and the baseline's closed 1 s GOP and audio.
func TestPremiumOutput(t *testing.T) {
	base := ChannelOutput(Profile{Width: 1920, Height: 1080, Framerate: 25, AudioBitrate: 192, Encoder: EncoderNVENC})
	for _, tc := range []struct {
		class FormatClass
		hdr   bool
	}{{Format4KSDR, false}, {Format4KHDR, true}} {
		got, ok := PremiumOutput(tc.class, base)
		if !ok {
			t.Fatalf("%s: no output profile", tc.class)
		}
		want := OutputProfile{Width: 3840, Height: 2160, FPS: 25, HEVC: true, HDR: tc.hdr,
			Quality: 22, TargetKbps: 16000, MaxKbps: 24000, GOPSeconds: 1, AudioKbps: 192}
		if got != want {
			t.Errorf("%s: got %+v, want %+v", tc.class, got, want)
		}
	}
	if _, ok := PremiumOutput("", base); ok {
		t.Error("no premium class must yield no output profile")
	}
}
