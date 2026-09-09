//go:build ffmpeg

package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/playoutcert"
	"github.com/loomarr/loomarr/internal/prepared"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
)

// This is composition evidence with a fixed test pool and software packager.
// It does not run encoder detection or establish hardware capacity.
func TestDeclaredPreparationConvergesAndReopensOneHundredChannels(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ctx := t.Context()
	st, err := store.Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "cert.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	config := PlayoutCertificationConfig{FFmpeg: ffmpeg, QualityTier: playout.TierEfficient, Capacity: 4, ProgrammeDuration: 6 * time.Second}
	var indexes []int
	for index := range 101 {
		id := fmt.Sprintf("prepared-%03d", index)
		config.Channels = append(config.Channels, playoutcert.Channel{ID: id})
		row := store.Channel{Channel: schedule.Channel{ID: id, Name: id, Number: index + 1, Status: schedule.StatusLive}}
		row.Policy.Playout = &schedule.PlayoutPolicy{Backend: schedule.PlayoutBackendInternal}
		if _, err := st.SaveChannel(ctx, row); err != nil {
			t.Fatal(err)
		}
		if index < 100 {
			indexes = append(indexes, index)
		}
	}
	var paths [2]string
	for variant := range paths {
		paths[variant] = filepath.Join(t.TempDir(), "source.mp4")
		if err := generateSyntheticSource(ctx, ffmpeg, paths[variant], variant, syntheticSourceProfile{keyframeInterval: 25, audioChannels: 2, duration: 6 * time.Second}); err != nil {
			t.Fatal(err)
		}
	}
	sources := certificationSources{paths: make(map[string][2]string)}
	for _, channel := range config.Channels {
		sources.paths[channel.ID] = paths
	}
	root := t.TempDir()
	lib, err := prepared.NewLibrary(root)
	if err != nil {
		t.Fatal(err)
	}
	target := &PlayoutCertificationTarget{store: st, encoder: playout.EncoderVAAPI,
		programmeEvidence: &syntheticProgrammeEvidence{assets: make(map[string][2]map[string]syntheticAssetTruth)}}
	epoch := time.Now().UTC().Add(-time.Second)
	runtime, clock, err := target.prepareDeclared(ctx, config, sources, lib, prepared.NewFFmpegPackager(ffmpeg), syntheticProgrammeSchedule{epoch: epoch, duration: config.ProgrammeDuration}, indexes)
	if err != nil {
		t.Fatal(err)
	}
	status := target.preparation.Status()
	if status.LastRunAt.IsZero() || status.Readiness.Channels != 100 || status.Readiness.ReadyChannels != 100 || status.Readiness.ReadyBindings != 200 || status.Readiness.MissingBindings != 0 {
		t.Fatalf("normal planner did not converge: %+v", status)
	}
	at := epoch.Add(time.Second)
	window, ready, err := runtime.ResolvePrepared(ctx, playout.TuneRequest{ChannelID: config.Channels[0].ID}, at)
	want, _, _ := clock.airings(at, config.Channels[0].ID)
	if err != nil || !ready || window.Current.Identity != want {
		t.Fatalf("prepared and live programme truth differ: ready=%t err=%v got=%+v want=%+v", ready, err, window.Current.Identity, want)
	}
	if _, ready, err := runtime.ResolvePrepared(ctx, playout.TuneRequest{ChannelID: config.Channels[100].ID}, at); err != nil || ready {
		t.Fatalf("cold cohort was prepared: ready=%t err=%v", ready, err)
	}
	// Reopen the persistent index through the production resolver without a
	// preparation pass; tune-time readiness must survive that process boundary.
	lib2, err := prepared.NewLibrary(root)
	if err != nil {
		t.Fatal(err)
	}
	readiness, err := prepared.OpenReadiness(lib2)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := newCertificationPreparation(ctx, st, sources, clock, indexes, config)
	if err != nil {
		t.Fatal(err)
	}
	preparer := prepared.NewPreparer(prepared.PreparerDependencies{Library: lib2, Access: adapter})
	reopened := newPreparedRuntimeResolver(preparedRuntimeDependencies{Channels: adapter, Timeline: adapter, Sources: adapter, Lookup: preparer, Readiness: readiness,
		Now: time.Now, Policy: func() string { return "certification-" + string(config.QualityTier) }, Rendition: config.rendition})
	if _, ready, err := reopened.ResolvePrepared(ctx, playout.TuneRequest{ChannelID: config.Channels[0].ID}, at); err != nil || !ready {
		t.Fatalf("restart lost durable readiness: ready=%t err=%v", ready, err)
	}
	if err := os.WriteFile(paths[0], []byte("changed source revision"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := reopened.Plan(ctx, at, at.Add(time.Minute))
	if err != nil || plan.Summary.ReadyChannels == 100 || plan.Summary.MissingBindings == 0 {
		t.Fatalf("changed source stayed ready: err=%v summary=%+v", err, plan.Summary)
	}
	// The same lifecycle owner cancels and joins the scheduler even if its
	// first periodic tick has not happened.
	target.startPreparation(context.Background())
	if _, err := target.stop(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-target.preparationDone:
	default:
		t.Fatal("preparation scheduler survived shutdown")
	}
}
