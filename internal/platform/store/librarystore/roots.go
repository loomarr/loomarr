package librarystore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

// LibraryRoot is one local folder Loomarr may walk for media (#2151 decision 6).
type LibraryRoot struct {
	// Path is the folder as Loomarr opens it, absolute and cleaned.
	Path string
	// InUse is the maintainer's confirmation. A discovered folder starts unconfirmed; one typed by
	// hand starts confirmed.
	InUse bool
	// Readable and CheckedAt are the last readability check; a zero CheckedAt means never checked.
	Readable  bool
	CheckedAt time.Time
	// Empty is a readable root that held nothing at the last check: most often a share that isn't
	// mounted, whose mount point is an empty folder.
	Empty bool
	// ByHand marks a folder the maintainer typed rather than one an authority reported.
	ByHand  bool
	AddedAt time.Time
	// ScannedAt is when a walk of the root last completed; zero means never.
	ScannedAt time.Time
}

// LibraryRootPath is where one authority (a media server or an arr) calls a root: one row of the
// generalized path map. RootPath is "" when no local folder was found for it.
type LibraryRootPath struct {
	Authority     string
	AuthorityPath string
	RootPath      string
	// ByHand marks a mapping the maintainer set; discovery never rewrites it.
	ByHand bool
	SeenAt time.Time
}

// LibraryRootCheck is one readability check of a root.
type LibraryRootCheck struct {
	Path     string
	Readable bool
	// Empty is a readable root with nothing in it.
	Empty bool
}

// LibraryRootFlip is what one check of a root changed from the check before it.
type LibraryRootFlip struct {
	// Readable: the root went from opening to not, or back.
	Readable bool
	// Empty: the root went from holding something to holding nothing, or back.
	Empty bool
}

// LibraryRootDiscovery is the outcome of one discovery pass.
type LibraryRootDiscovery struct {
	// Found is every folder the answering authorities reported, each with the local folder derived
	// for it ("" when none was found).
	Found []LibraryRootPath
	// Complete names the authorities that answered. Their rows missing from Found are dropped; an
	// authority that failed keeps its rows, so an outage never forgets a folder.
	Complete []string
	// Checks are readability checks of roots, applied after Found.
	Checks []LibraryRootCheck
}

// LibraryRoots is every root and the whole path map, in path order.
type LibraryRoots struct {
	Roots []LibraryRoot
	Paths []LibraryRootPath
}

// LibraryRootStore keeps the scan roots and the path map that names them (#2151 decision 6).
type LibraryRootStore interface {
	// LibraryRoots reads every root and path-map row.
	LibraryRoots(ctx context.Context) (LibraryRoots, error)
	// RecordLibraryRootDiscovery applies one discovery pass atomically. A newly derived root is
	// added unconfirmed; an existing root keeps its confirmation. A hand-set mapping is never
	// rewritten. A root that is neither in use, typed by hand, nor named by any mapping is dropped.
	RecordLibraryRootDiscovery(ctx context.Context, d LibraryRootDiscovery, at time.Time) error
	// AddLibraryRoot records a folder the maintainer typed, confirmed. Adding an existing root
	// confirms it.
	AddLibraryRoot(ctx context.Context, check LibraryRootCheck, at time.Time) error
	// SetLibraryRootInUse confirms or releases a root. Releasing a hand-typed root that no mapping
	// names removes it. ErrNotFound when the root does not exist.
	SetLibraryRootInUse(ctx context.Context, path string, inUse bool) error
	// SetLibraryRootPath maps an authority's folder to a local root by hand, adding the root
	// (unconfirmed) when it is new. An empty rootPath hands the row back to discovery.
	// ErrNotFound when the authority never reported that folder.
	SetLibraryRootPath(ctx context.Context, authority, authorityPath, rootPath string, at time.Time) error
	// CheckLibraryRoot records one readability check of a root, as a scan pass makes it, and
	// reports what it flipped: readable and not, empty and not. A root never checked before does
	// not flip. ErrNotFound when the root does not exist.
	CheckLibraryRoot(ctx context.Context, check LibraryRootCheck, at time.Time) (LibraryRootFlip, error)
}

func (s *Store) LibraryRoots(ctx context.Context) (LibraryRoots, error) {
	var out LibraryRoots
	rows, err := s.h.QueryContext(ctx, `SELECT path, in_use, readable, empty, checked_at_ms, by_hand, added_at_ms, scanned_at_ms
		FROM library_roots ORDER BY path`)
	if err != nil {
		return out, fmt.Errorf("list library roots: %w", err)
	}
	for rows.Next() {
		var r LibraryRoot
		var checked, added, scanned int64
		if err := rows.Scan(&r.Path, &r.InUse, &r.Readable, &r.Empty, &checked, &r.ByHand, &added, &scanned); err != nil {
			_ = rows.Close()
			return out, fmt.Errorf("list library roots: %w", err)
		}
		if checked > 0 {
			r.CheckedAt = time.UnixMilli(checked).UTC()
		}
		if scanned > 0 {
			r.ScannedAt = time.UnixMilli(scanned).UTC()
		}
		r.AddedAt = time.UnixMilli(added).UTC()
		out.Roots = append(out.Roots, r)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return out, fmt.Errorf("list library roots: %w", err)
	}
	rows, err = s.h.QueryContext(ctx, `SELECT authority, authority_path, root_path, by_hand, seen_at_ms
		FROM library_root_paths ORDER BY authority, authority_path`)
	if err != nil {
		return out, fmt.Errorf("list library root paths: %w", err)
	}
	for rows.Next() {
		var p LibraryRootPath
		var seen int64
		if err := rows.Scan(&p.Authority, &p.AuthorityPath, &p.RootPath, &p.ByHand, &seen); err != nil {
			_ = rows.Close()
			return out, fmt.Errorf("list library root paths: %w", err)
		}
		p.SeenAt = time.UnixMilli(seen).UTC()
		out.Paths = append(out.Paths, p)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return out, fmt.Errorf("list library root paths: %w", err)
	}
	return out, nil
}

func (s *Store) RecordLibraryRootDiscovery(ctx context.Context, d LibraryRootDiscovery, at time.Time) error {
	if at.IsZero() {
		return fmt.Errorf("record library root discovery: time is required")
	}
	ms := at.UnixMilli()
	return s.libraryRootTx(ctx, "record library root discovery", func(tx storedb.Tx) error {
		for _, f := range d.Found {
			if f.Authority == "" || f.AuthorityPath == "" {
				return fmt.Errorf("found path needs an authority and a path")
			}
			if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO library_root_paths
				(authority, authority_path, root_path, by_hand, seen_at_ms) VALUES (?, ?, ?, FALSE, ?)
				ON CONFLICT (authority, authority_path) DO UPDATE SET seen_at_ms = excluded.seen_at_ms,
				root_path = CASE WHEN library_root_paths.by_hand THEN library_root_paths.root_path
				ELSE excluded.root_path END`), f.Authority, f.AuthorityPath, f.RootPath, ms); err != nil {
				return err
			}
		}
		for _, authority := range d.Complete {
			if _, err := tx.ExecContext(ctx, s.ph(`DELETE FROM library_root_paths
				WHERE authority = ? AND seen_at_ms < ? AND NOT by_hand`), authority, ms); err != nil {
				return err
			}
		}
		// Every mapped root exists as a row; a new one starts unconfirmed.
		if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO library_roots (path, added_at_ms)
			SELECT DISTINCT root_path, CAST(? AS BIGINT) FROM library_root_paths WHERE root_path <> ''
			ON CONFLICT (path) DO NOTHING`), ms); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM library_roots WHERE NOT in_use AND NOT by_hand
			AND path NOT IN (SELECT root_path FROM library_root_paths)`); err != nil {
			return err
		}
		return s.applyLibraryRootChecks(ctx, tx, d.Checks, ms)
	})
}

func (s *Store) applyLibraryRootChecks(ctx context.Context, tx storedb.Tx, checks []LibraryRootCheck, ms int64) error {
	for _, c := range checks {
		if _, err := tx.ExecContext(ctx, s.ph(`UPDATE library_roots SET readable = ?, empty = ?, checked_at_ms = ?
			WHERE path = ?`), c.Readable, c.Readable && c.Empty, ms, c.Path); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CheckLibraryRoot(ctx context.Context, check LibraryRootCheck, at time.Time) (LibraryRootFlip, error) {
	var flipped LibraryRootFlip
	if check.Path == "" || at.IsZero() {
		return flipped, fmt.Errorf("check library root: path and time are required")
	}
	err := s.libraryRootTx(ctx, "check library root", func(tx storedb.Tx) error {
		var readable, empty bool
		var checkedAt int64
		err := tx.QueryRowContext(ctx, s.ph(`SELECT readable, empty, checked_at_ms FROM library_roots WHERE path = ?`), check.Path).
			Scan(&readable, &empty, &checkedAt)
		if errors.Is(err, sql.ErrNoRows) {
			return storedb.ErrNotFound
		}
		if err != nil {
			return err
		}
		if checkedAt != 0 {
			flipped.Readable = readable != check.Readable
			flipped.Empty = empty != (check.Readable && check.Empty)
		}
		return s.applyLibraryRootChecks(ctx, tx, []LibraryRootCheck{check}, at.UnixMilli())
	})
	return flipped, err
}

func (s *Store) AddLibraryRoot(ctx context.Context, check LibraryRootCheck, at time.Time) error {
	if check.Path == "" || at.IsZero() {
		return fmt.Errorf("add library root: path and time are required")
	}
	ms := at.UnixMilli()
	return s.libraryRootTx(ctx, "add library root", func(tx storedb.Tx) error {
		if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO library_roots (path, in_use, by_hand, added_at_ms)
			VALUES (?, TRUE, TRUE, ?) ON CONFLICT (path) DO UPDATE SET in_use = TRUE`), check.Path, ms); err != nil {
			return err
		}
		return s.applyLibraryRootChecks(ctx, tx, []LibraryRootCheck{check}, ms)
	})
}

func (s *Store) SetLibraryRootInUse(ctx context.Context, path string, inUse bool) error {
	return s.libraryRootTx(ctx, "set library root in use", func(tx storedb.Tx) error {
		result, err := tx.ExecContext(ctx, s.ph(`UPDATE library_roots SET in_use = ? WHERE path = ?`), inUse, path)
		if err != nil {
			return err
		}
		if n, err := result.RowsAffected(); err != nil {
			return err
		} else if n == 0 {
			return storedb.ErrNotFound
		}
		_, err = tx.ExecContext(ctx, s.ph(`DELETE FROM library_roots WHERE path = ? AND NOT in_use AND by_hand
			AND path NOT IN (SELECT root_path FROM library_root_paths)`), path)
		return err
	})
}

func (s *Store) SetLibraryRootPath(ctx context.Context, authority, authorityPath, rootPath string, at time.Time) error {
	if at.IsZero() {
		return fmt.Errorf("set library root path: time is required")
	}
	return s.libraryRootTx(ctx, "set library root path", func(tx storedb.Tx) error {
		result, err := tx.ExecContext(ctx, s.ph(`UPDATE library_root_paths SET root_path = ?, by_hand = ?
			WHERE authority = ? AND authority_path = ?`), rootPath, rootPath != "", authority, authorityPath)
		if err != nil {
			return err
		}
		if n, err := result.RowsAffected(); err != nil {
			return err
		} else if n == 0 {
			return storedb.ErrNotFound
		}
		if rootPath == "" {
			return nil
		}
		_, err = tx.ExecContext(ctx, s.ph(`INSERT INTO library_roots (path, added_at_ms) VALUES (?, ?)
			ON CONFLICT (path) DO NOTHING`), rootPath, at.UnixMilli())
		return err
	})
}

func (s *Store) libraryRootTx(ctx context.Context, op string, write func(storedb.Tx) error) error {
	tx, err := s.h.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%s: begin: %w", op, err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := write(tx); err != nil {
		if errors.Is(err, storedb.ErrNotFound) {
			return storedb.ErrNotFound
		}
		return fmt.Errorf("%s: %w", op, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%s: commit: %w", op, err)
	}
	return nil
}
