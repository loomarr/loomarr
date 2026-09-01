//go:build integration

// Postgres conformance runs under the `integration` build tag so the default
// `make test` (which must pass without Docker — §19) skips it. `make test-pg`
// adds -tags=integration and requires Docker for testcontainers.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/loomarr/loomarr/internal/testkit/postgresimage"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// startPostgres spins up an ephemeral Postgres via testcontainers (§14, §19) and
// returns its DSN. One container is shared across the suite's sub-tests; each
// assertion gets a private database cloned from one migrated, boot-seeded template.
func startPostgres(t *testing.T) string {
	t.Helper()
	dsn, _ := startPostgresContainer(t)
	return dsn
}

func startPostgresContainer(t *testing.T) (string, testcontainers.Container) {
	t.Helper()
	ctx := context.Background()
	ctr, err := postgres.Run(ctx, postgresimage.Name(),
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

type postgresConformanceOpenFunc func(context.Context, string, bool) (Store, error)

func postgresDSNDatabase(dsn string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("parse postgres DSN: %w", err)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return "", fmt.Errorf("parse postgres DSN: unexpected scheme %q", u.Scheme)
	}
	database, err := url.PathUnescape(strings.TrimPrefix(u.EscapedPath(), "/"))
	if err != nil {
		return "", fmt.Errorf("decode postgres database name: %w", err)
	}
	if database == "" || strings.Contains(database, "/") {
		return "", fmt.Errorf("parse postgres DSN: invalid database path %q", u.Path)
	}
	return database, nil
}

func postgresDSNWithDatabase(dsn, database string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("parse postgres DSN: %w", err)
	}
	u.Path = "/" + database
	u.RawPath = ""
	return u.String(), nil
}

func newPostgresConformanceStoreFactory(t *testing.T, dsn string) NewStoreFunc {
	t.Helper()
	return newPostgresConformanceStoreFactoryWithOpen(t, dsn, Open)
}

// newPostgresConformanceStoreFactoryWithOpen migrates and boot-seeds the container's original
// database once, closes every connection to it (CREATE DATABASE ... TEMPLATE requires that), then
// hides database cloning and cleanup behind the same NewStoreFunc the shared assertions already use.
// Each returned Store owns a distinct database and still uses the production Postgres adapter.
func newPostgresConformanceStoreFactoryWithOpen(t *testing.T, dsn string, open postgresConformanceOpenFunc) NewStoreFunc {
	t.Helper()
	ctx := context.Background()
	templateDatabase, err := postgresDSNDatabase(dsn)
	if err != nil {
		t.Fatal(err)
	}

	template, err := open(ctx, dsn, true)
	if err != nil {
		t.Fatalf("build migrated postgres template: %v", err)
	}
	if err := template.Close(); err != nil {
		t.Fatalf("close migrated postgres template: %v", err)
	}

	adminDSN, err := postgresDSNWithDatabase(dsn, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	admin, err := sql.Open("pgx", adminDSN)
	if err != nil {
		t.Fatalf("open postgres maintenance connection: %v", err)
	}
	if err := admin.PingContext(ctx); err != nil {
		_ = admin.Close()
		t.Fatalf("ping postgres maintenance connection: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })

	var sequence atomic.Uint64
	return func(t *testing.T) Store {
		t.Helper()
		cloneDatabase := fmt.Sprintf("loomarr_conformance_%06d", sequence.Add(1))
		cloneIdentifier := pgx.Identifier{cloneDatabase}.Sanitize()
		templateIdentifier := pgx.Identifier{templateDatabase}.Sanitize()
		if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+cloneIdentifier+" WITH TEMPLATE "+templateIdentifier); err != nil {
			t.Fatalf("clone migrated postgres template: %v", err)
		}

		cloneDSN, err := postgresDSNWithDatabase(dsn, cloneDatabase)
		if err != nil {
			_, _ = admin.ExecContext(ctx, "DROP DATABASE "+cloneIdentifier+" WITH (FORCE)")
			t.Fatal(err)
		}
		st, err := open(ctx, cloneDSN, false)
		if err != nil {
			_, _ = admin.ExecContext(ctx, "DROP DATABASE "+cloneIdentifier+" WITH (FORCE)")
			t.Fatalf("open cloned postgres store: %v", err)
		}
		t.Cleanup(func() {
			if err := st.Close(); err != nil {
				t.Errorf("close cloned postgres store: %v", err)
			}
			if _, err := admin.ExecContext(context.Background(), "DROP DATABASE "+cloneIdentifier+" WITH (FORCE)"); err != nil {
				t.Errorf("drop cloned postgres database: %v", err)
			}
		})
		return st
	}
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
