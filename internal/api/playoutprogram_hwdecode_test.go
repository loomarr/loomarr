package api_test

import (
	"context"
	"io"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
)

// #1401: a VAAPI decoder that cannot decode one source ("Failed to sync surface … internal
// decoding error") is a fault of the DECODER for that source, not of the encoder or its capacity.
// Retrying `-hwaccel vaapi` on it fails identically, so recovery must switch that source to
// software decode while the encode stays on hardware.
const vaapiDecodeFault = "[AVHWFramesContext @ 0x7f1dac045400] Failed to sync surface 0xc: 23 (internal decoding error)"

type hwdecodeAttempt struct {
	input    string
	hwDecode bool
	encoder  string
}

// hwdecodeEncoder scripts each spawn: the closure decides the shell script from the attempt, and
// every attempt is recorded so the test can assert the decode/encode axes independently.
type hwdecodeEncoder struct {
	mu       sync.Mutex
	attempts []hwdecodeAttempt
	script   func(a hwdecodeAttempt) string
}

func (e *hwdecodeEncoder) start(ctx context.Context, args []string, _ func(playout.Progress)) (*playout.Process, error) {
	a := hwdecodeAttempt{hwDecode: slices.Contains(args, "-hwaccel")}
	for i, arg := range args {
		if arg == "-i" && i+1 < len(args) {
			a.input = args[i+1]
		}
		if arg == "-c:v" && i+1 < len(args) {
			a.encoder = args[i+1]
		}
	}
	e.mu.Lock()
	e.attempts = append(e.attempts, a)
	e.mu.Unlock()
	return playout.Start(ctx, "sh", []string{"-c", e.script(a)}, nil, nil)
}

func (e *hwdecodeEncoder) recorded() []hwdecodeAttempt {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]hwdecodeAttempt(nil), e.attempts...)
}

// perChannelSource lets one server serve two distinct sources on two channels.
type perChannelSource struct {
	*fakeResolver
	urls map[string]string
}

func (r *perChannelSource) AiringNow(ctx context.Context, channelID string) (playout.Airing, string, error) {
	airing, _, err := r.fakeResolver.AiringNow(ctx, channelID)
	return airing, r.urls[channelID], err
}

func hwdecodeHarness(t *testing.T, enc *hwdecodeEncoder) *playoutProgramHarness {
	t.Helper()
	return newPlayoutProgramHarness(t, playoutProgramHarnessConfig{
		Resolver: &perChannelSource{
			fakeResolver: &fakeResolver{
				airing:  playableAiring(0, time.Hour),
				profile: playout.Profile{Width: 1280, Height: 720, Framerate: 25, Encoder: playout.EncoderVAAPI, VideoBitrate: 4000, AudioBitrate: 128},
			},
			urls: map[string]string{"burns": "http://emby/v/burns", "other": "http://emby/v/other"},
		},
		Encoder: enc.start,
	})
}

func fetchProgram(t *testing.T, h *playoutProgramHarness, channel string) string {
	t.Helper()
	resp := getPlayout(t, h.Server, "/v1/playout/program/"+channel+"?token="+playoutToken)
	body, _ := io.ReadAll(resp.Body) // a mid-stream child failure aborts the body; the bytes before it still count
	return string(body)
}

// A decode crash AFTER output began (the production shape: mid-episode) marks the source; the
// supervisor's re-request for it then decodes in software. Another source is unaffected.
func TestPlayoutProgram_DecodeCrashMidStreamFallsBackToSoftwareDecodeForThatSourceOnly(t *testing.T) {
	enc := &hwdecodeEncoder{script: func(a hwdecodeAttempt) string {
		if a.input == "http://emby/v/burns" && a.hwDecode {
			return "printf part; echo " + shellQuote(vaapiDecodeFault) + " >&2; exit 251"
		}
		return "printf ok"
	}}
	h := hwdecodeHarness(t, enc)

	fetchProgram(t, h, "burns") // crashes mid-stream
	if got := fetchProgram(t, h, "burns"); got != "ok" {
		t.Fatalf("re-request body = %q, want a playing programme", got)
	}
	fetchProgram(t, h, "other")

	got := enc.recorded()
	if len(got) != 3 {
		t.Fatalf("spawns = %+v, want 3 (crash, software-decode retry, other source)", got)
	}
	if !got[0].hwDecode || got[0].encoder != "h264_vaapi" {
		t.Errorf("first attempt = %+v, want hardware decode + hardware encode", got[0])
	}
	if got[1].hwDecode || got[1].encoder != "h264_vaapi" {
		t.Errorf("recovery attempt = %+v, want software decode with the encode kept on hardware", got[1])
	}
	if !got[2].hwDecode {
		t.Errorf("unrelated source = %+v, want hardware decode (the fault is per source)", got[2])
	}
}

// The same fault before any output ("encoder produced NO OUTPUT" in production) must not be retried
// on the identical hardware path, nor treated as VRAM contention.
func TestPlayoutProgram_DecodeFaultWithNoOutputRetriesSoftwareDecodeNotHardware(t *testing.T) {
	enc := &hwdecodeEncoder{script: func(a hwdecodeAttempt) string {
		if a.hwDecode {
			return "echo " + shellQuote(vaapiDecodeFault) + " >&2; exit 251"
		}
		return "printf ok"
	}}
	var reclaimed int
	h := newPlayoutProgramHarness(t, playoutProgramHarnessConfig{
		Resolver: &fakeResolver{
			airing: playableAiring(0, time.Hour), url: "http://emby/v/burns",
			profile: playout.Profile{Width: 1280, Height: 720, Framerate: 25, Encoder: playout.EncoderVAAPI, VideoBitrate: 4000, AudioBitrate: 128},
		},
		Encoder:     enc.start,
		ReclaimVRAM: func(context.Context) { reclaimed++ },
	})

	if got := fetchProgram(t, h, "ch1"); got != "ok" {
		t.Fatalf("body = %q, want a playing programme", got)
	}
	got := enc.recorded()
	if len(got) != 2 || !got[0].hwDecode || got[1].hwDecode || got[1].encoder != "h264_vaapi" {
		t.Errorf("spawns = %+v, want [hw-decode failure, software-decode with hardware encode]", got)
	}
	if reclaimed != 0 {
		t.Errorf("VRAM reclaimed %d times for a decoder fault, want 0", reclaimed)
	}
}
