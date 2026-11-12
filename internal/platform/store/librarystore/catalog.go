package librarystore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"slices"
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
	// New, Changed and Stale cap each part; zero takes none of it. Changed are fetched records
	// the change feed named since (MarkLibraryCatalogChanged), oldest first.
	New     int
	Changed int
	Stale   int
	// Versions is the record shape each kind is read in now. A record saved in an older one is
	// due with the titles never fetched; a kind not listed is version 0. A record RefreshLibrary
	// marked (catalogRefresh) is due the same way, whatever its kind.
	Versions map[string]int
	// Language is the language records are read in now ("en-US"). A record saved in another is
	// due with the titles never fetched; empty checks none.
	Language string
}

// catalogRefresh is the version RefreshLibrary gives every fetched record: below any real one, so
// the record is fetched again with the new titles while the old one is still read.
const catalogRefresh = -1

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
	// CountDueLibraryCatalog counts the keys DueLibraryCatalog lists with the titles never fetched,
	// uncapped: every one due as new now. New and Stale are ignored.
	CountDueLibraryCatalog(ctx context.Context, due LibraryCatalogDue) (int, error)
	// LibraryCatalogTitle is the name and year (0 unknown) of a title whose details come from key;
	// "" when no title has it.
	LibraryCatalogTitle(ctx context.Context, key LibraryCatalogKey) (string, int, error)
	// SaveLibraryCatalog stores a fetched record, in the shape version and language it was read
	// in, and reports whether it differs from the one held.
	SaveLibraryCatalog(ctx context.Context, key LibraryCatalogKey, record string, version int, language string, at time.Time) (bool, error)
	// FailLibraryCatalog holds a key back from fetching until retryAt.
	FailLibraryCatalog(ctx context.Context, key LibraryCatalogKey, retryAt time.Time) error
	// MarkLibraryCatalogChanged marks the fetched records among keys as changed in the catalog,
	// due again ahead of the refresh, and counts them. Keys the cache does not hold are ignored.
	MarkLibraryCatalogChanged(ctx context.Context, keys []LibraryCatalogKey) (int, error)
	// LibraryCatalogFeedRead is when provider's change feed was last read through; false never.
	LibraryCatalogFeedRead(ctx context.Context, provider string) (time.Time, bool, error)
	// SaveLibraryCatalogFeedRead records that provider's change feed was read through at.
	SaveLibraryCatalogFeedRead(ctx context.Context, provider string, at time.Time) error
	// RewriteLibraryCatalogFiles forgets the identity digest of every present file of each title
	// whose details come from key (a series' episodes too; for an episode key, each episode that
	// has its id), so the scanner writes them again, and lists them.
	RewriteLibraryCatalogFiles(ctx context.Context, key LibraryCatalogKey) ([]LibraryFilePath, error)
}

func (s *Store) WantLibraryCatalog(ctx context.Context, keys []LibraryCatalogKey) error {
	for _, k := range keys {
		if k.String() == "" {
			return fmt.Errorf("want library catalog: provider, kind and id are required")
		}
		if _, err := s.h.ExecContext(ctx, s.ph(`INSERT INTO library_catalog (provider, kind, external_id)
			VALUES (?, ?, ?) ON CONFLICT (provider, kind, external_id) DO NOTHING`),
			k.Provider, k.Kind, k.ExternalID); err != nil {
			return fmt.Errorf("want library catalog: %w", err)
		}
	}
	return nil
}

func (s *Store) LibraryCatalogRecord(ctx context.Context, key LibraryCatalogKey) (string, bool, error) {
	var record string
	var fetched int64
	err := s.h.QueryRowContext(ctx, s.ph(`SELECT record, fetched_at_ms FROM library_catalog
		WHERE provider = ? AND kind = ? AND external_id = ?`), key.Provider, key.Kind, key.ExternalID).Scan(&record, &fetched)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("library catalog record: %w", err)
	}
	return record, fetched > 0, nil
}

// dueAsNew is the condition a key is due with the titles never fetched by, and its arguments: never
// fetched, or saved in an older shape than its kind is read in now, in another language, or marked
// by RefreshLibrary; and no failed fetch holding it back past due.Now.
func dueAsNew(due LibraryCatalogDue) (string, []any) {
	where, args := fmt.Sprintf(`(fetched_at_ms = 0 OR version = %d`, catalogRefresh), []any{}
	for _, kind := range slices.Sorted(maps.Keys(due.Versions)) {
		where += ` OR (kind = ? AND version < ?)`
		args = append(args, kind, due.Versions[kind])
	}
	if due.Language != "" {
		where += ` OR language <> ?`
		args = append(args, due.Language)
	}
	return where + `) AND retry_at_ms <= ?`, append(args, due.Now.UnixMilli())
}

func (s *Store) DueLibraryCatalog(ctx context.Context, due LibraryCatalogDue) ([]LibraryCatalogKey, error) {
	now := due.Now.UnixMilli()
	var out []LibraryCatalogKey
	if due.New > 0 {
		where, args := dueAsNew(due)
		keys, err := s.catalogKeys(ctx, `SELECT provider, kind, external_id FROM library_catalog
			WHERE `+where+` ORDER BY provider, kind, external_id LIMIT ?`, append(args, due.New)...)
		if err != nil {
			return nil, err
		}
		out = append(out, keys...)
	}
	if due.Changed > 0 {
		keys, err := s.catalogKeys(ctx, `SELECT provider, kind, external_id FROM library_catalog
			WHERE changed = ? AND fetched_at_ms > 0 AND retry_at_ms <= ? ORDER BY fetched_at_ms LIMIT ?`,
			true, now, due.Changed)
		if err != nil {
			return nil, err
		}
		for _, k := range keys {
			if !slices.Contains(out, k) {
				out = append(out, k)
			}
		}
	}
	if due.Stale > 0 {
		keys, err := s.catalogKeys(ctx, `SELECT provider, kind, external_id FROM library_catalog
			WHERE fetched_at_ms > 0 AND fetched_at_ms < ? AND retry_at_ms <= ? ORDER BY fetched_at_ms LIMIT ?`,
			due.StaleBefore.UnixMilli(), now, due.Stale)
		if err != nil {
			return nil, err
		}
		for _, k := range keys {
			if !slices.Contains(out, k) {
				out = append(out, k)
			}
		}
	}
	return out, nil
}

func (s *Store) catalogKeys(ctx context.Context, query string, args ...any) ([]LibraryCatalogKey, error) {
	rows, err := s.h.QueryContext(ctx, s.ph(query), args...)
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

func (s *Store) CountLibraryCatalog(ctx context.Context) (int, error) {
	var n int
	if err := s.h.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_catalog`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count library catalog: %w", err)
	}
	return n, nil
}

func (s *Store) LibraryCatalogTitle(ctx context.Context, key LibraryCatalogKey) (string, int, error) {
	var name string
	var year int
	err := s.h.QueryRowContext(ctx, s.ph(`SELECT name, year FROM library_items WHERE catalog = ? ORDER BY id LIMIT 1`),
		key.String()).Scan(&name, &year)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, nil
	}
	if err != nil {
		return "", 0, fmt.Errorf("library catalog title: %w", err)
	}
	return name, year, nil
}

func (s *Store) CountDueLibraryCatalog(ctx context.Context, due LibraryCatalogDue) (int, error) {
	where, args := dueAsNew(due)
	var n int
	if err := s.h.QueryRowContext(ctx, s.ph(`SELECT COUNT(*) FROM library_catalog WHERE `+where), args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("count due library catalog: %w", err)
	}
	return n, nil
}

func (s *Store) SaveLibraryCatalog(ctx context.Context, key LibraryCatalogKey, record string, version int, language string, at time.Time) (bool, error) {
	if key.String() == "" {
		return false, fmt.Errorf("save library catalog: provider, kind and id are required")
	}
	tx, err := s.h.Begin(ctx)
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
	if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO library_catalog (provider, kind, external_id, record,
		fetched_at_ms, retry_at_ms, version, language, changed)
		VALUES (?, ?, ?, ?, ?, 0, ?, ?, FALSE)
		ON CONFLICT (provider, kind, external_id) DO UPDATE SET record = excluded.record,
		fetched_at_ms = excluded.fetched_at_ms, retry_at_ms = 0, version = excluded.version,
		language = excluded.language, changed = FALSE`),
		key.Provider, key.Kind, key.ExternalID, record, at.UnixMilli(), version, language); err != nil {
		return false, fmt.Errorf("save library catalog: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("save library catalog: %w", err)
	}
	return held != record, nil
}

func (s *Store) FailLibraryCatalog(ctx context.Context, key LibraryCatalogKey, retryAt time.Time) error {
	if _, err := s.h.ExecContext(ctx, s.ph(`UPDATE library_catalog SET retry_at_ms = ?
		WHERE provider = ? AND kind = ? AND external_id = ?`),
		retryAt.UnixMilli(), key.Provider, key.Kind, key.ExternalID); err != nil {
		return fmt.Errorf("fail library catalog: %w", err)
	}
	return nil
}

// changedBatch bounds the keys one MarkLibraryCatalogChanged statement names: a feed can name
// thousands, and SQLite caps a statement's parameters.
const changedBatch = 500

func (s *Store) MarkLibraryCatalogChanged(ctx context.Context, keys []LibraryCatalogKey) (int, error) {
	// One statement per provider and kind, ids in batches.
	byKind := map[[2]string][]any{}
	for _, k := range keys {
		if k.String() != "" {
			pk := [2]string{k.Provider, k.Kind}
			byKind[pk] = append(byKind[pk], k.ExternalID)
		}
	}
	marked := 0
	for _, pk := range slices.SortedFunc(maps.Keys(byKind), func(a, b [2]string) int {
		return strings.Compare(a[0]+"\x00"+a[1], b[0]+"\x00"+b[1])
	}) {
		for chunk := range slices.Chunk(byKind[pk], changedBatch) {
			args := append([]any{true, pk[0], pk[1]}, chunk...)
			result, err := s.h.ExecContext(ctx, s.ph(`UPDATE library_catalog SET changed = ?
				WHERE provider = ? AND kind = ? AND fetched_at_ms > 0 AND external_id IN (?`+
				strings.Repeat(", ?", len(chunk)-1)+`)`), args...)
			if err != nil {
				return marked, fmt.Errorf("mark library catalog changed: %w", err)
			}
			n, err := result.RowsAffected()
			if err != nil {
				return marked, fmt.Errorf("mark library catalog changed: %w", err)
			}
			marked += int(n)
		}
	}
	return marked, nil
}

func (s *Store) LibraryCatalogFeedRead(ctx context.Context, provider string) (time.Time, bool, error) {
	var at int64
	err := s.h.QueryRowContext(ctx, s.ph(`SELECT read_at_ms FROM library_catalog_feeds WHERE provider = ?`),
		provider).Scan(&at)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("library catalog feed: %w", err)
	}
	return time.UnixMilli(at), true, nil
}

func (s *Store) SaveLibraryCatalogFeedRead(ctx context.Context, provider string, at time.Time) error {
	if _, err := s.h.ExecContext(ctx, s.ph(`INSERT INTO library_catalog_feeds (provider, read_at_ms) VALUES (?, ?)
		ON CONFLICT (provider) DO UPDATE SET read_at_ms = excluded.read_at_ms`), provider, at.UnixMilli()); err != nil {
		return fmt.Errorf("save library catalog feed: %w", err)
	}
	return nil
}

func (s *Store) RewriteLibraryCatalogFiles(ctx context.Context, key LibraryCatalogKey) ([]LibraryFilePath, error) {
	catalog := key.String()
	if catalog == "" {
		return nil, nil
	}
	const titles = `(SELECT id FROM library_items WHERE catalog = ?)`
	return s.rewriteFiles(ctx, `inventory_item_id IN `+titles+` OR inventory_series_id IN `+titles, catalog, catalog)
}
