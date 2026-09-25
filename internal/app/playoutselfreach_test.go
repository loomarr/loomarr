package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/diagnostics"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/schedule"
)

// deadPublicURL refuses connections at once: the shape of a public URL whose IP the host no longer
// has, without the multi-second dial timeout a black-holed address would add to the test.
const deadPublicURL = "http://127.0.0.1:1"

// TestInternalPlayoutSessionDoesNotDependOnThePublicURL is the #1447 regression: the session's
// parent fetched its own programme blocks through server.public_url, so a stale address meant no
// segment ever arrived. With the listener known, the same session must still produce bytes.
func TestInternalPlayoutSessionDoesNotDependOnThePublicURL(t *testing.T) {
	const format = "h264-1280x720-25-2500-128"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(api.PlayoutBlockAudioHeader, api.PlayoutBlockAudioPCM)
		w.Header().Set(api.PlayoutBroadcastFormatHeader, format)
		w.Header().Set(api.PlayoutAiringStartedAtHeader, time.Unix(1, 0).UTC().Format(time.RFC3339Nano))
		w.Header().Set(api.PlayoutAiringKindHeader, string(schedule.SlotProgram))
		w.Header().Set(api.PlayoutAiringContentHeader, "episode")
		w.Header().Set(api.PlayoutScheduleBlockHeader, "block_episode")
		_, _ = io.WriteString(w, "SEGMENT-BYTES")
	}))
	t.Cleanup(srv.Close)

	// A stand-in encoder that copies the block stream to stdout, so the assertion is "the block
	// reached the encoder", which only happens if the parent could open it.
	fakeFFmpeg := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(fakeFFmpeg, []byte("#!/bin/sh\nexec cat\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	listen := strings.TrimPrefix(srv.URL, "http://")
	spawn := playoutSpawner(fakeFFmpeg,
		func() string { return internalProgramBase(listen, deadPublicURL) },
		func() string { return "token" }, nil, nil, nil,
		func(context.Context) int { return 128 }, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	proc, err := spawn(ctx, "channel", playout.PlanBaseline)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer proc.Stop()
	buf := make([]byte, len("SEGMENT-BYTES"))
	if _, err := io.ReadFull(proc.Stdout, buf); err != nil || string(buf) != "SEGMENT-BYTES" {
		t.Fatalf("no block reached the encoder with a dead public URL: %q, %v", buf, err)
	}
}

func TestInternalProgramBase(t *testing.T) {
	cases := []struct{ name, listen, public, want string }{
		{"wildcard listener dials loopback", ":8080", deadPublicURL, "http://127.0.0.1:8080"},
		{"explicit host kept", "10.0.0.5:9000", deadPublicURL, "http://10.0.0.5:9000"},
		{"no listener falls back to the public URL", "", " http://loomarr:8080 ", "http://loomarr:8080"},
		{"nothing known", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := internalProgramBase(tc.listen, tc.public); got != tc.want {
				t.Fatalf("internalProgramBase(%q, %q) = %q, want %q", tc.listen, tc.public, got, tc.want)
			}
		})
	}
}

// buildPlayout must hand the session spawner the LISTENER, not the public URL, and fall back to the
// public URL only when the build has no listener (embedded/test builds).
func TestBuildPlayoutProgramBaseUsesTheListenAddress(t *testing.T) {
	set := visionSet(t, map[string]string{"server.public_url": "http://public.example:8080"})

	withListener := playoutDeps{listenAddr: ":18080"}.programBase(set)()
	if withListener != "http://127.0.0.1:18080" {
		t.Fatalf("with a listener the parent dials %q, want loopback on the bound port", withListener)
	}
	if got := (playoutDeps{}).programBase(set)(); got != "http://public.example:8080" {
		t.Fatalf("with no listener the parent dials %q, want the public URL fallback", got)
	}
}

func TestPublicURLProbeNamesAnUnreachableAddress(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/healthz" {
			t.Errorf("probed %q, want the liveness route", r.URL.Path)
		}
	}))
	t.Cleanup(up.Close)
	client := &http.Client{Timeout: time.Second}

	if ok, _ := publicURLProbe(visionSet(t, map[string]string{"server.public_url": up.URL}), client)(context.Background()); !ok {
		t.Fatal("a reachable public address must pass")
	}
	if ok, _ := publicURLProbe(visionSet(t, map[string]string{"server.public_url": deadPublicURL}), client)(context.Background()); ok {
		t.Fatal("an unreachable public address must fail")
	}

	got := publicURLFailureDetail("http://user:secret@10.0.0.9:8080")
	if !strings.Contains(got, "http://10.0.0.9:8080") || !strings.Contains(got, "check SERVER_PUBLIC_URL") ||
		strings.Contains(got, "secret") {
		t.Fatalf("failure detail = %q", got)
	}
}

func TestCurrentHealthReportsAnUnreachablePublicURL(t *testing.T) {
	now := time.Unix(100, 0)
	health := diagnostics.NewStartup(now, 1, "dev", []diagnostics.StartupCheck{
		{Key: diagnostics.StartupCheckPublicURL, Mode: diagnostics.HealthCheckContinuous, FreshFor: time.Minute},
	}, func() time.Time { return now })
	set := visionSet(t, map[string]string{"server.public_url": deadPublicURL})
	runner := newCurrentHealthRunner(health, nil, set, map[string]func(context.Context) (bool, string){
		diagnostics.StartupCheckPublicURL: publicURLProbe(set, &http.Client{Timeout: time.Second}),
	})
	if err := runner.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, check := range health.Health().Checks {
		if check.Key != diagnostics.StartupCheckPublicURL {
			continue
		}
		found = true
		if check.Status != diagnostics.HealthFailed || !strings.Contains(check.Detail, deadPublicURL) ||
			!strings.Contains(check.Detail, "check SERVER_PUBLIC_URL") {
			t.Fatalf("public URL check = %+v", check)
		}
	}
	if !found {
		t.Fatal("public URL check missing from Current Health")
	}
}
