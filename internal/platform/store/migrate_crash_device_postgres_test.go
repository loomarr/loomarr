//go:build integration

package store

import (
	"database/sql"
	"testing"
)

func TestPostgresCrashDeviceMigration(t *testing.T) {
	db, err := sql.Open("pgx", startPostgres(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testCrashDeviceMigration(t, db, DialectPostgres, "migrations/postgres")
}
