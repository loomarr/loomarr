package playout

import "testing"

// PlanCopy over every EncodePlan bucket (§9.1 V48). The per-stream decision is the whole point: an
// h264+EAC3 file copies the video and transcodes only the audio for a baseline client.
func TestPlanCopy(t *testing.T) {
	cases := []struct {
		name      string
		f         MediaFormat
		plan      EncodePlan
		wantVideo bool // CopyVideo
		wantAudio bool // CopyAudio
	}{
		{"h264+aac to baseline: full direct play", MediaFormat{VideoCodec: "h264", AudioCodec: "aac"}, PlanBaseline, true, true},
		// The real Simpsons case: h264 video + EAC3 audio to a baseline client. COPY the video (the
		// win), transcode only the audio. This is the whole point of the per-stream model.
		{"h264+eac3 to baseline: copy video, transcode audio", MediaFormat{VideoCodec: "h264", AudioCodec: "eac3"}, PlanBaseline, true, false},
		// A full (tuner) plan tolerates EAC3 → both copy.
		{"h264+eac3 to full: full copy", MediaFormat{VideoCodec: "h264", AudioCodec: "eac3"}, PlanFull, true, true},
		// HEVC video a baseline client can't play → video transcodes (aac audio copies).
		{"hevc+aac to baseline: transcode video, copy audio", MediaFormat{VideoCodec: "hevc", AudioCodec: "aac"}, PlanBaseline, false, true},
		// HEVC to an HEVC-capable client → video COPIES. The core V48 win.
		{"hevc+aac to hevc8: copy video, copy audio", MediaFormat{VideoCodec: "hevc", AudioCodec: "aac"}, PlanHEVC8, true, true},
		// HEVC + surround to hevc8: video copies (hevc ok), audio transcodes (hevc8 is aac-only).
		{"hevc+eac3 to hevc8: copy video, transcode audio", MediaFormat{VideoCodec: "hevc", AudioCodec: "eac3"}, PlanHEVC8, true, false},
		// HEVC + surround to hevc10: BOTH copy — the capable-native-app case.
		{"hevc+eac3 to hevc10: full copy", MediaFormat{VideoCodec: "hevc", AudioCodec: "eac3"}, PlanHEVC10, true, true},
		{"hevc to full: full copy", MediaFormat{VideoCodec: "hevc", AudioCodec: "aac"}, PlanFull, true, true},
		{"h264 no audio to baseline: copy video, audio n/a copies", MediaFormat{VideoCodec: "h264"}, PlanBaseline, true, true},
		{"mpeg2+aac to baseline: transcode video, copy audio", MediaFormat{VideoCodec: "mpeg2video", AudioCodec: "aac"}, PlanBaseline, false, true},
		{"case-insensitive", MediaFormat{VideoCodec: "H264", AudioCodec: "AAC"}, PlanBaseline, true, true},

		// ⚠ 10-bit gating — the axis a codec-name check alone misses. hevc8 proved it can decode 8-bit
		// HEVC ONLY, so a 10-bit HEVC source to hevc8 must TRANSCODE the video (copying it is the same
		// black frame as copying HEVC to baseline). hevc10/full copy it.
		{"hevc 10-bit to hevc8: transcode video (10-bit not allowed)", MediaFormat{VideoCodec: "hevc", AudioCodec: "aac", PixelFormat: "yuv420p10le"}, PlanHEVC8, false, true},
		{"hevc 10-bit to hevc10: copy video (10-bit allowed)", MediaFormat{VideoCodec: "hevc", AudioCodec: "aac", PixelFormat: "yuv420p10le"}, PlanHEVC10, true, true},
		{"hevc 10-bit to full: copy video", MediaFormat{VideoCodec: "hevc", AudioCodec: "aac", PixelFormat: "yuv420p10le"}, PlanFull, true, true},
	}
	for _, c := range cases {
		p := PlanCopy(c.f, c.plan)
		if p.CopyVideo != c.wantVideo || p.CopyAudio != c.wantAudio {
			t.Errorf("%s: PlanCopy = {video:%v audio:%v}, want {video:%v audio:%v}",
				c.name, p.CopyVideo, p.CopyAudio, c.wantVideo, c.wantAudio)
		}
	}
}

// resolve buckets a DeviceProfile into the richest plan it FULLY satisfies, rounding DOWN, and NEVER
// grants a capability the profile did not advertise (§9.1 V48 — the black-frame guard).
func TestResolvePlan(t *testing.T) {
	cases := []struct {
		name string
		p    DeviceProfile
		want EncodePlan
	}{
		{"empty profile → baseline (the safe default)", DeviceProfile{}, PlanBaseline},
		{"h264 only → baseline", DeviceProfile{Video: []string{"h264"}, Audio: []string{"aac"}}, PlanBaseline},
		{"hevc, 8-bit, no surround → hevc8", DeviceProfile{Video: []string{"h264", "hevc"}, Audio: []string{"aac"}}, PlanHEVC8},
		{"hevc via h265 alias → hevc8", DeviceProfile{Video: []string{"h265"}}, PlanHEVC8},
		// Round DOWN: HEVC + surround but NOT 10-bit → hevc8, not hevc10 (10-bit not proven).
		{"hevc + surround but 8-bit → hevc8 (rounds down)", DeviceProfile{Video: []string{"hevc"}, Audio: []string{"eac3"}}, PlanHEVC8},
		// Round DOWN: HEVC + 10-bit but no surround → hevc8 (surround not proven).
		{"hevc + 10-bit but no surround → hevc8 (rounds down)", DeviceProfile{Video: []string{"hevc"}, Video10Bit: true}, PlanHEVC8},
		// All three → hevc10, the richest a client profile earns.
		{"hevc + 10-bit + surround → hevc10", DeviceProfile{Video: []string{"hevc"}, Audio: []string{"eac3"}, Video10Bit: true}, PlanHEVC10},
		// Surround WITHOUT hevc is meaningless for the bucket (video gates first) → baseline.
		{"surround audio but no hevc → baseline", DeviceProfile{Audio: []string{"eac3", "ac3"}}, PlanBaseline},
		// A client profile NEVER resolves to full (that is the tuner-only set).
		{"even everything does not reach full", DeviceProfile{Video: []string{"hevc", "vp9", "av1"}, Audio: []string{"eac3", "ac3", "mp3"}, Video10Bit: true}, PlanHEVC10},
		{"case-insensitive codecs", DeviceProfile{Video: []string{"HEVC"}, Audio: []string{"EAC3"}, Video10Bit: true}, PlanHEVC10},
	}
	for _, c := range cases {
		if got := ResolvePlan(c.p); got != c.want {
			t.Errorf("%s: ResolvePlan = %v, want %v", c.name, got, c.want)
		}
	}
}

// EncodePlan ↔ wire token round-trips, and an unknown/empty token degrades to PlanBaseline — the SAFE
// default (h264/aac), the inverse of the retired ParseTarget whose unknown default was the BROAD set.
// This contract is load-bearing and is one of the two black-frame guards (the other is resolve).
func TestEncodePlan_ParseStringRoundTrip(t *testing.T) {
	for _, plan := range []EncodePlan{PlanBaseline, PlanHEVC8, PlanHEVC10, PlanFull} {
		if got := ParseEncodePlan(plan.String()); got != plan {
			t.Errorf("round-trip %v: String()=%q → ParseEncodePlan=%v", plan, plan.String(), got)
		}
	}
	// ⚠ Anything not a recognized token is PlanBaseline — including "", "browser"/"mediaserver" (the
	// retired tokens), and garbage. This is the client-edge safety default: unknown ⇒ h264/aac.
	for _, s := range []string{"", "browser", "mediaserver", "BASELINE", "tv", "hevc"} {
		if got := ParseEncodePlan(s); got != PlanBaseline {
			t.Errorf("ParseEncodePlan(%q) = %v, must default to PlanBaseline", s, got)
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
