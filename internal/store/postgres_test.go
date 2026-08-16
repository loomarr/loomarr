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
	dsn, _ := startPostgresContainer(t)
	return dsn
}

func startPostgresContainer(t *testing.T) (string, testcontainers.Container) {
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
	return dsn, ctr
}

// TestPostgresConformance runs the SAME suite as SQLite (AGENTS.md: one suite,
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
			// Reset the shared database by DROPPING THE SCHEMA, so the next sub-test's Open
			// re-runs every migration against an empty database.
			//
			// ⚠ **This replaces a hand-written TRUNCATE list, and the reason is worth keeping.**
			// The list read `titles, settings, channels, sessions, users, jobs, proposals, clips`
			// under a comment asking the next person to keep it in step with the schema. By the
			// time V52 arrived it covered roughly eight of twenty tables — and the failure it
			// produced was the worst kind: rows from earlier sub-tests survived into later ones,
			// so any assertion over a GLOBAL query passed on SQLite (which hands every sub-test a
			// fresh file) and failed only on Postgres.
			//
			// ⚠ **But the list was not purely drift, and simply completing it is WRONG.**
			// `filler_sources` and the taxonomy carry rows a MIGRATION seeds; truncating those
			// destroys fixture data nothing puts back, which is exactly what completing the list
			// did — it turned two Filler failures on. Nothing in the literal distinguished
			// "omitted by accident" from "omitted on purpose".
			//
			// Dropping the schema sidesteps both problems instead of balancing them: seeded rows
			// come back because the seeding migration runs again, new tables are covered the day
			// they exist, and the semantics finally MATCH the SQLite path (a genuinely fresh
			// database per sub-test) rather than approximating it.
			pg := s.(*sqlStore)
			ctx := context.Background()
			// CASCADE takes the foreign keys with it; recreating public restores the default
			// search_path the next connection expects. goose's bookkeeping table goes too, which
			// is what makes the migrations re-run rather than being skipped as already-applied.
			_, _ = pg.db.ExecContext(ctx, "DROP SCHEMA public CASCADE")
			_, _ = pg.db.ExecContext(ctx, "CREATE SCHEMA public")
			_ = s.Close()
		})
		return s
	}

	RunConformance(t, newStore)

	t.Run("ApprovalQuotaAcrossIndependentPools", func(t *testing.T) {
		primary, err := Open(context.Background(), dsn, true)
		if err != nil {
			t.Fatalf("open primary postgres store: %v", err)
		}
		secondary, err := openPostgres(context.Background(), dsn)
		if err != nil {
			_ = primary.Close()
			t.Fatalf("open independent postgres pool: %v", err)
		}
		// One connection per independent pool makes the old starvation shape
		// impossible to hide: the holder must finish guard + commit on its existing
		// transaction connection while the contender waits on its own.
		primary.(*sqlStore).db.SetMaxOpenConns(1)
		secondary.db.SetMaxOpenConns(1)
		t.Cleanup(func() {
			_ = secondary.Close()
			pg := primary.(*sqlStore)
			_, _ = pg.db.ExecContext(context.Background(), "DROP SCHEMA public CASCADE")
			_, _ = pg.db.ExecContext(context.Background(), "CREATE SCHEMA public")
			_ = primary.Close()
		})

		testProposalAutoApprovalQuotaAcrossStores(t, primary, secondary)
	})
}
