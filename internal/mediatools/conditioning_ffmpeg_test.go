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

	"github.com/loomarr/loomarr/internal/filler"
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
	// Evidence is on the container timeline (#1719): the delayed audio's silence starts where the
	// audio does, not at its own first frame.
	if len(got.Quality.Silence) == 0 || !got.ContainerStart.Available ||
		got.Quality.Silence[0].StartMs != audio.Start.Milliseconds-got.ContainerStart.Milliseconds {
		t.Errorf("delayed audio silence = %+v, want it to start at the audio's %dms on the container timeline", got.Quality.Silence, audio.Start.Milliseconds)
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

// A production mezzanine starts at the AAC priming offset, not zero, and its audio's first decoded
// frame carries the priming. Detector intervals go on the file's container timeline, like every
// other conditioning fact (#1540), so a silence that runs to the end stays inside the container
// and stays aligned with the picture (#1719).
func TestMeasureConditioningRealPrimedMezzanineKeepsTrailingSilenceOnTheContainerTimeline(t *testing.T) {
	tools := conditioningRealTools(t)
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "source.mp4")
	mezzanine := filepath.Join(dir, "mezzanine.mp4")
	// 8 s of tone, then 4 s of digital silence to the end of the file.
	if raw, err := exec.Command(ffmpeg, "-nostdin", "-v", "error",
		"-f", "lavfi", "-i", "testsrc=size=320x240:rate=30",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-filter_complex", "[1:a]volume=enable='gte(t,8)':volume=0[a]",
		"-map", "0:v", "-map", "[a]", "-t", "12",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-y", source).CombinedOutput(); err != nil {
		t.Fatalf("build source fixture: %v: %s", err, raw)
	}
	probe := filler.FFprobeNextTo(ffmpeg)
	input, err := probe(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	sourceQuality, err := mediatools.Transcode(context.Background(), mediatools.TranscodeRequest{
		In: source, Out: mezzanine, DurationMs: input.DurationMs, HadAudio: true, InputProbe: &input,
		TargetLUFS: -23, Profile: mediatools.DefaultMezzanine(), Probe: probe,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(sourceQuality.Silence) != 1 {
		t.Fatalf("transcode detector silence = %+v, want the one trailing span", sourceQuality.Silence)
	}

	got, err := tools.MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: mezzanine})
	if err != nil {
		t.Fatal(err)
	}
	if !got.ContainerStart.Available || got.ContainerStart.Milliseconds <= 0 {
		t.Fatalf("container start = %+v, want the measured priming offset of a real mezzanine", got.ContainerStart)
	}
	if err := mediatools.ValidateMediaQualityEvidence(got.Quality); err != nil {
		t.Fatalf("primed mezzanine quality %+v of %dms: %v", got.Quality, got.ContainerDurationMs, err)
	}
	if len(got.Quality.Silence) != 1 || got.Quality.Silence[0].EndMs != got.ContainerDurationMs {
		t.Fatalf("silence = %+v, want one span ending at the container end %dms", got.Quality.Silence, got.ContainerDurationMs)
	}
	// The picture starts where the source's did, shifted onto the container timeline; the silence
	// must move with it rather than with the audio's primed first frame.
	video := measuredConditioningStream(t, got.Streams, mediatools.StreamVideo)
	want := sourceQuality.Silence[0].StartMs + video.Start.Milliseconds - got.ContainerStart.Milliseconds
	if drift := got.Quality.Silence[0].StartMs - want; drift < -2 || drift > 2 {
		t.Fatalf("silence start = %dms, want %dms (source %dms on the container timeline)", got.Quality.Silence[0].StartMs, want, sourceQuality.Silence[0].StartMs)
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
