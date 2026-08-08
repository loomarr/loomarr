package playout

import "testing"

func TestPlanCopy(t *testing.T) {
	cases := []struct {
		name      string
		f         MediaFormat
		target    Target
		wantVideo bool // CopyVideo
		wantAudio bool // CopyAudio
	}{
		{"h264+aac to browser: full direct play", MediaFormat{VideoCodec: "h264", AudioCodec: "aac"}, TargetBrowser, true, true},
		// The real Simpsons case: h264 video + EAC3 audio to the browser. COPY the video (the win),
		// transcode only the audio. This is the whole point of the per-stream model.
		{"h264+eac3 to browser: copy video, transcode audio", MediaFormat{VideoCodec: "h264", AudioCodec: "eac3"}, TargetBrowser, true, false},
		// A media server tolerates EAC3 → both copy.
		{"h264+eac3 to media server: full copy", MediaFormat{VideoCodec: "h264", AudioCodec: "eac3"}, TargetMediaServer, true, true},
		// HEVC video the browser can't play → video transcodes (audio also transcodes, aac is fine here so copies).
		{"hevc+aac to browser: transcode video, copy audio", MediaFormat{VideoCodec: "hevc", AudioCodec: "aac"}, TargetBrowser, false, true},
		{"hevc to media server: full copy", MediaFormat{VideoCodec: "hevc", AudioCodec: "aac"}, TargetMediaServer, true, true},
		{"h264 no audio to browser: copy video, audio n/a copies", MediaFormat{VideoCodec: "h264"}, TargetBrowser, true, true},
		{"mpeg2+aac to browser: transcode video, copy audio", MediaFormat{VideoCodec: "mpeg2video", AudioCodec: "aac"}, TargetBrowser, false, true},
		{"case-insensitive", MediaFormat{VideoCodec: "H264", AudioCodec: "AAC"}, TargetBrowser, true, true},
	}
	for _, c := range cases {
		p := PlanCopy(c.f, c.target)
		if p.CopyVideo != c.wantVideo || p.CopyAudio != c.wantAudio {
			t.Errorf("%s: PlanCopy = {video:%v audio:%v}, want {video:%v audio:%v}",
				c.name, p.CopyVideo, p.CopyAudio, c.wantVideo, c.wantAudio)
		}
	}
}

// Target ↔ wire token round-trips, and an unknown/empty token degrades to the media server — the
// broader, historical default, so an old client or a hand-typed URL behaves as it always did. This
// contract is load-bearing: the playlist→program hop carries the target only as this string (§9.1).
func TestTarget_ParseStringRoundTrip(t *testing.T) {
	for _, target := range []Target{TargetBrowser, TargetMediaServer} {
		if got := ParseTarget(target.String()); got != target {
			t.Errorf("round-trip %v: String()=%q → ParseTarget=%v", target, target.String(), got)
		}
	}
	if ParseTarget("browser") != TargetBrowser {
		t.Error(`ParseTarget("browser") must be TargetBrowser`)
	}
	// Anything not exactly "browser" is the media server — including "", "mediaserver", and garbage.
	for _, s := range []string{"", "mediaserver", "MediaServer", "tv", "hevc"} {
		if ParseTarget(s) != TargetMediaServer {
			t.Errorf("ParseTarget(%q) must default to TargetMediaServer", s)
		}
	}
}

// DirectPlay (both copy) is the fast path indicator.
func TestCopyPlan_DirectPlay(t *testing.T) {
	if !(CopyPlan{CopyVideo: true, CopyAudio: true}).DirectPlay() {
		t.Fatal("both-copy must report DirectPlay")
	}
	if (CopyPlan{CopyVideo: true, CopyAudio: false}).DirectPlay() {
		t.Fatal("copy-video-transcode-audio is NOT full direct play")
	}
}
