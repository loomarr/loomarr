package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/loomarr/loomarr/internal/platform/store/schemadoc"
)

// SchemaReference builds docs/reference/database.md: it migrates a fresh SQLite database in dir
// to head, reads its tables and columns back, and renders schemadoc's descriptions over them. It
// fails, listing every gap, when a table or column the migrations create isn't described or a
// description names one that's gone (#2446).
func SchemaReference(ctx context.Context, dir string) ([]byte, error) {
	st, err := Open(ctx, "sqlite://"+filepath.Join(dir, "schema.db"), true)
	if err != nil {
		return nil, err
	}
	defer func() { _ = st.Close() }()
	live, err := schemadoc.IntrospectSQLite(ctx, PoolOf(st))
	if err != nil {
		return nil, err
	}
	doc, err := schemadoc.Load()
	if err != nil {
		return nil, err
	}
	if problems := schemadoc.Check(doc, live); len(problems) > 0 {
		return nil, fmt.Errorf("schema reference is out of date with the migrations "+
			"(edit internal/platform/store/schemadoc/areas/):\n  %s", strings.Join(problems, "\n  "))
	}
	if problems := schemadoc.Lint(doc, live); len(problems) > 0 {
		return nil, fmt.Errorf("schema breaks the schema lint (see internal/platform/store/schemadoc/lint.go):\n  %s",
			strings.Join(problems, "\n  "))
	}
	return schemadoc.Markdown(doc, live)
}

// commentPostgresSchema stores schemadoc's descriptions as COMMENT ON TABLE/COLUMN, so a SQL client
// browsing a Postgres install shows each one beside the data. It runs after every migrate, in one
// transaction, and comments only objects that exist, so a stale description never fails a boot
// (the schema reference test catches it instead).
func commentPostgresSchema(ctx context.Context, db *sql.DB) error {
	doc, err := schemadoc.Load()
	if err != nil {
		return err
	}
	live, err := schemadoc.IntrospectPostgres(ctx, db)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, c := range schemadoc.Comments(doc, live) {
		target := "TABLE " + quoteIdent(c.Table)
		if c.Column != "" {
			target = "COLUMN " + quoteIdent(c.Table) + "." + quoteIdent(c.Column)
		}
		// COMMENT takes no bind parameters; the text is quoted as a SQL string literal.
		if _, err := tx.ExecContext(ctx, "COMMENT ON "+target+" IS "+quoteLiteral(c.Text)); err != nil {
			return fmt.Errorf("comment on %s: %w", target, err)
		}
	}
	return tx.Commit()
}

// quoteLiteral is a SQL string literal. Postgres's standard_conforming_strings (on since 9.1)
// makes a backslash ordinary, so doubling the single quote is the whole escape.
func quoteLiteral(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
