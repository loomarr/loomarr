package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

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
	if problems := schemadoc.Lint(doc, live); len(problems) > 0 {
		return nil, fmt.Errorf("schema breaks the schema lint (see internal/platform/store/schemadoc/lint.go):\n  %s",
			strings.Join(problems, "\n  "))
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
		t := schemadoc.LiveTable{Name: name}
		// A partial index can't serve every lookup of a parent's children, so it doesn't count, unless
		// it only leaves out NULLs of its first column: an equality lookup never matches a NULL, so
		// SQLite uses such an index for every lookup by that column.
		if t.Indexes, err = groupedColumns(ctx, db, `SELECT il.name, ii.name FROM pragma_index_list(?1) il
			JOIN pragma_index_info(il.name) ii
			JOIN sqlite_master m ON m.type = 'index' AND m.name = il.name
			WHERE il.partial = 0 OR replace(replace(replace(lower(m.sql), char(10), ' '), '  ', ' '), '  ', ' ')
				LIKE '% where ' || (SELECT lower(name) FROM pragma_index_info(il.name) WHERE seqno = 0) || ' is not null'
			ORDER BY il.name, ii.seqno`, name); err != nil {
			return nil, err
		}
		if t.ForeignKeys, err = groupedColumns(ctx, db,
			`SELECT id, "from" FROM pragma_foreign_key_list(?1) ORDER BY id, seq`, name); err != nil {
			return nil, err
		}
		var ddl string
		if err := db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&ddl); err != nil {
			return nil, err
		}
		checked := checkedColumns(ddl)
		rows, err := db.QueryContext(ctx, `SELECT name, type, "notnull", pk FROM pragma_table_info(?) ORDER BY cid`, name)
		if err != nil {
			return nil, err
		}
		var primary []string // the primary key's columns by position: the table itself is that index
		for rows.Next() {
			var c schemadoc.LiveColumn
			var pk int
			if err := rows.Scan(&c.Name, &c.Type, &c.NotNull, &pk); err != nil {
				_ = rows.Close()
				return nil, err
			}
			c.PrimaryKey, c.References = pk > 0, refs[c.Name]
			c.Checked = checked[strings.ToLower(c.Name)]
			t.Columns = append(t.Columns, c)
			if pk > 0 {
				primary = append(primary, make([]string, max(0, pk-len(primary)))...)
				primary[pk-1] = c.Name
			}
		}
		if err := errors.Join(rows.Err(), rows.Close()); err != nil {
			return nil, err
		}
		if len(primary) > 0 {
			t.Indexes = append(t.Indexes, primary)
		}
		tables = append(tables, t)
	}
	return tables, nil
}

// groupedColumns runs a query of (group, column) rows, ordered by group then position, and returns
// each group's columns in order.
func groupedColumns(ctx context.Context, db *sql.DB, query string, args ...any) ([][]string, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out [][]string
	var last string
	for rows.Next() {
		var group, column string
		if err := rows.Scan(&group, &column); err != nil {
			return nil, err
		}
		if len(out) == 0 || group != last {
			out = append(out, nil)
		}
		last = group
		out[len(out)-1] = append(out[len(out)-1], column)
	}
	return out, rows.Err()
}

// checkedColumns returns the lower-cased identifiers named inside the CHECK constraints of a
// CREATE TABLE statement, column-level or table-level. An identifier counts if it appears anywhere
// in a constraint's parentheses, which over-counts a column only mentioned beside another; that is
// fine for a lint asking "is this column constrained at all".
func checkedColumns(ddl string) map[string]bool {
	out := map[string]bool{}
	upper := strings.ToUpper(ddl)
	for i := 0; ; {
		at := strings.Index(upper[i:], "CHECK")
		if at < 0 {
			return out
		}
		start := strings.Index(upper[i+at:], "(")
		if start < 0 {
			return out
		}
		start += i + at
		depth, end := 0, start
		for ; end < len(ddl); end++ {
			if ddl[end] == '(' {
				depth++
			} else if ddl[end] == ')' {
				depth--
				if depth == 0 {
					break
				}
			}
		}
		for _, word := range strings.FieldsFunc(ddl[start:min(end, len(ddl))], func(r rune) bool {
			return r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r)
		}) {
			out[strings.ToLower(word)] = true
		}
		i = end
	}
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
