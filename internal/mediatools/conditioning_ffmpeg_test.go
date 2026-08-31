//go:build ffmpeg

package mediatools_test

import (
	"context"
	"crypto/sha256"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/mediatools"
	"github.com/loomarr/loomarr/internal/testkit"
)

func conditioningRealTools(t *testing.T) *mediatools.FFmpegTools {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal("ffmpeg build-tag test requires ffmpeg")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Fatal("ffmpeg build-tag test requires ffprobe")
	}
	return mediatools.NewFFmpegTools(ffmpeg, ffprobe, "", "", "")
}

func TestMeasureConditioningRealFixtureTimingCadenceSkewAndLoudness(t *testing.T) {
	tools := conditioningRealTools(t)
	fixture := testkit.FillerConditioningMedia(t, t.TempDir()).OffsetLoudness
	got, err := tools.MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: fixture})
	if err != nil {
		t.Fatal(err)
	}
	video := measuredConditioningStream(t, got.Streams, mediatools.StreamVideo)
	audio := measuredConditioningStream(t, got.Streams, mediatools.StreamAudio)
	if !video.Start.Available || video.Start.Milliseconds != 0 {
		t.Errorf("video start = %+v, want measured zero", video.Start)
	}
	if !audio.Start.Available || audio.Start.Milliseconds < 105 || audio.Start.Milliseconds > 135 {
		t.Errorf("audio start = %+v, want independently muxed ~120ms delay", audio.Start)
	}
	if video.Cadence == nil || video.Cadence.Numerator != 30_000 || video.Cadence.Denominator != 1_001 {
		t.Errorf("video cadence = %+v, want exact 30000/1001", video.Cadence)
	}
	if !got.AVSkew.Start.Available || got.AVSkew.Start.Milliseconds != audio.Start.Milliseconds {
		t.Errorf("start skew = %+v, want audio start %dms", got.AVSkew.Start, audio.Start.Milliseconds)
	}
	if !got.Loudness.Available || got.Loudness.TruePeak.State != mediatools.TruePeakFinite {
		t.Fatalf("loudness = %+v, want finite measurements", got.Loudness)
	}
	if math.Abs(got.Loudness.IntegratedLUFS-(-55.8)) > 1.0 {
		t.Errorf("integrated loudness = %.1f LUFS, want about -55.8", got.Loudness.IntegratedLUFS)
	}
	if math.Abs(got.Loudness.TruePeak.DBTP-(-54.2)) > 1.0 {
		t.Errorf("true peak = %.1f dBTP, want about -54.2", got.Loudness.TruePeak.DBTP)
	}
	if len(got.Quality.Silence) == 0 || got.Quality.Silence[0].StartMs != 0 {
		t.Errorf("non-zero-start audio silence is not artifact-relative: %+v", got.Quality.Silence)
	}
}

func TestMeasureConditioningRealSilentFixtureIsPresentAndMeasurable(t *testing.T) {
	tools := conditioningRealTools(t)
	fixture := testkit.FillerConditioningMedia(t, t.TempDir()).Silent
	got, err := tools.MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: fixture})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Loudness.Available || got.Loudness.TruePeak.State != mediatools.TruePeakNegativeInfinity || got.Loudness.TruePeak.DBTP != 0 {
		t.Fatalf("silent true peak = %+v", got.Loudness)
	}
	if len(got.Quality.Silence) == 0 || got.Quality.Silence[0].StartMs != 0 || got.Quality.Silence[0].EndMs < 2_900 {
		t.Fatalf("silent evidence = %+v", got.Quality.Silence)
	}
}

func TestMeasureConditioningRealFixtureReusesNormalizedQualityEvidence(t *testing.T) {
	tools := conditioningRealTools(t)
	fixture := testkit.FillerConditioningMedia(t, t.TempDir()).Compilation
	got, err := tools.MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: fixture})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Quality.Black) < 2 || len(got.Quality.Silence) < 2 || len(got.Quality.Freeze) == 0 {
		t.Fatalf("quality evidence = %+v", got.Quality)
	}
	for _, spans := range [][]mediatools.Interval{got.Quality.Black, got.Quality.Silence, got.Quality.Freeze} {
		for _, span := range spans {
			if span.StartMs < 0 || span.EndMs > got.ContainerDurationMs || span.EndMs <= span.StartMs {
				t.Errorf("quality interval is not normalized: %+v of %dms", span, got.ContainerDurationMs)
			}
		}
	}
}

func TestMeasureConditioningRealFixtureClosesFreezeAcrossFinalColors(t *testing.T) {
	tools := conditioningRealTools(t)
	fixtures := testkit.FillerConditioningMedia(t, t.TempDir())
	for name, fixture := range map[string]string{
		"white": fixtures.WhiteFrozen,
		"black": fixtures.Silent,
		"other": fixtures.Compilation,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := tools.MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: fixture})
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Quality.Freeze) == 0 {
				t.Fatalf("final %s content emitted no completed freeze interval", name)
			}
			last := got.Quality.Freeze[len(got.Quality.Freeze)-1]
			if last.EndMs != got.ContainerDurationMs {
				t.Fatalf("final %s freeze end = %dms, want artifact boundary %dms", name, last.EndMs, got.ContainerDurationMs)
			}
		})
	}
}

func TestMeasureConditioningRealFixtureUsesSelectedStreamEOFs(t *testing.T) {
	tools := conditioningRealTools(t)
	fixtures := testkit.FillerConditioningMedia(t, t.TempDir())

	t.Run("shorter video", func(t *testing.T) {
		const decodedVideoEOFMs int64 = 3_003 // 90 frames at exact 30000/1001 cadence.
		got, err := tools.MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: fixtures.ShortVideo})
		if err != nil {
			t.Fatal(err)
		}
		video := measuredConditioningStream(t, got.Streams, mediatools.StreamVideo)
		audio := measuredConditioningStream(t, got.Streams, mediatools.StreamAudio)
		if !video.Duration.Available || !audio.Duration.Available || video.Duration.Milliseconds >= audio.Duration.Milliseconds {
			t.Fatalf("stream durations video=%+v audio=%+v, want shorter video", video.Duration, audio.Duration)
		}
		if len(got.Quality.Black) != 0 {
			t.Fatalf("terminator-only black leaked past video EOF: %+v", got.Quality.Black)
		}
		if len(got.Quality.Freeze) != 1 || got.Quality.Freeze[0].EndMs != decodedVideoEOFMs {
			t.Fatalf("boundary freeze = %+v, want independently known decoded video EOF %dms", got.Quality.Freeze, decodedVideoEOFMs)
		}
	})

	t.Run("shorter audio", func(t *testing.T) {
		const decodedAudioEOFMs int64 = 3_000 // Exactly 144000 decoded samples at 48kHz.
		got, err := tools.MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: fixtures.ShortAudio})
		if err != nil {
			t.Fatal(err)
		}
		video := measuredConditioningStream(t, got.Streams, mediatools.StreamVideo)
		audio := measuredConditioningStream(t, got.Streams, mediatools.StreamAudio)
		if !video.Duration.Available || !audio.Duration.Available || audio.Duration.Milliseconds >= video.Duration.Milliseconds {
			t.Fatalf("stream durations video=%+v audio=%+v, want shorter audio", video.Duration, audio.Duration)
		}
		if len(got.Quality.Silence) != 1 || got.Quality.Silence[0] != (mediatools.Interval{StartMs: 0, EndMs: decodedAudioEOFMs}) {
			t.Fatalf("silence = %+v, want independently known decoded audio EOF %dms", got.Quality.Silence, decodedAudioEOFMs)
		}
	})
}

func TestMeasureConditioningRealFixtureMatchesCutEdgesWithoutEchoingRequest(t *testing.T) {
	tools := conditioningRealTools(t)
	fixtures := testkit.FillerConditioningMedia(t, t.TempDir())
	child := filepath.Join(t.TempDir(), "segment.mp4")
	const intendedStart, intendedEnd = int64(12_517), int64(24_530)
	if err := tools.Cut(context.Background(), fixtures.Compilation, intendedStart, intendedEnd, child); err != nil {
		t.Fatal(err)
	}
	got, err := tools.MeasureConditioning(context.Background(), mediatools.ConditioningRequest{
		Path: child, ParentPath: fixtures.Compilation,
		IntendedCuts: []mediatools.Interval{{StartMs: intendedStart, EndMs: intendedEnd}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Cuts) != 1 || len(got.Cuts[0].Streams) != 2 {
		t.Fatalf("cut measurement = %+v", got.Cuts)
	}
	for _, stream := range got.Cuts[0].Streams {
		if stream.StartError.Available {
			t.Errorf("%s:%d ambiguous start was reported exact: %+v", stream.Kind, stream.Index, stream.StartError)
		}
		if !stream.EndError.Available || stream.EndError.Milliseconds <= 0 || stream.EndError.Milliseconds > 80 {
			t.Errorf("%s:%d end = %+v, want measured one-packet overshoot", stream.Kind, stream.Index, stream.EndError)
		}
	}
}

func TestMeasureConditioningRealFixtureDoesNotMutateArtifact(t *testing.T) {
	tools := conditioningRealTools(t)
	fixture := testkit.FillerConditioningMedia(t, t.TempDir()).OffsetLoudness
	before, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tools.MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: fixture}); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(after) != sha256.Sum256(before) {
		t.Error("measurement changed the operator artifact")
	}
}

func measuredConditioningStream(t *testing.T, streams []mediatools.ConditioningStream, kind mediatools.StreamKind) mediatools.ConditioningStream {
	t.Helper()
	for _, stream := range streams {
		if stream.Kind == kind {
			return stream
		}
	}
	t.Fatalf("no %s stream in %+v", kind, streams)
	return mediatools.ConditioningStream{}
}
