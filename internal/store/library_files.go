package store

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Header states of a library file.
const (
	LibraryHeaderPending = "pending"
	LibraryHeaderRead    = "read"
	LibraryHeaderFailed  = "failed"
)

// LibraryFile is one media file the scanner found under an in-use root (#2151 step 5).
type LibraryFile struct {
	// Path is the file as Loomarr opens it; Root is the scan root it was found under.
	Path string
	Root string
	// Size and ModifiedNs are the last walked stat, which together are the file's revision.
	Size       int64
	ModifiedNs int64
	// Identity digests what said what the file is when it was last written to the inventory.
	Identity string
	// Kind is what the file was last written as (movie, episode, extra), "" for a problem.
	Kind string
	// Header is LibraryHeaderPending, LibraryHeaderRead or LibraryHeaderFailed.
	Header string
	// Problem names why the file is skipped, "" for none.
	Problem string
	// Held marks a file a walk no longer found while that walk's removals wait for confirmation.
	// A held file is still present (zero MissingAt) until the removal is confirmed.
	Held      bool
	SeenAt    time.Time
	MissingAt time.Time
	// ItemID is the inventory item the file was last written as; SeriesID is an episode's series.
	// Either is "" until known.
	ItemID   string
	SeriesID string
}

// LibraryFileCounts counts the present files of every root.
type LibraryFileCounts struct {
	Files          int
	HeadersPending int
	Problems       int
	// Held counts held files by root and kind.
	Held []LibraryHeldCount
}

// LibraryHeldCount is how many files of one kind a root's held removal would remove.
type LibraryHeldCount struct {
	Root  string
	Kind  string
	Count int
}

// LibraryFileStore keeps the scanner's file table.
type LibraryFileStore interface {
	// LibraryFiles reads every file row of one root, missing ones included, in path order.
	LibraryFiles(ctx context.Context, root string) ([]LibraryFile, error)
	// SaveLibraryFiles writes rows whole, in one transaction.
	SaveLibraryFiles(ctx context.Context, files []LibraryFile) error
	// LibraryFileRoots lists every root that has file rows.
	LibraryFileRoots(ctx context.Context) ([]string, error)
	// DeleteLibraryFiles forgets every file row of a root.
	DeleteLibraryFiles(ctx context.Context, root string) error
	// SetLibraryRootScanned records that a walk of the root completed at `at`.
	SetLibraryRootScanned(ctx context.Context, root string, at time.Time) error
	// LibraryFileCounts counts present files across roots.
	LibraryFileCounts(ctx context.Context) (LibraryFileCounts, error)
}

func (s *sqlStore) LibraryFiles(ctx context.Context, root string) ([]LibraryFile, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT path, root, size, modified_ns, identity, kind, header,
		problem, held, seen_at, missing_at, item_id, series_id FROM library_files WHERE root = ? ORDER BY path`), root)
	if err != nil {
		return nil, fmt.Errorf("list library files: %w", err)
	}
	var out []LibraryFile
	for rows.Next() {
		var f LibraryFile
		var seen, missing int64
		if err := rows.Scan(&f.Path, &f.Root, &f.Size, &f.ModifiedNs, &f.Identity, &f.Kind, &f.Header, &f.Problem,
			&f.Held, &seen, &missing, &f.ItemID, &f.SeriesID); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("list library files: %w", err)
		}
		f.SeenAt = time.UnixMilli(seen).UTC()
		if missing > 0 {
			f.MissingAt = time.UnixMilli(missing).UTC()
		}
		out = append(out, f)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("list library files: %w", err)
	}
	return out, nil
}

func (s *sqlStore) SaveLibraryFiles(ctx context.Context, files []LibraryFile) error {
	if len(files) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save library files: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, s.ph(`INSERT INTO library_files
		(path, root, size, modified_ns, identity, kind, header, problem, held, seen_at, missing_at, item_id, series_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (path) DO UPDATE SET root = excluded.root, size = excluded.size,
		modified_ns = excluded.modified_ns, identity = excluded.identity, kind = excluded.kind,
		header = excluded.header, problem = excluded.problem, held = excluded.held,
		seen_at = excluded.seen_at, missing_at = excluded.missing_at,
		item_id = excluded.item_id, series_id = excluded.series_id`))
	if err != nil {
		return fmt.Errorf("save library files: %w", err)
	}
	defer func() { _ = stmt.Close() }()
	for _, f := range files {
		if f.Path == "" || f.Root == "" || f.SeenAt.IsZero() {
			return fmt.Errorf("save library files: path, root and seen time are required")
		}
		header := f.Header
		if header == "" {
			header = LibraryHeaderPending
		}
		var missing int64
		if !f.MissingAt.IsZero() {
			missing = f.MissingAt.UnixMilli()
		}
		if _, err := stmt.ExecContext(ctx, f.Path, f.Root, f.Size, f.ModifiedNs, f.Identity, f.Kind, header,
			f.Problem, f.Held, f.SeenAt.UnixMilli(), missing, f.ItemID, f.SeriesID); err != nil {
			return fmt.Errorf("save library file: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save library files: %w", err)
	}
	return nil
}

func (s *sqlStore) LibraryFileRoots(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT root FROM library_files ORDER BY root`)
	if err != nil {
		return nil, fmt.Errorf("list library file roots: %w", err)
	}
	var out []string
	for rows.Next() {
		var root string
		if err := rows.Scan(&root); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("list library file roots: %w", err)
		}
		out = append(out, root)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("list library file roots: %w", err)
	}
	return out, nil
}

func (s *sqlStore) DeleteLibraryFiles(ctx context.Context, root string) error {
	if _, err := s.db.ExecContext(ctx, s.ph(`DELETE FROM library_files WHERE root = ?`), root); err != nil {
		return fmt.Errorf("delete library files: %w", err)
	}
	return nil
}

func (s *sqlStore) SetLibraryRootScanned(ctx context.Context, root string, at time.Time) error {
	result, err := s.db.ExecContext(ctx, s.ph(`UPDATE library_roots SET scanned_at = ? WHERE path = ?`),
		at.UnixMilli(), root)
	if err != nil {
		return fmt.Errorf("set library root scanned: %w", err)
	}
	if n, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("set library root scanned: %w", err)
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *sqlStore) LibraryFileCounts(ctx context.Context) (LibraryFileCounts, error) {
	var c LibraryFileCounts
	err := s.db.QueryRowContext(ctx, s.ph(`SELECT
		COUNT(*),
		COALESCE(SUM(CASE WHEN header = ? AND problem = '' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN problem <> '' THEN 1 ELSE 0 END), 0)
		FROM library_files WHERE missing_at = 0`), LibraryHeaderPending).
		Scan(&c.Files, &c.HeadersPending, &c.Problems)
	if err != nil {
		return c, fmt.Errorf("count library files: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT root, kind, COUNT(*) FROM library_files
		WHERE held AND missing_at = 0 GROUP BY root, kind ORDER BY root, kind`)
	if err != nil {
		return c, fmt.Errorf("count held library files: %w", err)
	}
	for rows.Next() {
		var h LibraryHeldCount
		if err := rows.Scan(&h.Root, &h.Kind, &h.Count); err != nil {
			_ = rows.Close()
			return c, fmt.Errorf("count held library files: %w", err)
		}
		c.Held = append(c.Held, h)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return c, fmt.Errorf("count held library files: %w", err)
	}
	return c, nil
}
