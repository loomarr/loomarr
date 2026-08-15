package api_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/testkit"
)

func TestLiveTVReconnectUsesSerializedTransitionWithoutTunarr(t *testing.T) {
	st := openTestStore(t, t.TempDir()+"/livetv-reconnect.db")
	t.Cleanup(func() { _ = st.Close() })
	transition := &testkit.BackendTransition{TunersReset: 1}
	h := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store: st, Auth: api.NewTokenAuthorizer(adminToken), BackendTransition: transition,
		LiveConfig: func(string) string { return "" },
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	resp := do(t, srv, http.MethodPost, "/v1/setup/livetv-reconnect", adminToken, "")
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /v1/setup/livetv-reconnect = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var body struct {
		TunersReset int `json:"tunersReset"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.TunersReset != 1 {
		t.Fatalf("tunersReset = %d, want 1", body.TunersReset)
	}
	if got := transition.Reconnects(); got != 1 {
		t.Fatalf("serialized reconnect calls = %d, want 1", got)
	}
}
