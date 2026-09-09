//go:build ffmpeg

package playoutcert_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/app"
	"github.com/loomarr/loomarr/internal/playoutcert"
)

func TestOperatorCohortPreparedChannelsKeepIndependentSources(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 80*time.Second)
	defer cancel()
	root := t.TempDir()
	var programmes [2]map[string]any
	for index, colour := range []string{"black", "white"} {
		frequency := "440"
		luma := []int{0, 25}
		rate := []float64{0.012, 0.026}
		if index == 1 {
			frequency = "880"
			luma = []int{225, 255}
			rate = []float64{0.027, 0.050}
		}
		name := colour + ".mp4"
		file := filepath.Join(root, name)
		cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-f", "lavfi", "-i", "color=c="+colour+":s=320x180:r=25:d=12", "-f", "lavfi", "-i", "sine=frequency="+frequency+":sample_rate=48000:duration=12", "-map", "0:v:0", "-map", "1:a:0", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-c:a", "aac", "-ac", "2", "-b:a", "96k", "-shortest", file)
		if err := cmd.Run(); err != nil {
			t.Fatal("create independent input fixture:", err)
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		programmes[index] = map[string]any{"file": name, "bytes": len(data), "sha256": hex.EncodeToString(digest[:]), "signals": map[string]any{"luma": map[string]any{"min": luma[0], "max": luma[1]}, "zeroCrossingRate": map[string]any{"min": rate[0], "max": rate[1]}, "rmsDB": map[string]any{"min": -80, "max": -1}, "silence": false}}
	}
	channels := []playoutcert.Channel{{ID: "first", Roles: []string{"prepared"}}, {ID: "reversed", Roles: []string{"prepared"}}}
	document := map[string]any{"schemaVersion": 1, "channels": []any{map[string]any{"id": "first", "programmes": []any{programmes[0], programmes[1]}}, map[string]any{"id": "reversed", "programmes": []any{programmes[1], programmes[0]}}}}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(root, "cohort.json")
	if err := os.WriteFile(manifest, data, 0600); err != nil {
		t.Fatal(err)
	}
	cohort, err := playoutcert.LoadOperatorCohort(ctx, manifest, channels)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cohort.Close() }()
	target, err := app.NewPlayoutCertificationTarget(ctx, app.PlayoutCertificationConfig{Channels: channels, FFmpeg: ffmpeg, Capacity: 1, Grace: time.Second, ProgrammeDuration: 6 * time.Second, Cohort: cohort})
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
	// Only owned staged files may remain in use after preparation.
	for _, name := range []string{"black.mp4", "white.mp4"} {
		if err := os.Remove(filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	for index, channel := range channels {
		t.Run(channel.ID, func(t *testing.T) {
			config := playoutcert.Config{BaseURL: target.BaseURL, AdminBearer: target.AdminBearer, DeviceToken: target.DeviceToken, Channels: channels, ProgrammeEvidence: target.ProgrammeEvidence(), SignalDecoder: playoutcert.FFmpegSignalDecoder{Path: ffmpeg}, RequestTimeout: 10 * time.Second, ProgrammeBoundaryTimeout: 25 * time.Second, ProgrammeBoundaryLateObservation: 3 * time.Second, RawCaptureBytes: 256 << 10}
			result, err := playoutcert.ObserveProgrammeSignalsForTest(ctx, config, "prepared", index)
			if err != nil || result.Class != "ok" || result.Evidence.Transitions < 1 || result.Evidence.DecodedAudioSamplesDelta <= 0 || result.Evidence.DecodedFrameDelta <= 0 {
				t.Fatalf("operator programme qualification=%+v err=%v", result, err)
			}
		})
	}
}
