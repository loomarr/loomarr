//go:build ffmpeg

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playoutcert"
)

func TestCommandOperatorCohortPublishesExactInputIdentity(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 80*time.Second)
	defer cancel()
	dir := t.TempDir()
	var programmes []any
	// Reverse the built-in fixture order: silently generating default media must fail.
	for index, colour := range []string{"white", "black"} {
		frequency, luma, rate := "880", []int{225, 255}, []float64{0.027, 0.050}
		if index == 1 {
			frequency, luma, rate = "440", []int{0, 25}, []float64{0.012, 0.026}
		}
		name := "operator-" + colour + ".mp4"
		file := filepath.Join(dir, name)
		cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-f", "lavfi", "-i", "color=c="+colour+":s=320x180:r=25:d=12", "-f", "lavfi", "-i", "sine=frequency="+frequency+":sample_rate=48000:duration=12", "-map", "0:v:0", "-map", "1:a:0", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-c:a", "aac", "-ac", "2", "-b:a", "96k", "-shortest", file)
		if err := cmd.Run(); err != nil {
			t.Fatal("fixture media:", err)
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		programmes = append(programmes, map[string]any{"file": name, "bytes": len(data), "sha256": hex.EncodeToString(digest[:]), "signals": map[string]any{"luma": map[string]int{"min": luma[0], "max": luma[1]}, "zeroCrossingRate": map[string]float64{"min": rate[0], "max": rate[1]}, "rmsDB": map[string]int{"min": -80, "max": -1}, "silence": false}})
	}
	channels := []playoutcert.Channel{{ID: "operator-channel", Roles: []string{"prepared"}}}
	channelData, err := json.Marshal(manifest{SchemaVersion: 1, Channels: channels})
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(dir, "channels.json")
	if err := os.WriteFile(manifestPath, channelData, 0600); err != nil {
		t.Fatal(err)
	}
	corpus, err := json.Marshal(map[string]any{"schemaVersion": 1, "channels": []any{map[string]any{"id": channels[0].ID, "programmes": programmes}}})
	if err != nil {
		t.Fatal(err)
	}
	cohortPath := filepath.Join(dir, "private-corpus.json")
	if err := os.WriteFile(cohortPath, corpus, 0600); err != nil {
		t.Fatal(err)
	}
	corpusHash := sha256.Sum256(corpus)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := run(ctx, []string{"--manifest", manifestPath, "--operator-cohort", cohortPath, "--synthetic-capacity", "1", "--synthetic-grace", "100ms", "--cleanup-timeout", "5s", "--programme-boundary-timeout", "20s", "--suite-timeout", "70s", "--raw-capture-bytes", "262144", "--concurrency", "1", "--fan-in", "1"}, func(key string) string {
		if key == "LOOMARR_ARTIFACT_DIR" {
			return dir
		}
		return ""
	}, stdout, stderr)
	data, err := os.ReadFile(filepath.Join(dir, "playout-load-cert.json"))
	if err != nil {
		t.Fatalf("no report: code=%d stderr=%q err=%v", code, stderr.String(), err)
	}
	var report playoutcert.Report
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if code != 1 || stderr.Len() != 0 || report.Certified || report.AuditStatus != playoutcert.AuditPassed || report.Target.CohortManifestSHA256 != hex.EncodeToString(corpusHash[:]) || report.Target.ConfiguredChannels != 1 {
		t.Fatalf("operator publication code=%d audit=%s identity=%q stderr=%q", code, report.AuditStatus, report.Target.CohortManifestSHA256, stderr.String())
	}
	phase, ok := report.Phase("programme_boundary")
	if !ok || phase.Attempts != 2 || phase.Successes != 1 || phase.Failures != 1 || phase.HTTPClasses["cohort_missing"] != 1 || len(phase.ProgrammeBoundaries) != 2 {
		t.Fatalf("operator expected sequence not observed: %+v", phase)
	}
	prepared, absent := phase.ProgrammeBoundaries[0], phase.ProgrammeBoundaries[1]
	if prepared.Lane != "prepared" || prepared.Outcome != "ok" || prepared.Transitions < 1 || prepared.DecodedAudioSamplesDelta <= 0 || prepared.DecodedFrameDelta <= 0 || absent.Lane != "transcode" || absent.Outcome != "cohort_missing" {
		t.Fatalf("prepared evidence or absent-lane refusal lost: %+v", phase.ProgrammeBoundaries)
	}
	for _, private := range []string{dir, "private-corpus.json", "operator-white.mp4", "operator-black.mp4", "operator-channel"} {
		if strings.Contains(string(data), private) || strings.Contains(stdout.String(), private) {
			t.Fatal("publication contains private input")
		}
	}
}
