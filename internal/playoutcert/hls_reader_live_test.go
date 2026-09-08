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

func TestSyntheticLiveHLSReaderContinuesAcrossProgrammeChanges(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	channels := []playoutcert.Channel{
		{ID: "cold", Roles: []string{"transcode_h264", "audio_aac"}},
		{ID: "prepared-control", Roles: []string{"prepared"}},
	}
	target, err := app.NewPlayoutCertificationTarget(ctx, app.PlayoutCertificationConfig{
		Channels: channels, FFmpeg: ffmpeg, Capacity: 1, Grace: time.Second, ProgrammeDuration: 6 * time.Second,
	})
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
	config := playoutcert.Config{BaseURL: target.BaseURL, AdminBearer: target.AdminBearer, DeviceToken: target.DeviceToken, RequestTimeout: 20 * time.Second}
	stream, err := playoutcert.ColdHLSStreamForTest(ctx, config, channels[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	type signal struct {
		video *playoutcert.DecodedVideoSignal
		audio *playoutcert.DecodedAudioSignal
	}
	events := make(chan signal, 64)
	decodeCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	var decoderErr error
	send := func(event signal) {
		select {
		case events <- event:
		case <-decodeCtx.Done():
		}
	}
	go func() {
		decoderErr = (playoutcert.FFmpegSignalDecoder{Path: ffmpeg}).DecodeSignals(decodeCtx, stream,
			func(v playoutcert.DecodedVideoSignal) { send(signal{video: &v}) },
			func(a playoutcert.DecodedAudioSignal) { send(signal{audio: &a}) })
		close(done)
	}()
	defer func() {
		stop()
		if err := stream.Close(); err != nil {
			t.Error(err)
		}
		<-done
	}()
	var transitions, lateVideo, lateAudio int
	lastPhase := -1
	var firstTransition time.Time
	for {
		select {
		case <-done:
			t.Fatalf("HLS decoder ended before continued media: %v", decoderErr)
		case <-ctx.Done():
			t.Fatalf("HLS reader stopped progressing: transitions=%d late=%d/%d", transitions, lateVideo, lateAudio)
		case event := <-events:
			if event.video != nil {
				phase := -1
				if event.video.Luma < 25 {
					phase = 0
				}
				if event.video.Luma > 225 {
					phase = 1
				}
				if phase >= 0 {
					if lastPhase >= 0 && lastPhase != phase {
						transitions++
						if firstTransition.IsZero() {
							firstTransition = time.Now()
						}
					}
					lastPhase = phase
				}
				if !firstTransition.IsZero() && time.Since(firstTransition) > 6*time.Second {
					lateVideo++
				}
			}
			if event.audio != nil && !firstTransition.IsZero() && time.Since(firstTransition) > 6*time.Second {
				lateAudio++
			}
			if transitions >= 3 && lateVideo >= 10 && lateAudio >= 10 {
				t.Logf("ordinary HLS reader: transitions=%d late video/audio=%d/%d", transitions, lateVideo, lateAudio)
				return
			}
		}
	}
}
