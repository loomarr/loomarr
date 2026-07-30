package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/store"
)

func serverWithPanels(t *testing.T, opts api.Options) (*httptest.Server, store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/p.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	opts.Store = st
	opts.Auth = api.NewTokenAuthorizer(adminToken)
	opts.Log = slog.New(slog.DiscardHandler)
	srv := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), opts))
	t.Cleanup(srv.Close)
	return srv, st
}

// §19 negative: both panels expose machine state (what is broken, what the install has been
// doing), which §11 keeps to admins.
func TestDashboardPanels_RequireAdmin(t *testing.T) {
	srv, _ := serverWithPanels(t, api.Options{})
	for _, path := range []string{"/v1/system/services", "/v1/activity"} {
		resp := do(t, srv, http.MethodGet, path, "", "") // no token
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("GET %s without admin → %d, want 401", path, resp.StatusCode)
		}
	}
}

// The panel always reports Loomarr's own row, even with nothing else configured — it is the
// one component that must be up for anything else to be reported at all.
func TestSystemServices_AlwaysReportsLoomarrItself(t *testing.T) {
	srv, _ := serverWithPanels(t, api.Options{})

	resp := do(t, srv, http.MethodGet, "/v1/system/services", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("services → %d, want 200", resp.StatusCode)
	}
	var view api.ServicesView
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if !view.Loomarr.OK {
		t.Error("loomarr row is not OK, but it answered the request")
	}
	// The row carries identity, not health: version, platform and the live schema.
	if !strings.Contains(view.Loomarr.Target, "sqlite") || !strings.Contains(view.Loomarr.Target, "schema ") {
		t.Errorf("loomarr target = %q, want backend + schema", view.Loomarr.Target)
	}
	// Rows is empty rather than null on an unconfigured install, so a client can map over it.
	if view.Rows == nil {
		t.Error("rows = null, want an empty array")
	}
}

// ⚠ A failing row must say WHERE to go. A red dot with no destination is a puzzle, not a
// diagnosis — which is what V31's "Fix →" clause is for.
func TestSystemServices_FailingRowRoutesToItsSettings(t *testing.T) {
	// Reuses settings_test.go's fakeSettings, which already scripts media_server as passing
	// and everything else as failing — a second double would be one more thing to keep in
	// agreement with the real interface.
	srv, _ := serverWithPanels(t, api.Options{
		Settings:   &fakeSettings{},
		LiveConfig: func(key string) string { return map[string]string{"seerr.url": "http://seerr.lan:5055"}[key] },
	})

	resp := do(t, srv, http.MethodGet, "/v1/system/services", adminToken, "")
	var view api.ServicesView
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}

	var failing *api.ServiceRow
	for i := range view.Rows {
		if view.Rows[i].Name == "requester" {
			failing = &view.Rows[i]
		}
	}
	if failing == nil {
		t.Fatalf("no requester row in %+v", view.Rows)
	}
	if failing.OK {
		t.Error("requester reports OK although the probe failed")
	}
	// The three things a failing row owes the operator: what broke, where it was pointed,
	// and where to fix it.
	if failing.Hint == "" {
		t.Error("no hint on a failing row")
	}
	if failing.Target != "http://seerr.lan:5055" {
		t.Errorf("target = %q, want the probed address", failing.Target)
	}
	if failing.SettingsGroup == "" {
		t.Error(`no settingsGroup — "Fix →" would have nowhere to send the operator`)
	}
}

// The feed reads the PERSISTED table, so it survives a restart — which is V32's gate. A feed
// tapping the in-memory SSE bus could satisfy neither half of that.
func TestListActivity_ReadsPersistedRowsNewestFirst(t *testing.T) {
	srv, st := serverWithPanels(t, api.Options{})
	ctx := context.Background()

	for i, text := range []string{"older entry", "newer entry"} {
		if err := st.RecordActivity(ctx, store.Activity{
			At: int64(1_700_000_000 + i*60), Kind: store.ActivityKindTitle,
			Level: store.ActivityInfo, Text: text,
		}); err != nil {
			t.Fatal(err)
		}
	}

	resp := do(t, srv, http.MethodGet, "/v1/activity", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("activity → %d, want 200", resp.StatusCode)
	}
	var body struct {
		Activity []store.Activity `json:"activity"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Activity) != 2 {
		t.Fatalf("got %d rows, want 2", len(body.Activity))
	}
	if body.Activity[0].Text != "newer entry" {
		t.Errorf("first row = %q, want the newest", body.Activity[0].Text)
	}
}

// A fresh install has no history. That is an empty array, never null — a client should not
// have to special-case its first run.
func TestListActivity_EmptyIsAnArray(t *testing.T) {
	srv, _ := serverWithPanels(t, api.Options{})

	resp := do(t, srv, http.MethodGet, "/v1/activity", adminToken, "")
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	if string(raw["activity"]) != "[]" {
		t.Errorf("activity = %s, want []", raw["activity"])
	}
}
