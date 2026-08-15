//go:build integration

package testkit

import (
	"context"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/store"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// PostgresStore opens a fresh migrated production Postgres store in a disposable
// container. It is integration-tagged so ordinary unit tests remain offline.
func PostgresStore(t testing.TB) store.Store {
	t.Helper()
	ctx := context.Background()
	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("loomarr"),
		postgres.WithUsername("loomarr"),
		postgres.WithPassword("loomarr"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		t.Fatalf("start Postgres test container: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(container) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("Postgres connection string: %v", err)
	}
	st, err := store.Open(ctx, dsn, true)
	if err != nil {
		t.Fatalf("open Postgres test store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}
