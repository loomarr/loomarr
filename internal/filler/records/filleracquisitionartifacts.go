package records

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/loomarr/loomarr/internal/filler/acquisition"
)

const acquisitionArtifactSelect = `SELECT id, acquisition_id, source_id, provider, source_url, remote_id,
	staging_path, media_path, sidecar_path, media_sha256, media_bytes, clip_hash, state,
	repair_reason, completed_at, updated_at, provider_archive_entry, provider_archive_committed, gap
	FROM filler_acquisition_artifacts`

// UpsertAcquisitionArtifacts records one downloader result atomically. Publication may begin only
// after this returns, so partial manifest persistence can never make only part of a source visible.
func (s *sqlStore) UpsertAcquisitionArtifacts(ctx context.Context, artifacts []acquisition.AcquisitionArtifact) error {
	for _, artifact := range artifacts {
		if err := artifact.Validate(); err != nil {
			return fmt.Errorf("validate filler acquisition artifact %q: %w", artifact.ID, err)
		}
	}
	if len(artifacts) == 0 {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin filler acquisition artifact update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	query := s.ph(`INSERT INTO filler_acquisition_artifacts
		(id, acquisition_id, source_id, provider, source_url, remote_id, staging_path, media_path,
		 sidecar_path, media_sha256, media_bytes, clip_hash, state, repair_reason, completed_at, updated_at,
		 provider_archive_entry, provider_archive_committed, gap)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		 acquisition_id=excluded.acquisition_id, source_id=excluded.source_id,
		 provider=excluded.provider, source_url=excluded.source_url, remote_id=excluded.remote_id,
		 staging_path=excluded.staging_path, media_path=excluded.media_path,
		 sidecar_path=excluded.sidecar_path, media_sha256=excluded.media_sha256,
		 media_bytes=excluded.media_bytes, clip_hash=excluded.clip_hash,
		 state=excluded.state, repair_reason=excluded.repair_reason,
		 completed_at=excluded.completed_at, updated_at=excluded.updated_at,
		 provider_archive_entry=excluded.provider_archive_entry,
		 provider_archive_committed=excluded.provider_archive_committed, gap=excluded.gap`)
	for _, artifact := range artifacts {
		if _, err := tx.ExecContext(ctx, query,
			artifact.ID, artifact.AcquisitionID, artifact.SourceID, artifact.Provider, artifact.SourceURL, artifact.RemoteID,
			artifact.StagingPath, artifact.MediaPath, artifact.SidecarPath, artifact.MediaSHA256,
			artifact.MediaBytes, artifact.ClipHash, string(artifact.State), artifact.RepairReason,
			epoch(artifact.CompletedAt), epoch(artifact.UpdatedAt), artifact.ProviderArchiveEntry,
			artifact.ProviderArchiveCommitted, artifact.Gap); err != nil {
			return fmt.Errorf("upsert filler acquisition artifact %s: %w", artifact.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit filler acquisition artifacts: %w", err)
	}
	return nil
}

func scanAcquisitionArtifact(sc scannable) (acquisition.AcquisitionArtifact, error) {
	var artifact acquisition.AcquisitionArtifact
	var state string
	var completedAt, updatedAt int64
	if err := sc.Scan(
		&artifact.ID, &artifact.AcquisitionID, &artifact.SourceID, &artifact.Provider,
		&artifact.SourceURL, &artifact.RemoteID, &artifact.StagingPath, &artifact.MediaPath, &artifact.SidecarPath,
		&artifact.MediaSHA256, &artifact.MediaBytes, &artifact.ClipHash, &state,
		&artifact.RepairReason, &completedAt, &updatedAt, &artifact.ProviderArchiveEntry,
		&artifact.ProviderArchiveCommitted, &artifact.Gap,
	); err != nil {
		return acquisition.AcquisitionArtifact{}, err
	}
	artifact.State = acquisition.AcquisitionArtifactState(state)
	artifact.CompletedAt = fromEpoch(completedAt)
	artifact.UpdatedAt = fromEpoch(updatedAt)
	return artifact, nil
}

// AcquisitionArtifactForClip resolves ownership from the intended watch path, its paired sidecar,
// or the catalog hash bound before intake moves the file. The sidecar path deliberately retains
// the original publication name until successful consumption; it is therefore the durable retry
// binding when intake has recorded the destination but the filesystem move then fails. Newest wins
// only to tolerate an operator deliberately reusing a filename across separate acquisition runs.
func (s *sqlStore) AcquisitionArtifactForClip(
	ctx context.Context,
	mediaPath, clipHash string,
) (acquisition.AcquisitionArtifact, bool, error) {
	sidecarPath := strings.TrimSuffix(mediaPath, filepath.Ext(mediaPath)) + ".info.json"
	artifact, err := scanAcquisitionArtifact(s.db.QueryRowContext(ctx, s.ph(acquisitionArtifactSelect+`
		WHERE media_path = ? OR sidecar_path = ? OR (? <> '' AND clip_hash = ?)
		ORDER BY updated_at DESC, id DESC LIMIT 1`), mediaPath, sidecarPath, clipHash, clipHash))
	if errors.Is(err, sql.ErrNoRows) {
		return acquisition.AcquisitionArtifact{}, false, nil
	}
	if err != nil {
		return acquisition.AcquisitionArtifact{}, false, fmt.Errorf("resolve filler acquisition artifact: %w", err)
	}
	return artifact, true, nil
}

// ListRecoverableAcquisitionArtifacts returns bounded non-consumed rows for startup and explicit
// repair. It includes repair rows because they retain bytes and an actionable reason.
func (s *sqlStore) ListRecoverableAcquisitionArtifacts(ctx context.Context, limit int) ([]acquisition.AcquisitionArtifact, error) {
	return s.listRecoverableAcquisitionArtifacts(ctx, acquisition.AcquisitionArtifactCursor{}, limit)
}

// ListRecoverableAcquisitionArtifactsAfter continues a stable oldest-first scan. Recovery may
// update a row's timestamp, so its cursor is captured before processing rather than inferred from
// the row subsequently persisted.
func (s *sqlStore) ListRecoverableAcquisitionArtifactsAfter(ctx context.Context, after acquisition.AcquisitionArtifactCursor, limit int) ([]acquisition.AcquisitionArtifact, error) {
	return s.listRecoverableAcquisitionArtifacts(ctx, after, limit)
}

func (s *sqlStore) listRecoverableAcquisitionArtifacts(ctx context.Context, after acquisition.AcquisitionArtifactCursor, limit int) ([]acquisition.AcquisitionArtifact, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	states := []string{string(acquisition.ArtifactStaged), string(acquisition.ArtifactPublished), string(acquisition.ArtifactRepair)}
	rows, err := s.db.QueryContext(ctx, s.ph(acquisitionArtifactSelect+`
		WHERE state IN (?, ?, ?) AND (updated_at > ? OR (updated_at = ? AND id > ?))
		ORDER BY updated_at, id LIMIT ?`), states[0], states[1], states[2], epoch(after.UpdatedAt), epoch(after.UpdatedAt), after.ID, limit)
	if err != nil {
		return nil, fmt.Errorf("list recoverable filler acquisition artifacts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	artifacts := make([]acquisition.AcquisitionArtifact, 0)
	for rows.Next() {
		artifact, err := scanAcquisitionArtifact(rows)
		if err != nil {
			return nil, fmt.Errorf("scan recoverable filler acquisition artifact: %w", err)
		}
		artifacts = append(artifacts, artifact)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list recoverable filler acquisition artifacts: %w", err)
	}
	return artifacts, nil
}

// ListAcquisitionRemoteStates is the planner's high-water mark. It reads only identity/state
// columns, not paths or digests, so a large artifact history does not become a large object graph.
func (s *sqlStore) ListAcquisitionRemoteStates(ctx context.Context) (map[string]acquisition.ExistingRemoteState, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT provider, source_id, remote_id, state
		FROM filler_acquisition_artifacts WHERE remote_id <> ''`)
	if err != nil {
		return nil, fmt.Errorf("list filler acquisition remote states: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]acquisition.ExistingRemoteState{}
	for rows.Next() {
		var provider, sourceID, remoteID, state string
		if err := rows.Scan(&provider, &sourceID, &remoteID, &state); err != nil {
			return nil, fmt.Errorf("scan filler acquisition remote state: %w", err)
		}
		identity := acquisition.RemoteIdentity{Provider: provider, SourceID: sourceID, RemoteID: remoteID}
		remoteState := acquisition.RemoteQueued
		if acquisition.AcquisitionArtifactState(state) == acquisition.ArtifactConsumed {
			remoteState = acquisition.RemoteCatalogued
		}
		if out[identity.Key()] != acquisition.RemoteCatalogued {
			out[identity.Key()] = remoteState
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list filler acquisition remote states: %w", err)
	}
	return out, nil
}
