package scan

import "testing"

func TestResolutionLabelReadsWidthToo(t *testing.T) {
	for _, tc := range []struct {
		w, h int
		want string
	}{
		{1920, 1080, "1080p"},
		{1920, 1088, "1080p"},
		{1920, 800, "1080p"}, // scope: the width says what the height hides
		{3840, 1600, "4K"},
		{3840, 2160, "4K"},
		{1280, 718, "720p"},
		{1440, 1080, "1080p"},
		{720, 480, "480p"},
		{0, 0, ""},
	} {
		if got := resolutionLabel(tc.w, tc.h); got != tc.want {
			t.Errorf("%dx%d = %q, want %q", tc.w, tc.h, got, tc.want)
		}
	}
}

func TestHDRLabel(t *testing.T) {
	for _, tc := range []struct {
		transfer string
		hdr      bool
		want     string
	}{
		{"smpte2084", true, "HDR10"},
		{"arib-std-b67", true, "HLG"},
		{"", true, "HDR"},
		{"bt709", false, ""},
	} {
		if got := hdrLabel(tc.transfer, tc.hdr); got != tc.want {
			t.Errorf("%q/%v = %q, want %q", tc.transfer, tc.hdr, got, tc.want)
		}
	}
}

func TestAudioFormatLabel(t *testing.T) {
	for _, tc := range []struct{ codec, profile, want string }{
		{"eac3", "", "Dolby Digital Plus"},
		{"ac3", "", "Dolby Digital"},
		{"truehd", "", "Dolby TrueHD"},
		{"dts", "DTS-HD MA", "DTS-HD MA"},
		{"dts", "", "DTS"},
		{"pcm_s24le", "", "PCM"},
		{"wmapro", "", "WMAPRO"},
		{"", "", ""},
	} {
		if got := audioFormatLabel(tc.codec, tc.profile); got != tc.want {
			t.Errorf("%q/%q = %q, want %q", tc.codec, tc.profile, got, tc.want)
		}
	}
}

func TestLanguageLabel(t *testing.T) {
	for _, tc := range []struct{ in, code, name string }{
		{"eng", "en", "English"},
		{"en", "en", "English"},
		{"fre", "fr", "French"},
		{"ger", "de", "German"},
		{"und", "", ""},
		{"", "", ""},
		{"zzz", "zzz", ""},
	} {
		code, name := languageLabel(tc.in)
		if code != tc.code || name != tc.name {
			t.Errorf("%q = %q, %q; want %q, %q", tc.in, code, name, tc.code, tc.name)
		}
	}
}
