package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mantonx/loomarr/internal/taxonomy"
)

// The taxonomy store (§10 V45a): the `taxa` graph (operator-editable) and the denormalised
// `clip_tags` rows. The graph is source of truth; clip_tags is a derived cache the reindex rebuilds.

// StoreTaxon is the persisted form of a taxonomy.Taxon. Synonyms/aliases are JSON arrays in the row.
type StoreTaxon struct {
	taxonomy.Taxon
	UpdatedAt time.Time
}

// ListTaxa returns the whole taxonomy graph, ordered by axis then slug (the stable order the served
// vocabulary and any diff rely on).
func (s *sqlStore) ListTaxa(ctx context.Context) ([]taxonomy.Taxon, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT slug, label, parent, axis, synonyms, aliases FROM taxa ORDER BY axis, slug`)
	if err != nil {
		return nil, fmt.Errorf("list taxa: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []taxonomy.Taxon
	for rows.Next() {
		var t taxonomy.Taxon
		var syn, ali string
		if err := rows.Scan(&t.Slug, &t.Label, &t.Parent, &t.Axis, &syn, &ali); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(syn), &t.Synonyms)
		_ = json.Unmarshal([]byte(ali), &t.RetiredAliases)
		out = append(out, t)
	}
	return out, rows.Err()
}

// UpsertTaxon writes (or updates) a taxon — the operator-edit path (§10 V45a). ⚠ It does NOT reindex:
// changing the graph means a clip's rollups may now differ, so the caller runs the reindex job after
// a graph edit. Keeping the two separate lets a bulk edit reindex once at the end rather than per row.
func (s *sqlStore) UpsertTaxon(ctx context.Context, t taxonomy.Taxon, at time.Time) error {
	syn, _ := json.Marshal(nonNil(t.Synonyms))
	ali, _ := json.Marshal(nonNil(t.RetiredAliases))
	_, err := s.db.ExecContext(ctx, s.ph(
		`INSERT INTO taxa (slug, label, parent, axis, synonyms, aliases, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(slug) DO UPDATE SET
		   label=excluded.label, parent=excluded.parent, axis=excluded.axis,
		   synonyms=excluded.synonyms, aliases=excluded.aliases, updated_at=excluded.updated_at`),
		t.Slug, t.Label, t.Parent, string(t.Axis), string(syn), string(ali), epoch(at))
	if err != nil {
		return fmt.Errorf("upsert taxon %s: %w", t.Slug, err)
	}
	return nil
}

// DeleteTaxon removes a taxon, REPARENTING its children to the deleted node's parent (§10 V45a).
// Returns ErrNotFound if the slug does not exist.
//
// ⚠ **Reparenting, not orphaning, is the intentional "remove a middle category" behaviour.** Deleting
// `ale-family` (between `lager` and `beer`) promotes `lager`'s parent to `beer`, so the lineage
// survives minus the removed level. WITHOUT this, a child kept its dead parent slug, and the rollup
// derivation then emitted that deleted slug as a phantom ancestor curation could never match — the
// bug the reindex conformance surfaced. (Forest.Ancestors ALSO now stops at a dangling parent, so a
// child orphaned by any OTHER path — a typo'd Upsert parent, a partial import — is safe too; this
// makes the common case preserve lineage rather than merely not-corrupt it.)
//
// ⚠ The caller must reindex afterward: reparenting changes surviving clips' rollups, and a clip
// tagged with the DELETED leaf itself loses that leaf. Both recompute in the next reindex.
func (s *sqlStore) DeleteTaxon(ctx context.Context, slug string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete taxon %s: begin: %w", slug, err)
	}
	defer func() { _ = tx.Rollback() }()

	// Find the deleted node's parent first — children inherit it.
	var parent string
	err = tx.QueryRowContext(ctx, s.ph(`SELECT parent FROM taxa WHERE slug = ?`), slug).Scan(&parent)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("delete taxon %s: lookup: %w", slug, err)
	}

	// Promote children to the grandparent. If the deleted node was a root (parent ""), its children
	// become roots — the correct degenerate case (a top-level category removed).
	if _, err := tx.ExecContext(ctx, s.ph(`UPDATE taxa SET parent = ? WHERE parent = ?`), parent, slug); err != nil {
		return fmt.Errorf("delete taxon %s: reparent children: %w", slug, err)
	}
	if _, err := tx.ExecContext(ctx, s.ph(`DELETE FROM taxa WHERE slug = ?`), slug); err != nil {
		return fmt.Errorf("delete taxon %s: %w", slug, err)
	}
	return tx.Commit()
}

// SeedTaxonomy writes the default forest IF AND ONLY IF `taxa` is empty (§10 V45a). Idempotent and
// cheap — run at boot after migrations. The empty-guard is what makes it safe on every open: a fresh
// install gets the seed, an operator-edited install keeps its graph untouched, a re-open no-ops.
func (s *sqlStore) SeedTaxonomy(ctx context.Context, seed []taxonomy.Taxon, at time.Time) error {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM taxa`).Scan(&n); err != nil {
		return fmt.Errorf("seed taxonomy count: %w", err)
	}
	if n > 0 {
		return nil // already seeded or operator-edited — never clobber
	}
	for _, t := range seed {
		if err := s.UpsertTaxon(ctx, t, at); err != nil {
			return err
		}
	}
	return nil
}

// SetClipTags REPLACES a clip's tags with the denormalised rollup expansion of the given LEAF slugs
// (§10 V45a). Each leaf is expanded through `forest` to its ancestors; the full set (leaf-flagged) is
// written, replacing whatever the clip had. This is the single writer of `clip_tags` for a clip.
//
// ⚠ A full replace, not a merge — re-tagging a clip must not accumulate stale leaves, and rollups must
// always reflect the CURRENT graph (an ancestor that was remapped away should vanish). The reindex
// job calls this per clip to rebuild rollups after a graph edit, re-deriving from the same leaves.
func (s *sqlStore) SetClipTags(ctx context.Context, clipHash string, leaves []string, forest *taxonomy.Forest, at time.Time) error {
	// Expand leaves → the full leaf+rollup set, de-duplicated (two leaves can share an ancestor:
	// beer and spirits both roll up to alcohol/drinks — the ancestor is stored once, as a rollup).
	rowByTaxon := map[string]bool{} // taxon → isLeaf (leaf wins if a slug is both a leaf and a rollup)
	for _, leaf := range leaves {
		for _, r := range forest.WithRollups(leaf) {
			if r.Leaf {
				rowByTaxon[r.Slug] = true
			} else if _, ok := rowByTaxon[r.Slug]; !ok {
				rowByTaxon[r.Slug] = false
			}
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("set clip tags: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, s.ph(`DELETE FROM clip_tags WHERE clip_hash = ?`), clipHash); err != nil {
		return fmt.Errorf("set clip tags: clear: %w", err)
	}
	for taxon, leaf := range rowByTaxon {
		if _, err := tx.ExecContext(ctx, s.ph(
			`INSERT INTO clip_tags (clip_hash, taxon, leaf) VALUES (?, ?, ?)`),
			clipHash, taxon, leaf); err != nil {
			return fmt.Errorf("set clip tags: insert %s: %w", taxon, err)
		}
	}
	return tx.Commit()
}

// GetClipTags returns a clip's tags. `leavesOnly` restricts to the asserted leaves (what to re-derive
// rollups from); false returns the full leaf+rollup set (what curation matches against).
func (s *sqlStore) GetClipTags(ctx context.Context, clipHash string, leavesOnly bool) ([]string, error) {
	q := `SELECT taxon FROM clip_tags WHERE clip_hash = ?`
	args := []any{clipHash}
	if leavesOnly {
		q += ` AND leaf = ?`
		args = append(args, true)
	}
	q += ` ORDER BY taxon`
	rows, err := s.db.QueryContext(ctx, s.ph(q), args...)
	if err != nil {
		return nil, fmt.Errorf("get clip tags: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var taxon string
		if err := rows.Scan(&taxon); err != nil {
			return nil, err
		}
		out = append(out, taxon)
	}
	return out, rows.Err()
}

// ListClipHashesLeaves returns every clip that has at least one LEAF tag, with its leaves — the
// reindex job's work list (it re-derives rollups for each from the current graph). Ordered by hash so
// a paginated reindex is stable.
func (s *sqlStore) ListClipHashesLeaves(ctx context.Context) (map[string][]string, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT clip_hash, taxon FROM clip_tags WHERE leaf = ? ORDER BY clip_hash`), true)
	if err != nil {
		return nil, fmt.Errorf("list clip leaves: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string][]string{}
	for rows.Next() {
		var hash, taxon string
		if err := rows.Scan(&hash, &taxon); err != nil {
			return nil, err
		}
		out[hash] = append(out[hash], taxon)
	}
	return out, rows.Err()
}

// RebuildClosure recomputes the `taxa_closure` table from the given forest (§10 V45a) — one row per
// (ancestor, descendant) pair in the graph's transitive closure, INCLUDING the self-pair. This is the
// ONLY writer of taxa_closure, and it runs whenever the GRAPH edits (rare), NOT per clip: the graph
// walk (taxonomy.Forest.Ancestors) stays a Go concern and produces a handful of rows, so the hot
// rollup rebuild is a plain SQL join with no recursion.
//
// ⚠ A full replace inside one transaction. The closure is a pure function of the graph, so a partial
// rebuild would leave a clip's rollups deriving from a half-updated closure — worse than the old one.
// Delete-all + re-insert is safe because it is a few hundred rows (~55 taxa) and it is atomic.
func (s *sqlStore) RebuildClosure(ctx context.Context, forest *taxonomy.Forest, at time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("rebuild closure: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM taxa_closure`); err != nil {
		return fmt.Errorf("rebuild closure: clear: %w", err)
	}
	for _, t := range forest.All() {
		// The self-pair (distance 0): a leaf's own row in the rollup join. Without it the bulk
		// INSERT..SELECT would expand rollups but never write the asserted leaf itself.
		if err := s.insertClosure(ctx, tx, t.Slug, t.Slug); err != nil {
			return err
		}
		for _, anc := range forest.Ancestors(t.Slug) {
			if err := s.insertClosure(ctx, tx, anc, t.Slug); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// insertClosure writes one (ancestor, descendant) pair.
func (s *sqlStore) insertClosure(ctx context.Context, tx *sql.Tx, ancestor, descendant string) error {
	if _, err := tx.ExecContext(ctx, s.ph(
		`INSERT INTO taxa_closure (ancestor, descendant) VALUES (?, ?)`),
		ancestor, descendant); err != nil {
		return fmt.Errorf("insert closure %s<-%s: %w", ancestor, descendant, err)
	}
	return nil
}

// RebuildRollups recomputes the ROLLUP rows of clip_tags for every clip in ONE set-based statement
// (§10 V45a) — the reindex the doc calls for, expressed as a bulk join rather than an app-side loop
// that would N+1 once the catalog balloons past the auto-fetch throttle.
//
// It preserves each clip's asserted LEAF rows (the operator/tagger's assertions — those change only
// via SetClipTags) and re-derives every ROLLUP from the current closure:
//
//	DELETE the rollups (leaf = false);
//	INSERT, for each surviving leaf row, every ancestor from taxa_closure that is NOT the leaf itself.
//
// ⚠ Dialect-neutral by construction. Every inserted row is a ROLLUP, so `leaf` is a constant boolean
// FALSE literal — Postgres rejects an integer `0` into a BOOLEAN column (error 42804; see migration
// 00029's note), while SQLite accepts the FALSE keyword (a 1/0 alias since 3.23, which modernc tracks),
// so the FALSE literal is the one form both dialects take. The leaf rows are never deleted, so they are
// not re-inserted here.
//
// ⚠ RebuildClosure must have run against the current graph FIRST — this reads taxa_closure, so a stale
// closure yields stale rollups. The graph-edit path calls them in order (closure, then rollups).
func (s *sqlStore) RebuildRollups(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("rebuild rollups: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Drop every derived rollup; the asserted leaves stay.
	if _, err := tx.ExecContext(ctx, s.ph(`DELETE FROM clip_tags WHERE leaf = ?`), false); err != nil {
		return fmt.Errorf("rebuild rollups: clear: %w", err)
	}

	// Re-derive: for each asserted leaf, insert its strict ancestors (closure minus the self-pair) as
	// rollup rows. DISTINCT collapses two leaves of one clip that share an ancestor (beer + spirits
	// both roll up to alcohol — the ancestor is stored once). The ancestor may coincide with ANOTHER
	// leaf the clip asserts (a clip tagged both `drinks` and `beer`); the ON CONFLICT keeps the leaf
	// row rather than overwriting it with a rollup — a leaf is never demoted to a rollup.
	if _, err := tx.ExecContext(ctx, s.ph(
		`INSERT INTO clip_tags (clip_hash, taxon, leaf)
		 SELECT DISTINCT l.clip_hash, c.ancestor, FALSE
		 FROM clip_tags l
		 JOIN taxa_closure c ON c.descendant = l.taxon
		 WHERE l.leaf = ? AND c.ancestor <> c.descendant
		 ON CONFLICT (clip_hash, taxon) DO NOTHING`), true); err != nil {
		return fmt.Errorf("rebuild rollups: insert: %w", err)
	}
	return tx.Commit()
}

// nonNil returns a non-nil slice so json.Marshal emits `[]` not `null` — keeps the stored JSON stable.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
