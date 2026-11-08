package store

import (
	"context"
	"fmt"
	"time"
)

// LibraryIDMapping is one media-server item id and the library item it names (#2151 step 15a).
// ItemID is empty when the id did not pair; Reason then says why.
type LibraryIDMapping struct {
	MediaID string
	// Kind is movie, episode or series.
	Kind   string
	ItemID string
	Reason string
}

// LibraryIDMapStore keeps the cutover's id map.
type LibraryIDMapStore interface {
	// ReplaceLibraryIDMap replaces the whole map with one pass's answer.
	ReplaceLibraryIDMap(ctx context.Context, mappings []LibraryIDMapping, at time.Time) error
	// LibraryIDMap is the map, ordered by media id.
	LibraryIDMap(ctx context.Context) ([]LibraryIDMapping, error)
}

func (s *sqlStore) ReplaceLibraryIDMap(ctx context.Context, mappings []LibraryIDMapping, at time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("replace library id map: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM library_id_map`); err != nil {
		return fmt.Errorf("replace library id map: %w", err)
	}
	insert, err := tx.PrepareContext(ctx, s.ph(`INSERT INTO library_id_map (media_id, kind, item_id, reason, mapped_at_ms)
		VALUES (?, ?, ?, ?, ?)`))
	if err != nil {
		return fmt.Errorf("replace library id map: %w", err)
	}
	defer func() { _ = insert.Close() }()
	for _, m := range mappings {
		if _, err := insert.ExecContext(ctx, m.MediaID, m.Kind, m.ItemID, m.Reason, at.UnixMilli()); err != nil {
			return fmt.Errorf("replace library id map: %s: %w", m.MediaID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("replace library id map: %w", err)
	}
	return nil
}

func (s *sqlStore) LibraryIDMap(ctx context.Context) ([]LibraryIDMapping, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT media_id, kind, item_id, reason FROM library_id_map ORDER BY media_id`)
	if err != nil {
		return nil, fmt.Errorf("library id map: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []LibraryIDMapping{}
	for rows.Next() {
		var m LibraryIDMapping
		if err := rows.Scan(&m.MediaID, &m.Kind, &m.ItemID, &m.Reason); err != nil {
			return nil, fmt.Errorf("library id map: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("library id map: %w", err)
	}
	return out, nil
}
