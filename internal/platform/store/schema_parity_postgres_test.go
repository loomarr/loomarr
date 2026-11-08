//go:build integration

package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store/schemadoc"
)

// SQLite and Postgres are migrated by separate files; this holds them to one schema (#2793).
func TestSQLiteAndPostgresSchemasMatch(t *testing.T) {
	ctx := context.Background()
	lite, err := Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "parity.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lite.Close() }()
	pg, err := Open(ctx, startPostgres(t), true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pg.Close() }()

	sqliteTables, err := introspectSQLite(ctx, PoolOf(lite))
	if err != nil {
		t.Fatal(err)
	}
	postgresTables, err := introspectPostgres(ctx, pg.(*sqlStore).db)
	if err != nil {
		t.Fatal(err)
	}
	if problems := schemadoc.Parity(sqliteTables, postgresTables); len(problems) > 0 {
		t.Fatalf("SQLite and Postgres schemas differ (%d):\n  %s", len(problems), strings.Join(problems, "\n  "))
	}
}
