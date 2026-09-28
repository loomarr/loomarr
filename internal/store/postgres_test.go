//go:build integration

// Postgres conformance runs under the `integration` build tag so the default
// `make test` (which must pass without Docker — §19) skips it. `make test-pg`
// adds -tags=integration and requires Docker for testcontainers.
package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/loomarr/loomarr/internal/store/storetest"
	"github.com/testcontainers/testcontainers-go"
)

// startPostgres spins up an ephemeral Postgres via testcontainers (§14, §19) and
// returns its DSN. One container is shared across the suite's sub-tests; each
// assertion gets a private database cloned from one migrated, boot-seeded template.
func startPostgres(t *testing.T) string {
	t.Helper()
	dsn, _ := storetest.StartPostgres(t)
	return dsn
}

func startPostgresContainer(t *testing.T) (string, testcontainers.Container) {
	t.Helper()
	return storetest.StartPostgres(t)
}

type postgresConformanceOpenFunc func(context.Context, string, bool) (Store, error)

func newPostgresConformanceStoreFactory(t *testing.T, dsn string) NewStoreFunc {
	t.Helper()
	return newPostgresConformanceStoreFactoryWithOpen(t, dsn, Open)
}

// newPostgresConformanceStoreFactoryWithOpen hides database cloning and cleanup behind the same
// NewStoreFunc the shared assertions already use (storetest.PostgresClones).
func newPostgresConformanceStoreFactoryWithOpen(t *testing.T, dsn string, open postgresConformanceOpenFunc) NewStoreFunc {
	t.Helper()
	return storetest.PostgresClones(t, dsn, storetest.OpenFunc[Store](open))
}

// TestPostgresConformance runs the SAME suite as SQLite (AGENTS.md: one suite,
// two backends — never forked). The concurrent-claim case is the point: it
// exercises real FOR UPDATE SKIP LOCKED row locking, which SQLite can't.
//
// Requires Docker; run via `make test-pg` (guarded off the default `make test`).
func TestPostgresConformance(t *testing.T) {
	dsn := startPostgres(t)
	newStore := newPostgresConformanceStoreFactory(t, dsn)

	RunConformance(t, newStore)

	t.Run("ApprovalQuotaAcrossIndependentPools", func(t *testing.T) {
		primary := newStore(t)
		secondary, err := openPostgres(context.Background(), primary.(*sqlStore).dsn)
		if err != nil {
			t.Fatalf("open independent postgres pool: %v", err)
		}
		// One connection per independent pool makes the old starvation shape
		// impossible to hide: the holder must finish guard + commit on its existing
		// transaction connection while the contender waits on its own.
		primary.(*sqlStore).db.SetMaxOpenConns(1)
		secondary.db.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = secondary.Close() })

		testProposalAutoApprovalQuotaAcrossStores(t, primary, secondary)
	})
}

func TestPostgresFillerPullCommitMigrationPreservesHistoricalRuns(t *testing.T) {
	db, err := sql.Open("pgx", startPostgres(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testFillerPullCommitMigration(t, db, DialectPostgres, "migrations/postgres")
}
