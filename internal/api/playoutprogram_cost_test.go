package api_test

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
)

// A finished live programme reports its encoder CPU and the media it produced, so the budget can
// correct the capacity probe's synthetic cost with real content (#1512).
func TestPlayoutProgram_FinishedTranscodeReportsItsCostToTheBudget(t *testing.T) {
	sessions := &fakePlayoutSessions{}
	enc := func(ctx context.Context, _ []string, progress func(playout.Progress)) (*playout.Process, error) {
		progress(playout.Progress{OutTimeMS: 30_000})
		return playout.Start(ctx, "sh", []string{"-c", "printf ok"}, nil, nil)
	}
	resolver := &perChannelSource{
		fakeResolver: &fakeResolver{
			airing:       playableAiring(0, time.Hour),
			profile:      playout.Profile{Width: 1280, Height: 720, Framerate: 25, Encoder: playout.EncoderVAAPI, VideoBitrate: 4000, AudioBitrate: 128},
			sourceFormat: playout.MediaFormat{VideoCodec: "hevc", Width: 1920, Height: 1080, FrameRate: 25, PixelFormat: "yuv420p10le"},
		},
		urls: map[string]string{"burns": "http://emby/v/burns"},
	}
	h := newPlayoutProgramHarness(t, playoutProgramHarnessConfig{Resolver: resolver, Encoder: enc, Observer: sessions})
	if got := fetchProgram(t, h, "burns"); got != "ok" {
		t.Fatalf("programme body = %q", got)
	}
	// The offline card (no source URL) encodes a still: it must not refine any class's cost.
	if got := fetchProgram(t, h, "nosource"); got != "ok" {
		t.Fatalf("card body = %q", got)
	}

	sessions.mu.Lock()
	defer sessions.mu.Unlock()
	if len(sessions.observedCosts) != 1 {
		t.Fatalf("observed costs = %+v, want one report for the finished transcode", sessions.observedCosts)
	}
	got := sessions.observedCosts[0]
	if got.class != playout.ClassHEVC10 || got.media != 30*time.Second {
		t.Fatalf("report = %+v, want hevc10_1080p over 30s of media", got)
	}
}
