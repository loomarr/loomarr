//go:build integration

package storetest

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
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver the clone admin uses
	"github.com/loomarr/loomarr/internal/testkit/postgresimage"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// StartPostgres spins up an ephemeral Postgres via testcontainers (§14, §19) and returns its DSN
// and container. One container is shared across a suite's sub-tests; each assertion gets a private
// database cloned from one migrated, boot-seeded template.
func StartPostgres(t *testing.T) (string, testcontainers.Container) {
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

// PostgresDSNDatabase returns the database named by a postgres:// DSN.
func PostgresDSNDatabase(dsn string) (string, error) {
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

// PostgresDSNWithDatabase returns dsn pointed at another database on the same server.
func PostgresDSNWithDatabase(dsn, database string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("parse postgres DSN: %w", err)
	}
	u.Path = "/" + database
	u.RawPath = ""
	return u.String(), nil
}

// PostgresClones migrates and boot-seeds the container's original database once, closes every
// connection to it (CREATE DATABASE ... TEMPLATE requires that), then hides database cloning and
// cleanup behind a factory. Each returned store owns a distinct database and still uses the
// production Postgres adapter.
func PostgresClones[S Closer](t *testing.T, dsn string, open OpenFunc[S]) func(*testing.T) S {
	t.Helper()
	ctx := context.Background()
	templateDatabase, err := PostgresDSNDatabase(dsn)
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

	adminDSN, err := PostgresDSNWithDatabase(dsn, "postgres")
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
	return func(t *testing.T) S {
		t.Helper()
		cloneDatabase := fmt.Sprintf("loomarr_conformance_%06d", sequence.Add(1))
		cloneIdentifier := pgx.Identifier{cloneDatabase}.Sanitize()
		templateIdentifier := pgx.Identifier{templateDatabase}.Sanitize()
		if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+cloneIdentifier+" WITH TEMPLATE "+templateIdentifier); err != nil {
			t.Fatalf("clone migrated postgres template: %v", err)
		}

		cloneDSN, err := PostgresDSNWithDatabase(dsn, cloneDatabase)
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
