package clipstore

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

// ReplaceClipIdentity re-keys a clip after Loomarr transforms its bytes (§10).
//
// The hash is content identity, not an intake-time label. Updating only clips.path would leave
// taxonomy, pipeline state, split lineage, artwork ownership and channel overrides attached to
// the bytes that no longer exist. Every database reference therefore moves in one transaction;
// the caller publishes the verified content-addressed file before entering this method and keeps
// the original file until it commits.
func (s *Store) ReplaceClipIdentity(ctx context.Context, oldHash string, c Clip) error {
	if oldHash == "" || c.Hash == "" || c.Path == "" {
		return fmt.Errorf("replace clip identity: old hash, new hash and path are required")
	}
	// A transform can be byte-for-byte stable. In that case there is no identity graph to
	// re-key: running the reference statements below with the same value needlessly deletes the
	// still-valid fingerprint and can collide with unique sibling keys on existing databases.
	// Update only the transformed media facts. Keep the Tunarr registration when even the path is
	// unchanged; otherwise the next catalog sync must register the new location.
	if oldHash == c.Hash {
		res, err := s.h.ExecContext(ctx, s.h.Rebind(`UPDATE clips
			SET path = ?,
			    tunarr_program_id = CASE WHEN path = ? THEN tunarr_program_id ELSE NULL END,
			    duration_ms = ?, quality = ?, updated_at = ?
			WHERE hash = ?`), c.Path, c.Path, c.DurationMs, c.Quality, epoch(c.UpdatedAt), oldHash)
		if err != nil {
			return fmt.Errorf("replace clip identity %s: %w", oldHash, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("replace clip identity %s: %w", oldHash, err)
		}
		if n == 0 {
			return ErrNotFound
		}
		return nil
	}
	tx, err := s.h.Begin(ctx)
	if err != nil {
		return fmt.Errorf("replace clip identity %s: %w", oldHash, err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.replaceClipIdentityTx(ctx, tx, oldHash, c); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("replace clip identity %s: %w", oldHash, err)
	}
	return nil
}

// ReplaceAiringClip is ReplaceClipIdentity for a clip that stays on air through its re-encode
// (#2121). The re-key drops the defect record measured on the old bytes; defects, measured on the
// new bytes before the switch, takes its place in the same transaction, so the clip never airs
// with no verdict, nor with one for bytes it no longer has.
func (s *Store) ReplaceAiringClip(ctx context.Context, oldHash string, c Clip, defects ClipDefectRecord) error {
	if oldHash == "" || c.Hash == "" || c.Path == "" || oldHash == c.Hash {
		return fmt.Errorf("replace airing clip: old hash, a different new hash and path are required")
	}
	defects.ClipHash = c.Hash
	tx, err := s.h.Begin(ctx)
	if err != nil {
		return fmt.Errorf("replace airing clip %s: %w", oldHash, err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.replaceClipIdentityTx(ctx, tx, oldHash, c); err != nil {
		return err
	}
	if err := s.upsertClipDefects(ctx, tx, defects); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("replace airing clip %s: %w", oldHash, err)
	}
	return nil
}

func (s *Store) replaceClipIdentityTx(ctx context.Context, tx storedb.Tx, oldHash string, c Clip) error {
	restoreResearch, err := s.liftResearchAttemptsTx(ctx, tx, oldHash)
	if err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, s.h.Rebind(`UPDATE clips
		SET hash = ?, path = ?, tunarr_program_id = NULL, duration_ms = ?, quality = ?, updated_at = ?
		WHERE hash = ?`), c.Hash, c.Path, c.DurationMs, c.Quality, epoch(c.UpdatedAt), oldHash)
	if err != nil {
		return fmt.Errorf("replace clip identity %s: %w", oldHash, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("replace clip identity %s: %w", oldHash, err)
	}
	if n == 0 {
		return ErrNotFound
	}

	if err := s.rekeyClipReferencesTx(ctx, tx, oldHash, c.Hash); err != nil {
		return err
	}
	return restoreResearch(c.Hash)
}

// liftResearchAttemptsTx takes a clip's web-research attempts out of the table so its row can be
// re-keyed in place: their foreign key to clips(hash) has no ON UPDATE, so the UPDATE fails while
// they point at it. The returned func puts them back under the new hash, in the same transaction.
func (s *Store) liftResearchAttemptsTx(ctx context.Context, tx storedb.Tx, hash string) (func(newHash string) error, error) {
	type attempt struct {
		inputRevision        int64
		adapterVersion, kind string
		reservedAt           int64
	}
	rows, err := tx.QueryContext(ctx, s.h.Rebind(`SELECT input_revision, adapter_version, provider, reserved_at
		FROM filler_research_web_attempts WHERE clip_hash = ?`), hash)
	if err != nil {
		return nil, fmt.Errorf("replace clip identity %s research attempts: %w", hash, err)
	}
	var lifted []attempt
	for rows.Next() {
		var a attempt
		if err := rows.Scan(&a.inputRevision, &a.adapterVersion, &a.kind, &a.reservedAt); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("replace clip identity %s research attempts: %w", hash, err)
		}
		lifted = append(lifted, a)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("replace clip identity %s research attempts: %w", hash, err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("replace clip identity %s research attempts: %w", hash, err)
	}
	if _, err := tx.ExecContext(ctx, s.h.Rebind(`DELETE FROM filler_research_web_attempts WHERE clip_hash = ?`), hash); err != nil {
		return nil, fmt.Errorf("replace clip identity %s research attempts: %w", hash, err)
	}
	return func(newHash string) error {
		for _, a := range lifted {
			if _, err := tx.ExecContext(ctx, s.h.Rebind(`INSERT INTO filler_research_web_attempts
				(clip_hash, input_revision, adapter_version, provider, reserved_at) VALUES (?, ?, ?, ?, ?)`),
				newHash, a.inputRevision, a.adapterVersion, a.kind, a.reservedAt); err != nil {
				return fmt.Errorf("replace clip identity %s research attempts: %w", hash, err)
			}
		}
		return nil
	}, nil
}

// clipRekey is what re-keying a clip does to one column that names it by hash.
type clipRekey int

const (
	// rekeyMove points the row at the new identity. It records the clip's content or history,
	// which a byte transform does not change.
	rekeyMove clipRekey = iota
	// rekeyDrop deletes the row. It measured the old bytes; the next pass measures the new ones.
	rekeyDrop
	// rekeyByCode is moved by its own step in rekeyClipReferencesTx, because a plain UPDATE would
	// be wrong for it.
	rekeyByCode
	// rekeyKeep stays on the old hash on purpose.
	rekeyKeep
)

type clipReference struct {
	table, column string
	rekey         clipRekey
}

// clipReferences is every column that names a clip by its hash, outside clips.hash itself, and
// what a re-key does with it. A schema test fails when a migration adds a clip_hash column this
// list does not name, so a new table cannot silently stay behind on a retired identity.
var clipReferences = []clipReference{
	{"clip_tags", "clip_hash", rekeyMove},
	{"clips", "parent_hash", rekeyMove},
	{"filler_clip_pipeline", "clip_hash", rekeyMove},
	{"filler_split_proposals", "clip_hash", rekeyMove},
	// The manifest keeps the downloaded source bytes in media_sha256/media_path, but clip_hash is
	// its binding to the current catalog identity. If that binding stays on the retired intake
	// hash, the detail surface loses the exact public source URL after normalization.
	{"filler_acquisition_artifacts", "clip_hash", rekeyMove},
	// Screening verdicts, enrichment, research and their cost ledgers judge the content. A Ready
	// clip re-encoded from its master keeps them; its new evidence is screened again on its own.
	{"filler_split_shadow_decisions", "clip_hash", rekeyMove},
	{"filler_enrichment_axes", "clip_hash", rekeyMove},
	{"filler_enrichment_passes", "clip_hash", rekeyMove},
	{"filler_context_reports", "clip_hash", rekeyMove},
	{"filler_inference_evaluations", "clip_hash", rekeyMove},
	{"filler_ready_events", "clip_hash", rekeyMove},
	// Its foreign key to clips(hash) has no ON UPDATE, so replaceClipIdentityTx lifts these rows
	// out before it re-keys the clip row in place.
	{"filler_research_web_attempts", "clip_hash", rekeyMove},
	// Fingerprints describe the OLD bytes. Never re-key derived evidence onto the new content
	// identity; the first de-dup pass that sees the replacement computes it again.
	{"filler_clip_fingerprints", "clip_hash", rekeyDrop},
	// So does the defect record (#2034): the new bytes are measured on the next pass.
	{"filler_clip_defects", "clip_hash", rekeyDrop},
	// Copied onto the new hash by its own step: a re-key in place moves them by ON UPDATE CASCADE
	// first, but adopting a conditioned clip deletes the source row, which would cascade them away.
	{"clip_images", "clip_hash", rekeyByCode},
	{"filler_airings", "clip_hash", rekeyByCode},
	{"filler_clip_variants", "a_hash", rekeyByCode},
	{"filler_clip_variants", "b_hash", rekeyByCode},
	{"filler_inferior_copies", "survivor_hash", rekeyByCode},
	// The identities of a copy that lost a merge. Intake skips them so those bytes never return.
	{"filler_inferior_copies", "clip_hash", rekeyKeep},
	{"filler_inferior_copies", "source_hash", rekeyKeep},
}

func (s *Store) rekeyClipReferencesTx(ctx context.Context, tx storedb.Tx, oldHash, newHash string) error {
	for _, ref := range clipReferences {
		var query string
		var args []any
		switch ref.rekey {
		case rekeyMove:
			query, args = `UPDATE `+ref.table+` SET `+ref.column+` = ? WHERE `+ref.column+` = ?`, []any{newHash, oldHash}
		case rekeyDrop:
			query, args = `DELETE FROM `+ref.table+` WHERE `+ref.column+` = ?`, []any{oldHash}
		default:
			continue
		}
		if _, err := tx.ExecContext(ctx, s.h.Rebind(query), args...); err != nil {
			return fmt.Errorf("replace clip identity %s references in %s: %w", oldHash, ref.table, err)
		}
	}
	// A re-key in place has already moved these (ON UPDATE CASCADE), so this finds none; adopting a
	// conditioned clip has not, and its source row is deleted next, taking anything left behind.
	if _, err := tx.ExecContext(ctx, s.h.Rebind(`INSERT INTO clip_images (clip_hash, role, image_hash)
		SELECT ?, role, image_hash FROM clip_images WHERE clip_hash = ?
		ON CONFLICT (clip_hash, role, image_hash) DO NOTHING`), newHash, oldHash); err != nil {
		return fmt.Errorf("replace clip identity %s references in clip_images: %w", oldHash, err)
	}
	for _, step := range identityHistoryMoves(oldHash, newHash) {
		if _, err := tx.ExecContext(ctx, s.h.Rebind(step.query), step.args...); err != nil {
			return fmt.Errorf("replace clip identity %s history: %w", oldHash, err)
		}
	}
	if err := s.rekeyChannelClipRefs(ctx, tx, oldHash, newHash); err != nil {
		return fmt.Errorf("replace clip identity %s channel references: %w", oldHash, err)
	}
	return nil
}

// AdoptConditionedClip closes the catalog half of the owner-bound conditioning saga: the clip at
// sourceHash becomes target. The caller has already proved the exact source/target byte pair and
// its ownership (internal/filler/records's CommitConditioningPublication). This transaction permits
// only the three catalog shapes that proof can own: an ordinary source-only re-key, a source plus
// the held row Sync reconstructed from that sidecar, or the exact target-only state left by a
// re-key that committed before process loss.
func (s *Store) AdoptConditionedClip(ctx context.Context, sourceHash string, target Clip) error {
	if sourceHash == "" || target.Hash == "" || sourceHash == target.Hash || target.Path == "" {
		return ErrConditionedClipMismatch
	}

	tx, err := s.h.Begin(ctx)
	if err != nil {
		return fmt.Errorf("commit conditioning publication %s: %w", target.Hash, err)
	}
	defer func() { _ = tx.Rollback() }()

	type catalogState struct {
		found       bool
		path        string
		parentHash  string
		held        bool
		removedAt   int64
		isComposite bool
	}
	states := map[string]*catalogState{
		sourceHash:  {},
		target.Hash: {},
	}
	query := `SELECT hash, path, parent_hash, held, removed_at, is_composite
		FROM clips WHERE hash IN (?, ?)`
	query = storedb.LockRows(s.h.Dialect(), query)
	rows, err := tx.QueryContext(ctx, s.h.Rebind(query), sourceHash, target.Hash)
	if err != nil {
		return fmt.Errorf("commit conditioning publication %s classify: %w", target.Hash, err)
	}
	for rows.Next() {
		var hash string
		var state catalogState
		if err := rows.Scan(&hash, &state.path, &state.parentHash, &state.held, &state.removedAt, &state.isComposite); err != nil {
			_ = rows.Close()
			return fmt.Errorf("commit conditioning publication %s classify: %w", target.Hash, err)
		}
		state.found = true
		states[hash] = &state
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("commit conditioning publication %s classify: %w", target.Hash, err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("commit conditioning publication %s classify: %w", target.Hash, err)
	}
	sourceState := states[sourceHash]
	targetState := states[target.Hash]
	targetRemovedAt := epoch(target.RemovedAt)

	// The durable re-key already won. Exact filesystem evidence plus source absence and an exact
	// target row make clearing the still-owned marker idempotent; no catalog write is needed.
	if !sourceState.found {
		if targetState.found && targetState.path == target.Path && targetState.parentHash == target.ParentHash &&
			targetState.held == target.Held && targetState.removedAt == targetRemovedAt &&
			targetState.isComposite == target.IsComposite {
			return nil
		}
		return ErrConditionedClipMismatch
	}

	// The source row must still describe the owner whose metadata the staged target carries.
	if sourceState.parentHash != target.ParentHash || sourceState.held != target.Held ||
		sourceState.removedAt != targetRemovedAt || sourceState.isComposite != target.IsComposite {
		return ErrConditionedClipMismatch
	}

	if !targetState.found {
		if err := s.replaceClipIdentityTx(ctx, tx, sourceHash, target); err != nil {
			return err
		}
	} else {
		// Sync may reconstruct only this exact quarantine row. Keep that row in place and adopt it;
		// there is never a delete-then-reinsert window for the content-addressed target identity.
		if targetState.path != target.Path || targetState.parentHash != target.ParentHash ||
			!targetState.held || targetState.removedAt != 0 || targetState.isComposite {
			return ErrConditionedClipMismatch
		}
		res, err := tx.ExecContext(ctx, s.h.Rebind(`UPDATE clips SET
			(name, name_source, role, era, audience, source, license,
			 thumbnail, preview, thumb_image_hash, hover_image_hash, language, transcript,
			 brand, visible_text, is_composite, parent_hash, play_count,
			 last_played_at, removed_at, held,
			 created_at, reaped_at) =
			(SELECT name, name_source, role, era, audience, source, license,
			 thumbnail, preview, thumb_image_hash, hover_image_hash, language, transcript,
			 brand, visible_text, is_composite, parent_hash, play_count,
			 last_played_at, removed_at, held,
			 created_at, reaped_at FROM clips WHERE hash = ?),
			path = ?, tunarr_program_id = NULL, duration_ms = ?, quality = ?, updated_at = ?
			WHERE hash = ? AND path = ? AND parent_hash = ? AND held = ? AND removed_at = ? AND is_composite = ?`),
			sourceHash, target.Path, target.DurationMs, target.Quality, epoch(target.UpdatedAt),
			target.Hash, target.Path, target.ParentHash, true, int64(0), false)
		if err != nil {
			return fmt.Errorf("commit conditioning publication %s adopt target: %w", target.Hash, err)
		}
		if n, countErr := res.RowsAffected(); countErr != nil || n != 1 {
			if countErr != nil {
				return fmt.Errorf("commit conditioning publication %s count target: %w", target.Hash, countErr)
			}
			return ErrConditionedClipMismatch
		}
		if err := s.rekeyClipReferencesTx(ctx, tx, sourceHash, target.Hash); err != nil {
			return err
		}
		res, err = tx.ExecContext(ctx, s.h.Rebind(`DELETE FROM clips WHERE hash = ?`), sourceHash)
		if err != nil {
			return fmt.Errorf("commit conditioning publication %s retire source: %w", target.Hash, err)
		}
		if n, countErr := res.RowsAffected(); countErr != nil || n != 1 {
			if countErr != nil {
				return fmt.Errorf("commit conditioning publication %s count source: %w", target.Hash, countErr)
			}
			return ErrConditionedClipMismatch
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit conditioning publication %s: %w", target.Hash, err)
	}
	return nil
}

func (s *Store) rekeyChannelClipRefs(ctx context.Context, tx storedb.Tx, oldHash, newHash string) error {
	// The locking read: it holds the policy snapshots before they are decoded. Without that, a
	// concurrent channel CAS could commit between SELECT and UPDATE and this maintenance
	// transaction would overwrite its policy with the pre-edit JSON.
	rows, err := s.channels.LockChannelPolicies(ctx, tx)
	if err != nil {
		return err
	}
	type changedPolicy struct {
		id   string
		blob []byte
	}
	var changed []changedPolicy
	for rows.Next() {
		var id, raw string
		if err := rows.Scan(&id, &raw); err != nil {
			_ = rows.Close()
			return err
		}
		var policy schedule.ChannelPolicy
		if err := json.Unmarshal([]byte(raw), &policy); err != nil {
			_ = rows.Close()
			return fmt.Errorf("channel %s policy: %w", id, err)
		}
		if policy.Filler == nil {
			continue
		}
		touched := replaceClipRef(policy.Filler.Pinned, oldHash, newHash)
		touched = replaceClipRef(policy.Filler.Excluded, oldHash, newHash) || touched
		if !touched {
			continue
		}
		blob, err := json.Marshal(policy)
		if err != nil {
			_ = rows.Close()
			return fmt.Errorf("channel %s policy: %w", id, err)
		}
		changed = append(changed, changedPolicy{id: id, blob: blob})
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range changed {
		if err := s.channels.SetChannelPolicy(ctx, tx, item.id, string(item.blob)); err != nil {
			return err
		}
	}
	return nil
}

func replaceClipRef(refs []string, oldHash, newHash string) bool {
	changed := false
	for i := range refs {
		if refs[i] == oldHash {
			refs[i] = newHash
			changed = true
		}
	}
	return changed
}
