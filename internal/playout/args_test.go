package playout

import (
	"os/exec"
	"strings"
	"testing"
)

// Arg-shape tests are cheap and run everywhere. The ones that actually EXECUTE ffmpeg
// live in ffmpeg_live_test.go behind a build tag, because unit tests must not depend on
// a binary being present (CLAUDE.md: unit tests never touch the network, and the same
// spirit applies to external executables).

func argsAfter(args []string, flag string) (string, bool) {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

func joined(args []string) string { return strings.Join(args, " ") }

// The three details Tunarr's card taught us, asserted so a future "simplification"
// cannot quietly remove them (prior-art §5a).
func TestTestCardArgs_CarriesTheThreeLoadBearingFlags(t *testing.T) {
	got := joined(TestCardArgs(DefaultProfile(), "/f.ttf", "CH 1", ""))

	// A video-only MPEG-TS is a classic cause of a player refusing to play or showing
	// no timeline. The silent track is not optional.
	if !strings.Contains(got, "anullsrc") {
		t.Error("no anullsrc — a video-only MPEG-TS will not play reliably")
	}
	// Without -re, lavfi generates as fast as the CPU allows and floods the pipe.
	if !strings.Contains(got, "-re ") {
		t.Error("no -re — the synthetic source would race ahead of wall-clock")
	}
	// A generated source that EOFs ends the channel.
	if !strings.Contains(got, "-stream_loop -1") {
		t.Error("no -stream_loop -1 — the card would end")
	}
}

// Progress must be machine-readable on its own fd, not scraped from stderr: a chunked
// read can split a token across the buffer boundary (viewra §4).
func TestTestCardArgs_ProgressIsStructured(t *testing.T) {
	args := TestCardArgs(DefaultProfile(), "", "", "")
	if v, ok := argsAfter(args, "-progress"); !ok || !strings.HasPrefix(v, "pipe:") {
		t.Errorf("-progress = %q, want a pipe fd (never stderr scraping)", v)
	}
	if !strings.Contains(joined(args), "-nostats") {
		t.Error("want -nostats so the only progress output is the structured stream")
	}
}

// Every segment boundary must land on a keyframe, and segment durations must not vary —
// a TARGETDURATION that lies is a player error, not a warning.
func TestGopArgs_PinKeyframesAndDisableSceneDetection(t *testing.T) {
	got := joined(DefaultProfile().videoEncodeArgs())
	for _, want := range []string{"-g 25", "-keyint_min 25", "-sc_threshold 0"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

// Audio is fixed AAC stereo 48k. A varying audio layout across programs breaks `-c copy`
// on the parent exactly like video does.
func TestAudioEncodeArgs_FixedStereo48k(t *testing.T) {
	got := joined(DefaultProfile().audioEncodeArgs())
	for _, want := range []string{"-c:a aac", "-ac 2", "-ar 48000"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

// Each encoder family has its OWN option vocabulary, and an unknown option fails at INIT
// rather than being ignored — several (notably v4l2m2m on a Pi) are strict.
//
// The invariant is about `-tune` specifically, verified against real ffmpeg: only libx264
// and nvenc define it. `-preset` is NOT a safe thing to assert on, because QSV genuinely
// has its own `veryfast` (an int enum, `-h encoder=h264_qsv`) that merely shares libx264's
// spelling — an earlier version of this test asserted "no hardware encoder says veryfast"
// and was simply wrong about ffmpeg.
func TestVideoEncodeArgs_NoTuneOnEncodersThatLackIt(t *testing.T) {
	tuneless := []Encoder{
		EncoderVAAPI, EncoderQSV, EncoderAMF, EncoderVideoToolbox,
		EncoderRKMPP, EncoderV4L2M2M, EncoderVulkan,
	}
	for _, enc := range tuneless {
		got := joined(Profile{Encoder: enc, Framerate: 25}.videoEncodeArgs())
		if strings.Contains(got, "-tune") {
			t.Errorf("%s does not accept -tune; it would fail at init: %q", enc, got)
		}
	}
	// The two that DO define it should use it, with their own values.
	sw := joined(Profile{Encoder: EncoderSoftware, Framerate: 25}.videoEncodeArgs())
	if !strings.Contains(sw, "-preset veryfast") || !strings.Contains(sw, "-tune zerolatency") {
		t.Errorf("libx264 should be veryfast+zerolatency for live, got %q", sw)
	}
	nv := joined(Profile{Encoder: EncoderNVENC, Framerate: 25}.videoEncodeArgs())
	if !strings.Contains(nv, "-preset p4") || !strings.Contains(nv, "-tune hq") {
		t.Errorf("nvenc should use its own p4/hq vocabulary, got %q", nv)
	}
	// AMF speaks -quality, not -preset.
	amf := joined(Profile{Encoder: EncoderAMF, Framerate: 25}.videoEncodeArgs())
	if !strings.Contains(amf, "-quality") || strings.Contains(amf, "-preset") {
		t.Errorf("AMF uses -quality rather than -preset, got %q", amf)
	}
}

// Every family must at least be constructible — a missing case in the switch would emit a
// bare codec with no rate control, which encodes at whatever default the driver picks.
func TestVideoEncodeArgs_EveryFamilyGetsRateControl(t *testing.T) {
	for _, enc := range encoderPreference {
		got := joined(Profile{Encoder: enc, Framerate: 25, VideoBitrate: 4000}.videoEncodeArgs())
		if !strings.Contains(got, "-c:v "+string(enc)) {
			t.Errorf("%s: codec not set: %q", enc, got)
		}
		if !strings.Contains(got, "-b:v 4000k") || !strings.Contains(got, "-maxrate") {
			t.Errorf("%s: no bitrate cap — a live encoder can spike and blow a client buffer: %q", enc, got)
		}
		if !strings.Contains(got, "-g 25") {
			t.Errorf("%s: no pinned GOP — segment boundaries need keyframes: %q", enc, got)
		}
	}
}

// A channel name comes from the database and is operator-supplied. An unescaped
// apostrophe breaks the filter graph; an unescaped colon silently introduces ANOTHER
// filter option, which is worse than broken.
func TestDrawTextFilter_EscapesOperatorText(t *testing.T) {
	got := drawTextFilter("/f.ttf", "Bob's 90s: Movies", "", 720)
	if strings.Contains(got, "text='Bob's") {
		t.Errorf("apostrophe not escaped — broken filter graph: %q", got)
	}
	if !strings.Contains(got, `Bob\'s`) {
		t.Errorf("want an escaped apostrophe, got %q", got)
	}
	if !strings.Contains(got, `90s\: Movies`) {
		t.Errorf("want an escaped colon (it introduces a filter option), got %q", got)
	}
}

// No font ⇒ no drawtext. drawtext without a fontfile fails at INIT on a minimal image,
// so a missing font must degrade to a plain colour field rather than kill the channel.
func TestDrawTextFilter_MissingFontDegradesInsteadOfFailing(t *testing.T) {
	if got := drawTextFilter("", "CH 1", "", 720); got != "" {
		t.Errorf("no font must yield no filter, got %q", got)
	}
	args := TestCardArgs(DefaultProfile(), "", "CH 1", "")
	if strings.Contains(joined(args), "drawtext") {
		t.Error("args carry drawtext with no font — ffmpeg would fail at init")
	}
	// …but the card itself must still be produced.
	if !strings.Contains(joined(args), "color=c=black") {
		t.Error("the colour field must survive a missing font")
	}
}

// FindFont must not invent a path: a non-existent fontfile is the init failure above.
func TestFindFont_ReturnsOnlyAnExistingFile(t *testing.T) {
	got := FindFont()
	if got == "" {
		t.Skip("no font on this system — the degrade path is covered above")
	}
	if _, err := exec.LookPath("test"); err == nil { // trivially available
		if !fileExists(got) {
			t.Errorf("FindFont returned %q which does not exist", got)
		}
	}
}
