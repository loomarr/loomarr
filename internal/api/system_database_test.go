package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/api"
)

// errFake stands in for any copy failure the engine can report.
var errFake = errors.New("target ran out of disk after 506 of 1204 rows")

// fakeDatabase is a scriptable DatabaseService for the API-layer tests. It records what
// it was asked so the gate assertions can prove the request never reached the engine.
type fakeDatabase struct {
	status       api.DatabaseStatus
	checks       []api.DatabaseCheck
	preflightErr error
	preflightAt  string
	migrateErr   error
	migratedTo   string
	backupErr    error
	backupHit    bool
	switchErr    error
	switchedTo   string
}

func (f *fakeDatabase) Status(context.Context) (api.DatabaseStatus, error) { return f.status, nil }

func (f *fakeDatabase) Preflight(_ context.Context, dsn string) ([]api.DatabaseCheck, error) {
	f.preflightAt = dsn
	return f.checks, f.preflightErr
}

func (f *fakeDatabase) Backup(context.Context) (api.DatabaseBackup, error) {
	f.backupHit = true
	if f.backupErr != nil {
		return api.DatabaseBackup{}, f.backupErr
	}
	return api.DatabaseBackup{Path: "/data/backups/loomarr-x.db", Bytes: 4096, WrittenAt: 1_700_000_000}, nil
}

func (f *fakeDatabase) Migrate(_ context.Context, dsn string) error {
	if f.migrateErr != nil {
		return f.migrateErr
	}
	f.migratedTo = dsn
	return nil
}

func (f *fakeDatabase) Switchover(_ context.Context, dsn string) error {
	if f.switchErr != nil {
		return f.switchErr
	}
	f.switchedTo = dsn
	return nil
}

func serverWithDatabase(t *testing.T, svc api.DatabaseService) *httptest.Server {
	t.Helper()
	st := openTestStore(t, t.TempDir()+"/api.db")
	t.Cleanup(func() { _ = st.Close() })
	h := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store:    st,
		Auth:     testAuthorizer{},
		Log:      slog.New(slog.DiscardHandler),
		Database: svc,
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

// Every route is admin-only: a migration moves every row the instance owns, and the
// backup it writes carries every secret.
func TestSystemDatabase_RequiresAdmin(t *testing.T) {
	srv := serverWithDatabase(t, &fakeDatabase{})
	for _, tc := range []struct {
		method, path, body string
	}{
		{http.MethodGet, "/v1/system/database", ""},
		{http.MethodPost, "/v1/system/database/preflight", `{"dsn":"postgres://u:p@h:5432/d"}`},
		{http.MethodPost, "/v1/system/database/backup", ""},
		{http.MethodPost, "/v1/system/database/migrate", `{"dsn":"postgres://u:p@h:5432/d"}`},
		{http.MethodPost, "/v1/system/database/switchover", `{"dsn":"postgres://u:p@h:5432/d"}`},
	} {
		resp := do(t, srv, tc.method, tc.path, "", tc.body) // no token
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s without admin → %d, want 401", tc.method, tc.path, resp.StatusCode)
		}
	}
}

// An embedding that provides no database service gets an explicit 501 rather than a
// fabricated status. The production composition root provides status on both backends.
func TestSystemDatabase_NotConfigured501(t *testing.T) {
	srv := serverWithDatabase(t, nil)
	resp := do(t, srv, http.MethodGet, "/v1/system/database", adminToken, "")
	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("status with no service → %d, want 501", resp.StatusCode)
	}
}

// ⚠ THE PHASE GATE: the backup cannot be skipped, and it is the SERVER that says so.
// A client that ignores the disabled button and POSTs straight to /migrate is refused.
func TestSystemDatabase_BackupCannotBeSkipped(t *testing.T) {
	fake := &fakeDatabase{migrateErr: api.ErrNoBackup}
	srv := serverWithDatabase(t, fake)

	resp := do(t, srv, http.MethodPost, "/v1/system/database/migrate", adminToken,
		`{"dsn":"postgres://u:p@h:5432/d"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("migrate without a backup → %d, want 409", resp.StatusCode)
	}
	// The refusal must SAY why — an operator who gets a bare 409 will assume the target
	// is wrong and go debug Postgres.
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if detail, _ := body["detail"].(string); detail == "" {
		t.Error("the 409 must carry a reason")
	}
	if fake.migratedTo != "" {
		t.Error("the migration must not have run")
	}
}

// A target that has not passed preflight is refused for the same reason and separately,
// so the operator learns which precondition is missing.
func TestSystemDatabase_PreflightRequired(t *testing.T) {
	fake := &fakeDatabase{migrateErr: api.ErrPreflightFailed}
	srv := serverWithDatabase(t, fake)

	resp := do(t, srv, http.MethodPost, "/v1/system/database/migrate", adminToken,
		`{"dsn":"postgres://u:p@h:5432/d"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("migrate without preflight → %d, want 409", resp.StatusCode)
	}
	if fake.migratedTo != "" {
		t.Error("the migration must not have run")
	}
}

func TestSystemDatabase_PostgresMutationsFailAsConflicts(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		configure  func(*fakeDatabase)
	}{
		{
			name: "preflight", path: "/v1/system/database/preflight",
			configure: func(f *fakeDatabase) { f.preflightErr = api.ErrNotSQLite },
		},
		{
			name: "switchover", path: "/v1/system/database/switchover",
			configure: func(f *fakeDatabase) { f.switchErr = api.ErrNotSQLite },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeDatabase{}
			tc.configure(fake)
			srv := serverWithDatabase(t, fake)
			resp := do(t, srv, http.MethodPost, tc.path, adminToken,
				`{"dsn":"postgres://u:p@h:5432/d"}`)
			if resp.StatusCode != http.StatusConflict {
				t.Fatalf("Postgres %s -> %d, want 409", tc.name, resp.StatusCode)
			}
		})
	}
}

// Preflight reports failed checks as data with ok:false, not as an error — the operator
// needs to see WHICH check failed, which a 4xx with a single message cannot express.
func TestSystemDatabase_PreflightReportsFailedChecks(t *testing.T) {
	fake := &fakeDatabase{checks: []api.DatabaseCheck{
		{Name: "Reachable", Detail: "connected in 3ms", OK: true},
		{Name: "Target is empty", Detail: "10 table(s) already present", OK: false},
	}}
	srv := serverWithDatabase(t, fake)

	resp := do(t, srv, http.MethodPost, "/v1/system/database/preflight", adminToken,
		`{"dsn":"postgres://u:p@h:5432/d"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("preflight with a failing check → %d, want 200 (the checks are the payload)", resp.StatusCode)
	}
	var body struct {
		Checks []api.DatabaseCheck `json:"checks"`
		Passed bool                `json:"passed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Passed {
		t.Error("passed must be false when any check failed")
	}
	if len(body.Checks) != 2 {
		t.Fatalf("got %d checks, want both", len(body.Checks))
	}
	if body.Checks[1].OK {
		t.Error("the failing check must come back with ok:false")
	}
}

// Passed is true only when every check passed — a partial pass is not a pass.
func TestSystemDatabase_PreflightPassedIsUnanimous(t *testing.T) {
	fake := &fakeDatabase{checks: []api.DatabaseCheck{
		{Name: "Reachable", OK: true},
		{Name: "Version", OK: true},
	}}
	srv := serverWithDatabase(t, fake)
	resp := do(t, srv, http.MethodPost, "/v1/system/database/preflight", adminToken,
		`{"dsn":"postgres://u:p@h:5432/d"}`)
	var body struct {
		Passed bool `json:"passed"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if !body.Passed {
		t.Error("all-OK checks must report passed:true")
	}
}

// A copy failure is a 409 carrying the reason, not a 500 — the instance is fine, still
// on SQLite, and the operator's next step depends on which reason it was.
func TestSystemDatabase_MigrationFailureIsAConflictNotA500(t *testing.T) {
	fake := &fakeDatabase{migrateErr: errFake}
	srv := serverWithDatabase(t, fake)
	resp := do(t, srv, http.MethodPost, "/v1/system/database/migrate", adminToken,
		`{"dsn":"postgres://u:p@h:5432/d"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("copy failure → %d, want 409", resp.StatusCode)
	}
}

func TestSystemDatabase_PinnedMigrationIsRejectedBeforeQueueing(t *testing.T) {
	fake := &fakeDatabase{migrateErr: api.ErrDatabaseURLPinned}
	srv := serverWithDatabase(t, fake)
	resp := do(t, srv, http.MethodPost, "/v1/system/database/migrate", adminToken,
		`{"dsn":"postgres://u:p@h:5432/d"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("pinned migration → %d, want 409", resp.StatusCode)
	}
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if detail, _ := body["detail"].(string); !strings.Contains(detail, "pinned") {
		t.Fatalf("detail = %q, want pinned explanation", detail)
	}
	if fake.migratedTo != "" {
		t.Fatal("a pinned migration request reached the process requester")
	}
}

func TestSystemDatabase_MigrationRequesterMustExist(t *testing.T) {
	fake := &fakeDatabase{migrateErr: api.ErrMigrationUnavailable}
	srv := serverWithDatabase(t, fake)
	resp := do(t, srv, http.MethodPost, "/v1/system/database/migrate", adminToken,
		`{"dsn":"postgres://u:p@h:5432/d"}`)
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("migration without process requester → %d, want 501", resp.StatusCode)
	}
}

// Switchover always reports restartRequired: DATABASE_URL is a boot-time setting, so a
// response that did not say so would leave the operator thinking the move was live.
func TestSystemDatabase_SwitchoverRequiresRestart(t *testing.T) {
	fake := &fakeDatabase{}
	srv := serverWithDatabase(t, fake)
	resp := do(t, srv, http.MethodPost, "/v1/system/database/switchover", adminToken,
		`{"dsn":"postgres://u:p@h:5432/d"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("switchover → %d, want 200", resp.StatusCode)
	}
	var body struct {
		RestartRequired bool   `json:"restartRequired"`
		Note            string `json:"note"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.RestartRequired {
		t.Error("switchover must report restartRequired")
	}
	if body.Note == "" {
		t.Error("switchover must explain what happens next")
	}
	if fake.switchedTo != "postgres://u:p@h:5432/d" {
		t.Errorf("switched to %q, want the requested DSN", fake.switchedTo)
	}
}

func TestSystemDatabase_LegacySwitchoverFailsClosedWithoutVerification(t *testing.T) {
	fake := &fakeDatabase{switchErr: api.ErrMigrationNotVerified}
	srv := serverWithDatabase(t, fake)
	resp := do(t, srv, http.MethodPost, "/v1/system/database/switchover", adminToken,
		`{"dsn":"postgres://u:p@h:5432/d"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("unverified switchover → %d, want 409", resp.StatusCode)
	}
	if fake.switchedTo != "" {
		t.Fatal("unverified switchover was accepted")
	}
}
