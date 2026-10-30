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
