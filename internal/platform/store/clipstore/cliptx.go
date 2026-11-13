package clipstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

// ClipTx writes a clip's own columns and tags inside a transaction another part of the store began
// (Handle.Begin). The filler store's projections go through it rather than updating clips
// directly, so the clips table and its tag invariants keep one owner however a write is composed
// (#1747). Each method is one statement or one tag rewrite; none commits.
type ClipTx struct {
	s  *Store
	tx storedb.Querier
}

// ClipsOn is ClipTx over tx, which a caller began (storedb.Handle.Begin). The store's
// Handle.Clips is the extension stores' way in; it forwards here. A package function rather than a
// method so the aggregate does not promote it onto sqlStore, where the Extend wrapper would hide it.
func ClipsOn(s *Store, tx storedb.Querier) ClipTx { return ClipTx{s: s, tx: tx} }

// ClipGeography is the broadcast provenance a clip's geography columns hold.
type ClipGeography struct {
	Scope, Country, Network, Station, AirDate string
}

// Require reports ErrNotFound for a clip that does not exist.
func (c ClipTx) Require(ctx context.Context, hash string) error {
	var n int
	if err := c.tx.QueryRowContext(ctx, c.s.h.Rebind(`SELECT COUNT(*) FROM clips WHERE hash = ?`), hash).Scan(&n); err != nil {
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
	query := storedb.LockRows(c.s.h.Dialect(), `SELECT enrichment_revision FROM clips WHERE hash = ?`)
	var revision int64
	err := c.tx.QueryRowContext(ctx, c.s.h.Rebind(query), hash).Scan(&revision)
	return revision, err
}

// ClassifyRole sets the role of a clip that is still unclassified; a classified clip keeps its role.
// The opt-in roles are the exception (#2037). Sources file campaign ads and weather reports as
// commercials, so commercial, interstitial, political and news replace one another: an accepted
// answer can relabel a campaign ad, and a corrected rule can take a wrong label back. Enrichment's
// precedence has already decided the answer wins, so an operator's choice is never overwritten. A
// specific role, such as a PSA, is kept.
func (c ClipTx) ClassifyRole(ctx context.Context, hash string, role clips.Role, at time.Time) error {
	replaceable := []any{string(clips.Unclassified)}
	if optInGroup[role] {
		for r := range optInGroup {
			replaceable = append(replaceable, string(r))
		}
	}
	args := append([]any{string(role), epoch(at), hash}, replaceable...)
	_, err := c.tx.ExecContext(ctx, c.s.h.Rebind(`UPDATE clips SET role = ?, updated_at = ? WHERE hash = ? AND role IN (`+
		strings.TrimSuffix(strings.Repeat("?, ", len(replaceable)), ", ")+`)`), args...)
	return err
}

// optInGroup is the generic break-body roles and the opt-in roles they are filed as (#2037).
var optInGroup = map[clips.Role]bool{
	clips.Commercial: true, clips.Interstitial: true, clips.Political: true, clips.News: true,
}

// SetEra sets the clip's era year.
func (c ClipTx) SetEra(ctx context.Context, hash string, year int, at time.Time) error {
	_, err := c.tx.ExecContext(ctx, c.s.h.Rebind(`UPDATE clips SET era = ?, updated_at = ? WHERE hash = ?`), year, epoch(at), hash)
	return err
}

// FillAudience sets the audience of a clip that has none.
func (c ClipTx) FillAudience(ctx context.Context, hash, audience string, at time.Time) error {
	_, err := c.tx.ExecContext(ctx, c.s.h.Rebind(`UPDATE clips SET audience = ?, updated_at = ? WHERE hash = ? AND audience = ''`), audience, epoch(at), hash)
	return err
}

// SetBrand sets the clip's brand.
func (c ClipTx) SetBrand(ctx context.Context, hash, brand string, at time.Time) error {
	_, err := c.tx.ExecContext(ctx, c.s.h.Rebind(`UPDATE clips SET brand = ?, updated_at = ? WHERE hash = ?`), brand, epoch(at), hash)
	return err
}

// FillLanguage sets the language of a clip that has none.
func (c ClipTx) FillLanguage(ctx context.Context, hash, language string, at time.Time) error {
	_, err := c.tx.ExecContext(ctx, c.s.h.Rebind(`UPDATE clips SET language = ?, updated_at = ? WHERE hash = ? AND language = ''`), language, epoch(at), hash)
	return err
}

// SetGeography projects an accepted geography answer onto the clip, with the evidence reference that
// justified it. It overwrites: the enrichment axis already decided this answer beats the one it
// replaces. A fill-only projection left a stronger answer unable to correct an older one (#2061:
// Cleveland news stayed "national"). An operator's row (geo_evidence `operator`) is the one
// exception: the clip edit writes it directly, and rows edited before the axis existed have no axis
// state to protect them.
func (c ClipTx) SetGeography(ctx context.Context, hash string, g ClipGeography, evidence string, at time.Time) error {
	_, err := c.tx.ExecContext(ctx, c.s.h.Rebind(`UPDATE clips SET geographic_scope = ?, country = ?,
		network = ?, station = ?, air_date = ?, geo_evidence = ?, updated_at = ?
		WHERE hash = ? AND geo_evidence <> 'operator'`),
		g.Scope, g.Country, g.Network, g.Station, g.AirDate, evidence, epoch(at), hash)
	return err
}

// ClearSourceDefaultGeography returns a clip to unknown geography when its location came only from
// a registered-source default (geo_evidence `registered_source:<id>`). Locations from item metadata,
// content, research, or an operator carry other evidence and are left alone.
func (c ClipTx) ClearSourceDefaultGeography(ctx context.Context, hash string, at time.Time) error {
	_, err := c.tx.ExecContext(ctx, c.s.h.Rebind(`UPDATE clips SET geographic_scope = 'unknown', country = '',
		network = '', station = '', air_date = '', geo_evidence = '', updated_at = ?
		WHERE hash = ? AND geo_evidence LIKE 'registered_source:%'`), epoch(at), hash)
	return err
}

// AddLeafTags adds taxonomy leaves to the clip's asserted leaves and rebuilds its rollups, under
// the same taxonomy lock as graph edits. Leaves it already has are kept once.
func (c ClipTx) AddLeafTags(ctx context.Context, hash string, tags []string) error {
	if err := storedb.XactLock(ctx, c.s.h.Dialect(), c.tx, taxonomySharedLockSQL); err != nil {
		return fmt.Errorf("add clip tags: taxonomy lock: %w", err)
	}
	leaves, err := getClipTagsFrom(ctx, c.tx, c.s.h.Rebind, hash, true)
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
	_, err := c.tx.ExecContext(ctx, c.s.h.Rebind(`UPDATE clips SET
		enrichment_revision = CASE WHEN transcript <> ? THEN enrichment_revision + 1 ELSE enrichment_revision END,
		transcript = ?, updated_at = ? WHERE hash = ?`),
		transcript, transcript, epoch(at), hash)
	return err
}

// RecordVision stores what the vision rung read on screen, advancing the clip's enrichment
// revision when the text changed. An era the frames suggest is a Context observation, recorded by
// the caller, never a clip column.
func (c ClipTx) RecordVision(ctx context.Context, hash, visibleText string, at time.Time) error {
	_, err := c.tx.ExecContext(ctx, c.s.h.Rebind(`UPDATE clips SET
		enrichment_revision = CASE
			WHEN visible_text <> ? THEN enrichment_revision + 1
			ELSE enrichment_revision END,
		visible_text = ?,
		updated_at = ? WHERE hash = ?`),
		visibleText, visibleText, epoch(at), hash)
	return err
}

// The lifecycle transitions an admission decision applies. Each reports whether it changed the
// clip: false means the clip was not in the state the transition starts from.
//
// ⚠ Held and composite flags are bound, never literals (BOOLEAN on Postgres, INTEGER on SQLite).

// Admit files a held, present, classified clip into the playable catalog.
func (c ClipTx) Admit(ctx context.Context, hash string, at time.Time) (bool, error) {
	return c.affectedOne(ctx, `UPDATE clips SET held = ?, updated_at = ?
		WHERE hash = ? AND held = ? AND removed_at = 0 AND role <> ?`,
		false, epoch(at), hash, true, string(clips.Unclassified))
}

// Unfile takes a filed clip out of the catalog and holds it for review again.
func (c ClipTx) Unfile(ctx context.Context, hash string, at time.Time) (bool, error) {
	return c.affectedOne(ctx, `UPDATE clips SET held = ?, updated_at = ? WHERE hash = ? AND held = ?`,
		true, epoch(at), hash, false)
}

// Reject holds the clip and removes it from the catalog.
func (c ClipTx) Reject(ctx context.Context, hash string, at time.Time) (bool, error) {
	return c.affectedOne(ctx, `UPDATE clips SET held = ?, removed_at = ?, updated_at = ? WHERE hash = ?`,
		true, epoch(at), epoch(at), hash)
}

// RestoreHeld brings a held or removed clip back as present and held, for review. A reaped clip
// has no file and is never restored.
func (c ClipTx) RestoreHeld(ctx context.Context, hash string, at time.Time) (bool, error) {
	return c.affectedOne(ctx, `UPDATE clips SET held = ?, removed_at = 0, updated_at = ?
		WHERE hash = ? AND (held = ? OR removed_at <> 0) AND reaped_at IS NULL`,
		true, epoch(at), hash, true)
}

// RestoreForRetry brings the clip back as present and held whatever state it is in, for a pipeline
// retry of a rejected clip. Unlike RestoreHeld it does not require the clip to be held or removed.
// A reaped clip has no file and is never restored.
func (c ClipTx) RestoreForRetry(ctx context.Context, hash string, at time.Time) (bool, error) {
	return c.affectedOne(ctx, `UPDATE clips SET removed_at = 0, held = ?, updated_at = ?
		WHERE hash = ? AND reaped_at IS NULL`,
		true, epoch(at), hash)
}

// Pinned reports whether some channel pins the clip. On Postgres it locks the channel rows until
// the transaction ends, so no pin can arrive between this answer and the caller's write.
func (c ClipTx) Pinned(ctx context.Context, hash string) (bool, error) {
	pinned, err := c.s.pinnedClipsTx(ctx, c.tx)
	if err != nil {
		return false, err
	}
	_, ok := pinned[hash]
	return ok, nil
}

// Reap takes a held clip off the catalog for good before its files are deleted: removed, so
// nothing airs it, and reaped, so a sync that no longer finds the file keeps the row, and no
// restore or retry brings it back. A composite, a published clip or an already reaped one is left
// alone and reports false.
func (c ClipTx) Reap(ctx context.Context, hash string, at time.Time) (bool, error) {
	return c.affectedOne(ctx, `UPDATE clips SET
		removed_at = CASE WHEN removed_at = 0 THEN ? ELSE removed_at END, reaped_at = ?, updated_at = ?
		WHERE hash = ? AND held = ? AND is_composite = ? AND reaped_at IS NULL`,
		epoch(at), epoch(at), epoch(at), hash, true, false)
}

// ClipPublication is what terminal readiness checks about a clip before publishing it.
type ClipPublication struct {
	Held, Removed, Composite bool
	Source, Placement        string
}

// LockPublication reads the clip's publication state. On Postgres it locks the row until the
// transaction ends. A missing clip is ErrNotFound.
func (c ClipTx) LockPublication(ctx context.Context, hash string) (ClipPublication, error) {
	query := storedb.LockRows(c.s.h.Dialect(), `SELECT held, removed_at, is_composite, source, placement FROM clips WHERE hash = ?`)
	var p ClipPublication
	var removedAt int64
	err := c.tx.QueryRowContext(ctx, c.s.h.Rebind(query), hash).Scan(&p.Held, &removedAt, &p.Composite, &p.Source, &p.Placement)
	if errors.Is(err, sql.ErrNoRows) {
		return ClipPublication{}, ErrNotFound
	}
	p.Removed = removedAt != 0
	return p, err
}

// PublishReady sets the placement of a held, present, non-composite clip and releases it into the
// playable catalog.
func (c ClipTx) PublishReady(ctx context.Context, hash, placement string, at time.Time) (bool, error) {
	return c.affectedOne(ctx, `UPDATE clips SET placement = ?, held = ?, updated_at = ?
		WHERE hash = ? AND held = ? AND removed_at = 0 AND is_composite = ?`,
		placement, false, epoch(at), hash, true, false)
}

// ReleaseSplitParent makes a held reel a released composite once its cuts are confirmed. It
// reports false for a clip that is not held.
func (c ClipTx) ReleaseSplitParent(ctx context.Context, hash string, at time.Time) (bool, error) {
	return c.affectedOne(ctx, `UPDATE clips SET is_composite = ?, held = ?, updated_at = ? WHERE hash = ? AND held = ?`,
		true, false, epoch(at), hash, true)
}

// ReplaceSplitChildren is ReplaceSplitChildren inside the caller's transaction.
func (c ClipTx) ReplaceSplitChildren(ctx context.Context, parentHash string, keepHashes []string, at time.Time) (int, error) {
	return c.s.replaceSplitChildrenTx(ctx, c.tx, parentHash, keepHashes, at)
}

func (c ClipTx) affectedOne(ctx context.Context, query string, args ...any) (bool, error) {
	result, err := c.tx.ExecContext(ctx, c.s.h.Rebind(query), args...)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("inspect affected rows: %w", err)
	}
	return affected == 1, nil
}
