package catalogstore

import (
	"context"
	"database/sql"
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
func (s *Store) UpsertClipDefects(ctx context.Context, r ClipDefectRecord) error {
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
	_, err = s.h.ExecContext(ctx, s.h.Rebind(
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
func (s *Store) ListClipDefects(ctx context.Context) (map[string]ClipDefectRecord, error) {
	rows, err := s.h.QueryContext(ctx, `SELECT `+clipDefectColumns+` FROM filler_clip_defects`)
	if err != nil {
		return nil, fmt.Errorf("list clip defects: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]ClipDefectRecord{}
	var invalid []error
	for rows.Next() {
		r, err := scanClipDefects(rows)
		var corrupt *corruptClipDefects
		switch {
		case errors.As(err, &corrupt):
			invalid = append(invalid, err)
			continue
		case err != nil:
			return nil, err
		}
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

// ClipDefectLabel is the maintainer's label on a clip's defect record (#2034 step 4c).
type ClipDefectLabel struct {
	Label defects.Label
	// Era is the era the clip was labelled against. A label for another era band no longer
	// answers the question the floor is asking.
	Era        int
	LabelledAt time.Time
}

// SetClipDefectLabel labels a clip's defect record for the era the record was judged against, in
// the same statement, so a concurrent era correction cannot pair a label with the wrong era. An
// empty label clears it. ErrNotFound when the clip has no record: only a measured clip can be
// scored against.
func (s *Store) SetClipDefectLabel(ctx context.Context, clipHash string, label defects.Label, at time.Time) error {
	if label != "" && label != defects.LabelPass && label != defects.LabelFail {
		return fmt.Errorf("label clip defects %s: unknown label %q", clipHash, label)
	}
	query := `UPDATE filler_clip_defects SET label = ?, label_era = era, labelled_at = ? WHERE clip_hash = ?`
	args := []any{string(label), epoch(at), clipHash}
	if label == "" {
		query = `UPDATE filler_clip_defects SET label = '', label_era = 0, labelled_at = 0 WHERE clip_hash = ?`
		args = []any{clipHash}
	}
	res, err := s.h.ExecContext(ctx, s.h.Rebind(query), args...)
	if err != nil {
		return fmt.Errorf("label clip defects %s: %w", clipHash, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("label clip defects %s: %w", clipHash, err)
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListClipDefectLabels reads every labelled record's label, keyed by clip hash.
func (s *Store) ListClipDefectLabels(ctx context.Context) (map[string]ClipDefectLabel, error) {
	rows, err := s.h.QueryContext(ctx,
		`SELECT clip_hash, label, label_era, labelled_at FROM filler_clip_defects WHERE label <> ''`)
	if err != nil {
		return nil, fmt.Errorf("list clip defect labels: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]ClipDefectLabel{}
	for rows.Next() {
		var hash, label string
		var l ClipDefectLabel
		var at int64
		if err := rows.Scan(&hash, &label, &l.Era, &at); err != nil {
			return nil, fmt.Errorf("scan clip defect labels: %w", err)
		}
		l.Label, l.LabelledAt = defects.Label(label), fromEpoch(at)
		out[hash] = l
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list clip defect labels: %w", err)
	}
	return out, nil
}

// GetClipDefects reads one clip's record. A corrupt row reads as no record, so the clip is
// measured again rather than admitted on a verdict nobody can read.
func (s *Store) GetClipDefects(ctx context.Context, clipHash string) (ClipDefectRecord, bool, error) {
	r, err := scanClipDefects(s.h.QueryRowContext(ctx, s.h.Rebind(
		`SELECT `+clipDefectColumns+` FROM filler_clip_defects WHERE clip_hash = ?`), clipHash))
	var corrupt *corruptClipDefects
	switch {
	case errors.Is(err, sql.ErrNoRows), errors.As(err, &corrupt):
		return ClipDefectRecord{}, false, nil
	case err != nil:
		return ClipDefectRecord{}, false, fmt.Errorf("get clip defects %s: %w", clipHash, err)
	}
	return r, true, nil
}

const clipDefectColumns = `clip_hash, era, readings_json, assessment_json, judgement_json, judge_failures,
	verdict, verdict_reason, measured_at`

// corruptClipDefects is a row that scanned but whose JSON does not decode.
type corruptClipDefects struct{ err error }

func (e *corruptClipDefects) Error() string { return e.err.Error() }
func (e *corruptClipDefects) Unwrap() error { return e.err }

func scanClipDefects(row interface{ Scan(...any) error }) (ClipDefectRecord, error) {
	var r ClipDefectRecord
	var readings, assessment, judgement, verdict string
	var measuredAt int64
	if err := row.Scan(&r.ClipHash, &r.Era, &readings, &assessment, &judgement, &r.JudgeFailures, &verdict,
		&r.VerdictReason, &measuredAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return r, err
		}
		return r, fmt.Errorf("scan clip defects: %w", err)
	}
	r.Verdict = defects.Verdict(verdict)
	if err := errors.Join(json.Unmarshal([]byte(readings), &r.Readings),
		json.Unmarshal([]byte(assessment), &r.Assessment)); err != nil {
		return r, &corruptClipDefects{fmt.Errorf("clip defects %s: %w", r.ClipHash, err)}
	}
	if judgement != "" {
		r.Judgement = &defects.Judgement{}
		if err := json.Unmarshal([]byte(judgement), r.Judgement); err != nil {
			return r, &corruptClipDefects{fmt.Errorf("clip defects %s: %w", r.ClipHash, err)}
		}
	}
	r.MeasuredAt = fromEpoch(measuredAt)
	return r, nil
}

func (s *Store) pruneOrphanClipDefects(ctx context.Context) error {
	_, err := s.h.ExecContext(ctx, `DELETE FROM filler_clip_defects WHERE NOT EXISTS (
		SELECT 1 FROM clips c WHERE c.hash = filler_clip_defects.clip_hash)`)
	if err != nil {
		return fmt.Errorf("prune orphan clip defects: %w", err)
	}
	return nil
}
