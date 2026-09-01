package store

import (
	"context"
	"fmt"

	"github.com/loomarr/loomarr/internal/notifications"
)

func (s *sqlStore) ListNotificationReferenceRecipients(
	ctx context.Context,
	kind notifications.ReferenceKind,
	referenceID string,
) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT person_id
		FROM notification_reference_recipients
		WHERE reference_kind = ? AND reference_id = ?
		ORDER BY person_id`), kind, referenceID)
	if err != nil {
		return nil, fmt.Errorf("list notification reference recipients: %w", err)
	}
	defer func() { _ = rows.Close() }()
	recipients := make([]string, 0)
	for rows.Next() {
		var personID string
		if err := rows.Scan(&personID); err != nil {
			return nil, fmt.Errorf("scan notification reference recipient: %w", err)
		}
		recipients = append(recipients, personID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list notification reference recipients: %w", err)
	}
	return recipients, nil
}
