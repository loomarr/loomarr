package store

import (
	"context"
	"database/sql"
	"errors"
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
	live, err := introspectSQLite(ctx, PoolOf(st))
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
	return schemadoc.Markdown(doc, live)
}

// introspectSQLite reads every table's columns, in table order, with their keys and references.
func introspectSQLite(ctx context.Context, db *sql.DB) ([]schemadoc.LiveTable, error) {
	names, err := queryStrings(ctx, db,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	tables := make([]schemadoc.LiveTable, 0, len(names))
	for _, name := range names {
		refs, err := sqliteReferences(ctx, db, name)
		if err != nil {
			return nil, err
		}
		rows, err := db.QueryContext(ctx, `SELECT name, type, "notnull", pk FROM pragma_table_info(?) ORDER BY cid`, name)
		if err != nil {
			return nil, err
		}
		t := schemadoc.LiveTable{Name: name}
		for rows.Next() {
			var c schemadoc.LiveColumn
			var pk int
			if err := rows.Scan(&c.Name, &c.Type, &c.NotNull, &pk); err != nil {
				_ = rows.Close()
				return nil, err
			}
			c.PrimaryKey, c.References = pk > 0, refs[c.Name]
			t.Columns = append(t.Columns, c)
		}
		if err := errors.Join(rows.Err(), rows.Close()); err != nil {
			return nil, err
		}
		tables = append(tables, t)
	}
	return tables, nil
}

// sqliteReferences maps each single-column foreign key's column to "table.column". A foreign key
// that names no column points at the parent's primary key, which is shown as the table alone.
func sqliteReferences(ctx context.Context, db *sql.DB, table string) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT "table", "from", "to" FROM pragma_foreign_key_list(?)`, table)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	refs := map[string]string{}
	for rows.Next() {
		var parent, from string
		var to sql.NullString
		if err := rows.Scan(&parent, &from, &to); err != nil {
			return nil, err
		}
		refs[from] = parent
		if to.String != "" {
			refs[from] = parent + "." + to.String
		}
	}
	return refs, rows.Err()
}

// introspectPostgres reads the current schema's tables and columns, in table order. Keys and
// references aren't needed: Postgres only takes the comments, and the reference renders from SQLite.
func introspectPostgres(ctx context.Context, db *sql.DB) ([]schemadoc.LiveTable, error) {
	rows, err := db.QueryContext(ctx, `
SELECT c.table_name, c.column_name, c.data_type, c.is_nullable = 'NO'
  FROM information_schema.columns c
  JOIN information_schema.tables t ON t.table_schema = c.table_schema AND t.table_name = c.table_name
 WHERE c.table_schema = current_schema() AND t.table_type = 'BASE TABLE'
 ORDER BY c.table_name, c.ordinal_position`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var tables []schemadoc.LiveTable
	for rows.Next() {
		var table string
		var c schemadoc.LiveColumn
		if err := rows.Scan(&table, &c.Name, &c.Type, &c.NotNull); err != nil {
			return nil, err
		}
		if n := len(tables); n == 0 || tables[n-1].Name != table {
			tables = append(tables, schemadoc.LiveTable{Name: table})
		}
		tables[len(tables)-1].Columns = append(tables[len(tables)-1].Columns, c)
	}
	return tables, rows.Err()
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
	live, err := introspectPostgres(ctx, db)
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

func queryStrings(ctx context.Context, db *sql.DB, query string) ([]string, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
