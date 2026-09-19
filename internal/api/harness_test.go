package api_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/fillerdecision"
	"github.com/loomarr/loomarr/internal/store"
)

// apiHarness owns the invariant API-test lifecycle: an isolated migrated store,
// default admin/member authorization, the production router, and HTTP cleanup.
// Route-family harnesses add only their own behavioral dependencies around it.
type apiHarness struct {
	t      *testing.T
	Server *httptest.Server
	Store  store.Store
}

func newAPIHarness(t *testing.T) *apiHarness {
	t.Helper()
	st := openTestStore(t, filepath.Join(t.TempDir(), "api.db"))
	t.Cleanup(func() { _ = st.Close() })

	decisions, err := fillerdecision.New(st)
	if err != nil {
		t.Fatal(err)
	}
	decisions.WithDiagnosticRecovery(&apiDiagnosticRecovery{retrying: make(map[string]time.Time)})
	log := slog.New(slog.DiscardHandler)
	server := httptest.NewServer(api.Router(log, api.Options{
		Store:           st,
		Auth:            testAuthorizer{},
		Log:             log,
		BackupSQLite:    store.SQLiteBackuper(st),
		FillerDecisions: decisions,
	}))
	t.Cleanup(server.Close)
	return &apiHarness{t: t, Server: server, Store: st}
}

func (h *apiHarness) Do(method, path, token, body string) *http.Response {
	h.t.Helper()
	return do(h.t, h.Server, method, path, token, body)
}

func TestAPIHarnessOwnsDefaultsIsolationAndCleanup(t *testing.T) {
	var first *apiHarness
	t.Run("first", func(t *testing.T) {
		first = newAPIHarness(t)
		if got := first.Do(http.MethodPost, "/v1/titles", memberToken,
			`{"mediaType":"movie","tmdbId":41}`).StatusCode; got != http.StatusForbidden {
			t.Fatalf("member mutation = %d, want 403", got)
		}
		if got := first.Do(http.MethodPost, "/v1/titles", adminToken,
			`{"mediaType":"movie","tmdbId":42}`).StatusCode; got != http.StatusOK {
			t.Fatalf("admin mutation = %d, want 200", got)
		}
	})

	if _, err := first.Server.Client().Get(first.Server.URL + "/v1/titles/movie:tmdb:42"); err == nil {
		t.Fatal("server remained reachable after the owning test completed")
	}
	if _, err := first.Store.GetTitle(context.Background(), "movie:tmdb:42"); err == nil {
		t.Fatal("store remained open after the owning test completed")
	}

	second := newAPIHarness(t)
	if got := second.Do(http.MethodGet, "/v1/titles/movie:tmdb:42", adminToken, "").StatusCode; got != http.StatusNotFound {
		t.Fatalf("second harness observed first harness state: status %d, want 404", got)
	}
}
