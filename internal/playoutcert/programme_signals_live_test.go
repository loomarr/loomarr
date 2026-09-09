//go:build ffmpeg

package playoutcert_test

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/app"
	"github.com/loomarr/loomarr/internal/playoutcert"
)

func TestSyntheticHLSChecksExpectedProgrammeSignals(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 80*time.Second)
	defer cancel()
	channels := []playoutcert.Channel{{ID: "prepared", Roles: []string{"prepared"}}, {ID: "cold", Roles: []string{"transcode_h264", "audio_aac"}}}
	target, err := app.NewPlayoutCertificationTarget(ctx, app.PlayoutCertificationConfig{Channels: channels, FFmpeg: ffmpeg, Capacity: 1, Grace: time.Second, ProgrammeDuration: 6 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		closeCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if err := target.Close(closeCtx); err != nil {
			t.Error(err)
		}
	}()
	for index, lane := range []string{"prepared", "transcode"} {
		t.Run(lane, func(t *testing.T) {
			observeCtx, stop := context.WithTimeout(ctx, 25*time.Second)
			defer stop()
			config := playoutcert.Config{BaseURL: target.BaseURL, AdminBearer: target.AdminBearer, DeviceToken: target.DeviceToken, Channels: channels,
				ProgrammeEvidence: target.ProgrammeEvidence(), SignalDecoder: playoutcert.FFmpegSignalDecoder{Path: ffmpeg},
				RequestTimeout: 10 * time.Second, ProgrammeBoundaryTimeout: 25 * time.Second, ProgrammeBoundaryLateObservation: 3 * time.Second, RawCaptureBytes: 256 << 10}
			result, err := playoutcert.ObserveProgrammeSignalsForTest(observeCtx, config, lane, index)
			if err != nil {
				t.Fatal(err)
			}
			if result.Class != "ok" || result.Evidence.Transitions < 1 || result.Evidence.DecodedFrameDelta <= 0 || result.Evidence.DecodedAudioSamplesDelta <= 0 || result.Evidence.ReadDelta <= 0 || result.Evidence.BytesDelta <= 0 {
				t.Fatalf("decoded programme qualification: %+v", result)
			}
			t.Logf("qualified transitions=%d late video=%d audio samples=%d reads=%d bytes=%d", result.Evidence.Transitions, result.Evidence.DecodedFrameDelta, result.Evidence.DecodedAudioSamplesDelta, result.Evidence.ReadDelta, result.Evidence.BytesDelta)
		})
	}
}
