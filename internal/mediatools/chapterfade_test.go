package mediatools

import "testing"

func TestParseChapterFade(t *testing.T) {
	stdout := "frame:0    pts:0       pts_time:0\nlavfi.signalstats.YAVG=16\n" +
		"frame:1    pts:1001    pts_time:0.0417\nlavfi.signalstats.YAVG=16.2\n" +
		"frame:23   pts:23023   pts_time:0.959\nlavfi.signalstats.YAVG=17.4269\n"
	stderr := "[Parsed_volumedetect_0 @ 0x5] n_samples: 96000\n[Parsed_volumedetect_0 @ 0x5] mean_volume: -61.3 dB\n[Parsed_volumedetect_0 @ 0x5] max_volume: -40.1 dB\n"
	first, last, mean, ok := ParseChapterFade(stdout, stderr)
	if !ok || first != 16 || last != 17.4269 || mean != -61.3 {
		t.Fatalf("got first=%v last=%v mean=%v ok=%v", first, last, mean, ok)
	}
	if _, _, _, ok := ParseChapterFade(stdout, ""); ok {
		t.Fatal("no audio level must not parse as a checked fade")
	}
	if _, _, _, ok := ParseChapterFade("", stderr); ok {
		t.Fatal("no frames must not parse as a checked fade")
	}
}
