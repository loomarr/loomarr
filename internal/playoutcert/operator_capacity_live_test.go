//go:build ffmpeg

package playoutcert_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/app"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/playoutcert"
)

type operatorMediaProfile struct {
	id, video, audio   string
	width, height, gop int
	roles              []string
}

func TestOperatorCopyReportObservesActualAdmission(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	for _, tc := range []struct {
		name          string
		gop, wantCost int
	}{
		{name: "copy", gop: 1},
		{name: "unsafe seek needs encoding", gop: 250, wantCost: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
			defer cancel()
			manifest, channels := operatorCapacityCorpus(t, ctx, ffmpeg, t.TempDir(), []operatorMediaProfile{
				{"ordinary-copy-private", "h264", "eac3", 320, 180, tc.gop, []string{"copy", "audio_eac3"}},
			})
			cohort, err := playoutcert.LoadOperatorCohort(ctx, manifest, channels)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = cohort.Close() }()
			target, err := app.NewPlayoutCertificationTarget(ctx, app.PlayoutCertificationConfig{Channels: channels, FFmpeg: ffmpeg, Capacity: 1, Grace: 100 * time.Millisecond, ProgrammeDuration: 30 * time.Second, Cohort: cohort})
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
			config := playoutcert.Config{BaseURL: target.BaseURL, AdminBearer: target.AdminBearer, DeviceToken: target.DeviceToken, Channels: channels,
				Concurrency: 1, FanInViewers: 1, RequestTimeout: 25 * time.Second, CleanupTimeout: 8 * time.Second, CleanupPoll: 25 * time.Millisecond,
				WarmGrace: 100 * time.Millisecond, RawCaptureBytes: 2 << 20, Validator: playoutcert.FFprobeValidator{}, Decoder: playoutcert.FFmpegDecoder{},
				ProgrammeEvidence: target.ProgrammeEvidence(), ProgrammeBoundaryTimeout: 2 * time.Second, ProgrammeBoundaryLateObservation: 250 * time.Millisecond,
			}
			report, err := playoutcert.Run(ctx, config)
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"configured", "surf"} {
				phase := report.PhaseMust(name)
				if phase.Attempts != 2 || phase.Failures != 0 || phase.HTTPClasses["prepared_miss"] != 1 {
					t.Fatalf("%s: %+v", name, phase)
				}
			}
			phase := report.PhaseMust("copy_raw")
			if (phase.Failures > 0) != (tc.wantCost > 0) || phase.Resources.Maximum.TranscodeCost > report.Target.Capacity || len(phase.HeldContinuity) != 1 || !phase.HeldContinuity[0].DecodedFrame || phase.HeldContinuity[0].Outcome != "observed" {
				t.Fatalf("copy result: %+v", phase)
			}
			heldSamples := 0
			for _, sample := range report.Resources {
				if sample.Point == "copy_raw_held" {
					heldSamples++
					if sample.TranscodeCost != tc.wantCost || sample.SessionsActive != 1 || sample.ViewerActive != 1 {
						t.Fatalf("held playback cost: %+v", sample)
					}
				}
			}
			if heldSamples != 1 {
				t.Fatalf("held resource evidence missing: %d", heldSamples)
			}
			if tc.wantCost > 0 && (phase.HTTPClasses["copy_cost_mismatch"] == 0 || !slices.Contains(report.Failures, "copy_raw_failed")) {
				t.Fatalf("role hid video cost: %+v", phase)
			}
			if len(phase.Media) != 1 || phase.Media[0].VideoCodec != "h264" || phase.Media[0].AudioCodec != "aac" {
				t.Fatalf("baseline media not verified: %+v", phase.Media)
			}
			if report.PhaseMust("raw_capacity").HTTPClasses["transcode_cohort_insufficient"] == 0 || report.PhaseMust("programme_boundary").Failures == 0 {
				t.Fatal("copy coverage silently qualified missing capacity or boundary evidence")
			}
			publication, err := playoutcert.FinalizePublication(report)
			if err != nil || publication.AuditStatus() != playoutcert.AuditPassed || publication.Verdict() == playoutcert.VerdictCertified {
				t.Fatalf("copy report publication: %v / %s", err, publication.JSON())
			}
		})
	}
}

// This fixture supplies actual codecs; manifest roles do not stand in for the
// source probe, ordinary programme route or observed admission accounting.
func operatorCapacityCorpus(t *testing.T, ctx context.Context, ffmpeg, root string, profiles []operatorMediaProfile) (string, []playoutcert.Channel) {
	t.Helper()

	var channels []playoutcert.Channel
	var declarations []any
	var preparedSources []any
	for profileIndex, profile := range profiles {
		var programmes []any
		for index, colour := range []string{"black", "white"} {
			frequency, luma, rate := "440", []int{0, 25}, []float64{0.012, 0.026}
			if index == 1 {
				frequency, luma, rate = "880", []int{225, 255}, []float64{0.027, 0.050}
			}
			name := fmt.Sprintf("%s-%d.mkv", profile.id, index)
			file := filepath.Join(root, name)
			encoder := "libx264"
			if profile.video == "hevc" {
				encoder = "libx265"
			}
			args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-f", "lavfi", "-i", fmt.Sprintf("color=c=%s:s=%dx%d:r=25:d=32", colour, profile.width, profile.height), "-f", "lavfi", "-i", "sine=frequency=" + frequency + ":sample_rate=48000:duration=32", "-map", "0:v:0", "-map", "1:a:0", "-c:v", encoder, "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-bf", "0", "-g", fmt.Sprint(profile.gop)}
			if profile.video == "hevc" {
				args = append(args, "-x265-params", "pools=1:frame-threads=1:bframes=0:log-level=error")
			}
			args = append(args, "-c:a", profile.audio, "-ac", "2", "-b:a", "192k", "-shortest", file)
			if err := exec.CommandContext(ctx, ffmpeg, args...).Run(); err != nil {
				t.Fatalf("create %s: %v", profile.id, err)
			}
			observed, err := playout.FFprobeSourceNextTo(ffmpeg)(ctx, file)
			if err != nil || len(observed.Streams) != 2 || observed.Streams[0].Codec != profile.video || observed.Streams[1].Codec != profile.audio {
				t.Fatalf("fixture codec mismatch for %s: %+v %v", profile.id, observed, err)
			}
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(data)
			programmes = append(programmes, map[string]any{"file": name, "bytes": len(data), "sha256": hex.EncodeToString(digest[:]), "signals": map[string]any{"luma": map[string]int{"min": luma[0], "max": luma[1]}, "zeroCrossingRate": map[string]float64{"min": rate[0], "max": rate[1]}, "rmsDB": map[string]int{"min": -80, "max": -1}, "silence": false}})
		}
		channels = append(channels, playoutcert.Channel{ID: profile.id, Roles: profile.roles})
		declarations = append(declarations, map[string]any{"id": profile.id, "programmes": programmes})
		if profileIndex == 0 {
			preparedSources = programmes
		}
	}
	// An explicit prepared control keeps the other four Channels in ordinary playback.
	channels = append(channels, playoutcert.Channel{ID: "prepared-control", Roles: []string{"prepared"}})
	declarations = append(declarations, map[string]any{"id": "prepared-control", "programmes": preparedSources})
	data, err := json.Marshal(map[string]any{"schemaVersion": 1, "channels": declarations})
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(root, "corpus.json")
	if err := os.WriteFile(manifest, data, 0600); err != nil {
		t.Fatal(err)
	}
	return manifest, channels
}

func TestOperatorCohortCopyAndCodecTranscodesRespectCapacity(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	manifest, channels := operatorCapacityCorpus(t, ctx, ffmpeg, t.TempDir(), []operatorMediaProfile{
		{"copy-aac", "h264", "aac", 320, 180, 1, []string{"copy", "audio_aac"}},
		{"audio-eac3", "h264", "eac3", 320, 180, 1, []string{"copy", "audio_eac3"}},
		{"video-hevc", "hevc", "ac3", 160, 90, 250, []string{"transcode_hevc", "audio_ac3"}},
		{"video-h264", "h264", "aac", 640, 360, 250, []string{"transcode_h264", "audio_aac"}},
	})
	cohort, err := playoutcert.LoadOperatorCohort(ctx, manifest, channels)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cohort.Close() }()
	target, err := app.NewPlayoutCertificationTarget(ctx, app.PlayoutCertificationConfig{Channels: channels, FFmpeg: ffmpeg, Capacity: 1, Grace: 100 * time.Millisecond, ProgrammeDuration: 30 * time.Second, Cohort: cohort})
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
	config := playoutcert.NormalizeConfigForTest(playoutcert.Config{BaseURL: target.BaseURL, AdminBearer: target.AdminBearer, DeviceToken: target.DeviceToken, Channels: channels, Concurrency: 1, RequestTimeout: 30 * time.Second, CleanupTimeout: 8 * time.Second, CleanupPoll: 25 * time.Millisecond, WarmGrace: 100 * time.Millisecond, RawCaptureBytes: 2 << 20, Validator: playoutcert.FFprobeValidator{}, Decoder: playoutcert.FFmpegDecoder{}})
	endpoint, err := playoutcert.NewEndpointForTest(config)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := playoutcert.SampleResources(ctx, config, "baseline")
	if err != nil {
		t.Fatal(err)
	}
	var held []*playoutcert.HeldBurstForTest
	defer func() {
		for _, viewer := range held {
			viewer.Release()
		}
	}()
	for index := range 3 {
		viewer := playoutcert.StartHeldBurstForTest(ctx, endpoint, config, []int{index})
		held = append(held, viewer)
		results := viewer.Results()
		if len(results) != 1 || results[0].Class != "ok" || results[0].Media.VideoStreams != 1 || results[0].Media.AudioStreams != 1 || results[0].Media.VideoCodec != "h264" || results[0].Media.AudioCodec != "aac" {
			t.Fatalf("%s did not produce baseline A/V: %+v", channels[index].ID, results)
		}
		sample, err := playoutcert.SampleResources(ctx, config, "raw_capacity")
		cost := 0
		if index == 2 {
			cost = 1
		}
		if err != nil || sample.SessionsActive != index+1 || sample.TranscodeCost != cost || sample.Capacity != 1 {
			t.Fatalf("%s admission accounting: %+v %v", channels[index].ID, sample, err)
		}
	}
	overloaded, sample := playoutcert.RawBurstForTest(ctx, endpoint, config, []int{3})
	if len(overloaded) != 1 || overloaded[0].Class != "http_503" || sample.TranscodeCost != 1 || sample.SessionsActive != 3 {
		t.Fatalf("excess H264 transcode not bounded: %+v %+v", overloaded, sample)
	}
	var verified sync.WaitGroup
	for _, viewer := range held {
		verified.Go(func() { viewer.Verify(ctx) })
	}
	verified.Wait()
	for index, viewer := range held {
		if results := viewer.Results(); len(results) != 1 || results[0].Class != "ok" {
			t.Fatalf("held %s interrupted by overload: %+v", channels[index].ID, results)
		}
	}
	if class, sample := playoutcert.WaitForConvergenceForTest(ctx, endpoint, config, baseline); class != "ok" {
		t.Fatalf("release failed to converge: %s %+v", class, sample)
	}
	recovered, sample := playoutcert.RawBurstForTest(ctx, endpoint, config, []int{3})
	if len(recovered) != 1 || recovered[0].Class != "ok" || recovered[0].Media.VideoCodec != "h264" || recovered[0].Media.AudioCodec != "aac" || sample.TranscodeCost != 1 {
		t.Fatalf("H264 transcode did not recover released capacity: %+v %+v", recovered, sample)
	}
	if class, sample := playoutcert.WaitForConvergenceForTest(ctx, endpoint, config, baseline); class != "ok" {
		t.Fatalf("recovery cleanup failed: %s %+v", class, sample)
	}
}

// Retain the original sparse-keyframe sources: these match the output codec but
// require video encoding at the mid-programme seek. Copy-role labels cannot waive it.
func TestOperatorCohortLongGOPStartsUseTranscodeCapacity(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	manifest, channels := operatorCapacityCorpus(t, ctx, ffmpeg, t.TempDir(), []operatorMediaProfile{
		{"copy-aac", "h264", "aac", 320, 180, 250, []string{"copy", "audio_aac"}},
		{"audio-eac3", "h264", "eac3", 320, 180, 250, []string{"copy", "audio_eac3"}},
	})
	cohort, err := playoutcert.LoadOperatorCohort(ctx, manifest, channels)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cohort.Close() }()
	target, err := app.NewPlayoutCertificationTarget(ctx, app.PlayoutCertificationConfig{Channels: channels, FFmpeg: ffmpeg, Capacity: 2, Grace: 100 * time.Millisecond, ProgrammeDuration: 30 * time.Second, Cohort: cohort})
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
	config := playoutcert.NormalizeConfigForTest(playoutcert.Config{BaseURL: target.BaseURL, AdminBearer: target.AdminBearer, DeviceToken: target.DeviceToken, Channels: channels, RequestTimeout: 20 * time.Second, CleanupTimeout: 8 * time.Second, CleanupPoll: 25 * time.Millisecond, WarmGrace: 100 * time.Millisecond, RawCaptureBytes: 2 << 20, Validator: playoutcert.FFprobeValidator{}, Decoder: playoutcert.FFmpegDecoder{}})
	endpoint, err := playoutcert.NewEndpointForTest(config)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := playoutcert.SampleResources(ctx, config, "baseline")
	if err != nil {
		t.Fatal(err)
	}
	held := playoutcert.StartHeldBurstForTest(ctx, endpoint, config, []int{0, 1})
	defer held.Release()
	results := held.Results()
	if len(results) != 2 {
		t.Fatal("missing long-GOP streams")
	}
	for _, result := range results {
		if result.Class != "ok" || result.Media.VideoStreams != 1 || result.Media.AudioStreams != 1 || result.Media.VideoCodec != "h264" || result.Media.AudioCodec != "aac" {
			t.Fatalf("long-GOP startup still broken: %+v", results)
		}
	}
	sample, err := playoutcert.SampleResources(ctx, config, "raw_capacity")
	if err != nil || sample.TranscodeCost != 2 || sample.SessionsActive != 2 {
		t.Fatalf("unsafe copies did not reserve video slots: %+v %v", sample, err)
	}
	held.Verify(ctx)
	for _, result := range held.Results() {
		if result.Class != "ok" {
			t.Fatalf("long-GOP viewer did not keep decoding: %+v", held.Results())
		}
	}
	if class, sample := playoutcert.WaitForConvergenceForTest(ctx, endpoint, config, baseline); class != "ok" {
		t.Fatalf("long-GOP cleanup: %s %+v", class, sample)
	}
}
