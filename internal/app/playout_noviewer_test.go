package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

// #1512 G1: no encoding ahead of a viewer. With live channels configured and the whole application
// running (its jobs registered, its boot work done), no FFmpeg process starts; the first viewer's
// tune is what starts one. The second half keeps the first honest: the stub is on the real tune
// path, so its absence before the tune means nothing ran, not that the stub was unreachable.
func TestNoFFmpegRunsUntilAViewerTunes(t *testing.T) {
	t.Setenv("API_TOKEN", "g1-token")
	t.Setenv("PLAYOUT_BACKEND", schedule.PlayoutBackendInternal)

	dir := t.TempDir()
	ffmpeg := filepath.Join(dir, "ffmpeg")
	marker := ffmpeg + ".invoked"
	stub := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$0.invoked\"\nexit 99\n"
	if err := os.WriteFile(ffmpeg, []byte(stub), 0o755); err != nil { //nolint:gosec // executable test double
		t.Fatal(err)
	}
	t.Setenv("PLAYOUT_FFMPEG_PATH", ffmpeg)

	st := testkit.MigratedSQLiteStore(t)
	for i := range 8 {
		if _, err := st.SaveChannel(t.Context(), store.Channel{Channel: schedule.Channel{
			ID: fmt.Sprintf("g1-%d", i), Name: fmt.Sprintf("G1 %d", i), Number: i + 1,
			Strategy: schedule.Sequential, Status: schedule.StatusLive,
		}}); err != nil {
			t.Fatal(err)
		}
	}
	application, err := Build(t.Context(), st, slog.New(slog.DiscardHandler), Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })

	// Boot work (codec backfill, capability reuse) runs in the background; give it time to finish.
	time.Sleep(2 * time.Second)
	if b, err := os.ReadFile(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("FFmpeg ran with no viewer: %q (err %v)", b, err)
	}

	mint := httptest.NewRequest(http.MethodPost, "/v1/channels/g1-0/play-url", nil)
	mint.Header.Set("Authorization", "Bearer g1-token")
	mint.Header.Set("X-Loomarr-Csrf", "1")
	minted := httptest.NewRecorder()
	application.Handler().ServeHTTP(minted, mint)
	var play struct {
		RelativeURL string `json:"relativeUrl"`
	}
	if err := json.NewDecoder(minted.Body).Decode(&play); err != nil || play.RelativeURL == "" {
		t.Fatalf("play-url = %d %v", minted.Code, err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	tune := httptest.NewRequestWithContext(ctx, http.MethodGet, play.RelativeURL, nil)
	application.Handler().ServeHTTP(httptest.NewRecorder(), tune)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("a viewer tuned but no FFmpeg started: the stub is not on the tune path, so the idle half proves nothing")
}
