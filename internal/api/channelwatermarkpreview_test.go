package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
)

// fakeWatermarks records what the routes asked for and answers from fixed values.
type fakeWatermarks struct {
	png              string
	gotChange        schedule.WatermarkPolicy
	gotHoliday, gotC string
	gotNow           time.Time
}

func (f *fakeWatermarks) Preview(_ context.Context, id string, change schedule.WatermarkPolicy, holiday string) (string, bool, error) {
	if id != "ch-1" {
		return "", false, store.ErrNotFound
	}
	f.gotC, f.gotChange, f.gotHoliday = id, change, holiday
	return f.png, false, nil
}

func (f *fakeWatermarks) Options(_ context.Context, id string, now time.Time) (api.WatermarkOptions, error) {
	if id != "ch-1" {
		return api.WatermarkOptions{}, store.ErrNotFound
	}
	f.gotNow = now
	return api.WatermarkOptions{AutoCallsign: "SC", Era: "1980s", EraPalette: []string{"red", "amber", "cyan"},
		Holidays: []api.WatermarkHolidayOption{{ID: "halloween", Name: "Halloween", Start: "2026-10-01", End: "2026-10-31",
			Active: true, Picture: "pumpkin", Pictures: []api.WatermarkPicture{{ID: "pumpkin", Name: "Pumpkin"}}}}}, nil
}

func newWatermarkServer(t *testing.T, wm api.WatermarkPreviewer, now time.Time) *httptest.Server {
	t.Helper()
	opts := api.Options{Auth: testAuthorizer{}, Log: slog.New(slog.DiscardHandler), Now: func() time.Time { return now }}
	if wm != nil {
		opts.WatermarkPreview = wm
	}
	srv := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), opts))
	t.Cleanup(srv.Close)
	return srv
}

func getAs(t *testing.T, srv *httptest.Server, token, path string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

// The preview route passes the query's look, colour, size and holiday to the renderer and returns
// its PNG with the drawable header; it is admin-only and 404s an unknown channel.
func TestWatermarkPreviewRoute(t *testing.T) {
	png := filepath.Join(t.TempDir(), "bug.png")
	if err := os.WriteFile(png, []byte("\x89PNG fake"), 0o600); err != nil {
		t.Fatal(err)
	}
	wm := &fakeWatermarks{png: png}
	srv := newWatermarkServer(t, wm, time.Now())

	res := getAs(t, srv, adminToken, "/v1/channels/ch-1/watermark/preview?look=tint&color=pink&size=0.09&holiday=halloween")
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "image/png" || string(body) != "\x89PNG fake" {
		t.Fatalf("preview = %d %q %q", res.StatusCode, res.Header.Get("Content-Type"), body)
	}
	if res.Header.Get("X-Loomarr-Watermark-Drawable") != "false" {
		t.Fatalf("drawable header = %q, want false", res.Header.Get("X-Loomarr-Watermark-Drawable"))
	}
	if wm.gotChange.Look != "tint" || wm.gotChange.Color != "pink" || wm.gotChange.Size == nil || *wm.gotChange.Size != 0.09 || wm.gotHoliday != "halloween" {
		t.Fatalf("renderer asked for %+v holiday %q", wm.gotChange, wm.gotHoliday)
	}
	if res := getAs(t, srv, memberToken, "/v1/channels/ch-1/watermark/preview"); res.StatusCode != http.StatusForbidden {
		t.Fatalf("member preview = %d, want 403", res.StatusCode)
	}
	if res := getAs(t, srv, adminToken, "/v1/channels/nope/watermark/preview"); res.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown channel preview = %d, want 404", res.StatusCode)
	}
}

// The options route serves the renderer's options at the request clock; admin-only, 404 for an
// unknown channel, 501 without a renderer.
func TestWatermarkOptionsRoute(t *testing.T) {
	now := time.Date(2026, time.October, 13, 20, 0, 0, 0, time.UTC)
	wm := &fakeWatermarks{}
	srv := newWatermarkServer(t, wm, now)

	res := getAs(t, srv, adminToken, "/v1/channels/ch-1/watermark/options")
	var got api.WatermarkOptions
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("options = %d %v", res.StatusCode, err)
	}
	if !wm.gotNow.Equal(now) || got.AutoCallsign != "SC" || got.Era != "1980s" || len(got.Holidays) != 1 || !got.Holidays[0].Active {
		t.Fatalf("options = %+v at %v", got, wm.gotNow)
	}
	if res := getAs(t, srv, memberToken, "/v1/channels/ch-1/watermark/options"); res.StatusCode != http.StatusForbidden {
		t.Fatalf("member options = %d, want 403", res.StatusCode)
	}
	if res := getAs(t, srv, adminToken, "/v1/channels/nope/watermark/options"); res.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown channel options = %d, want 404", res.StatusCode)
	}
	if res := getAs(t, newWatermarkServer(t, nil, now), adminToken, "/v1/channels/ch-1/watermark/options"); res.StatusCode != http.StatusNotImplemented {
		t.Fatalf("no renderer = %d, want 501", res.StatusCode)
	}
}
