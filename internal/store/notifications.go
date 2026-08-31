package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/notifications"
)

const notificationIntentSelect = `SELECT id, topic, recipient_kind, recipient_id,
 reference_kind, reference_id, recipient_policy, template_json, idempotency_key,
 created_at, terminal_at FROM notification_intents`

const notificationAttemptSelect = `SELECT id, intent_id, means, destination_ref,
 destination_redacted, status, attempt_number, available_at, lease_owner,
 lease_expires_at, started_at, finished_at, provider_message_id, failure_class,
 outcome_code, created_at FROM notification_delivery_attempts`

func (s *sqlStore) CreateNotificationIntent(
	ctx context.Context,
	intent notifications.Intent,
	attempts []notifications.Attempt,
) (notifications.Intent, bool, error) {
	if err := intent.Validate(); err != nil {
		return notifications.Intent{}, false, fmt.Errorf("validate notification intent: %w", err)
	}
	if len(attempts) == 0 {
		return notifications.Intent{}, false, fmt.Errorf("notification intent requires a delivery decision")
	}
	allTerminal := true
	for _, attempt := range attempts {
		if err := attempt.Validate(); err != nil {
			return notifications.Intent{}, false, fmt.Errorf("validate notification attempt: %w", err)
		}
		if attempt.IntentID != intent.ID || attempt.AttemptNumber != 1 ||
			(attempt.Status != notifications.StatusQueued && attempt.Status != notifications.StatusSuppressed) {
			return notifications.Intent{}, false, fmt.Errorf("initial notification attempt has invalid ownership or state")
		}
		if attempt.Status == notifications.StatusQueued {
			allTerminal = false
		}
	}
	if allTerminal {
		intent.TerminalAt = intent.CreatedAt
	} else {
		intent.TerminalAt = time.Time{}
	}
	templateJSON, err := json.Marshal(intent.Template)
	if err != nil {
		return notifications.Intent{}, false, fmt.Errorf("marshal notification template data: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return notifications.Intent{}, false, fmt.Errorf("create notification intent: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, s.ph(`INSERT INTO notification_intents
		(id, topic, recipient_kind, recipient_id, reference_kind, reference_id,
		 recipient_policy, template_json, idempotency_key, created_at, terminal_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(idempotency_key) DO NOTHING`),
		intent.ID, string(intent.Topic), string(intent.RecipientKind), intent.RecipientID,
		string(intent.ReferenceKind), intent.ReferenceID, string(intent.Policy), string(templateJSON),
		intent.IdempotencyKey, epoch(intent.CreatedAt), epoch(intent.TerminalAt))
	if err != nil {
		if isConstraintViolation(err) {
			return notifications.Intent{}, false, notifications.ErrConflict
		}
		return notifications.Intent{}, false, fmt.Errorf("create notification intent: insert: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return notifications.Intent{}, false, fmt.Errorf("create notification intent: affected rows: %w", err)
	}
	if inserted == 0 {
		existing, err := scanNotificationIntent(tx.QueryRowContext(ctx, s.ph(
			notificationIntentSelect+` WHERE idempotency_key = ?`), intent.IdempotencyKey))
		if err != nil {
			return notifications.Intent{}, false, fmt.Errorf("create notification intent: load idempotent row: %w", err)
		}
		if !sameNotificationRequest(existing, intent) {
			return notifications.Intent{}, false, notifications.ErrConflict
		}
		if err := tx.Commit(); err != nil {
			return notifications.Intent{}, false, fmt.Errorf("create notification intent: commit idempotent read: %w", err)
		}
		return existing, false, nil
	}

	for _, attempt := range attempts {
		if err := insertNotificationAttempt(ctx, tx, s.ph, attempt); err != nil {
			if isConstraintViolation(err) {
				return notifications.Intent{}, false, notifications.ErrConflict
			}
			return notifications.Intent{}, false, fmt.Errorf("create notification intent: insert attempt: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return notifications.Intent{}, false, fmt.Errorf("create notification intent: commit: %w", err)
	}
	return intent, true, nil
}

func (s *sqlStore) GetNotificationIntent(ctx context.Context, id string) (notifications.Intent, error) {
	intent, err := scanNotificationIntent(s.db.QueryRowContext(ctx, s.ph(
		notificationIntentSelect+` WHERE id = ?`), id))
	if err != nil {
		return notifications.Intent{}, err
	}
	return intent, nil
}

func (s *sqlStore) ListNotificationIntentsByReference(
	ctx context.Context,
	kind notifications.ReferenceKind,
	id string,
) ([]notifications.Intent, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(notificationIntentSelect+
		` WHERE reference_kind = ? AND reference_id = ? ORDER BY created_at DESC, id DESC`), string(kind), id)
	if err != nil {
		return nil, fmt.Errorf("list notification intents by reference: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var intents []notifications.Intent
	for rows.Next() {
		intent, err := scanNotificationIntent(rows)
		if err != nil {
			return nil, err
		}
		intents = append(intents, intent)
	}
	return intents, rows.Err()
}

func (s *sqlStore) ListNotificationAttempts(ctx context.Context, intentID string) ([]notifications.Attempt, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(notificationAttemptSelect+
		` WHERE intent_id = ? ORDER BY attempt_number, id`), intentID)
	if err != nil {
		return nil, fmt.Errorf("list notification attempts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var attempts []notifications.Attempt
	for rows.Next() {
		attempt, err := scanNotificationAttempt(rows)
		if err != nil {
			return nil, err
		}
		attempts = append(attempts, attempt)
	}
	return attempts, rows.Err()
}

// ClaimDueNotificationAttempt conservatively closes an abandoned sending lease as ambiguous before
// claiming new work. Once an adapter started, a crashed process cannot prove remote non-acceptance.
func (s *sqlStore) ClaimDueNotificationAttempt(
	ctx context.Context,
	owner string,
	now time.Time,
	lease time.Duration,
) (notifications.Attempt, error) {
	if owner == "" || len(owner) > 200 || lease <= 0 || now.IsZero() {
		return notifications.Attempt{}, fmt.Errorf("claim notification attempt requires owner, time, and lease")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return notifications.Attempt{}, fmt.Errorf("claim notification attempt: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, s.ph(`UPDATE notification_delivery_attempts
		SET status='failed', lease_owner='', lease_expires_at=0, finished_at=?,
		    failure_class='ambiguous_acceptance', outcome_code='worker_interrupted'
		WHERE status='sending' AND lease_expires_at <= ?`), epoch(now), epoch(now)); err != nil {
		return notifications.Attempt{}, fmt.Errorf("claim notification attempt: recover expired leases: %w", err)
	}
	if err := terminalizeNotificationIntents(ctx, tx, s.ph, now); err != nil {
		return notifications.Attempt{}, fmt.Errorf("claim notification attempt: terminalize recovered intents: %w", err)
	}

	query := notificationAttemptSelect +
		` WHERE status = 'queued' AND available_at <= ? ORDER BY available_at, id LIMIT 1`
	if s.dialect == DialectPostgres {
		query += ` FOR UPDATE SKIP LOCKED`
	}
	attempt, err := scanNotificationAttempt(tx.QueryRowContext(ctx, s.ph(query), epoch(now)))
	if err == notifications.ErrNotFound {
		if commitErr := tx.Commit(); commitErr != nil {
			return notifications.Attempt{}, fmt.Errorf("claim notification attempt: commit recovery: %w", commitErr)
		}
		return notifications.Attempt{}, notifications.ErrNotFound
	}
	if err != nil {
		return notifications.Attempt{}, fmt.Errorf("claim notification attempt: select: %w", err)
	}
	result, err := tx.ExecContext(ctx, s.ph(`UPDATE notification_delivery_attempts
		SET status='sending', lease_owner=?, lease_expires_at=?, started_at=?
		WHERE id=? AND status='queued' AND available_at <= ?`),
		owner, epoch(now.Add(lease)), epoch(now), attempt.ID, epoch(now))
	if err != nil {
		return notifications.Attempt{}, fmt.Errorf("claim notification attempt: update: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return notifications.Attempt{}, notifications.ErrConflict
	}
	if err := tx.Commit(); err != nil {
		return notifications.Attempt{}, fmt.Errorf("claim notification attempt: commit: %w", err)
	}
	attempt.Status = notifications.StatusSending
	attempt.LeaseOwner = owner
	attempt.LeaseExpiresAt = now.Add(lease)
	attempt.StartedAt = now
	return attempt, nil
}

func (s *sqlStore) CompleteNotificationAttempt(ctx context.Context, completion notifications.Completion) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("complete notification attempt: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	current, err := scanNotificationAttempt(tx.QueryRowContext(ctx, s.ph(
		notificationAttemptSelect+` WHERE id = ?`), completion.AttemptID))
	if err != nil {
		return err
	}
	if current.Status != notifications.StatusSending || current.LeaseOwner != completion.LeaseOwner {
		return notifications.ErrConflict
	}
	if err := completion.Validate(current); err != nil {
		return fmt.Errorf("complete notification attempt: validate: %w", err)
	}
	result, err := tx.ExecContext(ctx, s.ph(`UPDATE notification_delivery_attempts
		SET status=?, lease_owner='', lease_expires_at=0, finished_at=?, provider_message_id=?,
		    failure_class=?, outcome_code=?
		WHERE id=? AND status='sending' AND lease_owner=?`),
		string(completion.Status), epoch(completion.FinishedAt), completion.ProviderMessageID,
		string(completion.FailureClass), string(completion.OutcomeCode), completion.AttemptID, completion.LeaseOwner)
	if err != nil {
		return fmt.Errorf("complete notification attempt: update: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return notifications.ErrConflict
	}
	if completion.Next != nil {
		if err := insertNotificationAttempt(ctx, tx, s.ph, *completion.Next); err != nil {
			if isConstraintViolation(err) {
				return notifications.ErrConflict
			}
			return fmt.Errorf("complete notification attempt: insert retry: %w", err)
		}
	}
	if err := terminalizeNotificationIntents(ctx, tx, s.ph, completion.FinishedAt); err != nil {
		return fmt.Errorf("complete notification attempt: terminalize intent: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("complete notification attempt: commit: %w", err)
	}
	return nil
}

func (s *sqlStore) PurgeTerminalNotifications(ctx context.Context, before time.Time) (int, error) {
	result, err := s.db.ExecContext(ctx, s.ph(
		`DELETE FROM notification_intents WHERE terminal_at > 0 AND terminal_at <= ?`), epoch(before))
	if err != nil {
		return 0, fmt.Errorf("purge terminal notifications: %w", err)
	}
	count, err := result.RowsAffected()
	return int(count), err
}

func insertNotificationAttempt(
	ctx context.Context,
	tx *sql.Tx,
	placeholder func(string) string,
	attempt notifications.Attempt,
) error {
	_, err := tx.ExecContext(ctx, placeholder(`INSERT INTO notification_delivery_attempts
		(id, intent_id, means, destination_ref, destination_redacted, status, attempt_number,
		 available_at, lease_owner, lease_expires_at, started_at, finished_at,
		 provider_message_id, failure_class, outcome_code, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		attempt.ID, attempt.IntentID, string(attempt.Means), attempt.DestinationRef,
		attempt.DestinationRedacted, string(attempt.Status), attempt.AttemptNumber,
		epoch(attempt.AvailableAt), attempt.LeaseOwner, epoch(attempt.LeaseExpiresAt),
		epoch(attempt.StartedAt), epoch(attempt.FinishedAt), attempt.ProviderMessageID,
		string(attempt.FailureClass), string(attempt.OutcomeCode), epoch(attempt.CreatedAt))
	return err
}

func terminalizeNotificationIntents(
	ctx context.Context,
	tx *sql.Tx,
	placeholder func(string) string,
	at time.Time,
) error {
	_, err := tx.ExecContext(ctx, placeholder(`UPDATE notification_intents
		SET terminal_at = ?
		WHERE terminal_at = 0 AND NOT EXISTS (
			SELECT 1 FROM notification_delivery_attempts attempt
			WHERE attempt.intent_id = notification_intents.id
			  AND attempt.status IN ('queued', 'sending')
		)`), epoch(at))
	return err
}

func scanNotificationIntent(sc scannable) (notifications.Intent, error) {
	var intent notifications.Intent
	var topic, recipientKind, referenceKind, policy, templateJSON string
	var createdAt, terminalAt int64
	if err := sc.Scan(&intent.ID, &topic, &recipientKind, &intent.RecipientID,
		&referenceKind, &intent.ReferenceID, &policy, &templateJSON, &intent.IdempotencyKey,
		&createdAt, &terminalAt); err == sql.ErrNoRows {
		return notifications.Intent{}, notifications.ErrNotFound
	} else if err != nil {
		return notifications.Intent{}, err
	}
	intent.Topic = notifications.Topic(topic)
	intent.RecipientKind = notifications.RecipientKind(recipientKind)
	intent.ReferenceKind = notifications.ReferenceKind(referenceKind)
	intent.Policy = notifications.RecipientPolicy(policy)
	if err := json.Unmarshal([]byte(templateJSON), &intent.Template); err != nil {
		return notifications.Intent{}, fmt.Errorf("decode notification template data: %w", err)
	}
	intent.CreatedAt = fromEpoch(createdAt)
	if terminalAt > 0 {
		intent.TerminalAt = fromEpoch(terminalAt)
	}
	return intent, nil
}

func scanNotificationAttempt(sc scannable) (notifications.Attempt, error) {
	var attempt notifications.Attempt
	var means, status, failureClass, outcomeCode string
	var availableAt, leaseExpiresAt, startedAt, finishedAt, createdAt int64
	if err := sc.Scan(&attempt.ID, &attempt.IntentID, &means, &attempt.DestinationRef,
		&attempt.DestinationRedacted, &status, &attempt.AttemptNumber, &availableAt,
		&attempt.LeaseOwner, &leaseExpiresAt, &startedAt, &finishedAt,
		&attempt.ProviderMessageID, &failureClass, &outcomeCode, &createdAt); err == sql.ErrNoRows {
		return notifications.Attempt{}, notifications.ErrNotFound
	} else if err != nil {
		return notifications.Attempt{}, err
	}
	attempt.Means = notifications.Means(means)
	attempt.Status = notifications.Status(status)
	attempt.FailureClass = notifications.FailureClass(failureClass)
	attempt.OutcomeCode = notifications.OutcomeCode(outcomeCode)
	attempt.AvailableAt = fromEpoch(availableAt)
	if leaseExpiresAt > 0 {
		attempt.LeaseExpiresAt = fromEpoch(leaseExpiresAt)
	}
	if startedAt > 0 {
		attempt.StartedAt = fromEpoch(startedAt)
	}
	if finishedAt > 0 {
		attempt.FinishedAt = fromEpoch(finishedAt)
	}
	attempt.CreatedAt = fromEpoch(createdAt)
	return attempt, nil
}

func sameNotificationRequest(a, b notifications.Intent) bool {
	return a.Topic == b.Topic && a.RecipientKind == b.RecipientKind && a.RecipientID == b.RecipientID &&
		a.ReferenceKind == b.ReferenceKind && a.ReferenceID == b.ReferenceID && a.Policy == b.Policy &&
		a.Template == b.Template
}
