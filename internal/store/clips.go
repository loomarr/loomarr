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
	// IncludeRemoved lifts the default exclusion of tombstoned clips (V35).
	//
	// ⚠ The DEFAULT is to exclude, and it has to be: pod assembly loads the catalog through
	// this same call with a zero filter, so an opt-OUT would mean a clip the operator removed
	// keeps airing until somebody remembers to pass a flag. Opt-in is the safe polarity.
	IncludeRemoved bool
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
		// ⚠ `removed_at` is omitted from DO UPDATE for the same reason, and it is the thing that
		// makes "Remove from catalog" survive a re-scan. `clips` is a synced CACHE, so the next
		// pass finds the file still on disk and upserts it; if the tombstone rode along it would
		// be reset to the scan's zero and the clip would silently reappear. SetClipsRemoved is
		// its only writer, exactly like RecordClipPlay is the counters'.
		//
		// excluded.play_count would reset every counter to the scan's zero on each pass —
		// silently, and only noticeable as "usage never goes up". Tags survive re-sync because
		// sync.go merges them before calling this; the counters survive because the SQL simply
		// never touches them after insert. RecordClipPlay is their only writer.
		`INSERT INTO clips (path, tunarr_program_id, name, kind, era, audience, category, duration_ms, rating, source, ai_tagged, quality, license, thumbnail, play_count, last_played_at, suggested_era, removed_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(path) DO UPDATE SET
		   tunarr_program_id=excluded.tunarr_program_id,
		   name=excluded.name, kind=excluded.kind, era=excluded.era, audience=excluded.audience,
		   category=excluded.category, duration_ms=excluded.duration_ms, rating=excluded.rating,
		   source=excluded.source, ai_tagged=excluded.ai_tagged, quality=excluded.quality,
		   license=excluded.license,
		   thumbnail=excluded.thumbnail,
		   suggested_era=excluded.suggested_era,
		   updated_at=excluded.updated_at`),
		c.Path, nullIfEmpty(c.TunarrProgramID), c.Name, string(c.Kind), c.Era, string(c.Audience), c.Category,
		c.DurationMs, c.Rating, c.Source, boolToInt(c.AITagged), c.Quality, c.License, c.Thumbnail,
		c.PlayCount, epoch(c.LastPlayedAt), c.SuggestedEra, epoch(c.RemovedAt), epoch(c.UpdatedAt))
	if err != nil {
		return fmt.Errorf("upsert clip %s: %w", c.Path, err)
	}
	return nil
}

const clipSelect = `SELECT path, tunarr_program_id, name, kind, era, audience, category, duration_ms,
	rating, source, ai_tagged, quality, license, thumbnail, play_count, last_played_at, suggested_era,
	removed_at, updated_at FROM clips`

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
	if !f.IncludeRemoved {
		where = append(where, "removed_at = 0")
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

func (s *sqlStore) UpdateClipTags(ctx context.Context, id string, era int, audience, category string, suggestedEra int, aiTagged bool, updatedAt time.Time) error {
	res, err := s.db.ExecContext(ctx, s.ph(
		// ⚠ suggested_era is CONDITIONAL (§10 era grounding, V34): writing an era confirms
		// the suggestion, so it clears in the same write; a NEW suggestion overwrites; and a
		// write carrying NEITHER (an era-less tag edit) leaves the existing suggestion alone —
		// the tag job re-classifies untagged clips every run, so wiping on a no-era result
		// would make the suggestion flap run to run.
		`UPDATE clips SET era = ?, audience = ?, category = ?,
		   suggested_era = CASE WHEN ? > 0 THEN 0 WHEN ? > 0 THEN ? ELSE suggested_era END,
		   ai_tagged = ?, updated_at = ? WHERE path = ?`),
		era, audience, category, era, suggestedEra, suggestedEra, boolToInt(aiTagged), epoch(updatedAt), id)
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
		removedAt    int64
		updatedAt    int64
	)
	err := sc.Scan(&c.Path, &tunarrID, &c.Name, &kind, &c.Era, &audience, &c.Category,
		&c.DurationMs, &c.Rating, &c.Source, &aiTagged, &c.Quality, &c.License, &c.Thumbnail,
		&c.PlayCount, &lastPlayedAt, &c.SuggestedEra, &removedAt, &updatedAt)
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
	if removedAt != 0 {
		c.RemovedAt = fromEpoch(removedAt)
	}
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

// SetClipsRemoved tombstones (or restores) clips by path — the Catalog tab's "Remove from
// catalog" (V35).
//
// ⚠ **This is the ONLY writer of `removed_at`**, exactly as RecordClipPlay is the only writer of
// the play counters, and for the same reason: `UpsertClip` deliberately omits the column from its
// DO UPDATE list, so the next scan cannot resurrect a removed clip by finding its file still on
// disk. Route the write anywhere else and that guarantee is gone.
//
// ⚠ It does NOT touch the file. Nothing in Loomarr deletes an operator's media — the button says
// remove from the CATALOG, and the file stays where they put it.
//
// Returns the number of rows affected; unknown paths are silently skipped, because a bulk action
// over a list the operator selected minutes ago races a re-scan and failing the whole batch for
// one stale row would be worse than removing the rest.
func (s *sqlStore) SetClipsRemoved(ctx context.Context, paths []string, at time.Time) (int, error) {
	if len(paths) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(paths))
	args := make([]any, 0, len(paths)+1)
	args = append(args, epoch(at))
	for i, p := range paths {
		placeholders[i] = "?"
		args = append(args, p)
	}
	q := `UPDATE clips SET removed_at = ? WHERE path IN (` + strings.Join(placeholders, ",") + `)`
	res, err := s.db.ExecContext(ctx, s.ph(q), args...)
	if err != nil {
		return 0, fmt.Errorf("set clips removed: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("set clips removed: %w", err)
	}
	return int(n), nil
}
