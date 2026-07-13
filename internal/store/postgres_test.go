//go:build integration

// Postgres conformance runs under the `integration` build tag so the default
// `make test` (which must pass without Docker — §19) skips it. `make test-pg`
// adds -tags=integration and requires Docker for testcontainers.
package store

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// startPostgres spins up an ephemeral Postgres via testcontainers (§14, §19) and
// returns its DSN. One container is shared across the suite's sub-tests; each
// sub-test still gets a freshly-migrated store on its own schema via newStore.
func startPostgres(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	ctr, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("loomarr"),
		postgres.WithUsername("loomarr"),
		postgres.WithPassword("loomarr"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(ctr) })

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("postgres connection string: %v", err)
	}
	return dsn
}

// TestPostgresConformance runs the SAME suite as SQLite (CLAUDE.md: one suite,
// two backends — never forked). The concurrent-claim case is the point: it
// exercises real FOR UPDATE SKIP LOCKED row locking, which SQLite can't.
//
// Requires Docker; run via `make test-pg` (guarded off the default `make test`).
func TestPostgresConformance(t *testing.T) {
	dsn := startPostgres(t)

	newStore := func(t *testing.T) Store {
		t.Helper()
		// Each sub-test migrates fresh. The migrations DROP+CREATE via goose on a
		// clean container database; sub-tests within one run share the DB but the
		// suite's assertions are written to be independent (distinct keys).
		s, err := Open(context.Background(), dsn, true)
		if err != nil {
			t.Fatalf("open postgres store: %v", err)
		}
		t.Cleanup(func() {
			// Truncate between sub-tests so shared-DB state doesn't leak. Keep
			// this list in step with the schema — every conformance table.
			pg := s.(*sqlStore)
			_, _ = pg.db.ExecContext(context.Background(),
				"TRUNCATE titles, settings, channels, sessions, users")
			_ = s.Close()
		})
		return s
	}

	RunConformance(t, newStore)
}
