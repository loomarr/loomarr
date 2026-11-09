package identitystore

import (
	"context"
	"fmt"

	"github.com/loomarr/loomarr/internal/people/notifications"
)

// requesters is where one reference kind's requesters live: a table whose reference column is a
// foreign key to the title or channel, so a requester goes with what they asked for, and with
// themselves (#2793).
type requesters struct{ table, column string }

func requesterTable(kind notifications.ReferenceKind) (requesters, error) {
	switch kind {
	case notifications.ReferenceTitle:
		return requesters{"title_requesters", "title_key"}, nil
	case notifications.ReferenceChannel:
		return requesters{"channel_requesters", "channel_id"}, nil
	}
	return requesters{}, fmt.Errorf("no requesters are recorded for %q", kind)
}

// ListNotificationReferenceRecipients returns who asked for the title or channel referenceID
// names, by person id.
func (s *Store) ListNotificationReferenceRecipients(
	ctx context.Context,
	kind notifications.ReferenceKind,
	referenceID string,
) ([]string, error) {
	t, err := requesterTable(kind)
	if err != nil {
		return nil, fmt.Errorf("list notification reference recipients: %w", err)
	}
	rows, err := s.h.QueryContext(ctx, s.h.Rebind(`SELECT person_id FROM `+t.table+`
		WHERE `+t.column+` = ? ORDER BY person_id`), referenceID)
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
