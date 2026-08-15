package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

func serverWithPanels(t *testing.T, opts api.Options) (*httptest.Server, store.Store) {
	t.Helper()
	st := openTestStore(t, t.TempDir()+"/p.db")
	t.Cleanup(func() { _ = st.Close() })
	opts.Store = st
	opts.Auth = api.NewTokenAuthorizer(adminToken)
	opts.Log = slog.New(slog.DiscardHandler)
	srv := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), opts))
	t.Cleanup(srv.Close)
	return srv, st
}

// §19 negative: these operational projections expose machine state (what is broken, what the
// install has been doing), which §11 keeps to admins.
func TestDashboardPanels_RequireAdmin(t *testing.T) {
	srv, _ := serverWithPanels(t, api.Options{})
	for _, path := range []string{"/v1/dashboard/summary", "/v1/system/services", "/v1/activity", "/v1/playout/status"} {
		resp := do(t, srv, http.MethodGet, path, "", "") // no token
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("GET %s without admin → %d, want 401", path, resp.StatusCode)
		}
	}
}

func TestDashboardSummaryCountsOperationalStates(t *testing.T) {
	srv, st := serverWithPanels(t, api.Options{})
	ctx := context.Background()

	for i, status := range []schedule.ChannelStatus{
		schedule.StatusLive, schedule.StatusBuilding, schedule.StatusPaused, schedule.StatusDetached,
	} {
		ch := store.Channel{}
		ch.ID = fmt.Sprintf("ch-%d", i)
		ch.Name = ch.ID
		ch.Number = i + 1
		ch.Strategy = schedule.Sequential
		ch.Status = status
		if _, err := st.SaveChannel(ctx, ch); err != nil {
			t.Fatal(err)
		}
	}
	for i, state := range []provision.State{
		provision.Wanted, provision.Requested, provision.Downloading,
		provision.Available, provision.Unavailable, provision.Unavailable,
	} {
		rec := provision.Record{Key: provision.Key(fmt.Sprintf("movie:tmdb:%d", i+1)), State: state}
		if err := st.UpsertTitle(ctx, rec); err != nil {
			t.Fatal(err)
		}
	}
	for i, status := range []string{"submitted", "submitted", "approved"} {
		p := store.Proposal{ID: fmt.Sprintf("p-%d", i), Status: status, ProposalJSON: `{}`,
			CreatedAt: time.Now(), UpdatedAt: time.Now()}
		if err := st.CreateProposal(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	resp := do(t, srv, http.MethodGet, "/v1/dashboard/summary", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("summary → %d, want 200", resp.StatusCode)
	}
	var body struct {
		OnAir, Channels, NeedsApproval, Acquiring, Unavailable int
		GeneratedAt                                            int64 `json:"generatedAt"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.OnAir != 1 || body.Channels != 3 || body.NeedsApproval != 2 || body.Acquiring != 3 || body.Unavailable != 2 {
		t.Errorf("summary = %+v", body)
	}
	if body.GeneratedAt <= 0 {
		t.Error("generatedAt was not populated")
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

func TestSystemServices_OmitsUnconfiguredOptionalIntegrations(t *testing.T) {
	srv, _ := serverWithPanels(t, api.Options{
		Settings:   &fakeSettings{},
		LiveConfig: func(string) string { return "" },
	})

	resp := do(t, srv, http.MethodGet, "/v1/system/services", adminToken, "")
	var view api.ServicesView
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if len(view.Rows) != 0 {
		t.Errorf("unconfigured services rendered as incidents: %+v", view.Rows)
	}
}

func TestSystemServices_ReportsConfiguredFixedTargetIntegration(t *testing.T) {
	srv, _ := serverWithPanels(t, api.Options{
		Settings: &fakeSettings{},
		LiveConfig: func(key string) string {
			if key == "tmdb.api_key" {
				return "configured-secret"
			}
			return ""
		},
	})

	resp := do(t, srv, http.MethodGet, "/v1/system/services", adminToken, "")
	var view api.ServicesView
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if len(view.Rows) != 1 || view.Rows[0].Name != "tmdb" || view.Rows[0].OK {
		t.Errorf("configured TMDB failure was not reported: %+v", view.Rows)
	}
}

// ⚠ A failing row must say WHERE to go. A red dot with no destination is a puzzle, not a
// diagnosis — which is what V31's "Fix →" clause is for.
func TestSystemServices_FailingRowRoutesToItsSettings(t *testing.T) {
	// Reuses settings_test.go's fakeSettings, which already scripts media_server as passing
	// and everything else as failing — a second double would be one more thing to keep in
	// agreement with the real interface.
	srv, _ := serverWithPanels(t, api.Options{
		Settings: &fakeSettings{},
		LiveConfig: func(key string) string {
			return map[string]string{"seerr.url": "http://operator:embedded-secret@seerr.lan:5055/base?api_key=query-secret#fragment"}[key]
		},
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
	if failing.Target != "http://seerr.lan:5055/base" {
		t.Errorf("target = %q, want useful address without embedded credentials", failing.Target)
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
