package ingest

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/testkit"
)

const secret = "hook-secret"

var fixedNow = time.Date(2026, 7, 13, 20, 0, 0, 0, time.UTC)

func newHandler(t *testing.T) (*Handler, store.Store, *testkit.MediaServer) {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/t.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ms := testkit.NewMediaServer(t)
	t.Cleanup(ms.Close)
	lib := library.New(library.Emby, ms.URL, ms.AdminToken, "dev")
	h := New(st, lib, secret, 12*time.Hour, func() time.Time { return fixedNow }, slog.New(slog.DiscardHandler))
	return h, st, ms
}

func post(t *testing.T, h *Handler, token string, body []byte) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/hooks/arr?token="+token, strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result()
}

// Bad secret → 401.
func TestBadSecret(t *testing.T) {
	h, _, _ := newHandler(t)
	resp := post(t, h, "wrong", testkit.Fixture(t, "radarr/test_webhook.json"))
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("bad secret → %d, want 401", resp.StatusCode)
	}
}

// Test webhook → 200 + records a per-app last-received timestamp (§6, §13).
func TestTestWebhookRecordsTimestamp(t *testing.T) {
	h, st, _ := newHandler(t)
	resp := post(t, h, secret, testkit.Fixture(t, "radarr/test_webhook.json"))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Test → %d, want 200", resp.StatusCode)
	}
	got, err := st.GetSetting(context.Background(), "webhook_last_received:Radarr")
	if err != nil {
		t.Fatalf("Test should record a timestamp for the app: %v", err)
	}
	if got == "" {
		t.Error("empty timestamp recorded")
	}
}

// Grab webhook for a tracked title → downloading, deadline reset to now+12h.
func TestGrabMovesToDownloading(t *testing.T) {
	h, st, _ := newHandler(t)
	ctx := context.Background()
	// The tracked record: matches the Radarr grab fixture's remoteMovie.tmdbId.
	key := provision.Key("movie:tmdb:1111867")
	must(t, st.UpsertTitle(ctx, provision.Record{
		Key: key, State: provision.Requested,
		Title:    provision.Title{MediaType: provision.Movie, TMDBID: 1111867},
		Deadline: fixedNow.Add(48 * time.Hour),
	}))

	resp := post(t, h, secret, testkit.Fixture(t, "radarr/grab_webhook.json"))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Grab → %d, want 200", resp.StatusCode)
	}
	rec, _ := st.GetTitle(ctx, key)
	if rec.State != provision.Downloading {
		t.Errorf("after Grab: %s, want downloading", rec.State)
	}
	if !rec.Deadline.Equal(fixedNow.Add(12 * time.Hour)) {
		t.Errorf("Grab should reset deadline to now+12h (§6/§4), got %v", rec.Deadline)
	}
}

// Import ("Download") webhook → available ONLY after library confirms (inv. 4).
func TestImportConfirmsViaLibraryThenAvailable(t *testing.T) {
	h, st, ms := newHandler(t)
	ctx := context.Background()
	key := provision.Key("movie:tmdb:1111867")
	must(t, st.UpsertTitle(ctx, provision.Record{
		Key: key, State: provision.Downloading,
		Title:    provision.Title{MediaType: provision.Movie, TMDBID: 1111867},
		Deadline: fixedNow.Add(12 * time.Hour),
	}))

	// The mock library only confirms tmdb.16153 as present; 1111867 is absent →
	// import must NOT flip to available (scan-lag / not-yet-visible case).
	resp := post(t, h, secret, testkit.Fixture(t, "radarr/import_webhook.json"))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Import → %d, want 200", resp.StatusCode)
	}
	rec, _ := st.GetTitle(ctx, key)
	if rec.State == provision.Available {
		t.Fatal("import marked available without library confirmation (invariant 4 violated)")
	}

	// Now make the library confirm this title, re-send: → available.
	ms.PresentTMDB = "1111867"
	resp = post(t, h, secret, testkit.Fixture(t, "radarr/import_webhook.json"))
	if resp.StatusCode != http.StatusOK {
		t.Fatal(resp.StatusCode)
	}
	rec, _ = st.GetTitle(ctx, key)
	if rec.State != provision.Available {
		t.Errorf("import with library confirming → %s, want available", rec.State)
	}
	if rec.LibraryID == "" {
		t.Error("available record should carry the library item id")
	}
}

// A webhook for a title we never tracked → 200, no row created (§6).
func TestUntrackedTitleIgnored(t *testing.T) {
	h, st, _ := newHandler(t)
	resp := post(t, h, secret, testkit.Fixture(t, "radarr/grab_webhook.json"))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("untracked Grab → %d, want 200", resp.StatusCode)
	}
	if _, err := st.GetTitle(context.Background(), "movie:tmdb:1111867"); err != store.ErrNotFound {
		t.Error("untracked webhook created a row; must be ignored")
	}
}

// Malformed JSON → 400.
func TestMalformedBody(t *testing.T) {
	h, _, _ := newHandler(t)
	resp := post(t, h, secret, []byte("{not json"))
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("malformed body → %d, want 400", resp.StatusCode)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
