//go:build !windows

package mediatools_test

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/mediatools"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestMeasureConditioningKeepsMissingTimingAndCadenceUnavailable(t *testing.T) {
	artifact := conditioningArtifact(t, "fixture.mp4")
	probe := conditioningExactProbe(t, `{"streams":[{"index":0,"codec_type":"video","duration":"10","avg_frame_rate":"0/0"},{"index":1,"codec_type":"audio","duration":"10"},{"index":2,"codec_type":"video","avg_frame_rate":"0/0"},{"index":3,"codec_type":"audio"}],"format":{"duration":"10.000"}}`)
	tools := mediatools.NewFFmpegTools(conditioningLoudnessExecutable(t, "-3.0"), probe, "", "", "")

	got, err := tools.MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: artifact})
	if err != nil {
		t.Fatal(err)
	}
	for _, stream := range got.Streams {
		if stream.Start.Available {
			t.Errorf("missing %s start became measured zero: %+v", stream.Kind, stream.Start)
		}
		if stream.Index >= 2 && stream.Duration.Available {
			t.Errorf("missing %s duration became measured zero: %+v", stream.Kind, stream.Duration)
		}
		if stream.Kind == mediatools.StreamVideo && stream.Cadence != nil {
			t.Errorf("N/A cadence = %+v, want unavailable", stream.Cadence)
		}
	}
	if got.AVSkew.Start.Available || got.AVSkew.End.Available {
		t.Errorf("skew derived from unavailable timing: %+v", got.AVSkew)
	}
}

func TestMeasureConditioningRejectsNonLocalRegularInputsBeforeTools(t *testing.T) {
	sentinel := filepath.Join(t.TempDir(), "invoked")
	tool := testkit.POSIXExecutable(t, "conditioning-tool", `touch "`+sentinel+`"; exit 99`)
	valid := conditioningArtifact(t, "valid.mp4")
	dir := t.TempDir()
	missing := filepath.Join(t.TempDir(), "missing.mp4")
	symlink := filepath.Join(t.TempDir(), "link.mp4")
	if err := os.Symlink(valid, symlink); err != nil {
		t.Fatal(err)
	}
	ancestorTarget := t.TempDir()
	if err := os.WriteFile(filepath.Join(ancestorTarget, "nested.mp4"), []byte("local fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	ancestorLink := filepath.Join(t.TempDir(), "linked-directory")
	if err := os.Symlink(ancestorTarget, ancestorLink); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{
		"empty":            "",
		"stdin":            "-",
		"URL":              "https://example.invalid/media.mp4",
		"pipe protocol":    "pipe:0",
		"directory":        dir,
		"missing":          missing,
		"symlink":          symlink,
		"ancestor symlink": filepath.Join(ancestorLink, "nested.mp4"),
	} {
		t.Run(name, func(t *testing.T) {
			_ = os.Remove(sentinel)
			_, err := mediatools.NewFFmpegTools(tool, tool, "", "", "").MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: path})
			if !errors.Is(err, mediatools.ErrConditioningOutput) {
				t.Fatalf("error = %v, want invalid output", err)
			}
			if _, statErr := os.Stat(sentinel); !os.IsNotExist(statErr) {
				t.Fatalf("media tool ran before rejecting %q", path)
			}
		})
	}

	t.Run("invalid parent", func(t *testing.T) {
		_ = os.Remove(sentinel)
		_, err := mediatools.NewFFmpegTools(tool, tool, "", "", "").MeasureConditioning(context.Background(), mediatools.ConditioningRequest{
			Path: valid, ParentPath: missing, IntendedCuts: []mediatools.Interval{{StartMs: 1, EndMs: 2}},
		})
		if !errors.Is(err, mediatools.ErrConditioningOutput) {
			t.Fatalf("error = %v, want invalid output", err)
		}
		if _, statErr := os.Stat(sentinel); !os.IsNotExist(statErr) {
			t.Fatal("artifact probe ran before invalid parent was rejected")
		}
	})
}

func TestMeasureConditioningFailsClosedAtResourceCeilings(t *testing.T) {
	t.Run("cuts", func(t *testing.T) {
		cuts := make([]mediatools.Interval, mediatools.ConditioningMaxCuts+1)
		_, err := (&mediatools.FFmpegTools{}).MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: "unused", ParentPath: "unused", IntendedCuts: cuts})
		if !errors.Is(err, mediatools.ErrConditioningResourceLimit) {
			t.Fatalf("nine cuts error = %v, want resource limit", err)
		}
	})

	artifact := conditioningArtifact(t, "fixture.mp4")
	t.Run("snapshot bytes", func(t *testing.T) {
		oversized := filepath.Join(t.TempDir(), "oversized.mp4")
		file, err := os.OpenFile(oversized, os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		if err := file.Truncate(mediatools.ConditioningMaxSnapshotBytes + 1); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		sentinel := filepath.Join(t.TempDir(), "invoked")
		tool := testkit.POSIXExecutable(t, "conditioning-tool", `touch "`+sentinel+`"; exit 99`)
		_, err = mediatools.NewFFmpegTools(tool, tool, "", "", "").MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: oversized})
		if !errors.Is(err, mediatools.ErrConditioningResourceLimit) {
			t.Fatalf("oversized sparse input error = %v, want resource limit", err)
		}
		if _, statErr := os.Stat(sentinel); !os.IsNotExist(statErr) {
			t.Fatal("media tool ran for oversized sparse input")
		}
	})
	t.Run("streams", func(t *testing.T) {
		var streams []string
		for i := range mediatools.ConditioningMaxStreams + 1 {
			streams = append(streams, `{"index":`+string(rune('0'+i))+`,"codec_type":"audio"}`)
		}
		probe := testkit.POSIXExecutable(t, "conditioning-tool", `printf '%s\n' '{"streams":[`+strings.Join(streams, ",")+`],"format":{"duration":"10.000"}}'`)
		_, err := mediatools.NewFFmpegTools(testkit.POSIXExecutable(t, "conditioning-tool", "exit 0"), probe, "", "", "").MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: artifact})
		if !errors.Is(err, mediatools.ErrConditioningResourceLimit) {
			t.Fatalf("nine streams error = %v, want resource limit", err)
		}
	})

	t.Run("duration", func(t *testing.T) {
		probe := testkit.POSIXExecutable(t, "conditioning-tool", `printf '%s\n' '{"streams":[],"format":{"duration":"120.001"}}'`)
		_, err := mediatools.NewFFmpegTools(testkit.POSIXExecutable(t, "conditioning-tool", "exit 0"), probe, "", "", "").MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: artifact})
		if !errors.Is(err, mediatools.ErrConditioningResourceLimit) {
			t.Fatalf("long media error = %v, want resource limit", err)
		}
	})
}

func TestMeasureConditioningPreflightsExactDurationBeforeFrameEnumeration(t *testing.T) {
	artifact := conditioningArtifact(t, "fixture.mp4")
	for name, duration := range map[string]string{
		"exact 120 seconds":        "120",
		"fraction above 120":       "120.000000001",
		"rounds below millisecond": "120.0004",
	} {
		t.Run(name, func(t *testing.T) {
			framesReached := filepath.Join(t.TempDir(), "frames-reached")
			probe := testkit.POSIXExecutable(t, "conditioning-tool", `case " $* " in
  *" -show_frames "*)
    touch "`+framesReached+`"
    printf '%s\n' '{"frames":[{"stream_index":0,"best_effort_timestamp_time":"0","duration_time":"120"}]}' ;;
  *) printf '%s\n' '{"streams":[{"index":0,"codec_type":"video","start_time":"0","duration":"`+duration+`","avg_frame_rate":"25/1"}],"format":{"duration":"`+duration+`"}}' ;;
esac`)
			got, err := mediatools.NewFFmpegTools(testkit.POSIXExecutable(t, "conditioning-tool", "exit 0"), probe, "", "", "").MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: artifact})
			if duration == "120" {
				if err != nil || got.ContainerDurationMs != 120_000 {
					t.Fatalf("exact limit measurement = %+v, %v", got, err)
				}
				if _, statErr := os.Stat(framesReached); statErr != nil {
					t.Fatalf("bounded frame enumeration did not run: %v", statErr)
				}
				return
			}
			if !errors.Is(err, mediatools.ErrConditioningResourceLimit) {
				t.Fatalf("duration %s error = %v, want resource limit", duration, err)
			}
			if _, statErr := os.Stat(framesReached); !os.IsNotExist(statErr) {
				t.Fatal("frame enumeration ran before rejecting exact over-limit duration")
			}
		})
	}
}

func TestMeasureConditioningBoundsSparseFrameEnumerationIndependentlyOfOutput(t *testing.T) {
	artifact := conditioningArtifact(t, "fixture.mp4")
	unbounded := filepath.Join(t.TempDir(), "unbounded")
	probe := testkit.POSIXExecutable(t, "conditioning-tool", `case " $* " in
  *" -show_frames "*)
    case " $* " in *" -read_intervals %+120.000001 "*) ;; *) touch "`+unbounded+`"; exit 91 ;; esac
    printf '%s\n' '{"frames":[{"stream_index":0,"best_effort_timestamp_time":"119","duration_time":"1"}]}' ;;
  *) printf '%s\n' '{"streams":[{"index":0,"codec_type":"video","start_time":"0","duration":"120","avg_frame_rate":"25/1"}],"format":{"duration":"120"}}' ;;
esac`)
	_, err := mediatools.NewFFmpegTools(testkit.POSIXExecutable(t, "conditioning-tool", "exit 0"), probe, "", "", "").MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: artifact})
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(unbounded); !os.IsNotExist(statErr) {
		t.Fatal("frame enumeration was not constrained by an internal read interval")
	}
}

func TestMeasureConditioningRejectsExactDecodedEOFPastLimit(t *testing.T) {
	artifact := conditioningArtifact(t, "fixture.mp4")
	detectorRan := filepath.Join(t.TempDir(), "detector-ran")
	probe := testkit.POSIXExecutable(t, "conditioning-tool", `case " $* " in
  *" -show_frames "*) printf '%s\n' '{"frames":[{"stream_index":0,"best_effort_timestamp_time":"0","duration_time":"120.000000001"}]}' ;;
  *) printf '%s\n' '{"streams":[{"index":0,"codec_type":"video","start_time":"0","duration":"120","avg_frame_rate":"25/1"}],"format":{"duration":"120"}}' ;;
esac`)
	ffmpeg := testkit.POSIXExecutable(t, "conditioning-tool", `touch "`+detectorRan+`"`)
	_, err := mediatools.NewFFmpegTools(ffmpeg, probe, "", "", "").MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: artifact})
	if !errors.Is(err, mediatools.ErrConditioningResourceLimit) {
		t.Fatalf("decoded EOF above 120s error = %v, want resource limit", err)
	}
	if _, statErr := os.Stat(detectorRan); !os.IsNotExist(statErr) {
		t.Fatal("detector ran after decoded EOF exceeded its exact limit")
	}
}

func TestMeasureConditioningPreservesCancellationDuringBoundedFrameProbe(t *testing.T) {
	artifact := conditioningArtifact(t, "fixture.mp4")
	framesReached := filepath.Join(t.TempDir(), "frames-reached")
	probe := testkit.POSIXExecutable(t, "conditioning-tool", `case " $* " in
  *" -show_frames "*) touch "`+framesReached+`"; while :; do :; done ;;
  *) printf '%s\n' '{"streams":[{"index":0,"codec_type":"video","start_time":"0","duration":"120","avg_frame_rate":"25/1"}],"format":{"duration":"120"}}' ;;
esac`)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := mediatools.NewFFmpegTools(probe, probe, "", "", "").MeasureConditioning(ctx, mediatools.ConditioningRequest{Path: artifact})
		done <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(framesReached); err == nil {
			cancel()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("bounded frame probe was not reached")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("frame probe cancellation error = %v, want caller cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("frame probe did not stop after cancellation")
	}
}

func TestMeasureConditioningCancelsAndCleansUpDuringSnapshotCopy(t *testing.T) {
	tempRoot := t.TempDir()
	t.Setenv("TMPDIR", tempRoot)
	artifact := filepath.Join(t.TempDir(), "slow-sparse.mp4")
	file, err := os.OpenFile(artifact, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	const sparseBytes = int64(64 << 20)
	if err := file.Truncate(sparseBytes); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	tool := testkit.POSIXExecutable(t, "conditioning-tool", `while :; do :; done`)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, runErr := mediatools.NewFFmpegTools(tool, tool, "", "", "").MeasureConditioning(ctx, mediatools.ConditioningRequest{Path: artifact})
		done <- runErr
	}()

	deadline := time.Now().Add(5 * time.Second)
	observedPartial := false
	for time.Now().Before(deadline) {
		entries, readErr := os.ReadDir(tempRoot)
		if readErr != nil {
			t.Fatal(readErr)
		}
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), "loomarr-conditioning-") {
				continue
			}
			files, readErr := os.ReadDir(filepath.Join(tempRoot, entry.Name()))
			if readErr != nil {
				continue
			}
			for _, snapshot := range files {
				info, statErr := snapshot.Info()
				if statErr == nil && info.Size() > 0 && info.Size() < sparseBytes {
					observedPartial = true
					cancel()
				}
			}
		}
		if observedPartial {
			break
		}
	}
	if !observedPartial {
		cancel()
		t.Fatal("did not observe a cancellable in-progress snapshot copy")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("copy cancellation error = %v, want context cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("snapshot copy did not stop after cancellation")
	}
	entries, err := os.ReadDir(tempRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("private snapshot cleanup left %v", entries)
	}
}

func TestMeasureConditioningFailsClosedOnMalformedAndOversizedToolOutput(t *testing.T) {
	artifact := conditioningArtifact(t, "fixture.mp4")
	t.Run("malformed probe JSON", func(t *testing.T) {
		probe := testkit.POSIXExecutable(t, "conditioning-tool", `printf 'not-json\n'`)
		_, err := mediatools.NewFFmpegTools(testkit.POSIXExecutable(t, "conditioning-tool", "exit 0"), probe, "", "", "").MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: artifact})
		if !errors.Is(err, mediatools.ErrConditioningOutput) {
			t.Fatalf("error = %v, want invalid output", err)
		}
	})
	t.Run("endless probe stdout", func(t *testing.T) {
		probe := testkit.POSIXExecutable(t, "conditioning-tool", `while :; do printf '0123456789abcdef'; done`)
		_, err := mediatools.NewFFmpegTools(testkit.POSIXExecutable(t, "conditioning-tool", "exit 0"), probe, "", "", "").MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: artifact})
		if !errors.Is(err, mediatools.ErrConditioningResourceLimit) {
			t.Fatalf("error = %v, want resource limit", err)
		}
	})
	t.Run("endless detector stderr", func(t *testing.T) {
		probe := conditioningExactProbe(t, `{"streams":[{"index":0,"codec_type":"video","start_time":"0","duration":"2","avg_frame_rate":"25/1"}],"format":{"duration":"2"}}`)
		ffmpeg := testkit.POSIXExecutable(t, "conditioning-tool", `while :; do printf '0123456789abcdef' >&2; done`)
		_, err := mediatools.NewFFmpegTools(ffmpeg, probe, "", "", "").MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: artifact})
		if !errors.Is(err, mediatools.ErrConditioningResourceLimit) {
			t.Fatalf("error = %v, want resource limit", err)
		}
	})
}

func TestMeasureConditioningPreservesContextCancellation(t *testing.T) {
	artifact := conditioningArtifact(t, "fixture.mp4")
	tool := testkit.POSIXExecutable(t, "conditioning-tool", `while :; do :; done`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := mediatools.NewFFmpegTools(tool, tool, "", "", "").MeasureConditioning(ctx, mediatools.ConditioningRequest{Path: artifact})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context cancellation", err)
	}
}

func TestMeasureConditioningDistinguishesAbsentFromMalformedProbeValues(t *testing.T) {
	artifact := conditioningArtifact(t, "fixture.mp4")
	for name, stream := range map[string]string{
		"missing index":    `{"codec_type":"video","start_time":"0","duration":"10","avg_frame_rate":"25/1"}`,
		"invalid start":    `{"index":0,"codec_type":"video","start_time":"oops","duration":"10","avg_frame_rate":"25/1"}`,
		"overflow start":   `{"index":0,"codec_type":"video","start_time":"1e40","duration":"10","avg_frame_rate":"25/1"}`,
		"invalid duration": `{"index":0,"codec_type":"video","start_time":"0","duration":"NaN","avg_frame_rate":"25/1"}`,
		"invalid cadence":  `{"index":0,"codec_type":"video","start_time":"0","duration":"10","avg_frame_rate":"30000/nope"}`,
		"cadence overflow": `{"index":0,"codec_type":"video","start_time":"0","duration":"10","avg_frame_rate":"999999999999999999999/1"}`,
	} {
		t.Run(name, func(t *testing.T) {
			probe := testkit.POSIXExecutable(t, "conditioning-tool", `printf '%s\n' '{"streams":[`+stream+`],"format":{"duration":"10"}}'`)
			_, err := mediatools.NewFFmpegTools(testkit.POSIXExecutable(t, "conditioning-tool", "exit 0"), probe, "", "", "").MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: artifact})
			if !errors.Is(err, mediatools.ErrConditioningOutput) {
				t.Fatalf("error = %v, want invalid output", err)
			}
		})
	}
}

func TestMeasureConditioningRepresentsSilentTruePeakWithoutNonFiniteFloat(t *testing.T) {
	artifact := conditioningArtifact(t, "silent.mp4")
	probe := conditioningExactProbe(t, `{"streams":[{"index":0,"codec_type":"audio","start_time":"0","duration":"2"}],"format":{"duration":"2"}}`)
	got, err := mediatools.NewFFmpegTools(conditioningLoudnessExecutable(t, "-inf"), probe, "", "", "").MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: artifact})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Loudness.Available || got.Loudness.TruePeak.State != mediatools.TruePeakNegativeInfinity || got.Loudness.TruePeak.DBTP != 0 {
		t.Fatalf("silent loudness = %+v", got.Loudness)
	}
	if _, err := json.Marshal(got.Loudness); err != nil {
		t.Fatalf("silent loudness is not finite-payload safe: %v", err)
	}
	if len(got.Quality.Silence) != 1 || got.Quality.Silence[0] != (mediatools.Interval{StartMs: 0, EndMs: 2_000}) {
		t.Fatalf("silent evidence = %+v, want full artifact", got.Quality.Silence)
	}
}

func TestMeasureConditioningRejectsMalformedDetectorScalar(t *testing.T) {
	artifact := conditioningArtifact(t, "fixture.mp4")
	probe := conditioningExactProbe(t, `{"streams":[{"index":0,"codec_type":"video","start_time":"0","duration":"2","avg_frame_rate":"25/1"}],"format":{"duration":"2"}}`)
	ffmpeg := testkit.POSIXExecutable(t, "conditioning-tool", `printf '%s\n' '[Parsed_blackdetect_2 @ 0xabc] black_start:1e40 black_end:2 black_duration:1' >&2`)
	_, err := mediatools.NewFFmpegTools(ffmpeg, probe, "", "", "").MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: artifact})
	if !errors.Is(err, mediatools.ErrConditioningOutput) {
		t.Fatalf("malformed detector scalar error = %v, want invalid output", err)
	}
}

func TestMeasureConditioningAcceptsCapturedFFmpegDetectorWireFormat(t *testing.T) {
	artifact := conditioningArtifact(t, "fixture.mp4")
	probe := conditioningExactProbe(t, `{"streams":[{"index":0,"codec_type":"video","start_time":"0","duration":"2","avg_frame_rate":"25/1"},{"index":1,"codec_type":"audio","start_time":"0","duration":"2"}],"format":{"duration":"2"}}`)
	ffmpeg := testkit.POSIXExecutable(t, "conditioning-tool", `cat >&2 <<'EOF'
[Parsed_blackdetect_2 @ 0x7f6e380050c0] black_start:0.1 black_end:0.4 black_duration:0.3
[Parsed_silencedetect_4 @ 0x7f6e38005600] silence_start: 0.5
[Parsed_silencedetect_4 @ 0x7f6e38005600] silence_end: 0.9 | silence_duration: 0.4
[Parsed_freezedetect_3 @ 0x7f6e38005340] lavfi.freezedetect.freeze_start: 1
[Parsed_freezedetect_3 @ 0x7f6e38005340] lavfi.freezedetect.freeze_duration: 0.5
[Parsed_freezedetect_3 @ 0x7f6e38005340] lavfi.freezedetect.freeze_end: 1.5
 I: -24.0 LUFS
 Peak: -3.0 dBFS
EOF`)
	got, err := mediatools.NewFFmpegTools(ffmpeg, probe, "", "", "").MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: artifact})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Quality.Black) != 1 || got.Quality.Black[0] != (mediatools.Interval{StartMs: 100, EndMs: 400}) ||
		len(got.Quality.Silence) != 1 || got.Quality.Silence[0] != (mediatools.Interval{StartMs: 500, EndMs: 900}) ||
		len(got.Quality.Freeze) != 1 || got.Quality.Freeze[0] != (mediatools.Interval{StartMs: 1_000, EndMs: 1_500}) {
		t.Fatalf("captured detector evidence = %+v", got.Quality)
	}
}

func TestMeasureConditioningAcceptsCapturedLegacyFFmpegDetectorWireFormat(t *testing.T) {
	artifact := conditioningArtifact(t, "fixture.mp4")
	probe := conditioningExactProbe(t, `{"streams":[{"index":0,"codec_type":"video","start_time":"0","duration":"30","avg_frame_rate":"25/1"},{"index":1,"codec_type":"audio","start_time":"0","duration":"30"}],"format":{"duration":"30"}}`)
	ffmpeg := testkit.POSIXExecutable(t, "conditioning-tool", `cat >&2 <<'EOF'
[blackdetect @ 0x7f9] black_start:1.28 black_end:2.52 black_duration:1.24
[silencedetect @ 0xabc] silence_start: 3.36
[silencedetect @ 0xabc] silence_end: 5.84 | silence_duration: 2.48
[freezedetect @ 0xdef] freeze_start: 10.5
[freezedetect @ 0xdef] freeze_duration: 12.25
[freezedetect @ 0xdef] freeze_end: 22.75
 I: -24.0 LUFS
 Peak: -3.0 dBFS
EOF`)
	got, err := mediatools.NewFFmpegTools(ffmpeg, probe, "", "", "").MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: artifact})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Quality.Black) != 1 || len(got.Quality.Silence) != 1 || len(got.Quality.Freeze) != 1 {
		t.Fatalf("legacy captured detector evidence = %+v", got.Quality)
	}
}

func TestMeasureConditioningAllowsOneMillisecondDetectorDurationTolerance(t *testing.T) {
	artifact := conditioningArtifact(t, "fixture.mp4")
	probe := conditioningExactProbe(t, `{"streams":[{"index":0,"codec_type":"video","start_time":"0","duration":"2","avg_frame_rate":"25/1"}],"format":{"duration":"2"}}`)
	ffmpeg := testkit.POSIXExecutable(t, "conditioning-tool", `printf '%s\n' '[Parsed_blackdetect_2 @ 0xabc] black_start:0.1 black_end:0.4 black_duration:0.3005' >&2`)
	got, err := mediatools.NewFFmpegTools(ffmpeg, probe, "", "", "").MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: artifact})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Quality.Black) != 1 || got.Quality.Black[0] != (mediatools.Interval{StartMs: 100, EndMs: 400}) {
		t.Fatalf("tolerated detector interval = %+v", got.Quality.Black)
	}
}

func TestMeasureConditioningUsesExplicitSelectedStreamEOFs(t *testing.T) {
	artifact := conditioningArtifact(t, "fixture.mp4")
	probe := testkit.POSIXExecutable(t, "conditioning-tool", `case " $* " in
  *" -show_frames "*) printf '%s\n' '{"frames":[{"stream_index":3,"best_effort_timestamp_time":"4","nb_samples":10004},{"stream_index":2,"best_effort_timestamp_time":"3","duration_time":"1.0004"},{"stream_index":3,"best_effort_timestamp_time":"0","nb_samples":10000},{"stream_index":2,"best_effort_timestamp_time":"0","duration_time":"1"}]}' ;;
  *) printf '%s\n' '{"streams":[{"index":5,"codec_type":"video","start_time":"0","duration":"3","avg_frame_rate":"25/1"},{"index":2,"codec_type":"video","start_time":"0","duration":"3.5","avg_frame_rate":"25/1"},{"index":7,"codec_type":"audio","start_time":"0","duration":"6","sample_rate":"10000"},{"index":3,"codec_type":"audio","start_time":"0","duration":"4.5","sample_rate":"10000"}],"format":{"duration":"6"}}' ;;
esac`)
	ffmpeg := testkit.POSIXExecutable(t, "conditioning-tool", `
case " $* " in *" -map 0:2 "*) ;; *) echo 'missing selected video map' >&2; exit 81 ;; esac
case " $* " in *" -map 0:3 "*) ;; *) echo 'missing selected audio map' >&2; exit 82 ;; esac
case " $* " in *" -map 0:5 "*|*" -map 0:7 "*) echo 'mapped unselected stream' >&2; exit 83 ;; esac
cat >&2 <<'EOF'
[Parsed_blackdetect_2 @ 0xabc] black_start:4.0004 black_end:4.0404 black_duration:0.04
[Parsed_silencedetect_4 @ 0xdef] silence_start: 0
[Parsed_silencedetect_4 @ 0xdef] silence_end: 5.0004 | silence_duration: 5.0004
[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_start: 1
[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_duration: 3.0404
[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_end: 4.0404
 I: -24.0 LUFS
 Peak: -3.0 dBFS
EOF`)
	got, err := mediatools.NewFFmpegTools(ffmpeg, probe, "", "", "").MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: artifact})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Quality.Black) != 0 {
		t.Fatalf("terminator-only black evidence = %+v, want absent at selected video EOF", got.Quality.Black)
	}
	if len(got.Quality.Freeze) != 1 || got.Quality.Freeze[0] != (mediatools.Interval{StartMs: 1_000, EndMs: 4_000}) {
		t.Fatalf("boundary freeze = %+v, want clamp to selected video EOF", got.Quality.Freeze)
	}
	if len(got.Quality.Silence) != 1 || got.Quality.Silence[0] != (mediatools.Interval{StartMs: 0, EndMs: 5_000}) {
		t.Fatalf("silence = %+v, want selected audio EOF", got.Quality.Silence)
	}
}

func TestMeasureConditioningRejectsSilencePastSelectedAudioEOF(t *testing.T) {
	artifact := conditioningArtifact(t, "fixture.mp4")
	probe := conditioningExactProbe(t, `{"streams":[{"index":0,"codec_type":"video","start_time":"0","duration":"6","avg_frame_rate":"25/1"},{"index":1,"codec_type":"audio","start_time":"0","duration":"5"}],"format":{"duration":"6"}}`)
	ffmpeg := testkit.POSIXExecutable(t, "conditioning-tool", `cat >&2 <<'EOF'
[Parsed_silencedetect_4 @ 0xdef] silence_start: 0
[Parsed_silencedetect_4 @ 0xdef] silence_end: 5.1 | silence_duration: 5.1
 I: -70.0 LUFS
 Peak: -inf dBFS
EOF`)
	_, err := mediatools.NewFFmpegTools(ffmpeg, probe, "", "", "").MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: artifact})
	if !errors.Is(err, mediatools.ErrConditioningOutput) {
		t.Fatalf("silence past selected audio EOF error = %v, want invalid output", err)
	}
}

func TestMeasureConditioningRejectsMissingSelectedStreamEOF(t *testing.T) {
	artifact := conditioningArtifact(t, "fixture.mp4")
	sentinel := filepath.Join(t.TempDir(), "detector-ran")
	probe := testkit.POSIXExecutable(t, "conditioning-tool", `printf '%s\n' '{"streams":[{"index":0,"codec_type":"video","start_time":"0","duration":"2","avg_frame_rate":"25/1"}],"format":{"duration":"2"}}'`)
	ffmpeg := testkit.POSIXExecutable(t, "conditioning-tool", `touch "`+sentinel+`"`)
	_, err := mediatools.NewFFmpegTools(ffmpeg, probe, "", "", "").MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: artifact})
	if !errors.Is(err, mediatools.ErrConditioningOutput) {
		t.Fatalf("missing selected decoder EOF error = %v, want invalid output", err)
	}
	if _, statErr := os.Stat(sentinel); !os.IsNotExist(statErr) {
		t.Fatal("detector ran without exact selected-stream EOF")
	}
}

func TestMeasureConditioningUsesExactDetectorDurationTolerance(t *testing.T) {
	artifact := conditioningArtifact(t, "fixture.mp4")
	probe := conditioningExactProbe(t, `{"streams":[{"index":0,"codec_type":"video","start_time":"0","duration":"2","avg_frame_rate":"25/1"}],"format":{"duration":"2"}}`)
	for name, detectorDuration := range map[string]string{
		"exactly one millisecond":              "0.299",
		"one point zero zero one milliseconds": "0.298999",
	} {
		t.Run(name, func(t *testing.T) {
			ffmpeg := testkit.POSIXExecutable(t, "conditioning-tool", `printf '%s\n' '[Parsed_blackdetect_2 @ 0xabc] black_start:0.1 black_end:0.4 black_duration:`+detectorDuration+`' >&2`)
			got, err := mediatools.NewFFmpegTools(ffmpeg, probe, "", "", "").MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: artifact})
			if detectorDuration == "0.299" {
				if err != nil {
					t.Fatalf("exact 1.000ms disagreement: %v", err)
				}
				if len(got.Quality.Black) != 1 || got.Quality.Black[0] != (mediatools.Interval{StartMs: 100, EndMs: 400}) {
					t.Fatalf("exactly tolerated interval = %+v", got.Quality.Black)
				}
				return
			}
			if !errors.Is(err, mediatools.ErrConditioningOutput) {
				t.Fatalf("1.001ms disagreement error = %v, want invalid output", err)
			}
		})
	}
}

func TestMeasureConditioningDoesNotExposeTerminatorOnlyIntervals(t *testing.T) {
	artifact := conditioningArtifact(t, "fixture.mp4")
	probe := conditioningExactProbe(t, `{"streams":[{"index":0,"codec_type":"video","start_time":"0","duration":"2","avg_frame_rate":"25/1"}],"format":{"duration":"2"}}`)
	ffmpeg := testkit.POSIXExecutable(t, "conditioning-tool", `cat >&2 <<'EOF'
[Parsed_blackdetect_2 @ 0xabc] black_start:2 black_end:2.04 black_duration:0.04
[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_start: 2
[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_duration: 0.04
[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_end: 2.04
EOF`)
	got, err := mediatools.NewFFmpegTools(ffmpeg, probe, "", "", "").MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: artifact})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Quality.Black) != 0 || len(got.Quality.Freeze) != 0 {
		t.Fatalf("terminator-only evidence leaked: %+v", got.Quality)
	}
}

func TestMeasureConditioningRejectsDetectorWireMutations(t *testing.T) {
	artifact := conditioningArtifact(t, "fixture.mp4")
	probe := conditioningExactProbe(t, `{"streams":[{"index":0,"codec_type":"video","start_time":"0","duration":"2","avg_frame_rate":"25/1"},{"index":1,"codec_type":"audio","start_time":"0","duration":"2"}],"format":{"duration":"2"}}`)
	for name, event := range map[string]string{
		"black start only":        "[Parsed_blackdetect_2 @ 0xabc] black_start:0.1",
		"black end only":          "[Parsed_blackdetect_2 @ 0xabc] black_end:0.5",
		"black missing duration":  "[Parsed_blackdetect_2 @ 0xabc] black_start:0.1 black_end:0.5",
		"black invalid duration":  "[Parsed_blackdetect_2 @ 0xabc] black_start:0.1 black_end:0.5 black_duration:nope",
		"black duration overflow": "[Parsed_blackdetect_2 @ 0xabc] black_start:0.1 black_end:0.5 black_duration:9223372036854776",
		"black duration mismatch": "[Parsed_blackdetect_2 @ 0xabc] black_start:0.1 black_end:0.5 black_duration:0.3",
		"black junk suffix":       "[Parsed_blackdetect_2 @ 0xabc] black_start:0.1 black_end:0.5 black_duration:0.4 junk",
		"duplicate black":         "[Parsed_blackdetect_2 @ 0xabc] black_start:0.1 black_end:0.5 black_duration:0.4\n[Parsed_blackdetect_2 @ 0xabc] black_start:0.1 black_end:0.5 black_duration:0.4",
		"negative black":          "[Parsed_blackdetect_2 @ 0xabc] black_start:-0.1 black_end:0.5 black_duration:0.6",
		"black past artifact":     "[Parsed_blackdetect_2 @ 0xabc] black_start:0.5 black_end:2.1 black_duration:1.6",
		"black end before start":  "[Parsed_blackdetect_2 @ 0xabc] black_start:1.5 black_end:0.5 black_duration:-1",
		"silence start only":      "[Parsed_silencedetect_4 @ 0xdef] silence_start: 0.1",
		"silence end only":        "[Parsed_silencedetect_4 @ 0xdef] silence_end: 0.5 | silence_duration: 0.4",
		"silence missing duration": "[Parsed_silencedetect_4 @ 0xdef] silence_start: 0.1\n" +
			"[Parsed_silencedetect_4 @ 0xdef] silence_end: 0.5",
		"silence invalid duration": "[Parsed_silencedetect_4 @ 0xdef] silence_start: 0.1\n" +
			"[Parsed_silencedetect_4 @ 0xdef] silence_end: 0.5 | silence_duration: nope",
		"silence duration mismatch": "[Parsed_silencedetect_4 @ 0xdef] silence_start: 0.1\n" +
			"[Parsed_silencedetect_4 @ 0xdef] silence_end: 0.5 | silence_duration: 0.3",
		"silence junk suffix": "[Parsed_silencedetect_4 @ 0xdef] silence_start: 0.1\n" +
			"[Parsed_silencedetect_4 @ 0xdef] silence_end: 0.5 | silence_duration: 0.4 junk",
		"duplicate silence": "[Parsed_silencedetect_4 @ 0xdef] silence_start: 0.1\n" +
			"[Parsed_silencedetect_4 @ 0xdef] silence_end: 0.5 | silence_duration: 0.4\n" +
			"[Parsed_silencedetect_4 @ 0xdef] silence_start: 0.1\n" +
			"[Parsed_silencedetect_4 @ 0xdef] silence_end: 0.5 | silence_duration: 0.4",
		"negative silence": "[Parsed_silencedetect_4 @ 0xdef] silence_start: -0.1\n" +
			"[Parsed_silencedetect_4 @ 0xdef] silence_end: 0.5 | silence_duration: 0.6",
		"silence past artifact": "[Parsed_silencedetect_4 @ 0xdef] silence_start: 0.5\n" +
			"[Parsed_silencedetect_4 @ 0xdef] silence_end: 2.1 | silence_duration: 1.6",
		"silence end before start": "[Parsed_silencedetect_4 @ 0xdef] silence_start: 1.5\n" +
			"[Parsed_silencedetect_4 @ 0xdef] silence_end: 0.5 | silence_duration: -1",
		"silence same line": "[Parsed_silencedetect_4 @ 0xdef] silence_start: 0.1 silence_end: 0.5 | silence_duration: 0.4",
		"silence repeated start": "[Parsed_silencedetect_4 @ 0xdef] silence_start: 0.1\n" +
			"[Parsed_silencedetect_4 @ 0xdef] silence_start: 0.2\n" +
			"[Parsed_silencedetect_4 @ 0xdef] silence_end: 0.5 | silence_duration: 0.3",
		"silence repeated end": "[Parsed_silencedetect_4 @ 0xdef] silence_start: 0.1\n" +
			"[Parsed_silencedetect_4 @ 0xdef] silence_end: 0.5 | silence_duration: 0.4\n" +
			"[Parsed_silencedetect_4 @ 0xdef] silence_end: 0.6 | silence_duration: 0.1",
		"freeze start only": "[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_start: 0.1",
		"freeze end only":   "[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_end: 0.5",
		"freeze missing end": "[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_start: 0.1\n" +
			"[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_duration: 0.4",
		"freeze missing duration": "[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_start: 0.1\n" +
			"[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_end: 0.5",
		"freeze invalid duration": "[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_start: 0.1\n" +
			"[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_duration: nope\n" +
			"[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_end: 0.5",
		"freeze duration mismatch": "[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_start: 0.1\n" +
			"[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_duration: 0.3\n" +
			"[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_end: 0.5",
		"freeze junk suffix": "[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_start: 0.1\n" +
			"[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_duration: 0.4\n" +
			"[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_end: 0.5 junk",
		"duplicate freeze": "[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_start: 0.1\n" +
			"[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_duration: 0.4\n" +
			"[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_end: 0.5\n" +
			"[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_start: 0.1\n" +
			"[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_duration: 0.4\n" +
			"[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_end: 0.5",
		"negative freeze": "[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_start: -0.1\n" +
			"[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_duration: 0.6\n" +
			"[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_end: 0.5",
		"freeze past artifact": "[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_start: 0.5\n" +
			"[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_duration: 1.6\n" +
			"[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_end: 2.1",
		"freeze end before start": "[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_start: 1.5\n" +
			"[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_duration: -1\n" +
			"[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_end: 0.5",
		"freeze same line": "[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_start: 0.1 " +
			"lavfi.freezedetect.freeze_duration: 0.4 lavfi.freezedetect.freeze_end: 0.5",
		"freeze repeated start": "[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_start: 0.1\n" +
			"[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_start: 0.2",
		"freeze repeated duration": "[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_start: 0.1\n" +
			"[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_duration: 0.4\n" +
			"[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_duration: 0.4\n" +
			"[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_end: 0.5",
		"freeze repeated end": "[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_start: 0.1\n" +
			"[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_duration: 0.4\n" +
			"[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_end: 0.5\n" +
			"[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_end: 0.6",
	} {
		t.Run(name, func(t *testing.T) {
			ffmpeg := testkit.POSIXExecutable(t, "conditioning-tool", `cat >&2 <<'EOF'
`+event+`
 I: -24.0 LUFS
 Peak: -3.0 dBFS
EOF`)
			_, err := mediatools.NewFFmpegTools(ffmpeg, probe, "", "", "").MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: artifact})
			if !errors.Is(err, mediatools.ErrConditioningOutput) {
				t.Fatalf("detector wire mutation error = %v, want invalid output", err)
			}
		})
	}
}

func TestMeasureConditioningRejectsDetectorIdentityAndGrammarMixing(t *testing.T) {
	artifact := conditioningArtifact(t, "fixture.mp4")
	probe := conditioningExactProbe(t, `{"streams":[{"index":0,"codec_type":"video","start_time":"0","duration":"2","avg_frame_rate":"25/1"},{"index":1,"codec_type":"audio","start_time":"0","duration":"2"}],"format":{"duration":"2"}}`)
	for name, events := range map[string]string{
		"distinct black instances": `[Parsed_blackdetect_2 @ 0xaaa] black_start:0.1 black_end:0.2 black_duration:0.1
[Parsed_blackdetect_3 @ 0xbbb] black_start:0.3 black_end:0.4 black_duration:0.1`,
		"interleaved silence instances": `[Parsed_silencedetect_4 @ 0xaaa] silence_start: 0.1
[Parsed_silencedetect_5 @ 0xbbb] silence_end: 0.5 | silence_duration: 0.4`,
		"interleaved freeze instances": `[Parsed_freezedetect_3 @ 0xaaa] lavfi.freezedetect.freeze_start: 0.1
[Parsed_freezedetect_4 @ 0xbbb] lavfi.freezedetect.freeze_duration: 0.4
[Parsed_freezedetect_3 @ 0xaaa] lavfi.freezedetect.freeze_end: 0.5`,
		"modern legacy freeze mixing": `[Parsed_freezedetect_3 @ 0xaaa] lavfi.freezedetect.freeze_start: 0.1
[freezedetect @ 0xbbb] freeze_duration: 0.4
[Parsed_freezedetect_3 @ 0xaaa] lavfi.freezedetect.freeze_end: 0.5`,
	} {
		t.Run(name, func(t *testing.T) {
			ffmpeg := testkit.POSIXExecutable(t, "conditioning-tool", `cat >&2 <<'EOF'
`+events+`
 I: -24.0 LUFS
 Peak: -3.0 dBFS
EOF`)
			_, err := mediatools.NewFFmpegTools(ffmpeg, probe, "", "", "").MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: artifact})
			if !errors.Is(err, mediatools.ErrConditioningOutput) {
				t.Fatalf("mixed detector identity error = %v, want invalid output", err)
			}
		})
	}
}

func TestMeasureConditioningRejectsAmbiguousLoudnessSummaries(t *testing.T) {
	artifact := conditioningArtifact(t, "fixture.mp4")
	probe := conditioningExactProbe(t, `{"streams":[{"index":0,"codec_type":"audio","start_time":"0","duration":"2"}],"format":{"duration":"2"}}`)
	for name, summary := range map[string]string{
		"duplicate integrated identical": " I: -24.0 LUFS\n I: -24.0 LUFS\n Peak: -3.0 dBFS",
		"duplicate integrated conflict":  " I: -24.0 LUFS\n I: -23.0 LUFS\n Peak: -3.0 dBFS",
		"duplicate peak identical":       " I: -24.0 LUFS\n Peak: -3.0 dBFS\n Peak: -3.0 dBFS",
		"duplicate peak conflict":        " I: -24.0 LUFS\n Peak: -3.0 dBFS\n Peak: -2.0 dBFS",
		"duplicate silent peak":          " I: -70.0 LUFS\n Peak: -inf dBFS\n Peak: -inf dBFS",
		"integrated junk suffix":         " I: -24.0 LUFS junk\n Peak: -3.0 dBFS",
		"peak junk suffix":               " I: -24.0 LUFS\n Peak: -3.0 dBFS junk",
	} {
		t.Run(name, func(t *testing.T) {
			ffmpeg := testkit.POSIXExecutable(t, "conditioning-tool", `printf '%s\n' '`+strings.ReplaceAll(summary, "'", "'\\''")+`' >&2`)
			_, err := mediatools.NewFFmpegTools(ffmpeg, probe, "", "", "").MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: artifact})
			if !errors.Is(err, mediatools.ErrConditioningOutput) {
				t.Fatalf("ambiguous loudness error = %v, want invalid output", err)
			}
		})
	}
}

func TestMeasureConditioningDoesNotNormalizeMalformedDetectorStartToFullDuration(t *testing.T) {
	artifact := conditioningArtifact(t, "fixture.mp4")
	probe := conditioningExactProbe(t, `{"streams":[{"index":0,"codec_type":"audio","start_time":"0","duration":"2"}],"format":{"duration":"2"}}`)
	ffmpeg := testkit.POSIXExecutable(t, "conditioning-tool", `cat >&2 <<'EOF'
[Parsed_silencedetect_4 @ 0xdef] silence_start: 0 silence_end: 1 | silence_duration: 1
 I: -70.0 LUFS
 Peak: -inf dBFS
EOF`)
	got, err := mediatools.NewFFmpegTools(ffmpeg, probe, "", "", "").MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: artifact})
	if !errors.Is(err, mediatools.ErrConditioningOutput) {
		t.Fatalf("malformed same-line detector error = %v, want invalid output", err)
	}
	if len(got.Quality.Silence) != 0 {
		t.Fatalf("malformed detector expanded to evidence: %+v", got.Quality.Silence)
	}
}

func TestMeasureConditioningNormalizesDetectorTimelines(t *testing.T) {
	artifact := conditioningArtifact(t, "offset.mp4")
	probe := conditioningExactProbe(t, `{"streams":[{"index":0,"codec_type":"video","start_time":"5","duration":"2","avg_frame_rate":"25/1"},{"index":1,"codec_type":"audio","start_time":"5","duration":"2"}],"format":{"duration":"2"}}`)
	ffmpeg := testkit.POSIXExecutable(t, "conditioning-tool", `case " $* " in
  *"setpts=PTS-STARTPTS"*"asetpts=PTS-STARTPTS"*) ;;
  *) exit 91 ;;
esac
cat >&2 <<'EOF'
[Parsed_blackdetect_2 @ 0xabc] black_start:0 black_end:0.5 black_duration:0.5
[Parsed_silencedetect_4 @ 0xdef] silence_start: 0
[Parsed_silencedetect_4 @ 0xdef] silence_end: 0.5 | silence_duration: 0.5
[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_start: 0
[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_duration: 1
[Parsed_freezedetect_3 @ 0x123] lavfi.freezedetect.freeze_end: 1
 I: -24.0 LUFS
 Peak: -3.0 dBFS
EOF`)
	got, err := mediatools.NewFFmpegTools(ffmpeg, probe, "", "", "").MeasureConditioning(context.Background(), mediatools.ConditioningRequest{Path: artifact})
	if err != nil {
		t.Fatal(err)
	}
	if got.Quality.Black[0].StartMs != 0 || got.Quality.Silence[0].StartMs != 0 || got.Quality.Freeze[0].StartMs != 0 {
		t.Fatalf("detector timelines are not artifact-relative: %+v", got.Quality)
	}
}

func TestMeasureConditioningMatchesReorderedPacketsPerStreamIndex(t *testing.T) {
	child := conditioningArtifact(t, "child.mp4")
	parent := conditioningArtifact(t, "parent.mp4")
	probe := conditioningPacketExecutable(t, "present")
	tools := mediatools.NewFFmpegTools(conditioningLoudnessExecutable(t, "-3.0"), probe, "", "", "")
	got, err := tools.MeasureConditioning(context.Background(), mediatools.ConditioningRequest{
		Path: child, ParentPath: parent, IntendedCuts: []mediatools.Interval{{StartMs: 4_000, EndMs: 8_000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Cuts) != 1 || len(got.Cuts[0].Streams) != 3 {
		t.Fatalf("cut streams = %+v", got.Cuts)
	}
	want := map[int][2]int64{0: {100, 100}, 1: {200, 200}, 2: {300, 300}}
	for _, stream := range got.Cuts[0].Streams {
		edges, ok := want[stream.Index]
		if !ok || !stream.StartError.Available || !stream.EndError.Available || stream.StartError.Milliseconds != edges[0] || stream.EndError.Milliseconds != edges[1] {
			t.Errorf("stream %s:%d = start %+v end %+v; want %v", stream.Kind, stream.Index, stream.StartError, stream.EndError, edges)
		}
	}
}

func TestMeasureConditioningDoesNotGuessEndFromMissingPacketDuration(t *testing.T) {
	for _, state := range []string{"missing", "zero"} {
		t.Run(state, func(t *testing.T) {
			child := conditioningArtifact(t, "child.mp4")
			parent := conditioningArtifact(t, "parent.mp4")
			got, err := mediatools.NewFFmpegTools(conditioningLoudnessExecutable(t, "-3.0"), conditioningPacketExecutable(t, state), "", "", "").MeasureConditioning(context.Background(), mediatools.ConditioningRequest{
				Path: child, ParentPath: parent, IntendedCuts: []mediatools.Interval{{StartMs: 4_000, EndMs: 8_000}},
			})
			if err != nil {
				t.Fatal(err)
			}
			for _, stream := range got.Cuts[0].Streams {
				if stream.Index == 1 && stream.EndError.Available {
					t.Fatalf("%s duration produced available end: %+v", state, stream)
				}
			}
		})
	}
}

func TestMeasureConditioningRejectsMalformedPacketDuration(t *testing.T) {
	child := conditioningArtifact(t, "child.mp4")
	parent := conditioningArtifact(t, "parent.mp4")
	_, err := mediatools.NewFFmpegTools(conditioningLoudnessExecutable(t, "-3.0"), conditioningPacketExecutable(t, "invalid"), "", "", "").MeasureConditioning(context.Background(), mediatools.ConditioningRequest{
		Path: child, ParentPath: parent, IntendedCuts: []mediatools.Interval{{StartMs: 4_000, EndMs: 8_000}},
	})
	if !errors.Is(err, mediatools.ErrConditioningOutput) {
		t.Fatalf("invalid packet duration error = %v, want invalid output", err)
	}
}

func conditioningPacketExecutable(t *testing.T, audioDurationState string) string {
	t.Helper()
	audioDuration := `"duration_time":"0.100",`
	switch audioDurationState {
	case "missing":
		audioDuration = ""
	case "invalid":
		audioDuration = `"duration_time":"invalid",`
	case "zero":
		audioDuration = `"duration_time":"0",`
	}
	return testkit.POSIXExecutable(t, "conditioning-tool", `case " $* " in
  *" -show_packets "*) ;;
  *" -show_frames "*) cat <<'EOF'
{"frames":[{"stream_index":0,"best_effort_timestamp_time":"0","duration_time":"10"},{"stream_index":1,"best_effort_timestamp_time":"0","nb_samples":10000}]}
EOF
     exit 0 ;;
  *) cat <<'EOF'
{"streams":[{"index":0,"codec_type":"video","start_time":"0","duration":"10","avg_frame_rate":"30000/1001"},{"index":1,"codec_type":"audio","start_time":"0","duration":"10","sample_rate":"1000"},{"index":2,"codec_type":"audio","start_time":"0","duration":"10"}],"format":{"duration":"10"}}
EOF
     exit 0 ;;
esac
path=""
interval=""
previous=""
for arg do
  if [ "$previous" = "-read_intervals" ]; then interval="$arg"; fi
  previous="$arg"
  path="$arg"
done
case "$path:$interval" in
  *child.mp4:0.000%3.000) packets='{"stream_index":2,"pts_time":"0.400","duration_time":"0.100","data_hash":"a2-late"},{"stream_index":1,"pts_time":"0.500","duration_time":"0.100","data_hash":"a1-late"},{"stream_index":0,"pts_time":"0.300","duration_time":"0.100","data_hash":"v-late"},{"stream_index":2,"pts_time":"0.100","duration_time":"0.100","data_hash":"same"},{"stream_index":1,"pts_time":"0.100","duration_time":"0.100","data_hash":"same"},{"stream_index":0,"pts_time":"0.000","duration_time":"0.100","data_hash":"v-start"}' ;;
  *child.mp4:7.000%10.000) packets='{"stream_index":0,"pts_time":"9.000","duration_time":"0.100","data_hash":"v-end"},{"stream_index":1,"pts_time":"9.000",`+audioDuration+`"data_hash":"same-end"},{"stream_index":2,"pts_time":"9.000","duration_time":"0.100","data_hash":"same-end"},{"stream_index":0,"pts_time":"8.000","duration_time":"0.100","data_hash":"v-old"},{"stream_index":1,"pts_time":"8.000","duration_time":"0.100","data_hash":"a1-old"},{"stream_index":2,"pts_time":"8.000","duration_time":"0.100","data_hash":"a2-old"}' ;;
  *parent.mp4:1.000%7.000) packets='{"stream_index":2,"pts_time":"4.300","duration_time":"0.100","data_hash":"same"},{"stream_index":1,"pts_time":"4.200","duration_time":"0.100","data_hash":"same"},{"stream_index":0,"pts_time":"4.100","duration_time":"0.100","data_hash":"v-start"}' ;;
  *parent.mp4:5.000%11.000) packets='{"stream_index":0,"pts_time":"8.000","duration_time":"0.100","data_hash":"v-old"},{"stream_index":2,"pts_time":"8.200","duration_time":"0.100","data_hash":"same-end"},{"stream_index":1,"pts_time":"8.100","duration_time":"0.100","data_hash":"same-end"},{"stream_index":0,"pts_time":"8.000","duration_time":"0.100","data_hash":"v-end"}' ;;
  *) exit 92 ;;
esac
printf '{"streams":[{"index":0,"codec_type":"video"},{"index":1,"codec_type":"audio"},{"index":2,"codec_type":"audio"}],"packets":[%s],"format":{"duration":"10"}}\n' "$packets"`)
}

func conditioningLoudnessExecutable(t *testing.T, peak string) string {
	t.Helper()
	return testkit.POSIXExecutable(t, "conditioning-tool", `cat >&2 <<'EOF'
[Parsed_silencedetect_4 @ 0xdef] silence_start: 0
[Parsed_silencedetect_4 @ 0xdef] silence_end: 2 | silence_duration: 2
 I: -70.0 LUFS
 Peak: `+peak+` dBFS
EOF`)
}

func conditioningExactProbe(t *testing.T, raw string) string {
	t.Helper()
	var probe struct {
		Streams []struct {
			Index        *int   `json:"index"`
			CodecType    string `json:"codec_type"`
			StartTime    string `json:"start_time,omitempty"`
			Duration     string `json:"duration,omitempty"`
			AvgFrameRate string `json:"avg_frame_rate,omitempty"`
			SampleRate   string `json:"sample_rate,omitempty"`
		} `json:"streams"`
		Frames []map[string]any `json:"frames,omitempty"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		t.Fatal(err)
	}
	for i := range probe.Streams {
		stream := &probe.Streams[i]
		if stream.Index == nil || stream.Duration == "" {
			continue
		}
		switch stream.CodecType {
		case "video":
			probe.Frames = append(probe.Frames, map[string]any{
				"stream_index": *stream.Index, "best_effort_timestamp_time": "0", "duration_time": stream.Duration,
			})
		case "audio":
			seconds, ok := new(big.Rat).SetString(stream.Duration)
			if !ok || seconds.Sign() <= 0 {
				continue
			}
			samples := new(big.Rat).Mul(seconds, big.NewRat(1_000_000, 1))
			if !samples.IsInt() || !samples.Num().IsInt64() {
				t.Fatalf("fake audio duration %q is not exactly representable at 1MHz", stream.Duration)
			}
			stream.SampleRate = "1000000"
			probe.Frames = append(probe.Frames, map[string]any{
				"stream_index": *stream.Index, "best_effort_timestamp_time": "0", "nb_samples": samples.Num().Int64(),
			})
		}
	}
	frames := probe.Frames
	probe.Frames = nil
	metadata, err := json.Marshal(probe)
	if err != nil {
		t.Fatal(err)
	}
	frameProbe, err := json.Marshal(struct {
		Frames []map[string]any `json:"frames"`
	}{Frames: frames})
	if err != nil {
		t.Fatal(err)
	}
	return testkit.POSIXExecutable(t, "conditioning-tool", `case " $* " in
  *" -show_frames "*) cat <<'EOF'
`+string(frameProbe)+`
EOF
    ;;
  *) cat <<'EOF'
`+string(metadata)+`
EOF
    ;;
esac`)
}

func conditioningArtifact(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("local fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
