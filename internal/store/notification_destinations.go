package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/loomarr/loomarr/internal/notifications"
)

const notificationDestinationSelect = `SELECT id, means, label, scope, owner_id, audience,
	topics_json, enabled, configuration_json, credentials_encrypted, created_at, updated_at
	FROM notification_destinations`

func (s *sqlStore) SaveNotificationDestinationRecord(ctx context.Context, destination notifications.DestinationRecord) error {
	if err := destination.Validate(); err != nil {
		return fmt.Errorf("validate notification destination record: %w", err)
	}
	topicsJSON, err := json.Marshal(destination.Topics)
	if err != nil {
		return fmt.Errorf("encode notification destination topics: %w", err)
	}
	configurationJSON, err := json.Marshal(orEmptyStringMap(destination.Configuration))
	if err != nil {
		return fmt.Errorf("encode notification destination configuration: %w", err)
	}
	_, err = s.db.ExecContext(ctx, s.ph(`INSERT INTO notification_destinations
		(id, means, label, scope, owner_id, audience, topics_json, enabled,
		 configuration_json, credentials_encrypted, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			means = excluded.means,
			label = excluded.label,
			scope = excluded.scope,
			owner_id = excluded.owner_id,
			audience = excluded.audience,
			topics_json = excluded.topics_json,
			enabled = excluded.enabled,
			configuration_json = excluded.configuration_json,
			credentials_encrypted = excluded.credentials_encrypted,
			updated_at = excluded.updated_at`),
		destination.ID, destination.Means, destination.Label, destination.Scope, destination.OwnerID,
		destination.Audience, string(topicsJSON), notificationBool(destination.Enabled), string(configurationJSON),
		destination.CredentialsEncrypted, epoch(destination.CreatedAt), epoch(destination.UpdatedAt))
	if err != nil {
		return fmt.Errorf("save notification destination: %w", err)
	}
	return nil
}

func (s *sqlStore) GetNotificationDestinationRecord(ctx context.Context, id string) (notifications.DestinationRecord, error) {
	destination, err := scanNotificationDestinationRecord(s.db.QueryRowContext(
		ctx, s.ph(notificationDestinationSelect+` WHERE id = ?`), id,
	))
	if err != nil {
		return notifications.DestinationRecord{}, err
	}
	return destination, nil
}

func (s *sqlStore) ListNotificationDestinationRecords(ctx context.Context) ([]notifications.DestinationRecord, error) {
	rows, err := s.db.QueryContext(ctx, notificationDestinationSelect+` ORDER BY updated_at DESC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list notification destinations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	destinations := make([]notifications.DestinationRecord, 0)
	for rows.Next() {
		destination, scanErr := scanNotificationDestinationRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		destinations = append(destinations, destination)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list notification destinations: %w", err)
	}
	return destinations, nil
}

func (s *sqlStore) ListNotificationDestinationHealth(
	ctx context.Context,
) (map[string]notifications.DestinationHealth, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT d.id,
		COALESCE(MAX(CASE WHEN a.status = 'delivered' THEN a.finished_at ELSE 0 END), 0),
		COALESCE(MAX(CASE WHEN a.status = 'failed' THEN a.finished_at ELSE 0 END), 0),
		COALESCE((SELECT failed.outcome_code FROM notification_delivery_attempts failed
			WHERE failed.destination_ref = d.id AND failed.status = 'failed'
			ORDER BY failed.finished_at DESC, failed.id DESC LIMIT 1), ''),
		COALESCE(SUM(CASE WHEN a.status = 'queued' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN a.status = 'failed' THEN 1 ELSE 0 END), 0)
	FROM notification_destinations d
	LEFT JOIN notification_delivery_attempts a ON a.destination_ref = d.id
	GROUP BY d.id ORDER BY d.id`)
	if err != nil {
		return nil, fmt.Errorf("list notification destination health: %w", err)
	}
	defer func() { _ = rows.Close() }()
	health := make(map[string]notifications.DestinationHealth)
	for rows.Next() {
		var id, outcome string
		var successAt, failureAt int64
		var queued, failed int
		if err := rows.Scan(&id, &successAt, &failureAt, &outcome, &queued, &failed); err != nil {
			return nil, fmt.Errorf("scan notification destination health: %w", err)
		}
		health[id] = notifications.DestinationHealth{
			LastSuccessAt: fromEpoch(successAt), LastFailureAt: fromEpoch(failureAt),
			LastFailureOutcome: notifications.OutcomeCode(outcome), QueuedCount: queued,
			TerminalFailureCount: failed,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list notification destination health: %w", err)
	}
	return health, nil
}

func (s *sqlStore) DeleteNotificationDestination(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, s.ph(`DELETE FROM notification_destinations WHERE id = ?`), id)
	if err != nil {
		return fmt.Errorf("delete notification destination: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return notifications.ErrNotFound
	}
	return nil
}

func scanNotificationDestinationRecord(sc scannable) (notifications.DestinationRecord, error) {
	var destination notifications.DestinationRecord
	var means, scope, audience, topicsJSON, configurationJSON string
	var enabled int
	var createdAt, updatedAt int64
	if err := sc.Scan(
		&destination.ID, &means, &destination.Label, &scope, &destination.OwnerID, &audience,
		&topicsJSON, &enabled, &configurationJSON, &destination.CredentialsEncrypted, &createdAt, &updatedAt,
	); errors.Is(err, sql.ErrNoRows) {
		return notifications.DestinationRecord{}, notifications.ErrNotFound
	} else if err != nil {
		return notifications.DestinationRecord{}, fmt.Errorf("scan notification destination: %w", err)
	}
	destination.Means = notifications.Means(means)
	destination.Scope = notifications.DestinationScope(scope)
	destination.Audience = notifications.RecipientKind(audience)
	destination.Enabled = enabled != 0
	destination.CreatedAt = fromEpoch(createdAt)
	destination.UpdatedAt = fromEpoch(updatedAt)
	if err := json.Unmarshal([]byte(topicsJSON), &destination.Topics); err != nil {
		return notifications.DestinationRecord{}, fmt.Errorf("decode notification destination topics: %w", err)
	}
	if err := json.Unmarshal([]byte(configurationJSON), &destination.Configuration); err != nil {
		return notifications.DestinationRecord{}, fmt.Errorf("decode notification destination configuration: %w", err)
	}
	return destination, nil
}

func orEmptyStringMap(values map[string]string) map[string]string {
	if values == nil {
		return map[string]string{}
	}
	return values
}

func notificationBool(value bool) int {
	if value {
		return 1
	}
	return 0
}
