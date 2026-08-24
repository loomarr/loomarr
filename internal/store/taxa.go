package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/taxonomy"
)

// The taxonomy store (§10 V45a): the `taxa` graph (operator-editable) and the denormalised
// `clip_tags` rows. The graph is source of truth; graph and per-clip writes own their derived rows.

// StoreTaxon is the persisted form of a taxonomy.Taxon. Synonyms/aliases are JSON arrays in the row.
type StoreTaxon struct {
	taxonomy.Taxon
	UpdatedAt time.Time
}

// TaxonomyEdit is one semantic graph mutation. Create distinguishes an accidental duplicate from
// an edit; Delete addresses Slug and ignores Taxon. Update keeps Taxon.Slug immutable.
type TaxonomyEdit struct {
	Create bool
	Delete bool
	Slug   string
	Taxon  taxonomy.Taxon
}

// TaxonomyEditImpact is the durable-library half of a prospective graph edit. Channel policy
// references are deliberately added by the app module: the store owns graph and clip accounting,
// while the channel domain owns the meaning of a saved filler selection.
type TaxonomyEditImpact struct {
	DirectStoredClips     int
	DescendantStoredClips int
	AffectedStoredClips   int
	PlayableClipHashes    []string
	Descendants           []taxonomy.Taxon
	ResolverTermsAdded    []string
	ResolverTermsRemoved  []string
}

type TaxonUsage struct {
	Asserted int
	Matched  int
	// Stored includes direct assignments on held, removed, and composite records too. Those clips
	// are outside playable coverage but still preserve library knowledge and block deletion.
	Stored int
}

type TaxonomyUsage struct {
	TotalClips  int
	TaggedClips int
	ByTaxon     map[string]TaxonUsage
	// ByAxis counts playable clips with at least one direct assertion on each independent axis.
	// It cannot be derived by summing taxa because one clip may assert several nodes on an axis.
	ByAxis map[taxonomy.Axis]int
}

// ListTaxa returns the whole taxonomy graph, ordered by axis then slug (the stable order the served
// vocabulary and any diff rely on).
func (s *sqlStore) ListTaxa(ctx context.Context) ([]taxonomy.Taxon, error) {
	return listTaxaFrom(ctx, s.db)
}

type taxonomyQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func listTaxaFrom(ctx context.Context, q taxonomyQueryer) ([]taxonomy.Taxon, error) {
	rows, err := q.QueryContext(ctx, `SELECT slug, label, parent, axis, synonyms, aliases FROM taxa ORDER BY axis, slug`)
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

// upsertTaxon is the boot-seed primitive. Operator edits go through ApplyTaxonomyEdit so validation,
// graph mutation, closure and rollups share one transaction.
func (s *sqlStore) upsertTaxon(ctx context.Context, t taxonomy.Taxon, at time.Time) error {
	return s.upsertTaxonExec(ctx, s.db, t, at)
}

type taxonomyExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (s *sqlStore) upsertTaxonTx(ctx context.Context, tx *sql.Tx, t taxonomy.Taxon, at time.Time) error {
	return s.upsertTaxonExec(ctx, tx, t, at)
}

func (s *sqlStore) upsertTaxonExec(ctx context.Context, exec taxonomyExecer, t taxonomy.Taxon, at time.Time) error {
	syn, _ := json.Marshal(nonNil(t.Synonyms))
	ali, _ := json.Marshal(nonNil(t.RetiredAliases))
	_, err := exec.ExecContext(ctx, s.ph(
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

// ApplyTaxonomyEdit is the taxonomy's deep write interface: one semantic change enters, and either
// the graph plus both derived caches commit together or nothing changes. Postgres takes a
// transaction-scoped advisory lock because its default READ COMMITTED transaction would otherwise
// let two graph editors validate different snapshots and interleave their closure replacements.
func (s *sqlStore) ApplyTaxonomyEdit(ctx context.Context, edit TaxonomyEdit, at time.Time) error {
	edit.Slug = strings.TrimSpace(edit.Slug)
	edit.Taxon = taxonomy.Canonicalize(edit.Taxon)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("edit taxonomy: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if s.dialect == DialectPostgres {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('loomarr-taxonomy'))`); err != nil {
			return fmt.Errorf("edit taxonomy: lock: %w", err)
		}
	}
	existing, err := listTaxaFrom(ctx, tx)
	if err != nil {
		return err
	}
	prospective, current, err := planTaxonomyEdit(existing, edit)
	if err != nil {
		return err
	}
	slug := current.Slug
	if edit.Create {
		slug = edit.Taxon.Slug
	}
	if edit.Delete {
		var direct int
		if err := tx.QueryRowContext(ctx, s.ph(`SELECT COUNT(*) FROM clip_tags WHERE taxon = ? AND leaf = ?`), slug, true).Scan(&direct); err != nil {
			return fmt.Errorf("delete taxon %s: count assertions: %w", slug, err)
		}
		if direct > 0 {
			return fmt.Errorf("%w: taxon %q is directly asserted on %d clips; retag them first", ErrTaxonConflict, slug, direct)
		}
	}

	if edit.Delete {
		parent := current.Parent
		if _, err := tx.ExecContext(ctx, s.ph(`UPDATE taxa SET parent = ?, updated_at = ? WHERE parent = ?`), parent, epoch(at), slug); err != nil {
			return fmt.Errorf("delete taxon %s: reparent children: %w", slug, err)
		}
		if _, err := tx.ExecContext(ctx, s.ph(`DELETE FROM clip_tags WHERE taxon = ?`), slug); err != nil {
			return fmt.Errorf("delete taxon %s: clear derived rows: %w", slug, err)
		}
		if _, err := tx.ExecContext(ctx, s.ph(`DELETE FROM taxa WHERE slug = ?`), slug); err != nil {
			return fmt.Errorf("delete taxon %s: %w", slug, err)
		}
	} else if err := s.upsertTaxonTx(ctx, tx, edit.Taxon, at); err != nil {
		return err
	}

	forest := taxonomy.New(prospective)
	if err := s.rebuildClosureTx(ctx, tx, forest); err != nil {
		return err
	}
	if err := s.rebuildRollupsTx(ctx, tx); err != nil {
		return err
	}
	if err := s.rebuildCategoryShadowsTx(ctx, tx, ""); err != nil {
		return err
	}
	return tx.Commit()
}

// PreviewTaxonomyEdit validates the same prospective graph ApplyTaxonomyEdit will use and returns
// the stored knowledge and playable clips whose derived lineage could change. It performs no write;
// callers must still treat ApplyTaxonomyEdit as authoritative if another editor wins the race.
func (s *sqlStore) PreviewTaxonomyEdit(ctx context.Context, edit TaxonomyEdit) (TaxonomyEditImpact, error) {
	edit.Slug = strings.TrimSpace(edit.Slug)
	edit.Taxon = taxonomy.Canonicalize(edit.Taxon)
	existing, err := s.ListTaxa(ctx)
	if err != nil {
		return TaxonomyEditImpact{}, err
	}
	_, current, err := planTaxonomyEdit(existing, edit)
	if err != nil {
		return TaxonomyEditImpact{}, err
	}

	impact := TaxonomyEditImpact{}
	if edit.Create {
		impact.ResolverTermsAdded = resolverTerms(edit.Taxon)
		return impact, nil
	}
	impact.Descendants = taxonomy.New(existing).Descendants(current.Slug)
	affected := []string{current.Slug}
	descendants := make([]string, 0, len(impact.Descendants))
	for _, child := range impact.Descendants {
		affected = append(affected, child.Slug)
		descendants = append(descendants, child.Slug)
	}
	if impact.DirectStoredClips, err = s.countStoredAssertions(ctx, []string{current.Slug}); err != nil {
		return TaxonomyEditImpact{}, err
	}
	if impact.DescendantStoredClips, err = s.countStoredAssertions(ctx, descendants); err != nil {
		return TaxonomyEditImpact{}, err
	}
	if impact.AffectedStoredClips, err = s.countStoredAssertions(ctx, affected); err != nil {
		return TaxonomyEditImpact{}, err
	}
	if impact.PlayableClipHashes, err = s.playableHashesWithAssertions(ctx, affected); err != nil {
		return TaxonomyEditImpact{}, err
	}

	beforeTerms := resolverTerms(current)
	if edit.Delete {
		impact.ResolverTermsRemoved = beforeTerms
	} else {
		afterTerms := resolverTerms(edit.Taxon)
		impact.ResolverTermsAdded, impact.ResolverTermsRemoved = termDiff(beforeTerms, afterTerms)
	}
	return impact, nil
}

func planTaxonomyEdit(existing []taxonomy.Taxon, edit TaxonomyEdit) ([]taxonomy.Taxon, taxonomy.Taxon, error) {
	idx := -1
	slug := edit.Taxon.Slug
	if edit.Delete {
		slug = edit.Slug
	}
	for i, t := range existing {
		if t.Slug == slug {
			idx = i
			break
		}
	}
	if edit.Create && idx >= 0 {
		return nil, taxonomy.Taxon{}, fmt.Errorf("%w: taxon %q already exists", ErrTaxonConflict, slug)
	}
	if !edit.Create && idx < 0 {
		return nil, taxonomy.Taxon{}, ErrNotFound
	}

	prospective := append([]taxonomy.Taxon(nil), existing...)
	var current taxonomy.Taxon
	switch {
	case edit.Delete:
		current = existing[idx]
		prospective = append(prospective[:idx], prospective[idx+1:]...)
		for i := range prospective {
			if prospective[i].Parent == slug {
				prospective[i].Parent = current.Parent
			}
		}
	case edit.Create:
		prospective = append(prospective, edit.Taxon)
	case !edit.Create:
		current = existing[idx]
		edit.Taxon.Slug = slug
		prospective[idx] = edit.Taxon
	}
	if err := taxonomy.Validate(prospective); err != nil {
		return nil, taxonomy.Taxon{}, err
	}
	return prospective, current, nil
}

func (s *sqlStore) countStoredAssertions(ctx context.Context, slugs []string) (int, error) {
	if len(slugs) == 0 {
		return 0, nil
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(slugs)), ",")
	args := make([]any, 0, len(slugs)+1)
	args = append(args, true)
	for _, slug := range slugs {
		args = append(args, slug)
	}
	var n int
	if err := s.db.QueryRowContext(ctx, s.ph(`SELECT COUNT(DISTINCT clip_hash) FROM clip_tags WHERE leaf = ? AND taxon IN (`+marks+`)`), args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("preview taxonomy stored assertions: %w", err)
	}
	return n, nil
}

func (s *sqlStore) playableHashesWithAssertions(ctx context.Context, slugs []string) ([]string, error) {
	if len(slugs) == 0 {
		return nil, nil
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(slugs)), ",")
	args := []any{true}
	for _, slug := range slugs {
		args = append(args, slug)
	}
	args = append(args, false, false, 0)
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT DISTINCT c.hash FROM clips c JOIN clip_tags ct ON ct.clip_hash = c.hash WHERE ct.leaf = ? AND ct.taxon IN (`+marks+`) AND c.held = ? AND c.is_composite = ? AND c.removed_at = ? ORDER BY c.hash`), args...)
	if err != nil {
		return nil, fmt.Errorf("preview taxonomy playable clips: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var hashes []string
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			return nil, err
		}
		hashes = append(hashes, hash)
	}
	return hashes, rows.Err()
}

func resolverTerms(t taxonomy.Taxon) []string {
	terms := append([]string{t.Slug}, t.Synonyms...)
	terms = append(terms, t.RetiredAliases...)
	seen := map[string]bool{}
	out := make([]string, 0, len(terms))
	for _, raw := range terms {
		term := strings.ToLower(strings.TrimSpace(raw))
		if term != "" && !seen[term] {
			seen[term] = true
			out = append(out, term)
		}
	}
	sort.Strings(out)
	return out
}

func termDiff(before, after []string) (added, removed []string) {
	beforeSet, afterSet := map[string]bool{}, map[string]bool{}
	for _, term := range before {
		beforeSet[term] = true
	}
	for _, term := range after {
		afterSet[term] = true
		if !beforeSet[term] {
			added = append(added, term)
		}
	}
	for _, term := range before {
		if !afterSet[term] {
			removed = append(removed, term)
		}
	}
	return added, removed
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
		if err := s.upsertTaxon(ctx, t, at); err != nil {
			return err
		}
	}
	return nil
}

// SetClipTags is the per-clip taxonomy transaction. It replaces asserted leaves, expands them
// through the CURRENT closure, and refreshes the compatibility category shadow before commit.
// Postgres takes a shared taxonomy lock so an operator graph edit cannot commit between those
// writes; importantly, the caller supplies no forest snapshot that can become stale while waiting.
func (s *sqlStore) SetClipTags(ctx context.Context, clipHash string, leaves []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("set clip tags: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if s.dialect == DialectPostgres {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock_shared(hashtext('loomarr-taxonomy'))`); err != nil {
			return fmt.Errorf("set clip tags: lock: %w", err)
		}
	}
	if err := s.setClipTagsTx(ctx, tx, clipHash, leaves); err != nil {
		return err
	}
	return tx.Commit()
}

// setClipTagsTx is the transactional taxonomy projection primitive shared by direct clip edits and
// semantic classifier writes. The caller owns the transaction and the shared taxonomy lock.
func (s *sqlStore) setClipTagsTx(ctx context.Context, tx *sql.Tx, clipHash string, leaves []string) error {
	var clipExists int
	if err := tx.QueryRowContext(ctx, s.ph(`SELECT COUNT(*) FROM clips WHERE hash = ?`), clipHash).Scan(&clipExists); err != nil {
		return fmt.Errorf("set clip tags: find clip: %w", err)
	}
	if clipExists == 0 {
		return ErrNotFound
	}
	uniqueLeaves := make([]string, 0, len(leaves))
	seenLeaves := make(map[string]bool, len(leaves))
	for _, leaf := range leaves {
		if seenLeaves[leaf] {
			continue
		}
		seenLeaves[leaf] = true
		var exists int
		if err := tx.QueryRowContext(ctx, s.ph(`SELECT COUNT(*) FROM taxa WHERE slug = ?`), leaf).Scan(&exists); err != nil {
			return fmt.Errorf("set clip tags: validate %s: %w", leaf, err)
		}
		if exists == 0 {
			return fmt.Errorf("%w: taxon %q no longer exists", ErrTaxonConflict, leaf)
		}
		uniqueLeaves = append(uniqueLeaves, leaf)
	}
	if _, err := tx.ExecContext(ctx, s.ph(`DELETE FROM clip_tags WHERE clip_hash = ?`), clipHash); err != nil {
		return fmt.Errorf("set clip tags: clear: %w", err)
	}
	for _, leaf := range uniqueLeaves {
		if _, err := tx.ExecContext(ctx, s.ph(
			`INSERT INTO clip_tags (clip_hash, taxon, leaf) VALUES (?, ?, ?)`),
			clipHash, leaf, true); err != nil {
			return fmt.Errorf("set clip tags: insert %s: %w", leaf, err)
		}
	}
	if err := s.rebuildClipRollupsTx(ctx, tx, clipHash); err != nil {
		return err
	}
	if err := s.rebuildCategoryShadowsTx(ctx, tx, clipHash); err != nil {
		return err
	}
	return nil
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

// rebuildTaxonomyDerived is the boot/upgrade backstop for all projections owned by the taxonomy.
// Live graph changes use ApplyTaxonomyEdit; boot uses the same exclusive Postgres lock so replicas
// cannot interleave full replacements. A single transaction keeps closure, rollups, and category on
// one generation while healing data written before graph edits became atomic.
//
// ⚠ A full replace inside one transaction. The closure is a pure function of the graph, so a partial
// rebuild would leave a clip's rollups deriving from a half-updated closure — worse than the old one.
// Delete-all + re-insert is safe because it is a few hundred rows (~55 taxa) and it is atomic.
func (s *sqlStore) rebuildTaxonomyDerived(ctx context.Context, forest *taxonomy.Forest) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("rebuild taxonomy: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if s.dialect == DialectPostgres {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('loomarr-taxonomy'))`); err != nil {
			return fmt.Errorf("rebuild taxonomy: lock: %w", err)
		}
	}
	if err := s.rebuildClosureTx(ctx, tx, forest); err != nil {
		return err
	}
	if err := s.rebuildRollupsTx(ctx, tx); err != nil {
		return err
	}
	if err := s.rebuildCategoryShadowsTx(ctx, tx, ""); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *sqlStore) rebuildClosureTx(ctx context.Context, tx *sql.Tx, forest *taxonomy.Forest) error {
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
	return nil
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

// rebuildRollupsTx recomputes every derived row in the graph-edit transaction. It is deliberately
// private: callers must not rebuild closure and rollups as two independently visible generations.
func (s *sqlStore) rebuildRollupsTx(ctx context.Context, tx *sql.Tx) error {
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
	return nil
}

func (s *sqlStore) rebuildClipRollupsTx(ctx context.Context, tx *sql.Tx, clipHash string) error {
	if _, err := tx.ExecContext(ctx, s.ph(
		`INSERT INTO clip_tags (clip_hash, taxon, leaf)
		 SELECT DISTINCT l.clip_hash, c.ancestor, FALSE
		 FROM clip_tags l
		 JOIN taxa_closure c ON c.descendant = l.taxon
		 WHERE l.clip_hash = ? AND l.leaf = ? AND c.ancestor <> c.descendant
		 ON CONFLICT (clip_hash, taxon) DO NOTHING`), clipHash, true); err != nil {
		return fmt.Errorf("rebuild clip rollups: insert: %w", err)
	}
	return nil
}

// rebuildCategoryShadowsTx keeps the compatibility `clips.category` field a pure function of the
// live taxonomy. Moving an asserted node off the product axis, or changing relative graph depth,
// can change PrimaryProductLeaf without touching the clip's assertions. Leaving the shadow behind
// would make legacy pod matching disagree with the taxonomy browser after an otherwise-atomic edit.
//
// One correlated UPDATE is deliberately used on both dialects. taxa_closure includes each node's
// self-pair, so its row count is the node's stable depth measure; ties use slug order exactly like
// taxonomy.Forest.PrimaryProductLeaf.
func (s *sqlStore) rebuildCategoryShadowsTx(ctx context.Context, tx *sql.Tx, clipHash string) error {
	query := `UPDATE clips SET category = COALESCE((
		SELECT ct.taxon
		FROM clip_tags ct
		JOIN taxa t ON t.slug = ct.taxon
		WHERE ct.clip_hash = clips.hash AND ct.leaf = TRUE AND t.axis = 'product'
		ORDER BY (SELECT COUNT(*) FROM taxa_closure tc WHERE tc.descendant = ct.taxon) DESC,
			ct.taxon ASC
		LIMIT 1
	), '')`
	var args []any
	if clipHash != "" {
		query += ` WHERE hash = ?`
		args = append(args, clipHash)
	}
	_, err := tx.ExecContext(ctx, s.ph(query), args...)
	if err != nil {
		return fmt.Errorf("rebuild taxonomy category shadows: %w", err)
	}
	return nil
}

// TaxonomyUsage accounts for the PLAYABLE catalog (the zero ClipFilter population): held,
// tombstoned and composite containers are excluded. Per-taxon Matched counts include descendants
// through denormalised rollups; Asserted counts only rows authored by a classifier/operator.
func (s *sqlStore) TaxonomyUsage(ctx context.Context) (TaxonomyUsage, error) {
	out := TaxonomyUsage{ByTaxon: map[string]TaxonUsage{}, ByAxis: map[taxonomy.Axis]int{}}
	active := `c.removed_at = 0 AND c.held = FALSE AND c.is_composite = FALSE`
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM clips c WHERE `+active).Scan(&out.TotalClips); err != nil {
		return out, fmt.Errorf("taxonomy usage total: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT ct.clip_hash)
		FROM clip_tags ct JOIN clips c ON c.hash = ct.clip_hash
		WHERE ct.leaf = TRUE AND `+active).Scan(&out.TaggedClips); err != nil {
		return out, fmt.Errorf("taxonomy usage tagged: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT ct.taxon,
		COUNT(DISTINCT CASE WHEN ct.leaf = TRUE THEN ct.clip_hash END),
		COUNT(DISTINCT ct.clip_hash)
		FROM clip_tags ct JOIN clips c ON c.hash = ct.clip_hash
		WHERE `+active+` GROUP BY ct.taxon ORDER BY ct.taxon`)
	if err != nil {
		return out, fmt.Errorf("taxonomy usage by taxon: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var slug string
		var usage TaxonUsage
		if err := rows.Scan(&slug, &usage.Asserted, &usage.Matched); err != nil {
			return out, err
		}
		out.ByTaxon[slug] = usage
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	if err := rows.Close(); err != nil {
		return out, err
	}
	axisRows, err := s.db.QueryContext(ctx, `SELECT t.axis, COUNT(DISTINCT ct.clip_hash)
		FROM clip_tags ct
		JOIN taxa t ON t.slug = ct.taxon
		JOIN clips c ON c.hash = ct.clip_hash
		WHERE ct.leaf = TRUE AND `+active+`
		GROUP BY t.axis ORDER BY t.axis`)
	if err != nil {
		return out, fmt.Errorf("taxonomy usage by axis: %w", err)
	}
	defer func() { _ = axisRows.Close() }()
	for axisRows.Next() {
		var axis taxonomy.Axis
		var count int
		if err := axisRows.Scan(&axis, &count); err != nil {
			return out, err
		}
		out.ByAxis[axis] = count
	}
	if err := axisRows.Err(); err != nil {
		return out, err
	}
	if err := axisRows.Close(); err != nil {
		return out, err
	}
	stored, err := s.db.QueryContext(ctx, `SELECT taxon, COUNT(DISTINCT clip_hash)
		FROM clip_tags WHERE leaf = TRUE GROUP BY taxon ORDER BY taxon`)
	if err != nil {
		return out, fmt.Errorf("taxonomy stored assertions: %w", err)
	}
	defer func() { _ = stored.Close() }()
	for stored.Next() {
		var slug string
		var count int
		if err := stored.Scan(&slug, &count); err != nil {
			return out, err
		}
		usage := out.ByTaxon[slug]
		usage.Stored = count
		out.ByTaxon[slug] = usage
	}
	return out, stored.Err()
}

// nonNil returns a non-nil slice so json.Marshal emits `[]` not `null` — keeps the stored JSON stable.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
