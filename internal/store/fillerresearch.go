package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/loomarr/loomarr/internal/fillerresearch"
)

var ErrFillerResearchStale = errors.New("filler context inputs changed during research")

func (s *sqlStore) ListFillerResearchCandidates(ctx context.Context, producer, producerVersion,
	adapter, adapterVersion string, limit int) ([]fillerresearch.Candidate, error) {
	if limit <= 0 {
		return []fillerresearch.Candidate{}, nil
	}
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT hash, enrichment_revision FROM clips
		WHERE removed_at = 0 AND is_composite = false
		  AND (source LIKE 'archive:%' OR source LIKE 'youtube:%')
		  AND (era = 0 OR country = '')
		  AND NOT EXISTS (
			SELECT 1 FROM filler_context_reports r
			WHERE r.clip_hash = clips.hash AND r.producer = ? AND r.producer_version = ?
			  AND r.adapter = ? AND r.adapter_version = ?
			  AND r.input_revision = clips.enrichment_revision
		  )
		ORDER BY created_at, hash LIMIT ?`), producer, producerVersion, adapter, adapterVersion, limit)
	if err != nil {
		return nil, fmt.Errorf("list filler context candidates: %w", err)
	}
	defer func() { _ = rows.Close() }()
	type identity struct {
		hash     string
		revision int64
	}
	var identities []identity
	for rows.Next() {
		var item identity
		if err := rows.Scan(&item.hash, &item.revision); err != nil {
			return nil, fmt.Errorf("list filler context candidates: %w", err)
		}
		identities = append(identities, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list filler context candidates: %w", err)
	}
	out := make([]fillerresearch.Candidate, 0, len(identities))
	for _, identity := range identities {
		clip, err := s.GetClip(ctx, identity.hash)
		if err != nil {
			return nil, fmt.Errorf("load filler context candidate %s: %w", identity.hash, err)
		}
		out = append(out, fillerresearch.Candidate{ClipHash: clip.Hash, Path: clip.Path, Name: clip.Name,
			SourceID: clip.Source, InputRevision: identity.revision, KnownEra: clip.Era, KnownCountry: clip.Country})
	}
	return out, nil
}

func (s *sqlStore) SaveFillerResearchReport(ctx context.Context, report fillerresearch.Report) error {
	if err := report.Validate(); err != nil {
		return err
	}
	raw, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("encode filler context report: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save filler context report: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	query := `SELECT enrichment_revision FROM clips WHERE hash = ?`
	if s.dialect == DialectPostgres {
		query += ` FOR UPDATE`
	}
	var revision int64
	if err := tx.QueryRowContext(ctx, s.ph(query), report.ClipHash).Scan(&revision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("save filler context report: read input revision: %w", err)
	}
	if revision != report.InputRevision {
		return ErrFillerResearchStale
	}
	if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO filler_context_reports
		(clip_hash, producer, producer_version, adapter, adapter_version, input_revision, report_json, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(clip_hash, producer, producer_version, adapter, adapter_version, input_revision)
		DO UPDATE SET report_json=excluded.report_json, completed_at=excluded.completed_at`),
		report.ClipHash, report.Producer, report.ProducerVersion, report.Packet.Adapter,
		report.Packet.AdapterVersion, report.InputRevision, string(raw), epoch(report.CompletedAt)); err != nil {
		return fmt.Errorf("save filler context report: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save filler context report: commit: %w", err)
	}
	return nil
}

func (s *sqlStore) LatestFillerResearchReport(ctx context.Context, clipHash string) (fillerresearch.Report, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, s.ph(`SELECT report_json FROM filler_context_reports
		WHERE clip_hash = ? ORDER BY completed_at DESC, producer_version DESC LIMIT 1`), clipHash).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return fillerresearch.Report{}, ErrNotFound
	}
	if err != nil {
		return fillerresearch.Report{}, fmt.Errorf("load filler context report: %w", err)
	}
	var report fillerresearch.Report
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		return fillerresearch.Report{}, fmt.Errorf("decode filler context report: %w", err)
	}
	if err := report.Validate(); err != nil {
		return fillerresearch.Report{}, fmt.Errorf("decode filler context report: %w", err)
	}
	return report, nil
}
