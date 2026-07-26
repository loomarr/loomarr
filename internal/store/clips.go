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
// the clip's PATH relative to FILLER_DIR (§9.1 — internal playout needs a playable
// input, and must not need Tunarr to discover Loomarr's own files). The Tunarr
// program uuid rides alongside, nullable, for Tunarr-backed filler-lists.
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
	// Query is a case-insensitive substring match on the clip name — the `name LIKE`
	// filter §7.2 prescribes for the clip corpus. Clip search lives here rather than in
	// /v1/search because a clip is not a provisionable title (§10), so it cannot be a
	// federated Candidate without leaking a non-title into the LLM grounding path.
	Query string
}

func (s *sqlStore) UpsertClip(ctx context.Context, c Clip) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		// ⚠ play_count / last_played_at are INSERTed (so a new row starts at 0) but deliberately
		// NOT in the DO UPDATE list. A re-sync knows nothing about plays, so writing
		// excluded.play_count would reset every counter to the scan's zero on each pass —
		// silently, and only noticeable as "usage never goes up". Tags survive re-sync because
		// sync.go merges them before calling this; the counters survive because the SQL simply
		// never touches them after insert. RecordClipPlay is their only writer.
		`INSERT INTO clips (path, tunarr_program_id, name, kind, era, audience, category, duration_ms, rating, source, ai_tagged, quality, thumbnail, play_count, last_played_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(path) DO UPDATE SET
		   tunarr_program_id=excluded.tunarr_program_id,
		   name=excluded.name, kind=excluded.kind, era=excluded.era, audience=excluded.audience,
		   category=excluded.category, duration_ms=excluded.duration_ms, rating=excluded.rating,
		   source=excluded.source, ai_tagged=excluded.ai_tagged, quality=excluded.quality,
		   thumbnail=excluded.thumbnail,
		   updated_at=excluded.updated_at`),
		c.Path, nullIfEmpty(c.TunarrProgramID), c.Name, string(c.Kind), c.Era, string(c.Audience), c.Category,
		c.DurationMs, c.Rating, c.Source, boolToInt(c.AITagged), c.Quality, c.Thumbnail,
		c.PlayCount, epoch(c.LastPlayedAt), epoch(c.UpdatedAt))
	if err != nil {
		return fmt.Errorf("upsert clip %s: %w", c.Path, err)
	}
	return nil
}

const clipSelect = `SELECT path, tunarr_program_id, name, kind, era, audience, category, duration_ms,
	rating, source, ai_tagged, quality, thumbnail, play_count, last_played_at, updated_at FROM clips`

func (s *sqlStore) GetClip(ctx context.Context, id string) (Clip, error) {
	return scanClip(s.db.QueryRowContext(ctx, s.ph(clipSelect+` WHERE path = ?`), id))
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
	if f.Query != "" {
		// LOWER() on both sides so the match is case-insensitive on BOTH dialects:
		// SQLite's LIKE folds case for ASCII by default while Postgres's does not, and
		// a search that behaves differently per backend is exactly the dialect fork the
		// store rules forbid. The term is escaped for LIKE metacharacters so a user
		// typing "%" searches for a percent sign rather than matching everything.
		where = append(where, "LOWER(name) LIKE LOWER(?) ESCAPE '\\'")
		args = append(args, "%"+escapeLike(f.Query)+"%")
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
	q += " ORDER BY path"

	rows, err := s.db.QueryContext(ctx, s.ph(q), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanClips(rows)
}

// ListUntaggedCommercials is the AI-tagging work list (§10) — sugar over the
// commercial-scoped UntaggedOnly filter.
func (s *sqlStore) ListUntaggedCommercials(ctx context.Context) ([]Clip, error) {
	return s.ListClips(ctx, ClipFilter{UntaggedOnly: true})
}

func (s *sqlStore) UpdateClipTags(ctx context.Context, id string, era int, audience, category string, aiTagged bool, updatedAt time.Time) error {
	res, err := s.db.ExecContext(ctx, s.ph(
		`UPDATE clips SET era = ?, audience = ?, category = ?, ai_tagged = ?, updated_at = ? WHERE path = ?`),
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

// RecordClipPlay increments a clip's play counter and stamps when it last aired (V28).
//
// ⚠ Called from PLAYOUT — when a filler item actually starts encoding — never from pod
// assembly. Assembly re-runs on every reconcile sweep and would count SCHEDULED rather than
// AIRED, inflating without bound (see migration 00017).
//
// A missing clip is NOT an error. Playout resolves a path that the catalog may have pruned
// between the schedule being built and the break airing; failing here would turn a stale
// catalog row into a playback error, and the counter is telemetry, not correctness. The row
// count is deliberately ignored for the same reason.
//
// updated_at is left alone: this is not a catalog edit, and touching it would make every clip
// look freshly re-synced in the UI's "last updated" column.
func (s *sqlStore) RecordClipPlay(ctx context.Context, id string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`UPDATE clips SET play_count = play_count + 1, last_played_at = ? WHERE path = ?`),
		epoch(at), id)
	if err != nil {
		return fmt.Errorf("record clip play %s: %w", id, err)
	}
	return nil
}

// UpdateClipKind corrects a clip's kind (§10). Kind drives pod ROLE — a bumper bookends
// a pod while a commercial fills it — so a mis-detected kind produces structurally wrong
// pods, not merely a mis-tagged clip.
func (s *sqlStore) UpdateClipKind(ctx context.Context, id, kind string, updatedAt time.Time) error {
	res, err := s.db.ExecContext(ctx, s.ph(
		`UPDATE clips SET kind = ?, updated_at = ? WHERE path = ?`),
		kind, epoch(updatedAt), id)
	if err != nil {
		return fmt.Errorf("update clip kind %s: %w", id, err)
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
	q := s.ph(`DELETE FROM clips WHERE path NOT IN (` + strings.Join(placeholders, ",") + `)`)
	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, fmt.Errorf("prune clips: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func scanClip(sc scannable) (Clip, error) {
	var (
		c        Clip
		kind     string
		audience string
		// tunarr_program_id is NULLABLE since §9.1 — an install with no Tunarr has none,
		// which is a supported configuration. sql.NullString rather than string so a NULL
		// scans cleanly instead of erroring on a nil-to-string conversion.
		tunarrID     sql.NullString
		aiTagged     int
		lastPlayedAt int64
		updatedAt    int64
	)
	err := sc.Scan(&c.Path, &tunarrID, &c.Name, &kind, &c.Era, &audience, &c.Category,
		&c.DurationMs, &c.Rating, &c.Source, &aiTagged, &c.Quality, &c.Thumbnail,
		&c.PlayCount, &lastPlayedAt, &updatedAt)
	if err == sql.ErrNoRows {
		return Clip{}, ErrNotFound
	}
	if err != nil {
		return Clip{}, err
	}
	c.Kind = filler.Kind(kind)
	c.Audience = filler.Audience(audience)
	c.TunarrProgramID = tunarrID.String // "" when NULL — the no-Tunarr case
	c.AITagged = aiTagged != 0
	c.LastPlayedAt = fromEpoch(lastPlayedAt)
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

// nullIfEmpty writes "" as SQL NULL.
//
// For tunarr_program_id specifically: the column is nullable because an install with no Tunarr
// has no uuid for its clips. Storing "" instead would make every such clip share a value, and
// any future UNIQUE constraint on it would then reject the second clip — whereas SQL NULLs do
// not collide. Distinguishing "no Tunarr" from "the empty uuid" also keeps the sync's
// preserve-a-known-uuid branch honest.
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// escapeLike neutralizes LIKE metacharacters so a search term is matched literally.
// Without it, a query of "%" matches every clip and "_" matches any single character —
// surprising for a plain search box, and a needless full-table scan.
func escapeLike(term string) string {
	r := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`)
	return r.Replace(term)
}
