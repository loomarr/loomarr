//go:build integration

package store

import (
	"database/sql"
	"github.com/loomarr/loomarr/internal/platform/store/storedb"
	"testing"
)

func TestPostgresCrashDeviceMigration(t *testing.T) {
	db, err := sql.Open("pgx", startPostgres(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testCrashDeviceMigration(t, db, storedb.DialectPostgres, "migrations/postgres")
}
