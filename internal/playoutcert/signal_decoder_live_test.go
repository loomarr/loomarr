//go:build ffmpeg

package playoutcert

import (
	"context"
	"io"
	"os"
	"os/exec"
	"sync/atomic"
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

func TestFFmpegSignalDecoderObservesHeldOpenMedia(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	fixtures := playoutcertfixture.SignalDecoderMedia(t, ctx, t.TempDir())
	media, err := os.ReadFile(fixtures.AV)
	if err != nil {
		t.Fatal(err)
	}
	input, writer := io.Pipe()
	defer func() { _ = input.Close(); _ = writer.Close() }()
	// Deliver the finite fixture but retain the source writer, as a live input
	// would. Metadata must arrive before EOF releases FFmpeg output buffers.
	written := make(chan error, 1)
	go func() { _, err := writer.Write(media); written <- err }()
	var videos, audios atomic.Int64
	var video []DecodedVideoSignal
	var audio []DecodedAudioSignal
	done := make(chan error, 1)
	go func() {
		done <- (FFmpegSignalDecoder{Path: ffmpeg}).DecodeSignals(ctx, input,
			func(v DecodedVideoSignal) { video = append(video, v); videos.Add(1) },
			func(a DecodedAudioSignal) { audio = append(audio, a); audios.Add(1) })
	}()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	waiting := true
	for waiting {
		select {
		case <-ticker.C:
			waiting = videos.Load() < 5 || audios.Load() < 5
		case <-timer.C:
			waiting = false
		}
	}
	beforeVideo, beforeAudio := videos.Load(), audios.Load()
	t.Logf("before source EOF: video=%d audio=%d", beforeVideo, beforeAudio)
	_ = writer.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("DecodeSignals: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("decoder did not join")
	}
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	assertOrderedSignalWindows(t, video, audio)
	if beforeVideo < 5 || beforeAudio < 5 {
		t.Fatalf("no live observations before source EOF: video=%d audio=%d", beforeVideo, beforeAudio)
	}
}
