//go:build integration

package store

import (
	"context"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store/schemadoc"
)

// Postgres must have every table and column described too (its schema is migrated separately and
// could grow a column SQLite lacks), and a booted store must carry the descriptions as comments.
func TestPostgresSchemaIsDescribedAndCommented(t *testing.T) {
	ctx := context.Background()
	dsn := startPostgres(t)
	st, err := Open(ctx, dsn, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	db := st.(*sqlStore).db

	live, err := introspectPostgres(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := schemadoc.Load()
	if err != nil {
		t.Fatal(err)
	}
	if problems := schemadoc.Check(doc, live); len(problems) > 0 {
		t.Fatalf("Postgres schema and descriptions disagree:\n  %s", strings.Join(problems, "\n  "))
	}

	var table, column string
	if err := db.QueryRowContext(ctx, `SELECT obj_description('users'::regclass, 'pg_class'),
	       col_description('users'::regclass, (SELECT attnum FROM pg_attribute
	                                            WHERE attrelid = 'users'::regclass AND attname = 'id'))`,
	).Scan(&table, &column); err != nil {
		t.Fatal(err)
	}
	if table == "" || column == "" {
		t.Fatalf("users comments = (%q, %q), want both set", table, column)
	}
	// Booting again re-applies them without error (every replica runs this after migrate).
	again, err := Open(ctx, dsn, true)
	if err != nil {
		t.Fatal(err)
	}
	_ = again.Close()
}
