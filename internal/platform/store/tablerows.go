package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// TableRowStore records how many rows each table holds, once a day (#2793): retention you can
// watch. A table that only grows shows as a rising line in database_table_rows.
type TableRowStore interface {
	// RecordTableRows counts every table's rows and keeps the counts as at's UTC day, replacing any
	// earlier count that day, then forgets days before `before`. It reports how many tables it
	// counted.
	RecordTableRows(ctx context.Context, at, before time.Time) (int, error)
}

// tablesNotCounted are the tables whose counts say nothing about retention: the migration
// ledger, and the history itself.
var tablesNotCounted = []string{"goose_db_version", "database_table_rows"}

func (s *sqlStore) RecordTableRows(ctx context.Context, at, before time.Time) (int, error) {
	tables, err := s.countedTables(ctx)
	if err != nil {
		return 0, fmt.Errorf("record table rows: %w", err)
	}
	if len(tables) == 0 {
		return 0, nil
	}
	counts := make(map[string]int64, len(tables))
	selects := make([]string, len(tables))
	for i, table := range tables {
		selects[i] = fmt.Sprintf(`SELECT %s, COUNT(*) FROM %s`, quoteLiteral(table), quoteIdent(table))
	}
	// One statement, so every count comes from the same snapshot.
	rows, err := s.db.QueryContext(ctx, `/* plan: reads whole */ `+strings.Join(selects, " UNION ALL "))
	if err != nil {
		return 0, fmt.Errorf("record table rows: count: %w", err)
	}
	for rows.Next() {
		var table string
		var n int64
		if err := rows.Scan(&table, &n); err != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("record table rows: scan: %w", err)
		}
		counts[table] = n
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return 0, fmt.Errorf("record table rows: count: %w", err)
	}

	day := Epoch(at.UTC().Truncate(24 * time.Hour))
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("record table rows: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, table := range tables {
		if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO database_table_rows (recorded_at, table_name, row_count)
			VALUES (?, ?, ?) ON CONFLICT (recorded_at, table_name) DO UPDATE SET row_count = excluded.row_count`),
			day, table, counts[table]); err != nil {
			return 0, fmt.Errorf("record table rows: %s: %w", table, err)
		}
	}
	if _, err := tx.ExecContext(ctx, s.ph(`DELETE FROM database_table_rows WHERE recorded_at < ?`), Epoch(before)); err != nil {
		return 0, fmt.Errorf("record table rows: forget old days: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("record table rows: %w", err)
	}
	return len(tables), nil
}

// countedTables lists the application's tables (River's included) in name order.
func (s *sqlStore) countedTables(ctx context.Context) ([]string, error) {
	query := `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite\_%' ESCAPE '\' ORDER BY name`
	if s.dialect == DialectPostgres {
		query = `SELECT table_name FROM information_schema.tables
			WHERE table_schema = current_schema() AND table_type = 'BASE TABLE' ORDER BY table_name`
	}
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		counted := true
		for _, skip := range tablesNotCounted {
			counted = counted && name != skip
		}
		if counted {
			tables = append(tables, name)
		}
	}
	return tables, rows.Err()
}
