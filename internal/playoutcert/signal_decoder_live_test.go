//go:build ffmpeg

package playoutcert

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/testkit/playoutcertfixture"
)

func TestFFmpegSignalDecoderDecodeSignalsRealAV(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	fixtures := playoutcertfixture.SignalDecoderMedia(t, ctx, t.TempDir())
	input, err := os.Open(fixtures.AV)
	if err != nil {
		t.Fatal(err)
	}
	var video []DecodedVideoSignal
	var audio []DecodedAudioSignal
	err = (FFmpegSignalDecoder{Path: ffmpeg}).DecodeSignals(ctx, input,
		func(value DecodedVideoSignal) { video = append(video, value) },
		func(value DecodedAudioSignal) { audio = append(audio, value) })
	if err != nil {
		t.Fatalf("DecodeSignals() error = %v", err)
	}
	assertOrderedSignalWindows(t, video, audio)
}

func TestFFmpegSignalDecoderDecodeSignalsRejectsNoAudio(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	fixtures := playoutcertfixture.SignalDecoderMedia(t, ctx, t.TempDir())
	input, err := os.Open(fixtures.VideoOnly)
	if err != nil {
		t.Fatal(err)
	}
	if err := (FFmpegSignalDecoder{Path: ffmpeg}).DecodeSignals(ctx, input, func(DecodedVideoSignal) {}, func(DecodedAudioSignal) {}); err == nil {
		t.Fatal("video-only media accepted")
	}
}

func assertOrderedSignalWindows(t *testing.T, video []DecodedVideoSignal, audio []DecodedAudioSignal) {
	t.Helper()
	if len(video) < 90 || len(audio) < 150 || !strictlyIncreasingVideo(video) || !strictlyIncreasingAudio(audio) {
		t.Fatalf("signal progress video=%d audio=%d", len(video), len(audio))
	}
	assertVideoWindow(t, video[4:46], 0, 20)
	assertVideoWindow(t, video[54:96], 230, 255)
	assertAudioWindow(t, audio[8:70], 0.012, 0.026)
	assertAudioWindow(t, audio[len(audio)-70:len(audio)-8], 0.027, 0.050)
}

func assertVideoWindow(t *testing.T, values []DecodedVideoSignal, minimum, maximum float64) {
	t.Helper()
	for _, value := range values {
		if value.Luma < minimum || value.Luma > maximum {
			t.Fatalf("video luma %f outside [%f,%f]", value.Luma, minimum, maximum)
		}
	}
}

func assertAudioWindow(t *testing.T, values []DecodedAudioSignal, minimum, maximum float64) {
	t.Helper()
	for _, value := range values {
		if value.Samples <= 0 || value.Silence || value.RMSDB >= 0 || value.RMSDB < -80 || value.ZeroCrossingRate < minimum || value.ZeroCrossingRate > maximum {
			t.Fatalf("audio signal %#v outside expected window", value)
		}
	}
}

func strictlyIncreasingVideo(values []DecodedVideoSignal) bool {
	for index := 1; index < len(values); index++ {
		if values[index].PTSUS <= values[index-1].PTSUS {
			return false
		}
	}
	return true
}

func strictlyIncreasingAudio(values []DecodedAudioSignal) bool {
	for index := 1; index < len(values); index++ {
		if values[index].PTSUS <= values[index-1].PTSUS {
			return false
		}
	}
	return true
}
