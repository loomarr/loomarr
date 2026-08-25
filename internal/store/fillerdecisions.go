package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/filleradmission"
	"github.com/loomarr/loomarr/internal/fillerdecision"
)

const fillerDecisionSelect = `SELECT id, clip_hash, evidence_hash, evidence_version,
	schema_version, policy_version, taxonomy_version, result_json, created_at
	FROM filler_admission_decisions`

func (s *sqlStore) PutFillerDecision(ctx context.Context, record fillerdecision.Record) error {
	if err := fillerdecision.ValidateRecord(record); err != nil {
		return err
	}
	payload, err := json.Marshal(record.Result)
	if err != nil {
		return fmt.Errorf("encode filler decision: %w", err)
	}
	kind, verdict, holdCode, retryable := decisionColumns(record)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin filler decision: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, s.ph(`INSERT INTO filler_admission_decisions (
		id, clip_hash, evidence_hash, evidence_version, schema_version, policy_version,
		taxonomy_version, outcome_kind, verdict, hold_code, retryable, result_json, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO NOTHING`),
		record.ID, record.ClipHash, record.EvidenceHash, record.EvidenceVersion,
		record.SchemaVersion, record.PolicyVersion, record.TaxonomyVersion, kind, verdict,
		holdCode, retryable, string(payload), fillerDecisionEpoch(record.CreatedAt))
	if err != nil {
		return fmt.Errorf("insert filler decision: %w", err)
	}
	inserted, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect filler decision insert: %w", err)
	}
	if inserted == 0 {
		existing, scanErr := scanFillerDecision(tx.QueryRowContext(ctx,
			s.ph(fillerDecisionSelect+` WHERE id = ?`), record.ID))
		if scanErr != nil {
			return fmt.Errorf("read existing filler decision: %w", scanErr)
		}
		if !sameFillerDecision(existing, record, payload) {
			return fillerdecision.ErrConflict
		}
		return nil
	}
	for _, evaluationID := range decisionEvaluationIDs(record.Result) {
		if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO filler_admission_decision_inference_refs
			(decision_id, evaluation_id) VALUES (?, ?)`), record.ID, evaluationID); err != nil {
			return fmt.Errorf("link filler decision inference evaluation %s: %w", evaluationID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit filler decision: %w", err)
	}
	return nil
}

func decisionColumns(record fillerdecision.Record) (string, string, string, int) {
	if record.Result.Decision != nil {
		return string(fillerdecision.OutcomeSemantic), string(record.Result.Decision.Verdict), "", 0
	}
	retryable := 0
	if record.Result.Hold.Retryable {
		retryable = 1
	}
	return string(fillerdecision.OutcomeOperational), "", string(record.Result.Hold.Code), retryable
}

func decisionEvaluationIDs(result filleradmission.Result) []string {
	var attribution []filleradmission.Attribution
	if result.Decision != nil {
		attribution = result.Decision.Attribution
	} else if result.Hold != nil {
		attribution = result.Hold.Attribution
	}
	seen := make(map[string]struct{}, len(attribution))
	ids := make([]string, 0, len(attribution))
	for _, item := range attribution {
		if item.EvaluationID == "" {
			continue
		}
		if _, ok := seen[item.EvaluationID]; ok {
			continue
		}
		seen[item.EvaluationID] = struct{}{}
		ids = append(ids, item.EvaluationID)
	}
	return ids
}

func sameFillerDecision(existing, proposed fillerdecision.Record, payload []byte) bool {
	existingPayload, err := json.Marshal(existing.Result)
	return err == nil && existing.ID == proposed.ID && existing.ClipHash == proposed.ClipHash &&
		existing.EvidenceHash == proposed.EvidenceHash && existing.EvidenceVersion == proposed.EvidenceVersion &&
		existing.SchemaVersion == proposed.SchemaVersion && existing.PolicyVersion == proposed.PolicyVersion &&
		existing.TaxonomyVersion == proposed.TaxonomyVersion && fillerDecisionEpoch(existing.CreatedAt) == fillerDecisionEpoch(proposed.CreatedAt) &&
		string(existingPayload) == string(payload)
}

func (s *sqlStore) GetFillerDecision(ctx context.Context, id string) (fillerdecision.Record, error) {
	record, err := scanFillerDecision(s.db.QueryRowContext(ctx, s.ph(fillerDecisionSelect+` WHERE id = ?`), id))
	if errors.Is(err, sql.ErrNoRows) {
		return fillerdecision.Record{}, ErrNotFound
	}
	if err != nil {
		return fillerdecision.Record{}, fmt.Errorf("get filler decision %s: %w", id, err)
	}
	return record, nil
}

func scanFillerDecision(sc scannable) (fillerdecision.Record, error) {
	var record fillerdecision.Record
	var payload string
	var createdAt int64
	if err := sc.Scan(&record.ID, &record.ClipHash, &record.EvidenceHash, &record.EvidenceVersion,
		&record.SchemaVersion, &record.PolicyVersion, &record.TaxonomyVersion, &payload, &createdAt); err != nil {
		return fillerdecision.Record{}, err
	}
	if err := json.Unmarshal([]byte(payload), &record.Result); err != nil {
		return fillerdecision.Record{}, fmt.Errorf("decode filler decision %s: %w", record.ID, err)
	}
	record.CreatedAt = fromFillerDecisionEpoch(createdAt)
	return record, nil
}

func (s *sqlStore) ListFillerDecisions(ctx context.Context, filter fillerdecision.DecisionFilter) (fillerdecision.DecisionPage, error) {
	where, args, err := fillerDecisionWhere(filter, true)
	if err != nil {
		return fillerdecision.DecisionPage{}, err
	}
	countWhere, countArgs, err := fillerDecisionWhere(filter, false)
	if err != nil {
		return fillerdecision.DecisionPage{}, err
	}
	var total int
	if err := s.db.QueryRowContext(ctx, s.ph(`SELECT COUNT(*) FROM filler_admission_decisions d`+countWhere), countArgs...).Scan(&total); err != nil {
		return fillerdecision.DecisionPage{}, fmt.Errorf("count filler decisions: %w", err)
	}
	query := fillerDecisionSelect + ` d` + where + ` ORDER BY d.created_at DESC, d.id DESC LIMIT ?`
	args = append(args, filter.Limit)
	rows, err := s.db.QueryContext(ctx, s.ph(query), args...)
	if err != nil {
		return fillerdecision.DecisionPage{}, fmt.Errorf("list filler decisions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	page := fillerdecision.DecisionPage{Rows: make([]fillerdecision.Record, 0), Total: total}
	for rows.Next() {
		record, err := scanFillerDecision(rows)
		if err != nil {
			return fillerdecision.DecisionPage{}, err
		}
		page.Rows = append(page.Rows, record)
	}
	return page, rows.Err()
}

func fillerDecisionWhere(filter fillerdecision.DecisionFilter, includeCursor bool) (string, []any, error) {
	if filter.Limit < 1 || filter.Limit > fillerdecision.MaxPageSize {
		return "", nil, fmt.Errorf("%w: invalid decision page limit", fillerdecision.ErrInvalid)
	}
	var clauses []string
	var args []any
	if filter.Kind != "" {
		if filter.Kind != fillerdecision.OutcomeSemantic && filter.Kind != fillerdecision.OutcomeOperational {
			return "", nil, fmt.Errorf("%w: unknown outcome kind", fillerdecision.ErrInvalid)
		}
		clauses, args = append(clauses, `d.outcome_kind = ?`), append(args, string(filter.Kind))
	}
	if filter.Verdict != "" {
		if filter.Verdict != filleradmission.VerdictAdmit && filter.Verdict != filleradmission.VerdictReject && filter.Verdict != filleradmission.VerdictReview {
			return "", nil, fmt.Errorf("%w: unknown verdict", fillerdecision.ErrInvalid)
		}
		clauses, args = append(clauses, `d.verdict = ?`), append(args, string(filter.Verdict))
	}
	if filter.RetryableOnly {
		clauses = append(clauses, `d.outcome_kind = 'operational'`, `d.retryable = 1`)
	}
	if filter.CurrentOnly {
		clauses = append(clauses, `NOT EXISTS (SELECT 1 FROM filler_admission_decisions newer
			WHERE newer.clip_hash = d.clip_hash AND (newer.created_at > d.created_at
			OR (newer.created_at = d.created_at AND newer.id > d.id)))`)
	}
	if filter.UnresolvedOnly {
		clauses = append(clauses, `d.outcome_kind = 'semantic'`, `d.verdict = 'review'`,
			`NOT EXISTS (SELECT 1 FROM filler_admission_actions a WHERE a.decision_id = d.id AND a.kind IN ('admit', 'reject', 'correct'))`)
	}
	if includeCursor && !filter.Cursor.BeforeCreatedAt.IsZero() {
		if filter.Cursor.BeforeID == "" {
			return "", nil, fmt.Errorf("%w: cursor id is required", fillerdecision.ErrInvalid)
		}
		at := fillerDecisionEpoch(filter.Cursor.BeforeCreatedAt)
		clauses = append(clauses, `(d.created_at < ? OR (d.created_at = ? AND d.id < ?))`)
		args = append(args, at, at, filter.Cursor.BeforeID)
	}
	if len(clauses) == 0 {
		return "", args, nil
	}
	return ` WHERE ` + strings.Join(clauses, ` AND `), args, nil
}

func (s *sqlStore) FillerDecisionCounts(ctx context.Context) (fillerdecision.Counts, error) {
	row := s.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(CASE WHEN d.outcome_kind = 'semantic' AND d.verdict = 'admit' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN d.outcome_kind = 'semantic' AND d.verdict = 'reject' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN d.outcome_kind = 'semantic' AND d.verdict = 'review' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN d.outcome_kind = 'operational' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN d.outcome_kind = 'operational' AND d.retryable = 1 THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN d.outcome_kind = 'semantic' AND d.verdict = 'review' AND NOT EXISTS (
			SELECT 1 FROM filler_admission_actions a WHERE a.decision_id = d.id
			AND a.kind IN ('admit', 'reject', 'correct')) THEN 1 ELSE 0 END), 0)
		FROM filler_admission_decisions d
		WHERE NOT EXISTS (SELECT 1 FROM filler_admission_decisions newer
			WHERE newer.clip_hash = d.clip_hash AND (newer.created_at > d.created_at
			OR (newer.created_at = d.created_at AND newer.id > d.id)))`)
	var counts fillerdecision.Counts
	if err := row.Scan(&counts.Admitted, &counts.Rejected, &counts.Reviews, &counts.Operational,
		&counts.Retryable, &counts.UnresolvedReviews); err != nil {
		return fillerdecision.Counts{}, fmt.Errorf("count filler decision states: %w", err)
	}
	return counts, nil
}

func (s *sqlStore) CommitFillerDecisionAction(ctx context.Context, action fillerdecision.Action) error {
	if err := fillerdecision.ValidateAction(action); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin filler decision action: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var outcome, verdict string
	lock := `SELECT d.outcome_kind, d.verdict FROM filler_admission_decisions d WHERE d.id = ?
		AND NOT EXISTS (SELECT 1 FROM filler_admission_decisions newer
			WHERE newer.clip_hash = d.clip_hash AND (newer.created_at > d.created_at
			OR (newer.created_at = d.created_at AND newer.id > d.id)))`
	if s.dialect == DialectPostgres {
		lock += ` FOR UPDATE`
	}
	if err := tx.QueryRowContext(ctx, s.ph(lock), action.DecisionID).Scan(&outcome, &verdict); errors.Is(err, sql.ErrNoRows) {
		var exists int
		if countErr := tx.QueryRowContext(ctx, s.ph(`SELECT COUNT(*) FROM filler_admission_decisions WHERE id = ?`), action.DecisionID).Scan(&exists); countErr != nil {
			return fmt.Errorf("check stale filler decision: %w", countErr)
		}
		if exists > 0 {
			return fillerdecision.ErrActionStale
		}
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("lock filler decision: %w", err)
	}

	existing, found, err := getFillerDecisionAction(ctx, tx, s.ph, action.ID)
	if err != nil {
		return err
	}
	if found {
		if sameFillerDecisionAction(existing, action) {
			return nil
		}
		return fillerdecision.ErrConflict
	}
	latest, hasLatest, err := latestFillerDecisionAction(ctx, tx, s.ph, action.DecisionID)
	if err != nil {
		return err
	}
	if hasLatest {
		if action.SupersedesID != latest.ID {
			return fillerdecision.ErrActionStale
		}
	} else if action.SupersedesID != "" {
		return fillerdecision.ErrActionStale
	}
	if !actionAllowed(outcome, filleradmission.Verdict(verdict), latest, hasLatest, action.Kind) {
		return fillerdecision.ErrActionNotAllowed
	}
	_, err = tx.ExecContext(ctx, s.ph(`INSERT INTO filler_admission_actions
		(id, decision_id, kind, actor_id, reason, answer, corrected_verdict, supersedes_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`), action.ID, action.DecisionID, action.Kind, action.ActorID,
		action.Reason, action.Answer, action.CorrectedVerdict, action.SupersedesID, fillerDecisionEpoch(action.CreatedAt))
	if err != nil {
		return fmt.Errorf("insert filler decision action: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit filler decision action: %w", err)
	}
	return nil
}

func actionAllowed(outcome string, verdict filleradmission.Verdict, latest fillerdecision.Action, hasLatest bool, next fillerdecision.ActionKind) bool {
	if outcome != string(fillerdecision.OutcomeSemantic) {
		return false
	}
	state := string(verdict)
	if hasLatest {
		switch latest.Kind {
		case fillerdecision.ActionAdmit, fillerdecision.ActionRestore:
			state = string(filleradmission.VerdictAdmit)
		case fillerdecision.ActionReject:
			state = string(filleradmission.VerdictReject)
		case fillerdecision.ActionCorrect:
			state = string(latest.CorrectedVerdict)
		case fillerdecision.ActionReverse:
			state = "reversed"
		}
	}
	switch state {
	case string(filleradmission.VerdictReview):
		return next == fillerdecision.ActionAdmit || next == fillerdecision.ActionReject ||
			next == fillerdecision.ActionCorrect || next == fillerdecision.ActionAbandon
	case string(filleradmission.VerdictAdmit):
		return next == fillerdecision.ActionReverse
	case string(filleradmission.VerdictReject), "reversed":
		return next == fillerdecision.ActionRestore
	default:
		return false
	}
}

const fillerDecisionActionSelect = `SELECT id, decision_id, kind, actor_id, reason, answer,
	corrected_verdict, supersedes_id, created_at FROM filler_admission_actions`

type actionRowQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func getFillerDecisionAction(ctx context.Context, q actionRowQueryer, ph placeholder, id string) (fillerdecision.Action, bool, error) {
	action, err := scanFillerDecisionAction(q.QueryRowContext(ctx, ph(fillerDecisionActionSelect+` WHERE id = ?`), id))
	if errors.Is(err, sql.ErrNoRows) {
		return fillerdecision.Action{}, false, nil
	}
	if err != nil {
		return fillerdecision.Action{}, false, fmt.Errorf("get filler decision action: %w", err)
	}
	return action, true, nil
}

func latestFillerDecisionAction(ctx context.Context, q actionRowQueryer, ph placeholder, decisionID string) (fillerdecision.Action, bool, error) {
	action, err := scanFillerDecisionAction(q.QueryRowContext(ctx, ph(fillerDecisionActionSelect+
		` WHERE decision_id = ? AND kind <> 'abandon' ORDER BY created_at DESC, id DESC LIMIT 1`), decisionID))
	if errors.Is(err, sql.ErrNoRows) {
		return fillerdecision.Action{}, false, nil
	}
	if err != nil {
		return fillerdecision.Action{}, false, fmt.Errorf("get latest filler decision action: %w", err)
	}
	return action, true, nil
}

func sameFillerDecisionAction(a, b fillerdecision.Action) bool {
	return a.ID == b.ID && a.DecisionID == b.DecisionID && a.Kind == b.Kind && a.ActorID == b.ActorID &&
		a.Reason == b.Reason && a.Answer == b.Answer && a.CorrectedVerdict == b.CorrectedVerdict &&
		a.SupersedesID == b.SupersedesID
}

func scanFillerDecisionAction(sc scannable) (fillerdecision.Action, error) {
	var action fillerdecision.Action
	var createdAt int64
	if err := sc.Scan(&action.ID, &action.DecisionID, &action.Kind, &action.ActorID, &action.Reason,
		&action.Answer, &action.CorrectedVerdict, &action.SupersedesID, &createdAt); err != nil {
		return fillerdecision.Action{}, err
	}
	action.CreatedAt = fromFillerDecisionEpoch(createdAt)
	return action, nil
}

func (s *sqlStore) ListFillerDecisionActions(ctx context.Context, filter fillerdecision.ActionFilter) (fillerdecision.ActionPage, error) {
	if filter.Limit < 1 || filter.Limit > fillerdecision.MaxPageSize {
		return fillerdecision.ActionPage{}, fmt.Errorf("%w: invalid action page limit", fillerdecision.ErrInvalid)
	}
	var clauses []string
	var args []any
	if filter.DecisionID != "" {
		clauses, args = append(clauses, `decision_id = ?`), append(args, filter.DecisionID)
	}
	countClauses := append([]string(nil), clauses...)
	countArgs := append([]any(nil), args...)
	if !filter.Cursor.BeforeCreatedAt.IsZero() {
		if filter.Cursor.BeforeID == "" {
			return fillerdecision.ActionPage{}, fmt.Errorf("%w: cursor id is required", fillerdecision.ErrInvalid)
		}
		at := fillerDecisionEpoch(filter.Cursor.BeforeCreatedAt)
		clauses = append(clauses, `(created_at < ? OR (created_at = ? AND id < ?))`)
		args = append(args, at, at, filter.Cursor.BeforeID)
	}
	where := sqlWhere(clauses)
	var total int
	if err := s.db.QueryRowContext(ctx, s.ph(`SELECT COUNT(*) FROM filler_admission_actions`+sqlWhere(countClauses)), countArgs...).Scan(&total); err != nil {
		return fillerdecision.ActionPage{}, fmt.Errorf("count filler decision actions: %w", err)
	}
	args = append(args, filter.Limit)
	rows, err := s.db.QueryContext(ctx, s.ph(fillerDecisionActionSelect+where+` ORDER BY created_at DESC, id DESC LIMIT ?`), args...)
	if err != nil {
		return fillerdecision.ActionPage{}, fmt.Errorf("list filler decision actions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	page := fillerdecision.ActionPage{Rows: make([]fillerdecision.Action, 0), Total: total}
	for rows.Next() {
		action, err := scanFillerDecisionAction(rows)
		if err != nil {
			return fillerdecision.ActionPage{}, err
		}
		page.Rows = append(page.Rows, action)
	}
	return page, rows.Err()
}

func (s *sqlStore) ListFillerDecisionActivity(ctx context.Context, cursor fillerdecision.Cursor, limit int) (fillerdecision.ActivityPage, error) {
	if limit < 1 || limit > fillerdecision.MaxPageSize {
		return fillerdecision.ActivityPage{}, fmt.Errorf("%w: invalid activity page limit", fillerdecision.ErrInvalid)
	}
	base := `SELECT 'decision:' || d.id AS event_id, '' AS action_id, d.id AS decision_id, d.clip_hash, '' AS actor_id,
		'' AS reason, CASE d.verdict WHEN 'admit' THEN 'automatic_admit' WHEN 'reject' THEN 'automatic_reject'
		ELSE 'review_requested' END AS event_kind, d.created_at
		FROM filler_admission_decisions d WHERE d.outcome_kind = 'semantic'
		UNION ALL
		SELECT 'action:' || a.id, a.id, a.decision_id, d.clip_hash, a.actor_id, a.reason,
		CASE a.kind WHEN 'admit' THEN 'review_admit' WHEN 'reject' THEN 'review_reject'
		WHEN 'correct' THEN 'correction' WHEN 'abandon' THEN 'review_abandoned'
		WHEN 'restore' THEN 'restore' ELSE 'reversal' END, a.created_at
		FROM filler_admission_actions a JOIN filler_admission_decisions d ON d.id = a.decision_id`
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM (`+base+`) activity_count`).Scan(&total); err != nil {
		return fillerdecision.ActivityPage{}, fmt.Errorf("count filler decision activity: %w", err)
	}
	query := `SELECT event_id, action_id, decision_id, clip_hash, actor_id, reason, event_kind, created_at FROM (` + base + `) activity`
	var args []any
	if !cursor.BeforeCreatedAt.IsZero() {
		if cursor.BeforeID == "" {
			return fillerdecision.ActivityPage{}, fmt.Errorf("%w: cursor id is required", fillerdecision.ErrInvalid)
		}
		at := fillerDecisionEpoch(cursor.BeforeCreatedAt)
		query += ` WHERE (created_at < ? OR (created_at = ? AND event_id < ?))`
		args = append(args, at, at, cursor.BeforeID)
	}
	query += ` ORDER BY created_at DESC, event_id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, s.ph(query), args...)
	if err != nil {
		return fillerdecision.ActivityPage{}, fmt.Errorf("list filler decision activity: %w", err)
	}
	defer func() { _ = rows.Close() }()
	page := fillerdecision.ActivityPage{Rows: make([]fillerdecision.ActivityItem, 0), Total: total}
	for rows.Next() {
		var item fillerdecision.ActivityItem
		var createdAt int64
		if err := rows.Scan(&item.ID, &item.ActionID, &item.DecisionID, &item.ClipHash, &item.ActorID, &item.Reason, &item.Kind, &createdAt); err != nil {
			return fillerdecision.ActivityPage{}, err
		}
		item.CreatedAt = fromFillerDecisionEpoch(createdAt)
		page.Rows = append(page.Rows, item)
	}
	return page, rows.Err()
}

func sqlWhere(clauses []string) string {
	if len(clauses) == 0 {
		return ""
	}
	return ` WHERE ` + strings.Join(clauses, ` AND `)
}

// Decision/action ordering needs finer precision than the legacy store epoch. Two operator
// actions can occur inside one second; collapsing them would make the random id tie-break choose
// the apparent latest transition. Nanoseconds preserve the committed order and remain one shared
// INTEGER/BIGINT representation across SQLite and Postgres.
func fillerDecisionEpoch(at time.Time) int64 {
	if at.IsZero() {
		return 0
	}
	return at.UnixNano()
}

func fromFillerDecisionEpoch(value int64) time.Time {
	if value == 0 {
		return time.Time{}
	}
	return time.Unix(0, value).UTC()
}
