package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/store"
)

// fakeJobs is a scriptable JobService for the API-layer tests.
type fakeJobs struct {
	list      []api.JobView
	triggered []string
	unknown   string // a name that Trigger reports as not-found
}

func (f *fakeJobs) List(context.Context) ([]api.JobView, error) { return f.list, nil }
func (f *fakeJobs) Trigger(_ context.Context, name string) error {
	if name == f.unknown {
		return api.ErrJobNotFound
	}
	f.triggered = append(f.triggered, name)
	return nil
}

func serverWithJobs(t *testing.T, svc api.JobService) *httptest.Server {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/api.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store: st,
		Auth:  api.NewTokenAuthorizer(adminToken),
		Log:   slog.New(slog.DiscardHandler),
		Jobs:  svc,
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

// The jobs routes are admin-only (§18.1: they expose acquisition internals + can trigger work).
func TestJobs_RequiresAdmin(t *testing.T) {
	srv := serverWithJobs(t, &fakeJobs{})
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/v1/jobs"},
		{http.MethodPost, "/v1/jobs/reconcile/run"},
	} {
		resp := do(t, srv, tc.method, tc.path, "", "") // no token
		if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s without admin → %d, want 401/403", tc.method, tc.path, resp.StatusCode)
		}
		_ = resp.Body.Close()
	}
}

func TestJobs_ListReturnsJobs(t *testing.T) {
	svc := &fakeJobs{list: []api.JobView{
		{Name: "reconcile", Title: "Reconcile downloads", Schedule: "0 */5 * * * *", ScheduleKey: "job.reconcile.schedule", LastResult: "ok"},
		{Name: "channel-sweep", Title: "Reconcile channels with Tunarr", Schedule: "0 */10 * * * *"},
	}}
	srv := serverWithJobs(t, svc)

	resp := do(t, srv, http.MethodGet, "/v1/jobs", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list → %d", resp.StatusCode)
	}
	var body struct {
		Jobs []api.JobView `json:"jobs"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if len(body.Jobs) != 2 || body.Jobs[0].Name != "reconcile" || body.Jobs[0].Schedule != "0 */5 * * * *" {
		t.Errorf("list body = %+v, want 2 jobs led by reconcile@every-5-min", body.Jobs)
	}
}

func TestJobs_RunTriggers(t *testing.T) {
	svc := &fakeJobs{unknown: "ghost"}
	srv := serverWithJobs(t, svc)

	resp := do(t, srv, http.MethodPost, "/v1/jobs/reconcile/run", adminToken, "")
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("run → %d, want 202", resp.StatusCode)
	}
	if len(svc.triggered) != 1 || svc.triggered[0] != "reconcile" {
		t.Errorf("triggered = %v, want [reconcile]", svc.triggered)
	}

	// An unknown job name → 404.
	resp = do(t, srv, http.MethodPost, "/v1/jobs/ghost/run", adminToken, "")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("run unknown → %d, want 404", resp.StatusCode)
	}
}

// With no scheduler wired (nil service), the routes report unavailable rather than 500.
func TestJobs_UnavailableWhenNoScheduler(t *testing.T) {
	srv := serverWithJobs(t, nil)
	resp := do(t, srv, http.MethodGet, "/v1/jobs", adminToken, "")
	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("list with no scheduler → %d, want 501", resp.StatusCode)
	}
}
