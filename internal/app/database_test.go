package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/testkit"
)

// The backup gate lives HERE, not in the handler — the handler only maps sentinels to
// status codes. These tests drive the service directly so the gate is proven where it is
// enforced rather than where it is reported.

func newTestDatabaseService(t *testing.T) (*databaseService, string) {
	t.Helper()
	dir := t.TempDir()
	st := testkit.MigratedSQLiteStore(t)
	backups := filepath.Join(dir, "backups")
	return newDatabaseService(st, dir, func() string { return backups }, nil), backups
}

// ⚠ THE PHASE GATE. Migrate must refuse without a backup — and the target DSN here is
// deliberately unreachable, so if the gate did NOT fire the call would fail with a
// connection error instead. Asserting the sentinel proves the refusal happened BEFORE
// anything was opened.
func TestMigrateRefusedWithoutABackup(t *testing.T) {
	svc, _ := newTestDatabaseService(t)
	const dsn = "postgres://u:p@127.0.0.1:1/loomarr?sslmode=disable&connect_timeout=1"

	err := svc.Migrate(context.Background(), dsn)
	if !errors.Is(err, api.ErrNoBackup) && !errors.Is(err, api.ErrPreflightFailed) {
		t.Fatalf("migrate without preconditions → %v, want a gate sentinel", err)
	}
	// Specifically: with no preflight AND no backup, preflight is reported first because
	// it is the earlier step — the operator is told the next thing to do, not the last.
	if !errors.Is(err, api.ErrPreflightFailed) {
		t.Errorf("want ErrPreflightFailed first, got %v", err)
	}
}

// With preflight satisfied but no backup, the backup gate is what fires.
func TestBackupGateFiresAfterPreflight(t *testing.T) {
	svc, _ := newTestDatabaseService(t)
	const dsn = "postgres://u:p@127.0.0.1:1/loomarr?sslmode=disable&connect_timeout=1"

	// Simulate a passing preflight without needing a live Postgres: the gate reads the
	// service's own recorded state, which is exactly the point — it is not client input.
	svc.mu.Lock()
	svc.preflighted = dsn
	svc.mu.Unlock()

	if err := svc.Migrate(context.Background(), dsn); !errors.Is(err, api.ErrNoBackup) {
		t.Fatalf("migrate with preflight but no backup → %v, want ErrNoBackup", err)
	}
}

// The gate reads SERVER state, so a caller cannot satisfy it by asserting it did a
// backup. This is the difference between a gate and a disabled button.
func TestBackupGateCannotBeSatisfiedByTheCaller(t *testing.T) {
	svc, backups := newTestDatabaseService(t)
	const dsn = "postgres://u:p@127.0.0.1:1/loomarr?sslmode=disable&connect_timeout=1"
	svc.mu.Lock()
	svc.preflighted = dsn
	svc.mu.Unlock()

	// The ONLY way past the gate is for the server to write a real file.
	bk, err := svc.Backup(context.Background())
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if _, err := os.Stat(bk.Path); err != nil {
		t.Fatalf("backup reported %s but nothing is there: %v", bk.Path, err)
	}
	if !strings.HasPrefix(bk.Path, backups) {
		t.Errorf("backup written to %s, want it under the configured backup.dir %s", bk.Path, backups)
	}
	if bk.Bytes == 0 {
		t.Error("a zero-byte backup is not a backup")
	}

	// Now the gate passes and only the process-level request is queued. The HTTP-owned
	// module must never open the target or copy data itself.
	var requested string
	svc.WithMigrationRequest(func(target string) error {
		requested = target
		return nil
	})
	if err = svc.Migrate(context.Background(), dsn); err != nil {
		t.Fatalf("queue migration: %v", err)
	}
	if requested != dsn {
		t.Fatalf("requested target = %q, want %q", requested, dsn)
	}
	status, _ := svc.Status(context.Background())
	if status.Phase != "migrating" {
		t.Errorf("accepted request phase = %q, want migrating", status.Phase)
	}
}

// Editing any connection field invalidates the preflight: a pass is about one target,
// not about the operator's intentions.
func TestPreflightAuthorizationIsPerTarget(t *testing.T) {
	svc, _ := newTestDatabaseService(t)
	svc.mu.Lock()
	svc.preflighted = "postgres://u:p@host-a:5432/loomarr"
	svc.backup = &api.DatabaseBackup{Path: "/x", Bytes: 1, WrittenAt: 1}
	svc.mu.Unlock()

	err := svc.Migrate(context.Background(), "postgres://u:p@host-b:5432/loomarr")
	if !errors.Is(err, api.ErrPreflightFailed) {
		t.Fatalf("migrating to a DIFFERENT target than the one preflighted → %v, want ErrPreflightFailed", err)
	}
}

func TestMigrateRefusesAnEnvironmentPinBeforeQueueing(t *testing.T) {
	t.Setenv("DATABASE_URL", "sqlite:///data/loomarr.db")
	svc, _ := newTestDatabaseService(t)
	called := false
	svc.WithMigrationRequest(func(string) error { called = true; return nil })

	err := svc.Migrate(context.Background(), "postgres://u:p@db:5432/loomarr")
	if !errors.Is(err, api.ErrDatabaseURLPinned) {
		t.Fatalf("pinned migration = %v, want ErrDatabaseURLPinned", err)
	}
	if called {
		t.Fatal("a pinned migration must be rejected before queueing")
	}
}

func TestMigrateFailsClosedWithoutAProcessRequester(t *testing.T) {
	svc, _ := newTestDatabaseService(t)
	const dsn = "postgres://u:p@db:5432/loomarr"
	svc.mu.Lock()
	svc.preflighted = dsn
	svc.backup = &api.DatabaseBackup{Path: "/backup", Bytes: 1, WrittenAt: 1}
	svc.mu.Unlock()

	if err := svc.Migrate(context.Background(), dsn); !errors.Is(err, api.ErrMigrationUnavailable) {
		t.Fatalf("migration without requester = %v, want ErrMigrationUnavailable", err)
	}
}

func TestMigrationRequestFailureIsVisibleAndRetryable(t *testing.T) {
	svc, _ := newTestDatabaseService(t)
	const dsn = "postgres://u:p@db:5432/loomarr"
	want := errors.New("restart request queue is full")
	svc.WithMigrationRequest(func(string) error { return want })
	svc.mu.Lock()
	svc.preflighted = dsn
	svc.backup = &api.DatabaseBackup{Path: "/backup", Bytes: 1, WrittenAt: 1}
	svc.mu.Unlock()

	if err := svc.Migrate(context.Background(), dsn); !errors.Is(err, want) {
		t.Fatalf("migration request = %v, want %v", err, want)
	}
	status, _ := svc.Status(context.Background())
	if status.Phase != "failed" || !strings.Contains(status.Error, "queue is full") {
		t.Fatalf("failed status = %+v", status)
	}
	// A rejected enqueue is not left marked running; a corrected requester can retry.
	svc.WithMigrationRequest(func(string) error { return nil })
	if err := svc.Migrate(context.Background(), dsn); err != nil {
		t.Fatalf("retry after rejected enqueue: %v", err)
	}
}

func TestLastMigrationErrorCarriesAcrossGeneration(t *testing.T) {
	svc, _ := newTestDatabaseService(t)
	svc.WithLastError("target parity mismatch; still running on SQLite")
	status, _ := svc.Status(context.Background())
	if status.Phase != "failed" || status.Error == "" {
		t.Fatalf("status = %+v, want failed with the carried error", status)
	}
}

// Status tells the UI whether to offer a migration at all, rather than leaving it to
// infer one from the absence of something.
func TestStatusReportsBackendAndOffersMigration(t *testing.T) {
	svc, _ := newTestDatabaseService(t)
	st, err := svc.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Backend != "sqlite" {
		t.Errorf("backend = %q, want sqlite", st.Backend)
	}
	if !st.CanMigrate {
		t.Error("a SQLite install must be offered a migration")
	}
	if st.Phase != "idle" || st.Parity != "unknown" {
		t.Errorf("fresh service = phase %q parity %q, want idle/unknown", st.Phase, st.Parity)
	}
}

// Switchover writes the bootstrap file the NEXT boot reads — the whole point of the
// step. Asserting the file's content because "roll back by reverting one config line"
// is only true if there is a line to revert.
func TestSwitchoverWritesTheBootstrapFile(t *testing.T) {
	svc, _ := newTestDatabaseService(t)
	const dsn = "postgres://u:p@db:5432/loomarr"
	seedVerifiedSwitchover(svc, dsn)

	if err := svc.Switchover(context.Background(), dsn); err != nil {
		t.Fatalf("switchover: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(svc.dataDir, "bootstrap.json"))
	if err != nil {
		t.Fatalf("read bootstrap file: %v", err)
	}
	if !strings.Contains(string(raw), dsn) {
		t.Errorf("bootstrap file does not carry the new DSN:\n%s", raw)
	}
}

// An env-pinned DATABASE_URL always wins at boot, so writing the file would produce a
// switch that silently does not happen. Refusing is the honest answer.
func TestSwitchoverRefusesWhenPinnedByEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "sqlite:///data/loomarr.db")
	svc, _ := newTestDatabaseService(t)
	const dsn = "postgres://u:p@db:5432/loomarr"
	seedVerifiedSwitchover(svc, dsn)

	err := svc.Switchover(context.Background(), dsn)
	if !errors.Is(err, api.ErrDatabaseURLPinned) {
		t.Fatalf("pinned switchover = %v, want ErrDatabaseURLPinned", err)
	}
	if _, statErr := os.Stat(filepath.Join(svc.dataDir, "bootstrap.json")); statErr == nil {
		t.Error("nothing should have been written")
	}
}

func TestSwitchoverFailsClosedWithoutExactVerifiedState(t *testing.T) {
	const dsn = "postgres://u:p@db:5432/loomarr"
	for _, tc := range []struct {
		name  string
		alter func(*databaseService)
	}{
		{name: "idle", alter: func(*databaseService) {}},
		{name: "wrong target", alter: func(s *databaseService) {
			seedVerifiedSwitchover(s, "postgres://u:p@other:5432/loomarr")
		}},
		{name: "parity unknown", alter: func(s *databaseService) {
			seedVerifiedSwitchover(s, dsn)
			s.parity = "unknown"
		}},
		{name: "still running", alter: func(s *databaseService) {
			seedVerifiedSwitchover(s, dsn)
			s.running = true
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := newTestDatabaseService(t)
			tc.alter(svc)
			if err := svc.Switchover(context.Background(), dsn); !errors.Is(err, api.ErrMigrationNotVerified) {
				t.Fatalf("switchover = %v, want ErrMigrationNotVerified", err)
			}
			if _, err := os.Stat(filepath.Join(svc.dataDir, "bootstrap.json")); err == nil {
				t.Fatal("unverified switchover wrote bootstrap.json")
			}
		})
	}
}

func seedVerifiedSwitchover(svc *databaseService, dsn string) {
	svc.mu.Lock()
	svc.preflighted = dsn
	svc.backup = &api.DatabaseBackup{Path: "/backup", Bytes: 1, WrittenAt: 1}
	svc.phase = "verified"
	svc.parity = "match"
	svc.mu.Unlock()
}
