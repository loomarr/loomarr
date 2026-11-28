//go:build integration

package store

import (
	"database/sql"
	"github.com/loomarr/loomarr/internal/platform/store/storedb"
	"testing"
)

func TestPostgresReconcileFindingsMigration(t *testing.T) {
	db, err := sql.Open("pgx", startPostgres(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testReconcileFindingsMigration(t, db, storedb.DialectPostgres, "migrations/postgres")
}
