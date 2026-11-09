package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// LibraryCollection is one of the household's own collections (#2151 decision 28): a name, a
// description, and library titles in the household's order.
type LibraryCollection struct {
	ID          string
	Name        string
	Description string
	// ImportedFrom names the media server it was brought over from at cutover; "" made in Loomarr.
	ImportedFrom string
	// ImportedID is the media server's id for the collection it was brought over from; "" made in
	// Loomarr. An update keeps it.
	ImportedID string
	// Members are library item ids, in order. A title that has left the library stays here, so it
	// comes back in its place.
	Members   []string
	UpdatedAt time.Time
}

// LibraryCollectionStore keeps the household's collections.
type LibraryCollectionStore interface {
	// ListLibraryCollections lists every collection, by name.
	ListLibraryCollections(ctx context.Context) ([]LibraryCollection, error)
	// LibraryCollection reads one; ErrNotFound when there is none.
	LibraryCollection(ctx context.Context, id string) (LibraryCollection, error)
	// CreateLibraryCollection saves a new collection and returns its id; c.ID is ignored.
	CreateLibraryCollection(ctx context.Context, c LibraryCollection, at time.Time) (string, error)
	// UpdateLibraryCollection replaces a collection's name, description and members; ErrNotFound
	// when there is none.
	UpdateLibraryCollection(ctx context.Context, c LibraryCollection, at time.Time) error
	// DeleteLibraryCollection deletes a collection and its members; ErrNotFound when there is none.
	DeleteLibraryCollection(ctx context.Context, id string) error
	// LibraryCollectionMemberIDs lists each collection's members still in the library, with the
	// provider ids a channel's lineup knows them by.
	LibraryCollectionMemberIDs(ctx context.Context) ([]LibraryCollectionMember, error)
}

// LibraryCollectionMember is one title of a collection, by its external ids (none when it has
// none).
type LibraryCollectionMember struct {
	CollectionID string
	// Kind is the title's library kind: movie or series.
	Kind string
	// IDs are every external id the title is known by, by namespace then value.
	IDs []LibraryID
}

func (s *sqlStore) LibraryCollectionMemberIDs(ctx context.Context) ([]LibraryCollectionMember, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT m.collection_id, i.kind, i.id
		FROM library_collection_members m JOIN library_items i ON i.id = m.inventory_item_id
		ORDER BY m.collection_id, m.position`)
	if err != nil {
		return nil, fmt.Errorf("library collection members: %w", err)
	}
	var out []LibraryCollectionMember
	var items []string
	for rows.Next() {
		var m LibraryCollectionMember
		var item string
		if err := rows.Scan(&m.CollectionID, &m.Kind, &item); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("library collection members: %w", err)
		}
		out, items = append(out, m), append(items, item)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("library collection members: %w", err)
	}
	byItem, err := s.libraryIDsOf(ctx, items)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].IDs = byItem[items[i]]
	}
	return out, nil
}

func (s *sqlStore) ListLibraryCollections(ctx context.Context) ([]LibraryCollection, error) {
	return s.readLibraryCollections(ctx, "")
}

func (s *sqlStore) LibraryCollection(ctx context.Context, id string) (LibraryCollection, error) {
	out, err := s.readLibraryCollections(ctx, id)
	if err != nil {
		return LibraryCollection{}, err
	}
	if len(out) == 0 {
		return LibraryCollection{}, ErrNotFound
	}
	return out[0], nil
}

// readLibraryCollections reads every collection, or the one with id when id is not "".
func (s *sqlStore) readLibraryCollections(ctx context.Context, id string) ([]LibraryCollection, error) {
	where, args := "", []any{}
	if id != "" {
		where, args = " WHERE c.id = ?", []any{id}
	}
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT c.id, c.name, c.description, c.imported_from, c.imported_id, c.updated_at_ms,
		COALESCE(m.inventory_item_id, '') FROM library_collections c
		LEFT JOIN library_collection_members m ON m.collection_id = c.id`+where+`
		ORDER BY LOWER(c.name), c.id, m.position`), args...)
	if err != nil {
		return nil, fmt.Errorf("list library collections: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []LibraryCollection{}
	for rows.Next() {
		var c LibraryCollection
		var updated int64
		var member string
		if err := rows.Scan(&c.ID, &c.Name, &c.Description, &c.ImportedFrom, &c.ImportedID, &updated, &member); err != nil {
			return nil, fmt.Errorf("scan library collection: %w", err)
		}
		if n := len(out); n == 0 || out[n-1].ID != c.ID {
			c.UpdatedAt = time.UnixMilli(updated)
			c.Members = []string{}
			out = append(out, c)
		}
		if member != "" {
			last := &out[len(out)-1]
			last.Members = append(last.Members, member)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list library collections: %w", err)
	}
	return out, nil
}

func (s *sqlStore) CreateLibraryCollection(ctx context.Context, c LibraryCollection, at time.Time) (string, error) {
	id := strings.ToLower(rand.Text())
	err := s.writeLibraryCollection(ctx, id, c, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, s.ph(`INSERT INTO library_collections (id, name, description, imported_from,
			imported_id, created_at_ms, updated_at_ms) VALUES (?, ?, ?, ?, ?, ?, ?)`),
			id, c.Name, c.Description, c.ImportedFrom, c.ImportedID, at.UnixMilli(), at.UnixMilli())
		return err
	})
	if err != nil {
		return "", fmt.Errorf("create library collection: %w", err)
	}
	return id, nil
}

func (s *sqlStore) UpdateLibraryCollection(ctx context.Context, c LibraryCollection, at time.Time) error {
	err := s.writeLibraryCollection(ctx, c.ID, c, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, s.ph(`UPDATE library_collections SET name = ?, description = ?,
			updated_at_ms = ? WHERE id = ?`), c.Name, c.Description, at.UnixMilli(), c.ID)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n == 0 {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("update library collection: %w", err)
	}
	return nil
}

// writeLibraryCollection runs row, which writes the collection's own row, then replaces its
// members, in one transaction. A title listed twice keeps its first place.
func (s *sqlStore) writeLibraryCollection(ctx context.Context, id string, c LibraryCollection, row func(*sql.Tx) error) error {
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("a collection needs a name")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := row(tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, s.ph(`DELETE FROM library_collection_members WHERE collection_id = ?`), id); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, item := range c.Members {
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO library_collection_members (collection_id, inventory_item_id, position)
			VALUES (?, ?, ?)`), id, item, len(seen)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *sqlStore) DeleteLibraryCollection(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete library collection: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, s.ph(`DELETE FROM library_collections WHERE id = ?`), id)
	if err != nil {
		return fmt.Errorf("delete library collection: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("delete library collection: %w", err)
	} else if n == 0 {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, s.ph(`DELETE FROM library_collection_members WHERE collection_id = ?`), id); err != nil {
		return fmt.Errorf("delete library collection: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete library collection: %w", err)
	}
	return nil
}
