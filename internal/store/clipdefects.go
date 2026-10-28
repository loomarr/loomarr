package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/defects"
)

// ClipDefectRecord is one clip's latest defect measurement and the verdict it earned (#2034).
type ClipDefectRecord struct {
	ClipHash   string
	Readings   defects.Readings
	Era        int // the era the verdict was judged against
	Assessment defects.Assessment
	// Judgement is the vision judge's ruling on a borderline assessment; nil until one is had.
	Judgement *defects.Judgement
	// JudgeFailures counts asks that brought back no ruling since the last one.
	JudgeFailures int
	// Verdict is the final standing: the assessment, with a borderline one settled by the judge.
	Verdict       defects.Verdict
	VerdictReason string
	MeasuredAt    time.Time
}

// UpsertClipDefects records a clip's measurement and verdict, replacing any earlier one.
func (s *sqlStore) UpsertClipDefects(ctx context.Context, r ClipDefectRecord) error {
	if r.ClipHash == "" || r.Readings.Version == "" || r.Assessment.Version == "" || r.Verdict == "" {
		return fmt.Errorf("upsert clip defects: clip hash, measurement and floor versions, and a verdict are required")
	}
	readings, err := json.Marshal(r.Readings)
	if err != nil {
		return fmt.Errorf("marshal clip defects %s: %w", r.ClipHash, err)
	}
	assessment, err := json.Marshal(r.Assessment)
	if err != nil {
		return fmt.Errorf("marshal clip defects %s: %w", r.ClipHash, err)
	}
	judgement := ""
	if r.Judgement != nil {
		raw, err := json.Marshal(r.Judgement)
		if err != nil {
			return fmt.Errorf("marshal clip defects %s: %w", r.ClipHash, err)
		}
		judgement = string(raw)
	}
	_, err = s.db.ExecContext(ctx, s.ph(
		`INSERT INTO filler_clip_defects (clip_hash, measurement_version, floor_version, era, outcome,
		   readings_json, assessment_json, judgement_json, judge_failures, verdict, verdict_reason, measured_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(clip_hash) DO UPDATE SET measurement_version = excluded.measurement_version,
		   floor_version = excluded.floor_version, era = excluded.era, outcome = excluded.outcome,
		   readings_json = excluded.readings_json, assessment_json = excluded.assessment_json,
		   judgement_json = excluded.judgement_json, judge_failures = excluded.judge_failures,
		   verdict = excluded.verdict, verdict_reason = excluded.verdict_reason, measured_at = excluded.measured_at`),
		r.ClipHash, r.Readings.Version, r.Assessment.Version, r.Era, string(r.Assessment.Outcome),
		string(readings), string(assessment), judgement, r.JudgeFailures, string(r.Verdict), r.VerdictReason,
		epoch(r.MeasuredAt))
	if err != nil {
		return fmt.Errorf("upsert clip defects %s: %w", r.ClipHash, err)
	}
	return nil
}

// ErrCorruptClipDefects marks a ListClipDefects result that omitted unreadable rows. The returned
// map holds every readable row; any other error means the read itself failed.
var ErrCorruptClipDefects = errors.New("corrupt clip defect records")

// ListClipDefects reads every record in one query. A corrupt row is omitted and reported under
// ErrCorruptClipDefects, so it is measured again rather than read as a verdict.
func (s *sqlStore) ListClipDefects(ctx context.Context) (map[string]ClipDefectRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT clip_hash, era, readings_json, assessment_json, judgement_json, judge_failures, verdict,
		   verdict_reason, measured_at FROM filler_clip_defects`)
	if err != nil {
		return nil, fmt.Errorf("list clip defects: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]ClipDefectRecord{}
	var invalid []error
	for rows.Next() {
		var r ClipDefectRecord
		var readings, assessment, judgement, verdict string
		var measuredAt int64
		if err := rows.Scan(&r.ClipHash, &r.Era, &readings, &assessment, &judgement, &r.JudgeFailures, &verdict,
			&r.VerdictReason, &measuredAt); err != nil {
			return nil, fmt.Errorf("scan clip defects: %w", err)
		}
		r.Verdict = defects.Verdict(verdict)
		if err := errors.Join(json.Unmarshal([]byte(readings), &r.Readings),
			json.Unmarshal([]byte(assessment), &r.Assessment)); err != nil {
			invalid = append(invalid, fmt.Errorf("clip defects %s: %w", r.ClipHash, err))
			continue
		}
		if judgement != "" {
			r.Judgement = &defects.Judgement{}
			if err := json.Unmarshal([]byte(judgement), r.Judgement); err != nil {
				invalid = append(invalid, fmt.Errorf("clip defects %s: %w", r.ClipHash, err))
				continue
			}
		}
		r.MeasuredAt = fromEpoch(measuredAt)
		out[r.ClipHash] = r
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list clip defects: %w", err)
	}
	if len(invalid) > 0 {
		return out, fmt.Errorf("%w: %w", ErrCorruptClipDefects, errors.Join(invalid...))
	}
	return out, nil
}

func (s *sqlStore) pruneOrphanClipDefects(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM filler_clip_defects WHERE NOT EXISTS (
		SELECT 1 FROM clips c WHERE c.hash = filler_clip_defects.clip_hash)`)
	if err != nil {
		return fmt.Errorf("prune orphan clip defects: %w", err)
	}
	return nil
}
