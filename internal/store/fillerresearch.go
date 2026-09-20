package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillerresearch"
)

var ErrFillerResearchStale = errors.New("filler context inputs changed during research")

func (s *sqlStore) ReserveFillerResearchWebRequest(ctx context.Context, month string,
	provider fillerresearch.WebProvider, limit int, attempt fillerresearch.WebAttempt) (fillerresearch.WebUsage, error) {
	if len(month) != 7 || limit < 1 || (provider != fillerresearch.WebProviderBrave && provider != fillerresearch.WebProviderSearXNG) {
		return fillerresearch.WebUsage{}, fillerresearch.ErrInvalid
	}
	if (strings.TrimSpace(attempt.ClipHash) != "" || attempt.InputRevision != 0 ||
		strings.TrimSpace(attempt.AdapterVersion) != "" || !attempt.ReservedAt.IsZero()) && !attempt.Tracked() {
		return fillerresearch.WebUsage{}, fillerresearch.ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fillerresearch.WebUsage{}, fmt.Errorf("reserve filler web search: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if attempt.Tracked() {
		result, err := tx.ExecContext(ctx, s.ph(`INSERT INTO filler_research_web_attempts
			(clip_hash, input_revision, adapter_version, provider, reserved_at)
			VALUES (?, ?, ?, ?, ?) ON CONFLICT(clip_hash, input_revision, adapter_version) DO NOTHING`),
			attempt.ClipHash, attempt.InputRevision, attempt.AdapterVersion, string(provider), epoch(attempt.ReservedAt))
		if err != nil {
			return fillerresearch.WebUsage{}, fmt.Errorf("reserve filler web search attempt: %w", err)
		}
		inserted, err := result.RowsAffected()
		if err != nil {
			return fillerresearch.WebUsage{}, fmt.Errorf("reserve filler web search attempt: affected rows: %w", err)
		}
		if inserted != 1 {
			return fillerresearch.WebUsage{}, fillerresearch.ErrWebSearchAttempted
		}
	}
	if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO filler_research_web_usage
		(month, request_count, last_provider, last_success_at, last_failure_at)
		VALUES (?, 0, '', 0, 0) ON CONFLICT(month) DO NOTHING`), month); err != nil {
		return fillerresearch.WebUsage{}, fmt.Errorf("reserve filler web search: initialize: %w", err)
	}
	result, err := tx.ExecContext(ctx, s.ph(`UPDATE filler_research_web_usage
		SET request_count = request_count + 1, last_provider = ?
		WHERE month = ? AND request_count < ?`), string(provider), month, limit)
	if err != nil {
		return fillerresearch.WebUsage{}, fmt.Errorf("reserve filler web search: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fillerresearch.WebUsage{}, fmt.Errorf("reserve filler web search: affected rows: %w", err)
	}
	if updated != 1 {
		return fillerresearch.WebUsage{}, fillerresearch.ErrWebSearchLimit
	}
	usage, err := scanFillerResearchWebUsage(tx.QueryRowContext(ctx, s.ph(`SELECT month, request_count,
		last_provider, last_success_at, last_failure_at FROM filler_research_web_usage WHERE month = ?`), month))
	if err != nil {
		return fillerresearch.WebUsage{}, err
	}
	if err := tx.Commit(); err != nil {
		return fillerresearch.WebUsage{}, fmt.Errorf("reserve filler web search: commit: %w", err)
	}
	return usage, nil
}

func (s *sqlStore) CompleteFillerResearchWebRequest(ctx context.Context, month string, success bool, at time.Time) error {
	column := "last_failure_at"
	if success {
		column = "last_success_at"
	}
	result, err := s.db.ExecContext(ctx, s.ph(`UPDATE filler_research_web_usage SET `+column+` = ? WHERE month = ?`), epoch(at), month)
	if err != nil {
		return fmt.Errorf("complete filler web search: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("complete filler web search: affected rows: %w", err)
	}
	if updated != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *sqlStore) FillerResearchWebUsage(ctx context.Context, month string) (fillerresearch.WebUsage, error) {
	usage, err := scanFillerResearchWebUsage(s.db.QueryRowContext(ctx, s.ph(`SELECT month, request_count,
		last_provider, last_success_at, last_failure_at FROM filler_research_web_usage WHERE month = ?`), month))
	if errors.Is(err, sql.ErrNoRows) {
		return fillerresearch.WebUsage{Month: month}, nil
	}
	return usage, err
}

type fillerResearchRowScanner interface{ Scan(...any) error }

func scanFillerResearchWebUsage(row fillerResearchRowScanner) (fillerresearch.WebUsage, error) {
	var (
		usage                 fillerresearch.WebUsage
		provider              string
		lastSuccess, lastFail int64
	)
	if err := row.Scan(&usage.Month, &usage.RequestCount, &provider, &lastSuccess, &lastFail); err != nil {
		return fillerresearch.WebUsage{}, err
	}
	usage.LastProvider = fillerresearch.WebProvider(provider)
	usage.LastSuccessAt = fromEpoch(lastSuccess)
	usage.LastFailureAt = fromEpoch(lastFail)
	return usage, nil
}

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
