package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// LibraryCatalogKey names one title in a catalog: the provider ("tmdb"), the kind it is fetched as
// (movie or series) and its id there. The zero key is none.
type LibraryCatalogKey struct {
	Provider   string
	Kind       string
	ExternalID string
}

// String is the key as library_items.catalog holds it: "tmdb:movie:603", or "" for none.
func (k LibraryCatalogKey) String() string {
	if k.Provider == "" || k.Kind == "" || k.ExternalID == "" {
		return ""
	}
	return k.Provider + ":" + k.Kind + ":" + k.ExternalID
}

// ParseLibraryCatalogKey reads a key String wrote; false for "" or anything else.
func ParseLibraryCatalogKey(s string) (LibraryCatalogKey, bool) {
	parts := strings.SplitN(s, ":", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return LibraryCatalogKey{}, false
	}
	return LibraryCatalogKey{Provider: parts[0], Kind: parts[1], ExternalID: parts[2]}, true
}

// LibraryCatalogDue narrows DueLibraryCatalog: titles never fetched, then titles last fetched
// before StaleBefore, oldest first, leaving out any whose failed fetch waits past Now.
type LibraryCatalogDue struct {
	Now         time.Time
	StaleBefore time.Time
	// New and Stale cap each part; zero takes none of it.
	New   int
	Stale int
}

// LibraryFilePath is one file the scanner keeps, by its root and absolute path.
type LibraryFilePath struct {
	Root string
	Path string
}

// LibraryCatalogStore caches what catalogs say about the scanner's titles (#2151, decision 20).
// The walk reads it; only the enricher fetches.
type LibraryCatalogStore interface {
	// WantLibraryCatalog notes that a title has these catalog keys, keeping what is cached.
	WantLibraryCatalog(ctx context.Context, keys []LibraryCatalogKey) error
	// LibraryCatalogRecord is the cached record (JSON) and whether one was fetched. A fetched title
	// the catalog does not know has an empty record.
	LibraryCatalogRecord(ctx context.Context, key LibraryCatalogKey) (string, bool, error)
	// DueLibraryCatalog lists the keys to fetch now.
	DueLibraryCatalog(ctx context.Context, due LibraryCatalogDue) ([]LibraryCatalogKey, error)
	// CountLibraryCatalog counts every cached key.
	CountLibraryCatalog(ctx context.Context) (int, error)
	// SaveLibraryCatalog stores a fetched record and reports whether it differs from the one held.
	SaveLibraryCatalog(ctx context.Context, key LibraryCatalogKey, record string, at time.Time) (bool, error)
	// FailLibraryCatalog holds a key back from fetching until retryAt.
	FailLibraryCatalog(ctx context.Context, key LibraryCatalogKey, retryAt time.Time) error
	// RewriteLibraryCatalogFiles forgets the identity digest of every present file of each title
	// whose details come from key (a series' episodes too), so the scanner writes them again, and
	// lists them.
	RewriteLibraryCatalogFiles(ctx context.Context, key LibraryCatalogKey) ([]LibraryFilePath, error)
}

func (s *sqlStore) WantLibraryCatalog(ctx context.Context, keys []LibraryCatalogKey) error {
	for _, k := range keys {
		if k.String() == "" {
			return fmt.Errorf("want library catalog: provider, kind and id are required")
		}
		if _, err := s.db.ExecContext(ctx, s.ph(`INSERT INTO library_catalog (provider, kind, external_id)
			VALUES (?, ?, ?) ON CONFLICT (provider, kind, external_id) DO NOTHING`),
			k.Provider, k.Kind, k.ExternalID); err != nil {
			return fmt.Errorf("want library catalog: %w", err)
		}
	}
	return nil
}

func (s *sqlStore) LibraryCatalogRecord(ctx context.Context, key LibraryCatalogKey) (string, bool, error) {
	var record string
	var fetched int64
	err := s.db.QueryRowContext(ctx, s.ph(`SELECT record, fetched_at FROM library_catalog
		WHERE provider = ? AND kind = ? AND external_id = ?`), key.Provider, key.Kind, key.ExternalID).Scan(&record, &fetched)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("library catalog record: %w", err)
	}
	return record, fetched > 0, nil
}

func (s *sqlStore) DueLibraryCatalog(ctx context.Context, due LibraryCatalogDue) ([]LibraryCatalogKey, error) {
	now := due.Now.UnixMilli()
	var out []LibraryCatalogKey
	if due.New > 0 {
		keys, err := s.catalogKeys(ctx, `SELECT provider, kind, external_id FROM library_catalog
			WHERE fetched_at = 0 AND retry_at <= ? ORDER BY provider, kind, external_id LIMIT ?`, now, due.New)
		if err != nil {
			return nil, err
		}
		out = append(out, keys...)
	}
	if due.Stale > 0 {
		keys, err := s.catalogKeys(ctx, `SELECT provider, kind, external_id FROM library_catalog
			WHERE fetched_at > 0 AND fetched_at < ? AND retry_at <= ? ORDER BY fetched_at LIMIT ?`,
			due.StaleBefore.UnixMilli(), now, due.Stale)
		if err != nil {
			return nil, err
		}
		out = append(out, keys...)
	}
	return out, nil
}

func (s *sqlStore) catalogKeys(ctx context.Context, query string, args ...any) ([]LibraryCatalogKey, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(query), args...)
	if err != nil {
		return nil, fmt.Errorf("due library catalog: %w", err)
	}
	var out []LibraryCatalogKey
	for rows.Next() {
		var k LibraryCatalogKey
		if err := rows.Scan(&k.Provider, &k.Kind, &k.ExternalID); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("due library catalog: %w", err)
		}
		out = append(out, k)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("due library catalog: %w", err)
	}
	return out, nil
}

func (s *sqlStore) CountLibraryCatalog(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_catalog`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count library catalog: %w", err)
	}
	return n, nil
}

func (s *sqlStore) SaveLibraryCatalog(ctx context.Context, key LibraryCatalogKey, record string, at time.Time) (bool, error) {
	if key.String() == "" {
		return false, fmt.Errorf("save library catalog: provider, kind and id are required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("save library catalog: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var held string
	err = tx.QueryRowContext(ctx, s.ph(`SELECT record FROM library_catalog
		WHERE provider = ? AND kind = ? AND external_id = ?`), key.Provider, key.Kind, key.ExternalID).Scan(&held)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("save library catalog: %w", err)
	}
	if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO library_catalog (provider, kind, external_id, record, fetched_at, retry_at)
		VALUES (?, ?, ?, ?, ?, 0)
		ON CONFLICT (provider, kind, external_id) DO UPDATE SET record = excluded.record,
		fetched_at = excluded.fetched_at, retry_at = 0`),
		key.Provider, key.Kind, key.ExternalID, record, at.UnixMilli()); err != nil {
		return false, fmt.Errorf("save library catalog: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("save library catalog: %w", err)
	}
	return held != record, nil
}

func (s *sqlStore) FailLibraryCatalog(ctx context.Context, key LibraryCatalogKey, retryAt time.Time) error {
	if _, err := s.db.ExecContext(ctx, s.ph(`UPDATE library_catalog SET retry_at = ?
		WHERE provider = ? AND kind = ? AND external_id = ?`),
		retryAt.UnixMilli(), key.Provider, key.Kind, key.ExternalID); err != nil {
		return fmt.Errorf("fail library catalog: %w", err)
	}
	return nil
}

func (s *sqlStore) RewriteLibraryCatalogFiles(ctx context.Context, key LibraryCatalogKey) ([]LibraryFilePath, error) {
	catalog := key.String()
	if catalog == "" {
		return nil, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("rewrite library catalog files: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	const titles = `(SELECT id FROM library_items WHERE catalog = ?)`
	rows, err := tx.QueryContext(ctx, s.ph(`SELECT root, path FROM library_files
		WHERE missing_at = 0 AND (item_id IN `+titles+` OR series_id IN `+titles+`) ORDER BY path`), catalog, catalog)
	if err != nil {
		return nil, fmt.Errorf("rewrite library catalog files: %w", err)
	}
	var out []LibraryFilePath
	for rows.Next() {
		var f LibraryFilePath
		if err := rows.Scan(&f.Root, &f.Path); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("rewrite library catalog files: %w", err)
		}
		out = append(out, f)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("rewrite library catalog files: %w", err)
	}
	if _, err := tx.ExecContext(ctx, s.ph(`UPDATE library_files SET identity = ''
		WHERE missing_at = 0 AND (item_id IN `+titles+` OR series_id IN `+titles+`)`), catalog, catalog); err != nil {
		return nil, fmt.Errorf("rewrite library catalog files: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("rewrite library catalog files: %w", err)
	}
	return out, nil
}
