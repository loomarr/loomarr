package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Data migration: copy an established SQLite install into an empty Postgres (§18, V11).
//
// The contract, in one line: the SOURCE IS ONLY EVER READ. Every failure mode below ends
// with the operator still running on the database they started on — which is what makes
// "roll back by reverting one config line" true rather than aspirational. Nothing here
// writes to, vacuums, or locks the source beyond the read transaction.
//
// Three decisions worth stating, because each has a plausible-looking alternative:
//
//  1. **The table list is read from the DESTINATION catalog, never hardcoded.** goose has
//     just built the destination from the same embedded migrations, so its live catalog
//     *is* the schema by construction. A hardcoded list is the failure this repo has
//     already had once — `postgres_test.go`'s TRUNCATE list drifted to 8 of 10 tables — and
//     a migrator that silently skips a table nobody remembered is far worse than a test
//     that skips one.
//
//  2. **Every value is scanned as a string and coerced by the DESTINATION's column type.**
//     The dialects are deliberately near-identical (all timestamps are epoch BIGINT) but
//     not identical — `users.disabled` is INTEGER in SQLite and BOOLEAN in Postgres.
//     Scanning as NullString is what makes that a non-event: both drivers parse a string
//     into the target column's type, so "0"/"1" lands in a BOOLEAN correctly without any
//     special case (verified — the BOOL branch in `coerce` below is a second belt, not
//     the thing doing the work).
//
//     **Binary is the real exception.** `channel_icons.bytes` is BLOB/BYTEA, and routing retired-ok
//     it through a Go string corrupts every byte that is not valid UTF-8. That branch IS
//     load-bearing and has a test that fails without it. Coercing by destination type is
//     what keeps both cases — and any future divergence — in one rule instead of a list
//     of remembered exceptions.
//
//  3. **Row counts are compared per table, and a mismatch ABORTS before switchover.**
//     Copying and verifying are separate phases for a reason: a copy that reports success
//     is a claim, and parity is the evidence.
//
// Not in scope: Postgres→SQLite. The direction people migrate is toward the bigger
// database, and the reverse is served by the backup file plus reverting the config line.

// TableStat is one table's progress — the unit the UI's per-table bars render, and the
// unit parity is asserted in.
type TableStat struct {
	Table  string `json:"table"`
	Source int64  `json:"source"`
	Copied int64  `json:"copied"`
}

// MigrationProgress is a snapshot of a running (or finished) copy.
type MigrationProgress struct {
	Tables []TableStat `json:"tables"`
	// Table currently being copied; empty once finished.
	Current string `json:"current,omitempty"`
	Done    bool   `json:"done"`
	Err     string `json:"error,omitempty"`
}

// copyBatch is how many rows are read and inserted per round trip. Large enough that a
// 100k-row table isn't 100k round trips, small enough that a wide table (channel_icons retired-ok
// carries image BYTEA) doesn't build a multi-hundred-MB statement.
const copyBatch = 500

// MigrateData copies every table from src into dst and verifies row-count parity.
//
// dst MUST already be migrated (schema present) and empty of Loomarr rows — Preflight
// checks both. onProgress, if non-nil, is called as each table starts and finishes; it
// must not block (the caller publishes to an SSE bus, which drops rather than blocks).
//
// On any error the destination is left partially written and the source untouched. The
// caller's remedy is to wipe the destination and retry — which is exactly what the UI's
// retry does, and why the failure copy says the SQLite database was only read from.
func MigrateData(ctx context.Context, src, dst Store, onProgress func(MigrationProgress)) (MigrationProgress, error) {
	s, ok := src.(*sqlStore)
	if !ok {
		return MigrationProgress{}, errors.New("migrate: source is not a SQL store")
	}
	d, ok := dst.(*sqlStore)
	if !ok {
		return MigrationProgress{}, errors.New("migrate: destination is not a SQL store")
	}
	if s.dialect == d.dialect {
		// Not a safety rule so much as a "you did not mean this": the only reason to run
		// this is to change backend.
		return MigrationProgress{}, fmt.Errorf("migrate: source and destination are both %s", s.dialect)
	}

	tables, err := userTables(ctx, d)
	if err != nil {
		return MigrationProgress{}, err
	}

	prog := MigrationProgress{Tables: make([]TableStat, 0, len(tables))}
	report := func() {
		if onProgress != nil {
			onProgress(prog)
		}
	}

	for i, table := range tables {
		n, err := countRows(ctx, s.db, table)
		if err != nil {
			return prog, fmt.Errorf("count %s: %w", table, err)
		}
		prog.Tables = append(prog.Tables, TableStat{Table: table, Source: n})
		prog.Current = table
		report()

		copied, err := copyTable(ctx, s, d, table, func(n int64) {
			prog.Tables[i].Copied = n
			report()
		})
		if err != nil {
			prog.Err = err.Error()
			report()
			return prog, fmt.Errorf("copy %s: %w", table, err)
		}
		prog.Tables[i].Copied = copied
	}

	prog.Current = ""
	prog.Done = true
	report()
	return prog, nil
}

// VerifyParity re-counts every table on BOTH sides and reports the tables that disagree.
//
// Deliberately re-counts rather than trusting MigrateData's tallies: the copy's numbers
// come from the same code path that did the copying, so reusing them would make the check
// self-confirming. Counting the destination independently is the only version of this
// that can actually catch a bad copy.
func VerifyParity(ctx context.Context, src, dst Store) ([]TableStat, error) {
	s, ok1 := src.(*sqlStore)
	d, ok2 := dst.(*sqlStore)
	if !ok1 || !ok2 {
		return nil, errors.New("verify: not SQL stores")
	}
	tables, err := userTables(ctx, d)
	if err != nil {
		return nil, err
	}
	stats := make([]TableStat, 0, len(tables))
	for _, table := range tables {
		sn, err := countRows(ctx, s.db, table)
		if err != nil {
			return nil, fmt.Errorf("count source %s: %w", table, err)
		}
		dn, err := countRows(ctx, d.db, table)
		if err != nil {
			return nil, fmt.Errorf("count destination %s: %w", table, err)
		}
		stats = append(stats, TableStat{Table: table, Source: sn, Copied: dn})
	}
	return stats, nil
}

// ParityMismatches returns the subset of stats whose counts disagree. Empty means MATCH.
func ParityMismatches(stats []TableStat) []TableStat {
	var bad []TableStat
	for _, st := range stats {
		if st.Source != st.Copied {
			bad = append(bad, st)
		}
	}
	return bad
}

// userTables lists Loomarr's tables from the live catalog in an order safe to insert in:
// a table always follows the tables it references.
//
// goose's own version table is excluded: the destination earns its own by being migrated,
// and copying the source's would assert a schema history the destination did not live
// through.
//
// ⚠ The ORDER is load-bearing, not cosmetic. `sessions.user_id REFERENCES users(id)` is
// declared NOT DEFERRABLE, so inserting a session before its user fails outright — and
// alphabetically `sessions` sorts first. The dependency order is read from the catalog
// (see fkParents) rather than hand-maintained for the same reason the table list is: a
// hardcoded order is a standing obligation on whoever adds the next foreign key, and this
// repo has already had one such list drift.
func userTables(ctx context.Context, s *sqlStore) ([]string, error) {
	var q string
	switch s.dialect {
	case DialectPostgres:
		q = `SELECT tablename FROM pg_tables WHERE schemaname = current_schema()`
	case DialectSQLite:
		q = `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`
	default:
		return nil, fmt.Errorf("userTables: unknown dialect %q", s.dialect)
	}
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if name == "goose_db_version" {
			continue
		}
		out = append(out, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Alphabetical first so the result is deterministic; the sort below preserves this
	// order among tables that do not constrain each other.
	sort.Strings(out)

	parents, err := fkParents(ctx, s)
	if err != nil {
		return nil, err
	}
	return topoSort(out, parents), nil
}

// fkParents maps each table to the tables it references. Postgres only: SQLite is never
// a migration destination (§18), and it is the destination's constraints that decide
// insert order.
func fkParents(ctx context.Context, s *sqlStore) (map[string][]string, error) {
	if s.dialect != DialectPostgres {
		return nil, nil
	}
	const q = `
SELECT child.relname, parent.relname
FROM pg_constraint c
JOIN pg_class child  ON child.oid  = c.conrelid
JOIN pg_class parent ON parent.oid = c.confrelid
WHERE c.contype = 'f'`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("read foreign keys: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string][]string{}
	for rows.Next() {
		var child, parent string
		if err := rows.Scan(&child, &parent); err != nil {
			return nil, err
		}
		if child == parent {
			continue // self-reference: no ordering constraint between tables
		}
		out[child] = append(out[child], parent)
	}
	return out, rows.Err()
}

// topoSort returns tables with every parent before its children, preserving the input
// order otherwise.
//
// A cycle cannot be resolved by ordering at all (it would need deferred constraints), so
// rather than failing the whole migration this emits the remaining tables in input order
// and lets the insert report the real constraint error. Loomarr's schema has no cycles;
// this is about failing informatively if one is ever added.
func topoSort(tables []string, parents map[string][]string) []string {
	known := make(map[string]bool, len(tables))
	for _, t := range tables {
		known[t] = true
	}
	out := make([]string, 0, len(tables))
	done := make(map[string]bool, len(tables))
	visiting := map[string]bool{}

	var visit func(string)
	visit = func(t string) {
		if done[t] || visiting[t] {
			return // already placed, or a cycle — see the doc comment
		}
		visiting[t] = true
		for _, p := range parents[t] {
			if known[p] {
				visit(p)
			}
		}
		delete(visiting, t)
		done[t] = true
		out = append(out, t)
	}
	for _, t := range tables {
		visit(t)
	}
	return out
}

func countRows(ctx context.Context, db *sql.DB, table string) (int64, error) {
	// The table name is an identifier and cannot be a placeholder. It is safe here
	// because it came from the catalog query above, never from user input — the only
	// values that reach this are names the database itself reported.
	var n int64
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+quoteIdent(table)).Scan(&n)
	return n, err
}

// copyTable streams one table across in batches, coercing each value to what the
// DESTINATION column expects.
func copyTable(ctx context.Context, src, dst *sqlStore, table string, onRows func(int64)) (int64, error) {
	// ⚠ Both from the DESTINATION — it is authoritative for the column set, the order,
	// and the types `coerce` targets. Reading types from the source would silently pick
	// SQLite's BLOB for `channel_icons.bytes` instead of Postgres's BYTEA; that column is retired-ok
	// binary, and the difference decides whether icons survive as bytes or as mangled
	// UTF-8.
	cols, types, err := describe(ctx, dst, table)
	if err != nil {
		return 0, err
	}
	if len(cols) == 0 {
		return 0, nil
	}

	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = quoteIdent(c)
	}
	sel := `SELECT ` + strings.Join(quoted, ", ") + ` FROM ` + quoteIdent(table)

	rows, err := src.db.QueryContext(ctx, sel)
	if err != nil {
		return 0, fmt.Errorf("read: %w", err)
	}
	defer func() { _ = rows.Close() }()

	tx, err := dst.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }() // no-op after a successful Commit

	var total int64
	batch := make([]any, 0, copyBatch*len(cols))
	values := 0

	flush := func() error {
		if values == 0 {
			return nil
		}
		stmt := insertSQL(dst, table, quoted, values)
		if _, err := tx.ExecContext(ctx, stmt, batch...); err != nil {
			return fmt.Errorf("write: %w", err)
		}
		batch = batch[:0]
		values = 0
		return nil
	}

	for rows.Next() {
		scan := make([]any, len(cols))
		nulls := make([]sql.NullString, len(cols))
		for i := range scan {
			scan[i] = &nulls[i]
		}
		if err := rows.Scan(scan...); err != nil {
			return total, err
		}
		for i := range cols {
			v, err := coerce(nulls[i], types[i])
			if err != nil {
				return total, fmt.Errorf("column %s: %w", cols[i], err)
			}
			batch = append(batch, v)
		}
		values++
		total++
		if values >= copyBatch {
			if err := flush(); err != nil {
				return total, err
			}
			if onRows != nil {
				onRows(total)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return total, err
	}
	if err := flush(); err != nil {
		return total, err
	}
	if err := tx.Commit(); err != nil {
		return total, fmt.Errorf("commit: %w", err)
	}
	if onRows != nil {
		onRows(total)
	}
	return total, nil
}

// insertSQL builds a multi-row INSERT with the destination's placeholder style.
func insertSQL(dst *sqlStore, table string, quotedCols []string, rowCount int) string {
	var b strings.Builder
	b.WriteString(`INSERT INTO `)
	b.WriteString(quoteIdent(table))
	b.WriteString(` (`)
	b.WriteString(strings.Join(quotedCols, ", "))
	b.WriteString(`) VALUES `)
	one := `(` + strings.TrimSuffix(strings.Repeat("?, ", len(quotedCols)), ", ") + `)`
	for i := 0; i < rowCount; i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(one)
	}
	return dst.ph(b.String())
}

// describe reads a table's column names and declared types, in ordinal order, via an
// empty result set. Used against the DESTINATION for both — it is authoritative for the
// column set, the order, and (critically) the types `coerce` targets.
func describe(ctx context.Context, s *sqlStore, table string) ([]string, []*sql.ColumnType, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT * FROM `+quoteIdent(table)+` WHERE 1 = 0`)
	if err != nil {
		return nil, nil, fmt.Errorf("describe %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	names, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	types, err := rows.ColumnTypes()
	if err != nil {
		return nil, nil, err
	}
	return names, types, nil
}

// coerce converts a scanned source value into what the destination column accepts.
//
// Everything is scanned as NullString — a lossless wire form for the epoch-int and text
// columns this schema is made of — and converted per DESTINATION type.
//
// ⚠ Only the binary branch is load-bearing. Both drivers happily parse "0"/"1" into a
// BOOLEAN and "1.5" into a DOUBLE PRECISION, so those branches are defensive. BYTEA/BLOB
// is different in kind: a []byte that goes through a Go string comes back corrupted for
// any byte that is not valid UTF-8, which for a PNG is most of them.
func coerce(v sql.NullString, ct *sql.ColumnType) (any, error) {
	if !v.Valid {
		return nil, nil
	}
	name := strings.ToUpper(ct.DatabaseTypeName())
	switch name {
	case "BOOL", "BOOLEAN":
		// Defensive, not required: pgx already parses "0"/"1" into a BOOLEAN. Kept so the
		// conversion is explicit rather than relying on a driver's string handling, and
		// so a future driver that is stricter does not turn into a data bug.
		switch strings.ToLower(v.String) {
		case "1", "true", "t":
			return true, nil
		case "0", "false", "f", "":
			return false, nil
		default:
			return nil, fmt.Errorf("not a boolean: %q", v.String)
		}
	case "BYTEA", "BLOB":
		return []byte(v.String), nil
	default:
		// Everything else — TEXT, BIGINT, INTEGER, DOUBLE PRECISION, REAL — is accepted
		// as a string by both drivers, which parse it into the column's type. Passing
		// the raw string avoids a float64 round-trip that could lose precision on an
		// epoch value.
		return v.String, nil
	}
}

// quoteIdent double-quotes an identifier. Both SQLite and Postgres accept the SQL-standard
// form. Embedded quotes are doubled — no Loomarr table has one, but building an identifier
// that ignored them would be a SQL-injection shape even if unreachable today.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
