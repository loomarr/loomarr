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
	// HeldOnly restricts to clips waiting to be filed (§10 V38) — the Incoming queue's work
	// list. The inverse of the default exclusion below, not a lifting of it.
	HeldOnly bool
	// IncludeHeld lifts the default exclusion of HELD clips (§10 V38).
	//
	// ⚠ Same polarity as IncludeRemoved and for a sharper reason. Pod assembly loads the catalog
	// through this same call with a ZERO filter, so an opt-OUT would put every untagged,
	// unreviewed clip Loomarr has ever downloaded straight into a channel's breaks — the exact
	// thing holding exists to prevent. Opt-in means a new caller is safe by default and an
	// unsafe one has to say so in writing.
	IncludeHeld bool
	// Query is a case-insensitive substring match on the clip name — the `name LIKE`
	// filter §7.2 prescribes for the clip corpus. Clip search lives here rather than in
	// /v1/search because a clip is not a provisionable title (§10), so it cannot be a
	// federated Candidate without leaking a non-title into the LLM grounding path.
	Query string
	// AutoFiledOnly restricts to clips filed into the catalog WITHOUT a human (§10 V38) — the
	// Incoming tab's audit list, "what happened while nobody was looking".
	//
	// ⚠ Opt-in like every other narrowing flag here, so the zero filter keeps meaning "the whole
	// catalog". Added because the audit list was loading every clip in the install and dropping
	// all but the auto-filed ones in Go; the predicate belongs beside the column it reads.
	AutoFiledOnly bool
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
		// ⚠ `held` and `auto_filed` are omitted for the SAME reason, and the failure is worse
		// (V38). The folder scan re-upserts every file it finds with `held = false`; if that rode
		// along in the update list, one scan pass would file every held clip — clearing the
		// review queue and putting untagged, unreviewed clips straight into channels, with no
		// operator action and nothing in the logs. SetClipHeld is their only writer.
		//
		// `confidence` is omitted too. A scan knows nothing about tagging, and the Clip it builds
		// carries a zero score — so leaving it in the update list would blank a tagged clip's
		// confidence on the next pass, and a clip that had been trusted would start asking again
		// for no reason. `sync.go` also preserves it explicitly: belt and braces, exactly as that
		// file's own comment argues for the play counters.
		//
		// excluded.play_count would reset every counter to the scan's zero on each pass —
		// silently, and only noticeable as "usage never goes up". Tags survive re-sync because
		// sync.go merges them before calling this; the counters survive because the SQL simply
		// never touches them after insert. RecordClipPlay is their only writer.
		// ⚠ Keyed on `hash` since V38c — `path` is ordinary data now, and IS in the DO UPDATE list
		// because a clip's location can legitimately change (re-filed under a new extension, or
		// found in a different folder) while its identity does not. That is the whole point of
		// content addressing: the same bytes are the same clip wherever they sit.
		// ⚠ transcript / brand / visible_text / vision_tagged are INSERTed (a new row starts empty)
		// but deliberately OMITTED from DO UPDATE (§10 V44, migration 00038) — the same rule the
		// block above states for language/removed_at/held/counters. The folder scan knows none of
		// them; if they rode the update list, one re-sync would blank a transcribed or vision-tagged
		// clip and re-trigger ~341s of Whisper or a paid vision call over work already done. Their
		// writers are the job methods below — SetClipTranscript owns `transcript`, SetClipVisionTags
		// owns `visible_text`/`vision_tagged`, and `brand` is written by whichever grounded it
		// (SetClipBrand from text, SetClipVisionTags from the frame).
		`INSERT INTO clips (hash, path, tunarr_program_id, name, kind, era, audience, category, duration_ms, rating, source, ai_tagged, quality, license, thumbnail, preview, language, transcript, brand, visible_text, vision_tagged, play_count, last_played_at, suggested_era, removed_at, held, confidence, auto_filed, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(hash) DO UPDATE SET
		   path=excluded.path,
		   tunarr_program_id=excluded.tunarr_program_id,
		   name=excluded.name, kind=excluded.kind, era=excluded.era, audience=excluded.audience,
		   category=excluded.category, duration_ms=excluded.duration_ms, rating=excluded.rating,
		   source=excluded.source, ai_tagged=excluded.ai_tagged, quality=excluded.quality,
		   license=excluded.license,
		   thumbnail=excluded.thumbnail,
		   preview=excluded.preview,
		   language=excluded.language,
		   suggested_era=excluded.suggested_era,
		   updated_at=excluded.updated_at`),
		c.Hash, c.Path, nullIfEmpty(c.TunarrProgramID), c.Name, string(c.Kind), c.Era, string(c.Audience), c.Category,
		c.DurationMs, c.Rating, c.Source,
		// ⚠ Bound as real bools. `ai_tagged` used `boolToInt` until V38c, when 00033 rebuilt the
		// table and made it BOOLEAN on Postgres like `held`/`auto_filed` — so the helper that was
		// correct for it yesterday is a 42804 today. The dialect split is per COLUMN, and this
		// line is the fourth time it has bitten this session. `vision_tagged` joins them (00038).
		c.AITagged, c.Quality, c.License, c.Thumbnail, c.Preview, c.Language,
		c.Transcript, c.Brand, c.VisibleText, c.VisionTagged,
		c.PlayCount, epoch(c.LastPlayedAt), c.SuggestedEra, epoch(c.RemovedAt),
		c.Held, c.Confidence, c.AutoFiled, epoch(c.UpdatedAt))
	if err != nil {
		return fmt.Errorf("upsert clip %s: %w", c.Hash, err)
	}
	return nil
}

const clipSelect = `SELECT hash, path, tunarr_program_id, name, kind, era, audience, category, duration_ms,
	rating, source, ai_tagged, quality, license, thumbnail, preview, language, transcript, brand, visible_text, vision_tagged,
	play_count, last_played_at, suggested_era,
	removed_at, held, confidence, auto_filed, updated_at FROM clips`

func (s *sqlStore) GetClip(ctx context.Context, id string) (Clip, error) {
	return scanClip(s.db.QueryRowContext(ctx, s.ph(clipSelect+` WHERE hash = ?`), id))
}

// GetClipByPath looks a clip up by its location under FILLER_DIR.
//
// ⚠ **Needed because a clip's PATH and its IDENTITY stopped being the same string in V38c.**
// Identity is now the content hash (`14365f2b…`), while the path is the sharded location
// (`14/36/14365f2b….mp4`). `GetClip` queries `WHERE hash = ?`, so handing it a path matches
// nothing — silently, as an ordinary not-found.
//
// That is not hypothetical: `/v1/filler/media` did exactly this from V38c until V39 and 404'd for
// every clip in the catalog. Nothing noticed because no client called it until the player shipped.
//
// The media route needs this one rather than the hash lookup because its URL carries the path —
// which is what `ClipDTO` exposes, and what every other clip-shaped route already keys on.
func (s *sqlStore) GetClipByPath(ctx context.Context, path string) (Clip, error) {
	return scanClip(s.db.QueryRowContext(ctx, s.ph(clipSelect+` WHERE path = ?`), path))
}

// ListClips applies the filter as ANDed WHERE clauses (zero fields omitted). The
// UntaggedOnly flag adds "era=0 OR audience=” OR category=”" (any missing tag).
func (s *sqlStore) ListClips(ctx context.Context, f ClipFilter) ([]Clip, error) {
	where, args := clipWhere(f)
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

// CountClips answers "how many match?" without materialising the rows.
//
// ⚠ Shares `clipWhere` with ListClips rather than restating the predicate. Several callers wanted
// only a number and were loading every column of every row to take `len()` of the slice — on a
// large catalog that is the dominant cost of rendering a settings page. A second hand-written
// WHERE here would be free to drift from the one the listing (and the tagging job) actually use,
// which is the failure this deliberately forecloses.
func (s *sqlStore) CountClips(ctx context.Context, f ClipFilter) (int, error) {
	where, args := clipWhere(f)
	q := `SELECT COUNT(*) FROM clips`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	var n int
	if err := s.db.QueryRowContext(ctx, s.ph(q), args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("count clips: %w", err)
	}
	return n, nil
}

// CountClipsBySource returns the clip count per source — the Sources page's whole question.
//
// ⚠ Grouped in SQL. This was every row of the catalog loaded into Go to build a map of counters;
// the answer is one small map either way, so the load was pure waste. Honours the same default
// exclusions as the listing (removed and held clips are out unless asked for) so the per-source
// numbers add up to the catalog total a caller sees elsewhere.
func (s *sqlStore) CountClipsBySource(ctx context.Context, f ClipFilter) (map[string]int, error) {
	where, args := clipWhere(f)
	q := `SELECT source, COUNT(*) FROM clips`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " GROUP BY source"

	rows, err := s.db.QueryContext(ctx, s.ph(q), args...)
	if err != nil {
		return nil, fmt.Errorf("count clips by source: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int{}
	for rows.Next() {
		var src string
		var n int
		if err := rows.Scan(&src, &n); err != nil {
			return nil, err
		}
		out[src] = n
	}
	return out, rows.Err()
}

// clipWhere builds the ANDed predicate for a ClipFilter, shared by every query that has to mean
// the same thing by "matching clips" — the listing, the counts, and the per-source rollup.
func clipWhere(f ClipFilter) ([]string, []any) {
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
	// ⚠ THE chokepoint for the clip lifecycle (§10 V38). Held clips are excluded here, once,
	// rather than at each of the many callers — pod assembly, coverage, the filler-list builder,
	// the catalog listing, search. Filtering per call site is how one of them gets forgotten and
	// an unreviewed clip airs.
	//
	// ⚠ Compared as a BOUND PARAMETER, never a literal. `held` is INTEGER on sqlite and BOOLEAN
	// on Postgres (migration 00031), so `held = 0` is a type error on one of them — the same
	// dialect trap V37's 00029 hit with a literal `1`. Binding lets the driver render each.
	if f.HeldOnly {
		where = append(where, "held = ?")
		args = append(args, true)
	} else if !f.IncludeHeld {
		where = append(where, "held = ?")
		args = append(args, false)
	}
	if f.UntaggedOnly {
		// "Untagged" = a COMMERCIAL missing any match tag. Bumpers/station-ids/PSAs
		// serve their bookend role without era/audience/category, so the AI-tagging
		// job (§10) shouldn't spend an LLM call on them — only commercials need full
		// tags for pod matching.
		where = append(where, "kind = 'commercial' AND (era = 0 OR audience = '' OR category = '')")
	}
	if f.AutoFiledOnly {
		// ⚠ Bound, not a literal: `auto_filed` is INTEGER on sqlite and BOOLEAN on Postgres,
		// the same dialect trap the `held` comparison above documents.
		where = append(where, "auto_filed = ?")
		args = append(args, true)
	}
	return where, args
}

// ListUntaggedCommercials is the AI-tagging job's work list (§10) — sugar over the
// commercial-scoped UntaggedOnly filter.
//
// ⚠ **IncludeHeld is required, not optional** (§10 V38). Held clips are exactly the ones most in
// need of tagging — a held clip is waiting to be tagged and then filed — and the default catalog
// filter excludes them. Without this the tagger would silently skip every clip Loomarr had just
// downloaded, tag only what was already filed, and nothing would ever leave the review queue by
// itself. The auto-file threshold would appear to do nothing.
func (s *sqlStore) ListUntaggedCommercials(ctx context.Context) ([]Clip, error) {
	return s.ListClips(ctx, ClipFilter{UntaggedOnly: true, IncludeHeld: true})
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
		   ai_tagged = ?, updated_at = ? WHERE hash = ?`),
		// ⚠ A real bool, not boolToInt — `ai_tagged` became BOOLEAN on Postgres when 00033 rebuilt
		// the table (V38c). This writer and the upsert must agree, and they did not for a moment.
		era, audience, category, era, suggestedEra, suggestedEra, aiTagged, epoch(updatedAt), id)
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
		`UPDATE clips SET play_count = play_count + 1, last_played_at = ? WHERE hash = ?`),
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
		`UPDATE clips SET kind = ?, updated_at = ? WHERE hash = ?`),
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
	// ⚠ Keyed on `hash`, the clip's IDENTITY since V38c — not on `path`.
	//
	// This said `path NOT IN (…)` and was missed when identity moved off the path. The sync's
	// `keep` set carries hashes, and a hash never equals a path, so EVERY clip matched
	// "not in the keep set" and the whole catalog was deleted on every single sync. The route
	// reported "1 added, 1 pruned" forever and the catalog stayed empty — filler silently never
	// worked. Found by running the real binary; the conformance suite passed throughout because
	// its fixtures set `path` and `hash` to the same string.
	q := s.ph(`DELETE FROM clips WHERE hash NOT IN (` + strings.Join(placeholders, ",") + `)`)
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
		lastPlayedAt int64
		removedAt    int64
		// ⚠ All three are real bools since 00033 rebuilt the table. `aiTagged` was an INTEGER
		// scanned into an int until V38c — the rebuild made it BOOLEAN on Postgres like its two
		// neighbours, so the old int would now fail to scan. The dialect split is per COLUMN, and
		// a rebuild is exactly when a column can change sides.
		aiTagged  bool
		held      bool
		autoFiled bool
		// vision_tagged is BOOLEAN (00038), the same side of the dialect split as its three
		// neighbours above — scanned into a bool, never an int.
		visionTagged bool
		updatedAt    int64
	)
	err := sc.Scan(&c.Hash, &c.Path, &tunarrID, &c.Name, &kind, &c.Era, &audience, &c.Category,
		&c.DurationMs, &c.Rating, &c.Source, &aiTagged, &c.Quality, &c.License, &c.Thumbnail, &c.Preview, &c.Language,
		&c.Transcript, &c.Brand, &c.VisibleText, &visionTagged,
		&c.PlayCount, &lastPlayedAt, &c.SuggestedEra, &removedAt, &held, &c.Confidence, &autoFiled, &updatedAt)
	if err == sql.ErrNoRows {
		return Clip{}, ErrNotFound
	}
	if err != nil {
		return Clip{}, err
	}
	c.Kind = filler.Kind(kind)
	c.Audience = filler.Audience(audience)
	c.TunarrProgramID = tunarrID.String // "" when NULL — the no-Tunarr case
	c.AITagged = aiTagged
	c.VisionTagged = visionTagged
	c.LastPlayedAt = fromEpoch(lastPlayedAt)
	if removedAt != 0 {
		c.RemovedAt = fromEpoch(removedAt)
	}
	c.Held = held
	c.AutoFiled = autoFiled
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

// ⚠ `boolToInt` was here and is DELETED (V38c). Every bool column is BOOLEAN on Postgres now that
// 00033 rebuilt `clips`, so binding an int is a 42804 — and this helper was how that mistake kept
// happening: it sat next to correct code, looked like the local idiom, and was copied onto three
// columns where it was wrong. A helper whose only remaining use is a bug is worth deleting
// outright rather than leaving for the next person to reach for.

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

// SetClipsHeld files clips into the catalog (held=false) or sends them back to the review queue
// (held=true) — the Incoming tab's decisions (§10 V38).
//
// ⚠ **This is the ONLY writer of `held` and `auto_filed`**, for the same reason SetClipsRemoved
// owns the tombstone: `UpsertClip` deliberately omits both from its DO UPDATE list, so the folder
// scan cannot file a held clip by finding its file still on disk. Route the write anywhere else
// and one scan pass empties the review queue into live channels.
//
// `autoFiled` records that no human looked at these before they became playable. It is set when
// the threshold files them and cleared whenever a person decides — because the flag's whole job
// is to answer "which of these did I never see?", and a human decision makes that answer no.
//
// Returns rows affected; unknown paths are skipped for the same reason as SetClipsRemoved — a
// bulk action races a re-scan, and failing the batch for one stale row is worse than doing the
// rest.
func (s *sqlStore) SetClipsHeld(ctx context.Context, paths []string, held, autoFiled bool, at time.Time) (int, error) {
	if len(paths) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(paths))
	args := make([]any, 0, len(paths)+3)
	args = append(args, held, autoFiled, epoch(at))
	for i, p := range paths {
		placeholders[i] = "?"
		args = append(args, p)
	}
	q := `UPDATE clips SET held = ?, auto_filed = ?, updated_at = ? WHERE path IN (` +
		strings.Join(placeholders, ",") + `)`
	res, err := s.db.ExecContext(ctx, s.ph(q), args...)
	if err != nil {
		return 0, fmt.Errorf("set clips held: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("set clips held: %w", err)
	}
	return int(n), nil
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

// SetClipLanguage records what the detection job heard (§10 V40, migration 00036).
//
// ⚠ **The ONLY writer of `language`**, exactly like SetClipsRemoved owns the tombstone and
// RecordClipPlay owns the counters — and for the same reason: `UpsertClip` deliberately omits the
// column, so the folder scan cannot blank a detected language by finding the file still on disk.
// Without that omission every sync would reset the catalog to "not yet checked" and the job would
// re-detect everything on the next pass, which on the local backend is ~341s per clip under QEMU.
//
// Keyed by PATH rather than hash, because that is what the job carries and what every other
// clip-writing method here takes.
func (s *sqlStore) SetClipLanguage(ctx context.Context, path, language string, at time.Time) error {
	_, err := s.db.ExecContext(ctx,
		s.ph(`UPDATE clips SET language = ?, updated_at = ? WHERE path = ?`),
		language, epoch(at), path)
	if err != nil {
		return fmt.Errorf("set clip language %s: %w", path, err)
	}
	return nil
}

// SetClipTranscript records what the transcribe job heard (§10 V44, migration 00038).
//
// ⚠ **The ONLY writer of `transcript`**, exactly like SetClipLanguage owns `language` and for the
// identical reason: `UpsertClip` deliberately omits the column, so the folder scan cannot blank a
// transcribed clip by finding its file still on disk. Without that omission every sync would reset
// the catalog to "not yet transcribed" and the job would re-run Whisper on everything — ~341s per
// clip under QEMU.
//
// Keyed by PATH rather than hash, because that is what the job carries and what SetClipLanguage
// beside it takes. An empty transcript is a legitimate write: it records "checked, wordless", which
// is what stops the selective job from re-visiting a silent clip forever.
func (s *sqlStore) SetClipTranscript(ctx context.Context, path, transcript string, at time.Time) error {
	_, err := s.db.ExecContext(ctx,
		s.ph(`UPDATE clips SET transcript = ?, updated_at = ? WHERE path = ?`),
		transcript, epoch(at), path)
	if err != nil {
		return fmt.Errorf("set clip transcript %s: %w", path, err)
	}
	return nil
}

// SetClipBrand records a GROUNDED advertiser found by the TEXT tagger (§10 V44, migration 00038).
//
// ⚠ Writes `brand` and nothing else — deliberately narrower than SetClipVisionTags, which also
// stamps `vision_tagged` and owns `visible_text`. The text tagger grounds a brand in the clip's
// text signals (filename, sidecar, or the persisted transcript); the vision pass grounds one in the
// on-screen text. Both are legitimate writers of `brand`, and each writes only what it is entitled
// to: this must NOT touch `vision_tagged`, or a text-tagged clip would masquerade as one a vision
// pass had read, and a re-run would skip the frame it never actually looked at.
//
// The caller has already applied the grounding rule (validateTags keeps a brand only when it
// appears literally in the text), so this writes what it is given. Keyed by PATH, like the transcript
// and vision writers it sits beside and unlike the hash-keyed UpdateClipTags — the job carries the
// path. `UpsertClip` omits `brand` from its DO UPDATE for the same single-writer reason as every
// other job column, so a re-sync cannot blank a grounded brand and make the tagger re-derive it.
func (s *sqlStore) SetClipBrand(ctx context.Context, path, brand string, at time.Time) error {
	_, err := s.db.ExecContext(ctx,
		s.ph(`UPDATE clips SET brand = ?, updated_at = ? WHERE path = ?`),
		brand, epoch(at), path)
	if err != nil {
		return fmt.Errorf("set clip brand %s: %w", path, err)
	}
	return nil
}

// SetClipVisionTags records what a vision pass read off a clip's keyframes (§10 V44, migration
// 00038): the on-screen text it saw, a grounded brand, and — when the frame supported them — an
// era and category. It stamps `vision_tagged` so a re-run does not pay for the same clip twice.
//
// ⚠ **The ONLY writer of `visible_text` and `vision_tagged`.** `brand` it SHARES with SetClipBrand
// (the text tagger's grounded writer) — both write `brand` and only `brand`, and this one must not
// be confused for the sole owner or a text-grounded brand would look vision-read. Same rule as
// every job column here: `UpsertClip` omits them so a re-sync cannot undo the pass. The caller has
// already applied the grounding rule (a brand/era/category survives only if `visibleText` supports
// it), so this method writes what it is given — the validation lives in the domain, not the SQL.
//
// era and category are written only when > 0 / non-empty, so a vision pass that grounded a brand
// but no era leaves an existing text-tagged era untouched. Keyed by PATH, like its neighbours.
func (s *sqlStore) SetClipVisionTags(ctx context.Context, path, brand, visibleText string, era, suggestedEra int, category string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		// COALESCE-style guards inline: only overwrite era/category when the vision pass produced a
		// grounded value, otherwise keep what text tagging already found. brand and visible_text are
		// vision's own to own, so they are written unconditionally.
		//
		// ⚠ suggested_era is written ONLY when the pass grounded NO era (the ? > 0 guard on
		// suggestedEra) AND does not clobber an existing suggestion: it is the frame-heuristic tier's
		// output (B&W + 4:3 ⇒ "pre-1970s?"), a QUESTION for the operator, never a fact — the same
		// role validateTags gives an ungrounded LLM year. A grounded era needs no suggestion, so a
		// pass that read a year off the frame leaves suggested_era alone.
		`UPDATE clips SET brand = ?, visible_text = ?, vision_tagged = ?,
		   era = CASE WHEN ? > 0 THEN ? ELSE era END,
		   suggested_era = CASE WHEN ? > 0 THEN 0 WHEN ? > 0 AND era = 0 AND suggested_era = 0 THEN ? ELSE suggested_era END,
		   category = CASE WHEN ? <> '' THEN ? ELSE category END,
		   updated_at = ? WHERE path = ?`),
		brand, visibleText, true,
		era, era,
		era, suggestedEra, suggestedEra,
		category, category,
		epoch(at), path)
	if err != nil {
		return fmt.Errorf("set clip vision tags %s: %w", path, err)
	}
	return nil
}
