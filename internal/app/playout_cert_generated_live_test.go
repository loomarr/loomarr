//go:build ffmpeg

package app

import (
	"context"
	"os/exec"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/playoutcert"
)

func TestGeneratedCopySourcesKeepTranscodeLaneCold(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	config := PlayoutCertificationConfig{ProgrammeDuration: 30 * time.Second, Channels: []playoutcert.Channel{
		{ID: "prepared", Roles: []string{"prepared"}},
		{ID: "cold", Roles: []string{"transcode_h264", "audio_aac"}},
		{ID: "copy", Roles: []string{"copy", "audio_aac"}},
		{ID: "copy-peer", Roles: []string{"copy", "audio_aac"}},
	}}
	sources, err := prepareCertificationSources(ctx, config, t.TempDir(), ffmpeg)
	if err != nil {
		t.Fatal(err)
	}
	if sources.paths["prepared"] != sources.paths["cold"] || sources.paths["copy"] != sources.paths["copy-peer"] || sources.paths["copy"] == sources.paths["cold"] {
		t.Fatal("generated source pairs crossed copy/transcode ownership")
	}
	resolver := syntheticLiveResolver{sources: sources.paths, formats: sources.formats, tracks: sources.tracks}
	proof := playout.FFprobeCopyStartNextTo(ffmpeg)
	for _, channel := range config.Channels {
		if sources.signatures[channel.ID] != defaultCertificationSignatures() {
			t.Fatal("copy generation changed programme truth")
		}
		for _, source := range sources.paths[channel.ID] {
			if !slices.Contains(sources.private, source) {
				t.Fatal("generated source omitted from private audit")
			}
			plan, format := resolver.PlanFor(ctx, source, playout.PlanBaseline)
			if !slices.Contains(channel.Roles, "copy") {
				if plan.CopyVideo || plan.CopyAudio {
					t.Fatal("copy generation removed the cold transcode workload")
				}
				continue
			}
			plan = playout.ConformCopyPlan(format, plan, resolver.Profile(ctx), "h264")
			if !plan.CopyVideo || !plan.CopyAudio || format.VideoCodec != "h264" || format.AudioCodec != "aac" || format.AudioChannels != 2 || len(sources.tracks[source].Audio) != 1 {
				t.Fatalf("generated copy profile not measured/conformant: %+v %+v", plan, format)
			}
			for _, offset := range []time.Duration{time.Millisecond, 3237 * time.Millisecond, 15 * time.Second} {
				seek, ok := proof(ctx, source, offset, time.Second, format.FrameRate)
				if !ok || seek > offset || offset-seek >= 40*time.Millisecond {
					t.Fatalf("copy lacks actual seek proof at %s: %s %t", offset, seek, ok)
				}
			}
		}
	}
}
