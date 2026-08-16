package app

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/store"
)

func TestBuildHandler_FillerLayoutChangesOnlyAcrossGenerations(t *testing.T) {
	const token = "filler-layout-generation-token"
	t.Setenv("API_TOKEN", token)
	unsetForTest(t, "FILLER_DIR")
	unsetForTest(t, "FILLER_WATCH_DIR")

	root := t.TempDir()
	dsn := "sqlite://" + filepath.Join(root, "layout.db")
	appliedA := filepath.Join(root, "clips-a")
	watchA := filepath.Join(root, "watch-a")
	desiredB := filepath.Join(root, "clips-b")
	watchB := filepath.Join(root, "watch-b")

	st, err := store.Open(t.Context(), dsn, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetSetting(t.Context(), "filler.dir", appliedA); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSetting(t.Context(), "filler.watch_dir", watchA); err != nil {
		t.Fatal(err)
	}
	ctxA, cancelA := context.WithCancel(context.Background())
	hA, err := BuildHandler(ctxA, st, slog.New(slog.DiscardHandler), Overrides{Restart: func() {}})
	if err != nil {
		t.Fatal(err)
	}

	if got := fillerFolderTarget(t, hA, token); got != appliedA {
		t.Fatalf("generation A source target = %q, want %q", got, appliedA)
	}
	if lastPlayoutResolver == nil || lastPlayoutResolver.fillerDir != appliedA {
		t.Fatalf("generation A playout root = %v, want %q", lastPlayoutResolver, appliedA)
	}
	patchSettings(t, hA, token, map[string]string{
		"filler.dir": desiredB, "filler.watch_dir": watchB,
	})
	assertDesiredLayout(t, hA, token, desiredB, watchB)
	if got := fillerFolderTarget(t, hA, token); got != appliedA {
		t.Errorf("running generation source target = %q after save, want applied %q", got, appliedA)
	}
	if lastPlayoutResolver.fillerDir != appliedA {
		t.Errorf("running generation playout root = %q after save, want applied %q", lastPlayoutResolver.fillerDir, appliedA)
	}
	assertPendingLayout(t, restartCost(t, hA, token), true)

	// Drift is derived, not sticky: reverting both desired paths to the applied pair clears the
	// restart notice. Saving B again then restores it without changing any running consumer.
	patchSettings(t, hA, token, map[string]string{
		"filler.dir": appliedA, "filler.watch_dir": watchA,
	})
	assertPendingLayout(t, restartCost(t, hA, token), false)
	// Drift compares the canonical topology, not raw spelling. These values resolve to the
	// generation-A pair and therefore must not nag for a restart.
	patchSettings(t, hA, token, map[string]string{
		"filler.dir":       filepath.Join(root, "alias", "..", "clips-a"),
		"filler.watch_dir": watchA + string(filepath.Separator),
	})
	assertPendingLayout(t, restartCost(t, hA, token), false)
	patchSettings(t, hA, token, map[string]string{
		"filler.dir": desiredB, "filler.watch_dir": watchB,
	})
	assertPendingLayout(t, restartCost(t, hA, token), true)

	cancelA()
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st, err = store.Open(t.Context(), dsn, true)
	if err != nil {
		t.Fatal(err)
	}
	ctxB, cancelB := context.WithCancel(context.Background())
	t.Cleanup(cancelB)
	t.Cleanup(func() { _ = st.Close() })
	hB, err := BuildHandler(ctxB, st, slog.New(slog.DiscardHandler), Overrides{Restart: func() {}})
	if err != nil {
		t.Fatal(err)
	}
	if got := fillerFolderTarget(t, hB, token); got != desiredB {
		t.Errorf("generation B source target = %q, want newly applied %q", got, desiredB)
	}
	if lastPlayoutResolver == nil || lastPlayoutResolver.fillerDir != desiredB {
		t.Errorf("generation B playout root = %v, want newly applied %q", lastPlayoutResolver, desiredB)
	}
	if got := restartCost(t, hB, token); got.RestartRequired || len(got.PendingKeys) != 0 {
		t.Errorf("restart cost in generation B = %+v, want no pending layout", got)
	}
}

func assertPendingLayout(t *testing.T, restart api.RestartCost, want bool) {
	t.Helper()
	if !want {
		if restart.RestartRequired || len(restart.PendingKeys) != 0 {
			t.Errorf("restart cost = %+v, want no pending layout", restart)
		}
		return
	}
	wantKeys := []string{"filler.dir", "filler.watch_dir"}
	if !restart.RestartRequired || len(restart.PendingKeys) != len(wantKeys) {
		t.Fatalf("restart cost = %+v, want %v pending", restart, wantKeys)
	}
	for i, key := range wantKeys {
		if restart.PendingKeys[i] != key {
			t.Errorf("pendingKeys[%d] = %q, want %q", i, restart.PendingKeys[i], key)
		}
	}
}

func unsetForTest(t *testing.T, key string) {
	t.Helper()
	old, existed := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(key, old)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

func adminRequest(t *testing.T, h http.Handler, token, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func patchSettings(t *testing.T, h http.Handler, token string, edits map[string]string) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"edits": edits})
	if err != nil {
		t.Fatal(err)
	}
	rec := adminRequest(t, h, token, http.MethodPatch, "/v1/settings", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH /v1/settings = %d: %s", rec.Code, rec.Body.String())
	}
}

func fillerFolderTarget(t *testing.T, h http.Handler, token string) string {
	t.Helper()
	rec := adminRequest(t, h, token, http.MethodGet, "/v1/filler/sources", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/filler/sources = %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Sources []api.FillerSourceDTO `json:"sources"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	for _, source := range body.Sources {
		if source.Kind == "folder" {
			return source.Target
		}
	}
	t.Fatal("folder source missing")
	return ""
}

func restartCost(t *testing.T, h http.Handler, token string) api.RestartCost {
	t.Helper()
	rec := adminRequest(t, h, token, http.MethodGet, "/v1/system/restart", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/system/restart = %d: %s", rec.Code, rec.Body.String())
	}
	var cost api.RestartCost
	if err := json.NewDecoder(rec.Body).Decode(&cost); err != nil {
		t.Fatal(err)
	}
	return cost
}

func assertDesiredLayout(t *testing.T, h http.Handler, token, wantRoot, wantWatch string) {
	t.Helper()
	rec := adminRequest(t, h, token, http.MethodGet, "/v1/settings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/settings = %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Settings []api.SettingEntry `json:"settings"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"filler.dir": wantRoot, "filler.watch_dir": wantWatch}
	for _, entry := range body.Settings {
		if expected, ok := want[entry.Key]; ok {
			if entry.Apply != "restart" {
				t.Errorf("desired %s apply = %q, want restart", entry.Key, entry.Apply)
			}
			if entry.Value != expected {
				t.Errorf("desired %s = %q, want %q", entry.Key, entry.Value, expected)
			}
			delete(want, entry.Key)
		}
	}
	if len(want) != 0 {
		t.Errorf("desired layout settings missing: %v", want)
	}
}
