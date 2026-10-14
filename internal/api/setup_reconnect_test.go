package api_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestLiveTVReconnectUsesSerializedTransitionWithoutTunarr(t *testing.T) {
	st := openTestStore(t, t.TempDir()+"/livetv-reconnect.db")
	t.Cleanup(func() { _ = st.Close() })
	transition := &testkit.BackendTransition{TunersReset: 1}
	libraryConfigured := false
	h := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store: st, Auth: api.NewTokenAuthorizer(adminToken), BackendTransition: transition,
		LiveConfig:        func(string) string { return "" },
		LibraryConfigured: func() bool { return libraryConfigured },
	})

	assertReconnectStatus(t, h, http.StatusNotImplemented)
	if got := transition.Reconnects(); got != 0 {
		t.Fatalf("incomplete library made %d transition calls, want 0", got)
	}

	libraryConfigured = true
	recorder := assertReconnectStatus(t, h, http.StatusOK)
	var body struct {
		TunersReset int `json:"tunersReset"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.TunersReset != 1 {
		t.Fatalf("tunersReset = %d, want 1", body.TunersReset)
	}
	if got := transition.Reconnects(); got != 1 {
		t.Fatalf("serialized reconnect calls = %d, want 1", got)
	}

	libraryConfigured = false
	assertReconnectStatus(t, h, http.StatusNotImplemented)
	if got := transition.Reconnects(); got != 1 {
		t.Fatalf("cleared library advanced transition calls to %d, want 1", got)
	}
}

func assertReconnectStatus(t *testing.T, handler http.Handler, want int) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/v1/setup/livetv-reconnect", strings.NewReader(""))
	request.Header.Set("Authorization", "Bearer "+adminToken)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != want {
		t.Fatalf("POST /v1/setup/livetv-reconnect = %d, want %d", recorder.Code, want)
	}
	return recorder
}
