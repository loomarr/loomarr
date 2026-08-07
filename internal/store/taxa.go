package store

import (
	"context"
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

// DeleteTaxon removes a taxon. ⚠ The caller must reindex afterward (clips tagged under it lose that
// leaf; their rollups recompute). Returns ErrNotFound if the slug does not exist.
func (s *sqlStore) DeleteTaxon(ctx context.Context, slug string) error {
	res, err := s.db.ExecContext(ctx, s.ph(`DELETE FROM taxa WHERE slug = ?`), slug)
	if err != nil {
		return fmt.Errorf("delete taxon %s: %w", slug, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
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

// nonNil returns a non-nil slice so json.Marshal emits `[]` not `null` — keeps the stored JSON stable.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
