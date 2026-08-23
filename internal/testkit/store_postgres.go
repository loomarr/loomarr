//go:build integration

package testkit

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/store"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// PostgresStore opens one fresh migrated production Postgres store in a disposable
// container. It is integration-tagged so ordinary unit tests remain offline.
func PostgresStore(t testing.TB) store.Store {
	t.Helper()
	return PostgresStores(t, 1)[0]
}

// PostgresStores opens count independent production store adapters against one
// migrated disposable database. Use it for replica tests: sharing one Store value
// exercises a pool, while separate adapters exercise the cross-process database seam.
func PostgresStores(t testing.TB, count int) []store.Store {
	t.Helper()
	if count < 1 {
		t.Fatalf("PostgresStores count = %d, want at least one", count)
	}
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
	stores := make([]store.Store, 0, count)
	for i := range count {
		st, openErr := store.Open(ctx, dsn, i == 0)
		if openErr != nil {
			t.Fatalf("open Postgres test store %d: %v", i, openErr)
		}
		t.Cleanup(func() { _ = st.Close() })
		stores = append(stores, st)
	}
	return stores
}
