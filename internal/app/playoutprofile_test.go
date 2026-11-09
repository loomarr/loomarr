package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

type staticChannelReader struct{ channel store.Channel }

func (s staticChannelReader) GetChannel(context.Context, string) (store.Channel, error) {
	return s.channel, nil
}

func TestBuild_WiresMeasuredCapacityToAdmissionAndQuality(t *testing.T) {
	t.Setenv("API_TOKEN", "capacity-test-token")

	st := testkit.MigratedSQLiteStore(t)
	for key, value := range map[string]string{
		"playout.backend":      "internal",
		"playout.encoder":      "libx264",
		"playout.max_channels": "9",
		// The channel packager (the admission point) needs its scratch root, as on an install.
		"playout.hls_dir": t.TempDir(),
	} {
		if err := st.SetSetting(context.Background(), key, value); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	application, err := Build(ctx, st, slog.New(slog.DiscardHandler), Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })
	h := application.Handler()
	r := application.playoutResolver
	if r == nil {
		t.Fatal("Build wired no playout resolver")
	}
	// Detection is lazy; install the result a real encoder trial would publish without running
	// ffmpeg in a unit test. The configured 9 is deliberately above the measured 3.
	r.SetMaxChannels(3)

	req := httptest.NewRequest(http.MethodGet, "/v1/playout/sessions", nil)
	req.Header.Set("Authorization", "Bearer capacity-test-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/playout/sessions = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var telemetry api.PlayoutTelemetry
	if err := json.NewDecoder(rec.Body).Decode(&telemetry); err != nil {
		t.Fatal(err)
	}
	if telemetry.Capacity != 3 {
		t.Errorf("admission capacity = %d, want measured capacity 3", telemetry.Capacity)
	}
}

// ⚠ **THE ASSERTION THAT WAS MISSING.** The resolver's own test pins the invariant (an unset ladder
// input panics); this pins that Build actually satisfies it.
//
// Both are needed, and the gap between them is exactly where the original defect lived:
// `activeChannels` was back-patched after construction, and deleting the assignment left
// every test green while a viewer tuning in would panic.
//
// Reads the resolver Build really constructed rather than one assembled here, since
// a test-built resolver only proves the test knows how to fill a struct.
func TestBuild_WiresEveryLadderInput(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	// Playout is only wired on the internal backend; without this the resolver is nil and
	// the test would pass vacuously.
	if err := st.SetSetting(context.Background(), "playout.backend", "internal"); err != nil {
		t.Fatal(err)
	}

	application, err := Build(t.Context(), st, slog.New(slog.DiscardHandler), Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })
	r := application.playoutResolver
	if r == nil {
		t.Fatal("Build wired no playout resolver on the internal backend — " +
			"this test can no longer see what it is meant to guard")
	}

	for _, name := range r.MissingLadderInputs() {
		t.Errorf("Build left %s unset — Profile calls it unguarded, so a viewer "+
			"tuning in would panic", name)
	}
}
