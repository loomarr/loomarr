package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/mantonx/loomarr/internal/filler"
)

// Clip is the persisted form of a filler clip (§10). It embeds the domain
// filler.Clip; the store owns the persistence concerns (UpdatedAt). Identity is
// the media-server item id (library is source of truth, §4).
type Clip struct {
	filler.Clip
	UpdatedAt time.Time
}

// ClipFilter narrows a ListClips query. Any zero-value field is a wildcard, so a
// zero ClipFilter lists everything (the pod-assembly catalog load).
type ClipFilter struct {
	Kind     filler.Kind
	Era      int
	Audience filler.Audience
	Category string
	// UntaggedOnly restricts to clips missing era/audience/category — the AI
	// tagging job's work list (§10).
	UntaggedOnly bool
}

func (s *sqlStore) UpsertClip(ctx context.Context, c Clip) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`INSERT INTO clips (library_item_id, name, kind, era, audience, category, duration_ms, rating, source, ai_tagged, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(library_item_id) DO UPDATE SET
		   name=excluded.name, kind=excluded.kind, era=excluded.era, audience=excluded.audience,
		   category=excluded.category, duration_ms=excluded.duration_ms, rating=excluded.rating,
		   source=excluded.source, ai_tagged=excluded.ai_tagged, updated_at=excluded.updated_at`),
		c.LibraryItemID, c.Name, string(c.Kind), c.Era, string(c.Audience), c.Category,
		c.DurationMs, c.Rating, c.Source, boolToInt(c.AITagged), epoch(c.UpdatedAt))
	if err != nil {
		return fmt.Errorf("upsert clip %s: %w", c.LibraryItemID, err)
	}
	return nil
}

const clipSelect = `SELECT library_item_id, name, kind, era, audience, category, duration_ms,
	rating, source, ai_tagged, updated_at FROM clips`

func (s *sqlStore) GetClip(ctx context.Context, id string) (Clip, error) {
	return scanClip(s.db.QueryRowContext(ctx, s.ph(clipSelect+` WHERE library_item_id = ?`), id))
}

// ListClips applies the filter as ANDed WHERE clauses (zero fields omitted). The
// UntaggedOnly flag adds "era=0 OR audience=” OR category=”" (any missing tag).
func (s *sqlStore) ListClips(ctx context.Context, f ClipFilter) ([]Clip, error) {
	var where []string
	var args []any
	if f.Kind != "" {
		where = append(where, "kind = ?")
		args = append(args, string(f.Kind))
	}
	if f.Era > 0 {
		where = append(where, "era = ?")
		args = append(args, f.Era)
	}
	if f.Audience != "" {
		where = append(where, "audience = ?")
		args = append(args, string(f.Audience))
	}
	if f.Category != "" {
		where = append(where, "category = ?")
		args = append(args, f.Category)
	}
	if f.UntaggedOnly {
		// "Untagged" = a COMMERCIAL missing any match tag. Bumpers/station-ids/PSAs
		// serve their bookend role without era/audience/category, so the AI-tagging
		// job (§10) shouldn't spend an LLM call on them — only commercials need full
		// tags for pod matching.
		where = append(where, "kind = 'commercial' AND (era = 0 OR audience = '' OR category = '')")
	}
	q := clipSelect
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY library_item_id"

	rows, err := s.db.QueryContext(ctx, s.ph(q), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanClips(rows)
}

func (s *sqlStore) UpdateClipTags(ctx context.Context, id string, era int, audience, category string, aiTagged bool, updatedAt time.Time) error {
	res, err := s.db.ExecContext(ctx, s.ph(
		`UPDATE clips SET era = ?, audience = ?, category = ?, ai_tagged = ?, updated_at = ? WHERE library_item_id = ?`),
		era, audience, category, boolToInt(aiTagged), epoch(updatedAt), id)
	if err != nil {
		return fmt.Errorf("update clip tags %s: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteClipsNotIn prunes clips absent from the given id set (the sync reconcile).
// With an empty keep set it deletes all clips. Returns the count removed.
func (s *sqlStore) DeleteClipsNotIn(ctx context.Context, keepIDs []string) (int, error) {
	if len(keepIDs) == 0 {
		res, err := s.db.ExecContext(ctx, `DELETE FROM clips`)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		return int(n), nil
	}
	placeholders := make([]string, len(keepIDs))
	args := make([]any, len(keepIDs))
	for i, id := range keepIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	q := s.ph(`DELETE FROM clips WHERE library_item_id NOT IN (` + strings.Join(placeholders, ",") + `)`)
	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, fmt.Errorf("prune clips: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func scanClip(sc scannable) (Clip, error) {
	var (
		c         Clip
		kind      string
		audience  string
		aiTagged  int
		updatedAt int64
	)
	err := sc.Scan(&c.LibraryItemID, &c.Name, &kind, &c.Era, &audience, &c.Category,
		&c.DurationMs, &c.Rating, &c.Source, &aiTagged, &updatedAt)
	if err == sql.ErrNoRows {
		return Clip{}, ErrNotFound
	}
	if err != nil {
		return Clip{}, err
	}
	c.Kind = filler.Kind(kind)
	c.Audience = filler.Audience(audience)
	c.AITagged = aiTagged != 0
	c.UpdatedAt = fromEpoch(updatedAt)
	return c, nil
}

func scanClips(rows *sql.Rows) ([]Clip, error) {
	var out []Clip
	for rows.Next() {
		c, err := scanClip(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
