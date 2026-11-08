package identitystore

import (
	"context"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/people/notifications"
)

// PurgeOrphanNotificationReferenceRecipients deletes requesters of a title or channel that no
// longer exists, or who no longer exist themselves: news about it can never reach them. A row
// recorded after `before` is left alone, because approval records the requester before
// provisioning writes the title.
func (s *Store) PurgeOrphanNotificationReferenceRecipients(ctx context.Context, before time.Time) (int, error) {
	res, err := s.h.ExecContext(ctx, s.h.Rebind(`DELETE FROM notification_reference_recipients
		WHERE created_at < ? AND (
		    NOT EXISTS (SELECT 1 FROM users u WHERE u.id = notification_reference_recipients.person_id)
		 OR (reference_kind = ? AND NOT EXISTS (SELECT 1 FROM titles t
		        WHERE t.key = notification_reference_recipients.reference_id))
		 OR (reference_kind = ? AND NOT EXISTS (SELECT 1 FROM channels c
		        WHERE c.id = notification_reference_recipients.reference_id)))`),
		epoch(before), notifications.ReferenceTitle, notifications.ReferenceChannel)
	if err != nil {
		return 0, fmt.Errorf("purge orphan notification reference recipients: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (s *Store) ListNotificationReferenceRecipients(
	ctx context.Context,
	kind notifications.ReferenceKind,
	referenceID string,
) ([]string, error) {
	rows, err := s.h.QueryContext(ctx, s.h.Rebind(`SELECT person_id
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
