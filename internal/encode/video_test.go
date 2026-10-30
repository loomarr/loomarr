package encode

import (
	"strconv"
	"testing"
)

func argsAfter(args []string, flag string) (string, bool) {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

// Every family's file mode is constant quality with the frame-0 keyframe expression and no cap; a
// family without one (the generic encoders) encodes in software.
func TestFileVideoArgs_ConstantQualityPerFamily(t *testing.T) {
	for _, c := range []struct {
		family Family
		codec  string
		qFlag  string
	}{
		{FamilyNVENC, "h264_nvenc", "-cq"},
		{FamilyVAAPI, "h264_vaapi", "-qp"},
		{FamilyVideoToolbox, "h264_videotoolbox", "-q:v"},
		{FamilySoftware, "libx264", "-crf"},
		{FamilyGeneric, "libx264", "-crf"},
	} {
		args := FileVideoArgs(c.family, File{Quality: 21, KeyframeSeconds: 1})
		if got, _ := argsAfter(args, "-c:v"); got != c.codec {
			t.Errorf("%s: encoder %q, want %q", c.family, got, c.codec)
		}
		if got, _ := argsAfter(args, c.qFlag); got != "21" {
			t.Errorf("%s: %s %q, want the level 21: %q", c.family, c.qFlag, got, args)
		}
		if got, _ := argsAfter(args, "-force_key_frames"); got != "expr:gte(t,n_forced*1)" {
			t.Errorf("%s: keyframes %q: %q", c.family, got, args)
		}
		if _, capped := argsAfter(args, "-maxrate"); capped {
			t.Errorf("%s: a file encode has no bitrate cap: %q", c.family, args)
		}
	}
}

// A derivative runs on the host's family only once that family has a measured level; anything else
// (an unmeasured family, a generic encoder, no GPU) encodes in software rather than guessing.
func TestFileFamily_OnlyMeasuredFamiliesLeaveSoftware(t *testing.T) {
	for enc, want := range map[Encoder]Family{
		EncoderNVENC:    FamilyNVENC,
		EncoderSoftware: FamilySoftware,
		EncoderQSV:      FamilySoftware,
		EncoderAMF:      FamilySoftware,
	} {
		if got := FileFamily(HostFor(enc, false, GPUFilters{})); got != want {
			t.Errorf("%s: file family %s, want %s", enc, got, want)
		}
	}
	for _, f := range []Family{FamilyVAAPI, FamilyVideoToolbox} {
		host := HostProfile{Family: f}
		_, measured := FileLevel(f)
		if got := FileFamily(host); measured != (got == f) {
			t.Errorf("%s: file family %s with measured=%v", f, got, measured)
		}
	}
}

// Every engine, in both codecs, pins its GOP to the requested one so each packager fragment is one
// closed GOP (the frame-0 IDR and the cadence are measured in playout's
// TestLive_FragmentStartsOnAnIDR…), takes CRF only in software (hardware rate control has no CRF,
// and v4l2m2m rejects the option at init), and encodes HEVC on its own h264 engine's sibling (#1561
// ported these rules from the retired chain's builder). Derived from H264Engines so a new engine is
// covered with no edit here.
func TestVideoArgs_EveryEnginePinsTheGOPAndKeepsCRFInSoftware(t *testing.T) {
	v := Video{Quality: 23, TargetKbps: 3000, MaxKbps: 4500, GOPFrames: 50}
	for _, base := range H264Engines {
		host := HostFor(base, false, GPUFilters{})
		h264 := VideoArgs(host, v)
		hevcV := v
		hevcV.HEVC = true
		hevc := VideoArgs(host, hevcV)
		codec, _ := argsAfter(h264, "-c:v")
		if got, _ := argsAfter(hevc, "-c:v"); got != string(HEVCVariant(Encoder(codec))) {
			t.Errorf("%s: HEVC encoder %q, want %s's sibling %s", base, got, codec, HEVCVariant(Encoder(codec)))
		}
		for name, args := range map[string][]string{"h264": h264, "hevc": hevc} {
			if g, _ := argsAfter(args, "-g"); g != strconv.Itoa(v.GOPFrames) {
				t.Errorf("%s %s: -g %q, want the GOP %d: %q", base, name, g, v.GOPFrames, args)
			}
			if _, crf := argsAfter(args, "-crf"); crf != (host.Family == FamilySoftware) {
				t.Errorf("%s %s (%s): -crf present=%v, want it only in software: %q", base, name, host.Family, crf, args)
			}
		}
	}
}
