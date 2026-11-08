package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// sqliteIncrementalVacuum is PRAGMA auto_vacuum's INCREMENTAL mode: freed pages are tracked so
// PRAGMA incremental_vacuum can hand them back to the disk later, without rewriting the file.
const sqliteIncrementalVacuum = 2

// ensureIncrementalVacuum puts a SQLite database in auto_vacuum=INCREMENTAL (#2446).
//
// SQLite never shrinks a file by itself: pages freed by deleted rows go on a free list and are
// reused, but the file keeps its high-water size. The household database had been purging
// diagnostics for months and was 95% free pages, about 1.7 GB of a 1.8 GB file. A new database takes
// the mode at once; an existing one needs a single full VACUUM to switch, which this runs on the
// first open after upgrade and never again. Its cost is rewriting the live pages only (about 90 MB
// on that install), and it needs about that much free disk while it runs.
//
// It runs on one dedicated connection because VACUUM can't run inside a transaction, and before
// anything else uses the store, so nothing holds a read that VACUUM would wait on.
func ensureIncrementalVacuum(ctx context.Context, db *sql.DB) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	mode, err := sqliteAutoVacuum(ctx, conn)
	if err != nil || mode == sqliteIncrementalVacuum {
		return err
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA auto_vacuum = INCREMENTAL"); err != nil {
		return fmt.Errorf("set auto_vacuum: %w", err)
	}
	if mode, err = sqliteAutoVacuum(ctx, conn); err != nil || mode == sqliteIncrementalVacuum {
		return err // an empty database takes the mode without a VACUUM
	}
	if _, err := conn.ExecContext(ctx, "VACUUM"); err != nil {
		return fmt.Errorf("vacuum to switch auto_vacuum: %w", err)
	}
	return nil
}

func sqliteAutoVacuum(ctx context.Context, conn *sql.Conn) (int, error) {
	var mode int
	err := conn.QueryRowContext(ctx, "PRAGMA auto_vacuum").Scan(&mode)
	return mode, err
}

// Tidy hands the pages deleted rows left free back to the disk, returning how many bytes, and
// refreshes the planner's statistics. The nightly housekeeping pass ends with it, right after its
// purges free the most pages; Postgres does both itself (autovacuum).
func (s *sqlStore) Tidy(ctx context.Context) (freedBytes int64, err error) {
	if s.dialect != DialectSQLite {
		return 0, errors.New("Tidy: not a SQLite store")
	}
	var free, pageSize int64
	if err := s.db.QueryRowContext(ctx, "SELECT freelist_count, page_size FROM pragma_freelist_count, pragma_page_size").
		Scan(&free, &pageSize); err != nil {
		return 0, err
	}
	if _, err := s.db.ExecContext(ctx, "PRAGMA incremental_vacuum"); err != nil {
		return 0, fmt.Errorf("incremental vacuum: %w", err)
	}
	if err := optimizeSQLite(ctx, s.db); err != nil {
		return 0, fmt.Errorf("optimize: %w", err)
	}
	return free * pageSize, nil
}

// DatabaseTidier exposes Tidy for a SQLite store, nil otherwise.
func DatabaseTidier(st Store) interface {
	Tidy(ctx context.Context) (freedBytes int64, err error)
} {
	s, ok := adapterOf(st)
	if !ok || s.dialect != DialectSQLite {
		return nil
	}
	return s
}
