package api_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/playout"
)

// fakeRestart records that the loop was asked to rebuild. Counting rather than a bool so
// a test can prove a refused request never reached it.
type fakeRestart struct{ calls atomic.Int64 }

func (f *fakeRestart) Restart() { f.calls.Add(1) }

func serverWithRestart(t *testing.T, opts api.Options) *httptest.Server {
	t.Helper()
	st := openTestStore(t, t.TempDir()+"/r.db")
	t.Cleanup(func() { _ = st.Close() })
	opts.Store = st
	opts.Auth = api.NewTokenAuthorizer(adminToken)
	opts.Log = slog.New(slog.DiscardHandler)
	srv := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), opts))
	t.Cleanup(srv.Close)
	return srv
}

// §19 negative: restarting interrupts playback for every internally-streamed channel, so
// all three routes are admin-only.
func TestSystemRestart_RequiresAdmin(t *testing.T) {
	fake := &fakeRestart{}
	srv := serverWithRestart(t, api.Options{Restart: fake})

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/v1/system/restart"},
		{http.MethodPost, "/v1/system/restart"},
		{http.MethodPost, "/v1/system/reload"},
	} {
		resp := do(t, srv, tc.method, tc.path, "", "") // no token
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s without admin → %d, want 401", tc.method, tc.path, resp.StatusCode)
		}
	}
	// ⚠ The refusal must happen BEFORE the loop is asked to rebuild — a 403 that still
	// restarted the process would be a denial-of-service with an error message.
	if n := fake.calls.Load(); n != 0 {
		t.Errorf("a request with no admin token triggered %d restarts", n)
	}
}

// A handler built without a generation loop behind it (tests, the integration harness)
// reports 501 rather than offering a button that silently does nothing.
func TestSystemRestart_NotAvailable501(t *testing.T) {
	srv := serverWithRestart(t, api.Options{}) // no Restart wired

	resp := do(t, srv, http.MethodPost, "/v1/system/restart", adminToken, "")
	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("restart with no loop → %d, want 501", resp.StatusCode)
	}

	// The cost endpoint still answers — it says `available:false` so the UI can explain
	// the absence instead of rendering a control that cannot work.
	resp = do(t, srv, http.MethodGet, "/v1/system/restart", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("restart cost with no loop → %d, want 200", resp.StatusCode)
	}
	var cost api.RestartCost
	if err := json.NewDecoder(resp.Body).Decode(&cost); err != nil {
		t.Fatal(err)
	}
	if cost.Available {
		t.Error("available = true with no restart loop wired")
	}
}

func TestSystemRestart_TriggersTheLoop(t *testing.T) {
	fake := &fakeRestart{}
	srv := serverWithRestart(t, api.Options{Restart: fake})

	resp := do(t, srv, http.MethodPost, "/v1/system/restart", adminToken, "")
	// ⚠ 202, and the response arrives BEFORE the drain — a client that never gets a
	// reply cannot tell "restarting" from "crashed".
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("restart → %d, want 202", resp.StatusCode)
	}
	if n := fake.calls.Load(); n != 1 {
		t.Errorf("restart calls = %d, want 1", n)
	}
}

// ⚠ The confirm dialog must state consequences, not guess them. A restart is not free
// once Loomarr owns the encoder (§9.1): internally-played channels drop while
// Tunarr-backed ones keep playing, and only the server knows how many of each are live.
func TestSystemRestart_ReportsWhatItWouldCost(t *testing.T) {
	srv := serverWithRestart(t, api.Options{
		Restart:         &fakeRestart{},
		PlayoutObserver: &fakePlayoutSessions{stats: make([]playout.SessionStat, 3)},
		RestartDrift:    func() []string { return []string{"DATABASE_URL"} },
	})

	resp := do(t, srv, http.MethodGet, "/v1/system/restart", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("restart cost → %d, want 200", resp.StatusCode)
	}
	var cost api.RestartCost
	if err := json.NewDecoder(resp.Body).Decode(&cost); err != nil {
		t.Fatal(err)
	}
	if cost.StreamingChannels != 3 {
		t.Errorf("streamingChannels = %d, want 3 — the dialog says how many drop", cost.StreamingChannels)
	}
	if !cost.RestartRequired || len(cost.PendingKeys) != 1 || cost.PendingKeys[0] != "DATABASE_URL" {
		t.Errorf("restartRequired=%v pendingKeys=%v, want the changed key NAMED",
			cost.RestartRequired, cost.PendingKeys)
	}
	if !cost.Available {
		t.Error("available = false although a restart loop is wired")
	}
}

// A Tunarr-backed install streams nothing itself, so zero is the correct answer — and
// restartRequired stays false when no restart-scoped setting changed.
func TestSystemRestart_CostIsZeroWhenNothingIsStreaming(t *testing.T) {
	srv := serverWithRestart(t, api.Options{Restart: &fakeRestart{}})

	resp := do(t, srv, http.MethodGet, "/v1/system/restart", adminToken, "")
	var cost api.RestartCost
	if err := json.NewDecoder(resp.Body).Decode(&cost); err != nil {
		t.Fatal(err)
	}
	if cost.StreamingChannels != 0 {
		t.Errorf("streamingChannels = %d, want 0", cost.StreamingChannels)
	}
	if cost.RestartRequired {
		t.Error("restartRequired = true with no drift — this would nag about a restart nothing needs")
	}
}

// Reload re-probes without tearing anything down, and returns the SAME check shape the
// wizard renders — one probe implementation, so the two can never disagree.
func TestSystemReload_ReturnsChecksWithoutRestarting(t *testing.T) {
	fake := &fakeRestart{}
	srv := serverWithRestart(t, api.Options{Restart: fake})

	resp := do(t, srv, http.MethodPost, "/v1/system/reload", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reload → %d, want 200", resp.StatusCode)
	}
	var body struct {
		Checks []api.SetupCheck `json:"checks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	// An unwired build has no services to probe — an EMPTY list, never null, so a client
	// can map over it without special-casing.
	if body.Checks == nil {
		t.Error("checks = null, want an empty array")
	}
	// ⚠ Reload must not restart. It is the no-downtime option, and conflating the two
	// would drop every stream on what the operator was told was a safe action.
	if n := fake.calls.Load(); n != 0 {
		t.Errorf("reload triggered %d restarts, want 0", n)
	}
}
