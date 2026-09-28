package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

// ClipTx writes a clip's own columns and tags inside a transaction another part of the store began
// (Handle.Begin). The filler store's projections go through it rather than updating clips
// directly, so the clips table and its tag invariants keep one owner however a write is composed
// (#1747). Each method is one statement or one tag rewrite; none commits.
type ClipTx struct {
	s  *sqlStore
	tx Querier
}

// clipsIn is ClipTx over tx for code inside this package; Handle.Clips is the extension stores' way in.
func (s *sqlStore) clipsIn(tx Querier) ClipTx { return ClipTx{s: s, tx: tx} }

// ClipGeography is the broadcast provenance a clip's geography columns hold.
type ClipGeography struct {
	Scope, Country, Market, Network, Station, AirDate string
}

// Require reports ErrNotFound for a clip that does not exist.
func (c ClipTx) Require(ctx context.Context, hash string) error {
	var n int
	if err := c.tx.QueryRowContext(ctx, c.s.ph(`SELECT COUNT(*) FROM clips WHERE hash = ?`), hash).Scan(&n); err != nil {
		return fmt.Errorf("find clip: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// EnrichmentRevision reads the clip's descriptive-input revision. On Postgres it locks the row
// until the transaction ends, so a pass records against the revision it read.
func (c ClipTx) EnrichmentRevision(ctx context.Context, hash string) (int64, error) {
	query := `SELECT enrichment_revision FROM clips WHERE hash = ?`
	if c.s.dialect == DialectPostgres {
		query += ` FOR UPDATE`
	}
	var revision int64
	err := c.tx.QueryRowContext(ctx, c.s.ph(query), hash).Scan(&revision)
	return revision, err
}

// ClassifyKind sets the kind of a clip that is still unclassified; a classified clip keeps its kind.
func (c ClipTx) ClassifyKind(ctx context.Context, hash string, kind filler.Kind, at time.Time) error {
	_, err := c.tx.ExecContext(ctx, c.s.ph(`UPDATE clips SET kind = ?, updated_at = ? WHERE hash = ? AND kind = ?`),
		string(kind), epoch(at), hash, string(filler.Unclassified))
	return err
}

// SetEra sets the clip's era year.
func (c ClipTx) SetEra(ctx context.Context, hash string, year int, at time.Time) error {
	_, err := c.tx.ExecContext(ctx, c.s.ph(`UPDATE clips SET era = ?, updated_at = ? WHERE hash = ?`), year, epoch(at), hash)
	return err
}

// FillAudience sets the audience of a clip that has none.
func (c ClipTx) FillAudience(ctx context.Context, hash, audience string, at time.Time) error {
	_, err := c.tx.ExecContext(ctx, c.s.ph(`UPDATE clips SET audience = ?, updated_at = ? WHERE hash = ? AND audience = ''`), audience, epoch(at), hash)
	return err
}

// SetBrand sets the clip's brand.
func (c ClipTx) SetBrand(ctx context.Context, hash, brand string, at time.Time) error {
	_, err := c.tx.ExecContext(ctx, c.s.ph(`UPDATE clips SET brand = ?, updated_at = ? WHERE hash = ?`), brand, epoch(at), hash)
	return err
}

// FillLanguage sets the language of a clip that has none.
func (c ClipTx) FillLanguage(ctx context.Context, hash, language string, at time.Time) error {
	_, err := c.tx.ExecContext(ctx, c.s.ph(`UPDATE clips SET language = ?, updated_at = ? WHERE hash = ? AND language = ''`), language, epoch(at), hash)
	return err
}

// FillGeography sets the geography of a clip whose scope is unknown and country empty, with the
// evidence reference that justified it.
func (c ClipTx) FillGeography(ctx context.Context, hash string, g ClipGeography, evidence string, at time.Time) error {
	_, err := c.tx.ExecContext(ctx, c.s.ph(`UPDATE clips SET geographic_scope = ?, country = ?, market = ?,
		network = ?, station = ?, air_date = ?, geo_evidence = ?, updated_at = ?
		WHERE hash = ? AND (geographic_scope = '' OR geographic_scope = 'unknown') AND country = ''`),
		g.Scope, g.Country, g.Market, g.Network, g.Station, g.AirDate, evidence, epoch(at), hash)
	return err
}

// AddLeafTags adds taxonomy leaves to the clip's asserted leaves and rebuilds its rollups, under
// the same taxonomy lock as graph edits. Leaves it already has are kept once.
func (c ClipTx) AddLeafTags(ctx context.Context, hash string, tags []string) error {
	if c.s.dialect == DialectPostgres {
		if _, err := c.tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock_shared(hashtext('loomarr-taxonomy'))`); err != nil {
			return fmt.Errorf("add clip tags: taxonomy lock: %w", err)
		}
	}
	leaves, err := getClipTagsFrom(ctx, c.tx, c.s.ph, hash, true)
	if err != nil {
		return err
	}
	seen := make(map[string]bool, len(leaves)+len(tags))
	for _, leaf := range leaves {
		seen[leaf] = true
	}
	for _, tag := range tags {
		if !seen[tag] {
			leaves = append(leaves, tag)
			seen[tag] = true
		}
	}
	sort.Strings(leaves)
	return c.s.setClipTagsTx(ctx, c.tx, hash, leaves)
}

// RecordTranscript stores the clip's transcript, advancing its enrichment revision when it changed.
func (c ClipTx) RecordTranscript(ctx context.Context, hash, transcript string, at time.Time) error {
	_, err := c.tx.ExecContext(ctx, c.s.ph(`UPDATE clips SET
		enrichment_revision = CASE WHEN transcript <> ? THEN enrichment_revision + 1 ELSE enrichment_revision END,
		transcript = ?, updated_at = ? WHERE hash = ?`),
		transcript, transcript, epoch(at), hash)
	return err
}

// RecordVision stores what the vision rung read on screen and marks the clip vision-tagged,
// advancing its enrichment revision on a change. A suggested era is kept only while the clip has
// no era and no earlier suggestion.
func (c ClipTx) RecordVision(ctx context.Context, hash, visibleText string, suggestedEra int, at time.Time) error {
	_, err := c.tx.ExecContext(ctx, c.s.ph(`UPDATE clips SET
		enrichment_revision = CASE
			WHEN visible_text <> ? OR vision_tagged = ? THEN enrichment_revision + 1
			ELSE enrichment_revision END,
		visible_text = ?, vision_tagged = ?,
		suggested_era = CASE
			WHEN era > 0 THEN 0
			WHEN ? > 0 AND suggested_era = 0 THEN ?
			ELSE suggested_era END,
		updated_at = ? WHERE hash = ?`),
		visibleText, false, visibleText, true,
		suggestedEra, suggestedEra, epoch(at), hash)
	return err
}

// The lifecycle transitions an admission decision applies. Each reports whether it changed the
// clip: false means the clip was not in the state the transition starts from.
//
// ⚠ Held and composite flags are bound, never literals (BOOLEAN on Postgres, INTEGER on SQLite).

// Admit files a held, present, classified clip into the playable catalog.
func (c ClipTx) Admit(ctx context.Context, hash string, at time.Time) (bool, error) {
	return c.affectedOne(ctx, `UPDATE clips SET held = ?, auto_filed = ?, updated_at = ?
		WHERE hash = ? AND held = ? AND removed_at = 0 AND kind <> ?`,
		false, false, epoch(at), hash, true, string(filler.Unclassified))
}

// Unfile takes a filed clip out of the catalog and holds it for review again.
func (c ClipTx) Unfile(ctx context.Context, hash string, at time.Time) (bool, error) {
	return c.affectedOne(ctx, `UPDATE clips SET held = ?, auto_filed = ?, updated_at = ? WHERE hash = ? AND held = ?`,
		true, false, epoch(at), hash, false)
}

// Reject holds the clip and removes it from the catalog.
func (c ClipTx) Reject(ctx context.Context, hash string, at time.Time) (bool, error) {
	return c.affectedOne(ctx, `UPDATE clips SET held = ?, auto_filed = ?, removed_at = ?, updated_at = ? WHERE hash = ?`,
		true, false, epoch(at), epoch(at), hash)
}

// RestoreHeld brings a held or removed clip back as present and held, for review.
func (c ClipTx) RestoreHeld(ctx context.Context, hash string, at time.Time) (bool, error) {
	return c.affectedOne(ctx, `UPDATE clips SET held = ?, auto_filed = ?, removed_at = 0, updated_at = ?
		WHERE hash = ? AND (held = ? OR removed_at <> 0)`,
		true, false, epoch(at), hash, true)
}

// SettlePipeline settles the clip's pipeline row at `to`, done, if its disposition is one of from.
// The pipeline is core's until it moves (#1747), so a decision settles it here.
func (c ClipTx) SettlePipeline(ctx context.Context, hash string, from []filler.Disposition, to filler.Disposition, at time.Time) (bool, error) {
	marks := make([]string, len(from))
	args := make([]any, 0, len(from)+3)
	args = append(args, string(to), epoch(at))
	for i, disposition := range from {
		marks[i] = "?"
		args = append(args, string(disposition))
	}
	args = append(args, hash)
	return c.affectedOne(ctx, `UPDATE filler_clip_pipeline
		SET disposition = ?, status = 'done', next_run = 0, updated_at = ?
		WHERE disposition IN (`+strings.Join(marks, ",")+`) AND clip_hash = ?`, args...)
}

// AdvancePipeline moves the clip's pipeline row from one disposition to the next, leaving its
// status and schedule alone. Split confirmation completes the reel's row and starts its children's.
func (c ClipTx) AdvancePipeline(ctx context.Context, hash string, from, to filler.Disposition, at time.Time) (bool, error) {
	return c.affectedOne(ctx, `UPDATE filler_clip_pipeline SET disposition = ?, updated_at = ?
		WHERE clip_hash = ? AND disposition = ?`,
		string(to), epoch(at), hash, string(from))
}

// ReleaseSplitParent makes a held reel a released composite once its cuts are confirmed. It
// reports false for a clip that is not held.
func (c ClipTx) ReleaseSplitParent(ctx context.Context, hash string, at time.Time) (bool, error) {
	return c.affectedOne(ctx, `UPDATE clips SET is_composite = ?, held = ?, auto_filed = ?, updated_at = ? WHERE hash = ? AND held = ?`,
		true, false, false, epoch(at), hash, true)
}

// ReplaceSplitChildren is ReplaceSplitChildren inside the caller's transaction.
func (c ClipTx) ReplaceSplitChildren(ctx context.Context, parentHash string, keepHashes []string, at time.Time) (int, error) {
	return c.s.replaceSplitChildrenTx(ctx, c.tx, parentHash, keepHashes, at)
}

func (c ClipTx) affectedOne(ctx context.Context, query string, args ...any) (bool, error) {
	result, err := c.tx.ExecContext(ctx, c.s.ph(query), args...)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("inspect affected rows: %w", err)
	}
	return affected == 1, nil
}
