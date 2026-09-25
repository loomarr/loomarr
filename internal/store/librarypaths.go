package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Library item path cache (#1456) — the media server's own file path per item, so playout
// resolves a direct-play input without asking the media server at airtime.

// LibraryItemPath returns the cached server path for an item; ok=false when none is cached.
func (s *sqlStore) LibraryItemPath(ctx context.Context, itemID string) (string, bool, error) {
	var path string
	err := s.db.QueryRowContext(ctx, s.ph(
		`SELECT server_path FROM library_item_paths WHERE item_id = ?`), itemID).Scan(&path)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read library item path %s: %w", itemID, err)
	}
	return path, path != "", nil
}

// SetLibraryItemPath upserts the server path for an item. An empty id or path is a no-op: there
// is nothing identifiable to remember.
func (s *sqlStore) SetLibraryItemPath(ctx context.Context, itemID, serverPath string) error {
	if itemID == "" || serverPath == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, s.ph(
		`INSERT INTO library_item_paths (item_id, server_path, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(item_id) DO UPDATE SET server_path = excluded.server_path, updated_at = excluded.updated_at`),
		itemID, serverPath, epoch(time.Now()))
	if err != nil {
		return fmt.Errorf("write library item path %s: %w", itemID, err)
	}
	return nil
}
