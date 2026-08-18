//go:build integration

// The Postgres half of the backup-job wiring runs under the `integration` build tag so the
// default `make test` (which must pass without Docker — §19) skips it. `make test-pg` adds
// -tags=integration and requires Docker for testcontainers.
package app

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/store"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// postJob drives POST /v1/jobs/{name}/run through a real BuildHandler and returns the
// status code, so a caller can assert the SERVER's refusal rather than the UI's. Lives in
// the integration file because that is where its only caller is — in the default build it
// would be dead code, which `unused` correctly rejects.
func postJob(t *testing.T, st store.Store, name string) int {
	t.Helper()
	t.Setenv("API_TOKEN", "test-app-token")

	h, err := BuildHandler(t.Context(), st, slog.New(slog.DiscardHandler), Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/jobs/"+name+"/run", nil)
	req.Header.Set("Authorization", "Bearer test-app-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

func startPostgresForApp(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	ctr, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("loomarr"),
		postgres.WithUsername("loomarr"),
		postgres.WithPassword("loomarr"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second)))
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(ctr) })
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("postgres dsn: %v", err)
	}
	return dsn
}

// ⚠ THE GATE for the disabled-job concept: on Postgres the backup job is LISTED, carrying
// the reason it cannot run.
//
// The alternative — not registering it — is what this replaced, and it is also a claim: an
// absent row is indistinguishable, from the Tasks page alone, from a job that runs fine and
// has never failed. For backup specifically that ambiguity means an operator believing they
// are covered when they are not.
func TestBackupJob_DisabledOnPostgres(t *testing.T) {
	st, err := store.Open(context.Background(), startPostgresForApp(t), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	jobs := listJobs(t, st)
	got, ok := findJob(jobs, "backup")
	if !ok {
		t.Fatalf("backup job is ABSENT on Postgres — it must be listed as disabled, not omitted; jobs = %+v", jobs)
	}
	if got.DisabledReason == "" {
		t.Fatal("backup job on Postgres has no DisabledReason — it would read as a healthy job that has never run")
	}
	// The reason has to name the alternative, or it tells the operator they have a problem
	// without telling them what to do about it.
	if !strings.Contains(got.DisabledReason, "pg_dump") {
		t.Errorf("DisabledReason = %q, want it to point at pg_dump", got.DisabledReason)
	}
}

// Run-now on the disabled job is refused by the SERVER with a 409 — a disabled button is a
// hint, and this one would run a job whose writer does not exist on this backend.
func TestBackupJob_RunNowRefusedOnPostgres(t *testing.T) {
	st, err := store.Open(context.Background(), startPostgresForApp(t), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	resp := postJob(t, st, "backup")
	if resp != http.StatusConflict {
		t.Errorf("run-now on the disabled backup job → %d, want 409", resp)
	}
}
