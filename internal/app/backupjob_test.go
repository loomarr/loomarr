package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/testkit"
)

// jobView mirrors the fields of the /v1/jobs read model this test cares about.
type jobView struct {
	Name           string `json:"name"`
	DisabledReason string `json:"disabledReason"`
}

// listJobs drives GET /v1/jobs through a real Application.
//
// Going through the composition root and the HTTP surface — rather than reaching into the
// registry — is the point: the registry is local to composition, and what matters is what
// an operator's Tasks page actually shows. `API_TOKEN` is pinned via the environment so the
// break-glass admin path authenticates; there is deliberately no skip-on-403 escape here,
// because a test that can silently skip is a green tick asserting nothing.
func listJobs(t *testing.T, st store.Store) []jobView {
	t.Helper()
	t.Setenv("API_TOKEN", "test-app-token")

	h := buildTestApplication(t, st, Overrides{}).Handler()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/jobs", nil)
	req.Header.Set("Authorization", "Bearer test-app-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/jobs → %d, want 200", resp.StatusCode)
	}
	var body struct {
		Jobs []jobView `json:"jobs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body.Jobs
}

func findJob(jobs []jobView, name string) (jobView, bool) {
	for _, j := range jobs {
		if j.Name == name {
			return j, true
		}
	}
	return jobView{}, false
}

// On SQLite the backup job is registered and ENABLED — it is the install that can actually
// run it.
func TestBackupJob_EnabledOnSQLite(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)

	jobs := listJobs(t, st)
	got, ok := findJob(jobs, "backup")
	if !ok {
		t.Fatalf("no backup job registered on SQLite; jobs = %+v", jobs)
	}
	if got.DisabledReason != "" {
		t.Errorf("backup job on SQLite carries DisabledReason %q, want empty", got.DisabledReason)
	}
}
