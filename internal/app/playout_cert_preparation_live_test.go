//go:build ffmpeg

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
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
	encoder := playout.EncoderVAAPI // A fixed test pool; the default packager remains software.
	packager := prepared.NewFFmpegPackager(ffmpeg)
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
		sourceProfile := syntheticSourceProfile{keyframeInterval: 25, audioChannels: 2, duration: 6 * time.Second}
		if err := generateSyntheticSource(ctx, ffmpeg, paths[variant], variant, sourceProfile); err != nil {
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
	target := &PlayoutCertificationTarget{store: st, encoder: encoder,
		programmeEvidence: &syntheticProgrammeEvidence{assets: make(map[string][2]map[string]syntheticAssetTruth)}}
	epoch := time.Now().UTC().Add(-time.Second)
	runtime, clock, err := target.prepareDeclared(ctx, config, sources, lib, packager, syntheticProgrammeSchedule{epoch: epoch, duration: config.ProgrammeDuration}, indexes)
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
	// Sweep every position across successive programme boundaries through the
	// same prepared-origin operation used by the public prepared-only route.
	// Readiness must not advertise a publication that cannot serve a tune.
	pinned := epoch
	concrete, ok := runtime.(*preparedRuntimeResolver)
	if !ok {
		t.Fatal("declared target did not compose the production readiness resolver")
	}
	concrete.now = func() time.Time { return pinned }
	origin := playout.NewPreparedOrigin(lib, runtime)
	misses := 0
	var firstMiss time.Duration
	for offset := time.Duration(0); offset < 12*time.Second; offset += 10 * time.Millisecond {
		pinned = epoch.Add(offset)
		presentation, hit, err := origin.Tune(ctx, playout.TuneRequest{ChannelID: config.Channels[0].ID, Plan: playout.PlanBaseline, Delivery: playout.DeliveryHLS, PreparedOnly: true})
		if presentation.Release != nil {
			presentation.Release()
		}
		if err != nil || !hit {
			if misses == 0 {
				firstMiss = offset
			}
			misses++
		}
	}
	if misses != 0 {
		t.Fatalf("ready programme has %d prepared-only tune misses; first at %s", misses, firstMiss)
	}
	concrete.now = time.Now
	assertDeclaredPreparedHTTPReady(t, st, config, sources, clock, origin)
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

func assertDeclaredPreparedHTTPReady(t *testing.T, st store.Store, config PlayoutCertificationConfig, sources certificationSources, clock syntheticProgrammeSchedule, preparedOrigin *playout.PreparedOrigin) {
	t.Helper()
	const admin = "certification-test-admin"
	player := playout.NewOrigin(playout.OriginDependencies{Prepared: preparedOrigin})
	t.Cleanup(player.Quiesce)
	handler := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store: st, Auth: api.NewTokenAuthorizer(admin), Playout: player, PlayoutSecret: func() string { return "certification-test-device" },
		PlayoutResolver: syntheticLiveResolver{channels: st, sources: sources.paths, schedule: clock},
		LiveConfig: func(key string) string {
			if key == "playout.backend" {
				return schedule.PlayoutBackendInternal
			}
			if key == "server.public_url" {
				return "http://127.0.0.1"
			}
			return ""
		},
	})
	var signed []string
	for _, ch := range config.Channels[:100] {
		request := httptest.NewRequest(http.MethodPost, "/v1/channels/"+ch.ID+"/play-url", bytes.NewBufferString("{}"))
		request.Header.Set("Authorization", "Bearer "+admin)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		var body struct {
			RelativeURL string `json:"relativeUrl"`
		}
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil || body.RelativeURL == "" {
			t.Fatalf("play URL mint failed with HTTP %d", response.Code)
		}
		u, err := url.Parse(body.RelativeURL)
		if err != nil {
			t.Fatal("invalid test play URL")
		}
		q := u.Query()
		q.Set("mode", "prepared")
		u.RawQuery = q.Encode()
		signed = append(signed, u.String())
	}
	for pass := range 10 {
		codes := make([]int, len(signed))
		assetCodes := make([]int, len(signed))
		var workers sync.WaitGroup
		for worker := range 12 {
			workers.Go(func() {
				for index := worker; index < len(signed); index += 12 {
					response := httptest.NewRecorder()
					handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, signed[index], nil))
					codes[index] = response.Code
					if response.Code == http.StatusOK {
						base, _ := url.Parse(signed[index])
						for _, line := range strings.Split(response.Body.String(), "\n") {
							line = strings.TrimSpace(line)
							if line == "" || strings.HasPrefix(line, "#") {
								continue
							}
							ref, err := url.Parse(line)
							if err != nil {
								break
							}
							asset := httptest.NewRecorder()
							handler.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, base.ResolveReference(ref).String(), nil))
							assetCodes[index] = asset.Code
							break
						}
					}
				}
			})
		}
		workers.Wait()
		misses := 0
		for _, code := range codes {
			if code != http.StatusOK {
				misses++
			}
		}
		if misses != 0 {
			t.Fatalf("prepared-only HTTP pass %d missed %d ready channels; statuses=%v", pass, misses, codes)
		}
		for _, code := range assetCodes {
			if code != http.StatusOK {
				t.Fatalf("prepared asset pass %d failed: statuses=%v", pass, assetCodes)
			}
		}
	}
}
