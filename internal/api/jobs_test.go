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
	paused    map[string]bool // name -> last requested pause state
	unknown   string          // a name that Trigger reports as not-found
	disabled  string          // a name that Trigger reports as disabled on this backend
}

func (f *fakeJobs) List(context.Context) ([]api.JobView, error) { return f.list, nil }

func (f *fakeJobs) SetPaused(_ context.Context, name string, paused bool) error {
	if name == f.unknown {
		return api.ErrJobNotFound
	}
	if name == f.disabled {
		return api.ErrJobDisabled
	}
	if f.paused == nil {
		f.paused = map[string]bool{}
	}
	f.paused[name] = paused
	return nil
}
func (f *fakeJobs) Trigger(_ context.Context, name string) error {
	if name == f.unknown {
		return api.ErrJobNotFound
	}
	if name == f.disabled {
		return api.ErrJobDisabled
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
		Auth:  testAuthorizer{},
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
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s without admin → %d, want 401", tc.method, tc.path, resp.StatusCode)
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

// ⚠ Run-now on a job this backend cannot run is refused BY THE SERVER with a 409. The
// Tasks page hides the button, but a disabled control is a hint — anything a client can be
// shown, a client can skip.
//
// 409 rather than 404 on purpose: the job EXISTS and is listed (that is the whole point of
// a disabled job), it just cannot run here. A 404 would send an admin hunting for a name
// that is on their screen.
func TestJobs_RunDisabledIsConflictNotFound(t *testing.T) {
	svc := &fakeJobs{unknown: "ghost", disabled: "backup"}
	srv := serverWithJobs(t, svc)

	resp := do(t, srv, http.MethodPost, "/v1/jobs/backup/run", adminToken, "")
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("run disabled → %d, want 409", resp.StatusCode)
	}
	if len(svc.triggered) != 0 {
		t.Errorf("a disabled job was triggered: %v", svc.triggered)
	}
}

// The read model carries the reason, so the UI can state why rather than rendering a row
// that looks broken.
func TestJobs_ListCarriesDisabledReason(t *testing.T) {
	const reason = "Loomarr does not back up PostgreSQL itself — use pg_dump on your usual schedule."
	srv := serverWithJobs(t, &fakeJobs{list: []api.JobView{
		{Name: "reconcile", Title: "Reconcile"},
		{Name: "backup", Title: "Back up the database", DisabledReason: reason},
	}})

	resp := do(t, srv, http.MethodGet, "/v1/jobs", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list → %d, want 200", resp.StatusCode)
	}
	var body struct {
		Jobs []api.JobView `json:"jobs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Jobs) != 2 {
		t.Fatalf("jobs = %+v, want the disabled one LISTED alongside the enabled one", body.Jobs)
	}
	if body.Jobs[1].DisabledReason != reason {
		t.Errorf("disabledReason = %q, want the reason", body.Jobs[1].DisabledReason)
	}
	if body.Jobs[0].DisabledReason != "" {
		t.Errorf("an enabled job carries disabledReason %q, want empty", body.Jobs[0].DisabledReason)
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

// Pausing reaches the scheduler with the requested state, and resuming reaches it with false —
// the endpoint takes a body rather than being two routes, so "which way" is a real assertion.
func TestJobs_PauseAndResume(t *testing.T) {
	svc := &fakeJobs{}
	srv := serverWithJobs(t, svc)

	resp := do(t, srv, http.MethodPost, "/v1/jobs/reconcile/pause", adminToken, `{"paused":true}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		t.Fatalf("pause → %d, want 2xx", resp.StatusCode)
	}
	if !svc.paused["reconcile"] {
		t.Error("pause did not reach the scheduler as true")
	}

	r2 := do(t, srv, http.MethodPost, "/v1/jobs/reconcile/pause", adminToken, `{"paused":false}`)
	defer func() { _ = r2.Body.Close() }()
	if svc.paused["reconcile"] {
		t.Error("resume did not reach the scheduler as false")
	}
}

// ⚠ §19: a member cannot pause a job. Pausing stops acquisitions and channel rebuilds from
// happening at all, so it is squarely an admin action.
func TestJobs_PauseIsAdminOnly(t *testing.T) {
	svc := &fakeJobs{}
	srv := serverWithJobs(t, svc)

	resp := do(t, srv, http.MethodPost, "/v1/jobs/reconcile/pause", memberToken, `{"paused":true}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("member pause → %d, want 403", resp.StatusCode)
	}
	if _, ok := svc.paused["reconcile"]; ok {
		t.Error("a member's pause reached the scheduler — the refusal must be server-side")
	}
}

// A job this backend cannot run at all is a 409, not a 404: it exists and is on the admin's
// screen. Pausing it is refused because that is an environment fact, not a preference.
func TestJobs_PauseDisabledIs409(t *testing.T) {
	srv := serverWithJobs(t, &fakeJobs{disabled: "backup"})

	resp := do(t, srv, http.MethodPost, "/v1/jobs/backup/pause", adminToken, `{"paused":true}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("pause of a disabled job → %d, want 409", resp.StatusCode)
	}
}

// An unregistered name is a 404 — distinct from the 409 above, so an admin is not sent hunting
// for a job that is on their screen.
func TestJobs_PauseUnknownIs404(t *testing.T) {
	srv := serverWithJobs(t, &fakeJobs{unknown: "nope"})

	resp := do(t, srv, http.MethodPost, "/v1/jobs/nope/pause", adminToken, `{"paused":true}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("pause of an unknown job → %d, want 404", resp.StatusCode)
	}
}
