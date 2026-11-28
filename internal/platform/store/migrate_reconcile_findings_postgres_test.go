//go:build integration

package store

import (
	"database/sql"
	"testing"
)

func TestPostgresReconcileFindingsMigration(t *testing.T) {
	db, err := sql.Open("pgx", startPostgres(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testReconcileFindingsMigration(t, db, DialectPostgres, "migrations/postgres")
}
