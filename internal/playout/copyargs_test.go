package playout

import (
	"strings"
	"testing"
	"time"
)

// Direct play (§9.1 V47): CopyVideo ⇒ `-c:v copy`, and the transcode apparatus (scale filter,
// video encoder, hardware device/decode) is entirely absent — that absence is the speed win.
func TestProgramArgs_CopyVideoSkipsTranscode(t *testing.T) {
	got := joined(ProgramArgs(ProgramSpec{
		Profile: DefaultProfile(),
		Input:   testStreamURL,
		Limit:   time.Hour,
		Plan:    CopyPlan{CopyVideo: true, CopyAudio: true},
	}))

	if !strings.Contains(got, "-c:v copy") {
		t.Error("CopyVideo must emit -c:v copy")
	}
	// The transcode apparatus must be GONE — its presence would mean a wasted decode/encode.
	for _, banned := range []string{"scale=", "libx264", "-hwaccel", "-init_hw_device", "-b:v"} {
		if strings.Contains(got, banned) {
			t.Errorf("a video COPY must not carry transcode arg %q: %q", banned, got)
		}
	}
	if !strings.Contains(got, "-c:a copy") {
		t.Error("CopyAudio must emit -c:a copy")
	}
	// Still maps exactly one video + one audio, and still bounds the slot / seeks — copy changes
	// the codec flags, not the framing.
	if strings.Count(got, "-map ") != 2 {
		t.Errorf("copy still maps one video + one audio, got %q", got)
	}
}

// The direct-stream case (the real Simpsons file to the browser): copy the video, transcode ONLY
// the audio. The expensive video is untouched; a cheap AAC audio encode is the whole cost.
func TestProgramArgs_CopyVideoTranscodeAudio(t *testing.T) {
	got := joined(ProgramArgs(ProgramSpec{
		Profile: DefaultProfile(),
		Input:   testStreamURL,
		Limit:   time.Hour,
		Plan:    CopyPlan{CopyVideo: true, CopyAudio: false},
	}))

	if !strings.Contains(got, "-c:v copy") {
		t.Error("video must copy")
	}
	// No video transcode…
	if strings.Contains(got, "libx264") || strings.Contains(got, "scale=") {
		t.Errorf("video must not transcode: %q", got)
	}
	// …but the audio DOES encode (aac), so a `-c:v copy` alongside an audio encoder is the shape.
	if !strings.Contains(got, "aac") {
		t.Errorf("audio must transcode to aac: %q", got)
	}
	if strings.Contains(got, "-c:a copy") {
		t.Error("audio must NOT copy — it is incompatible")
	}
}

// The zero CopyPlan transcodes both — the fallback when a source cannot be probed, and what every
// pre-V47 test asserts through the transcodeArgs helper.
func TestProgramArgs_ZeroPlanTranscodesBoth(t *testing.T) {
	got := joined(ProgramArgs(ProgramSpec{Profile: DefaultProfile(), Input: testStreamURL, Limit: time.Hour}))
	if strings.Contains(got, "-c:v copy") || strings.Contains(got, "-c:a copy") {
		t.Errorf("zero plan must transcode both, not copy: %q", got)
	}
	if !strings.Contains(got, "libx264") {
		t.Errorf("zero plan transcodes video: %q", got)
	}
}
