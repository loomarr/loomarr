//go:build ffmpeg

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playoutcert"
)

func TestCommandSyntheticCertifiesWithoutExternalCredentials(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	dir := t.TempDir()
	channels := make([]playoutcert.Channel, 0, 105)
	for index := range 100 {
		channels = append(channels, playoutcert.Channel{ID: fmt.Sprintf("prepared-%03d", index+1), Roles: []string{"prepared"}})
	}
	for index := range 5 {
		channels = append(channels, playoutcert.Channel{ID: fmt.Sprintf("transcode-%03d", index+1), Roles: []string{"transcode_h264", "audio_aac"}})
	}
	manifestPath := filepath.Join(dir, "manifest.json")
	manifestBody, err := json.Marshal(manifest{SchemaVersion: 1, Channels: channels})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, manifestBody, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := run(ctx, []string{
		"--manifest", manifestPath, "--synthetic", "--certify", "--synthetic-capacity", "4",
		"--synthetic-grace", "1s", "--cleanup-timeout", "10s",
	}, func(key string) string {
		if key == "LOOMARR_ARTIFACT_DIR" {
			return dir
		}
		return ""
	}, stdout, stderr)
	if code != 0 {
		artifact, _ := os.ReadFile(filepath.Join(dir, "playout-load-cert.json"))
		t.Fatalf("code=%d stdout=%q stderr=%q artifact=%s", code, stdout.String(), stderr.String(), artifact)
	}
	var report playoutcert.Report
	artifact, err := os.ReadFile(filepath.Join(dir, "playout-load-cert.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(artifact, &report); err != nil {
		t.Fatal(err)
	}
	if !report.Certified || report.Target.ConfiguredChannels != 105 {
		t.Fatalf("report certified=%t configured=%d failures=%v", report.Certified, report.Target.ConfiguredChannels, report.Failures)
	}
}
