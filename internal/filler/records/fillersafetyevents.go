package records

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/loomarr/loomarr/internal/filler/screening"
	"github.com/loomarr/loomarr/internal/store"
)

func (s *sqlStore) appendSpokenSafetyEvent(ctx context.Context, tx store.Tx, event screening.SpokenSafetyLedgerEvent) error {
	payload, err := screening.CanonicalSpokenSafetyLedgerEvent(event)
	if err != nil {
		return err
	}

	existing, found, err := getSpokenSafetyEvent(ctx, tx, s.ph, event.ID)
	if err != nil {
		return err
	}
	if found {
		existingPayload, encodeErr := screening.CanonicalSpokenSafetyLedgerEvent(existing)
		if encodeErr == nil && sameSpokenSafetyEvent(existing, event, existingPayload, payload) {
			return nil
		}
		return screening.ErrSpokenSafetyLedgerConflict
	}

	prior, err := listSpokenSafetyEvents(ctx, tx, s.ph, event.RunID)
	if err != nil {
		return err
	}
	if err := screening.ValidateSpokenSafetyLedgerAppend(prior, event); err != nil {
		return screening.ErrSpokenSafetyLedgerConflict
	}

	var inferenceID any
	if id := screening.SpokenSafetyLedgerEventInferenceID(event); id != "" {
		inferenceID = id
	}
	if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO filler_spoken_safety_events (
		id, run_id, ordinal, kind, inference_id, payload_json, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?)`), event.ID, event.RunID, event.Ordinal, event.Kind,
		inferenceID, string(payload), fillerNanoEpoch(event.CreatedAt)); err != nil {
		return fmt.Errorf("insert spoken-safety event: %w", err)
	}
	return nil
}

func (s *sqlStore) ListSpokenSafetyEvents(ctx context.Context, runID string) ([]screening.SpokenSafetyLedgerEvent, error) {
	if runID == "" {
		return nil, screening.ErrSpokenSafetyLedgerInvalid
	}
	return listSpokenSafetyEvents(ctx, s.db, s.ph, runID)
}

type spokenSafetyQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func getSpokenSafetyEvent(ctx context.Context, q spokenSafetyQueryer, ph placeholder, id string) (screening.SpokenSafetyLedgerEvent, bool, error) {
	event, err := scanSpokenSafetyEvent(q.QueryRowContext(ctx,
		ph(spokenSafetyEventSelect+` WHERE id = ?`), id))
	if errors.Is(err, sql.ErrNoRows) {
		return screening.SpokenSafetyLedgerEvent{}, false, nil
	}
	if err != nil {
		return screening.SpokenSafetyLedgerEvent{}, false, fmt.Errorf("read spoken-safety event: %w", err)
	}
	return event, true, nil
}

func listSpokenSafetyEvents(ctx context.Context, q spokenSafetyQueryer, ph placeholder, runID string) ([]screening.SpokenSafetyLedgerEvent, error) {
	rows, err := q.QueryContext(ctx, ph(spokenSafetyEventSelect+` WHERE run_id = ? ORDER BY ordinal ASC`), runID)
	if err != nil {
		return nil, fmt.Errorf("list spoken-safety events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	events := []screening.SpokenSafetyLedgerEvent{}
	for rows.Next() {
		event, err := scanSpokenSafetyEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("scan spoken-safety event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate spoken-safety events: %w", err)
	}
	return events, nil
}

func scanSpokenSafetyEvent(row scannable) (screening.SpokenSafetyLedgerEvent, error) {
	var event screening.SpokenSafetyLedgerEvent
	var kind string
	var payload string
	var createdAt int64
	if err := row.Scan(&event.ID, &event.RunID, &event.Ordinal, &kind, &payload, &createdAt); err != nil {
		return screening.SpokenSafetyLedgerEvent{}, err
	}
	decoded, err := screening.DecodeSpokenSafetyLedgerEvent(screening.SpokenSafetyLedgerEventKind(kind), []byte(payload))
	if err != nil {
		return screening.SpokenSafetyLedgerEvent{}, err
	}
	decoded.ID, decoded.RunID, decoded.Ordinal = event.ID, event.RunID, event.Ordinal
	decoded.CreatedAt = fromFillerNanoEpoch(createdAt)
	if _, err := screening.CanonicalSpokenSafetyLedgerEvent(decoded); err != nil {
		return screening.SpokenSafetyLedgerEvent{}, err
	}
	return decoded, nil
}

func sameSpokenSafetyEvent(existing, proposed screening.SpokenSafetyLedgerEvent, existingPayload, proposedPayload []byte) bool {
	return existing.ID == proposed.ID && existing.RunID == proposed.RunID &&
		existing.Ordinal == proposed.Ordinal && existing.Kind == proposed.Kind &&
		fillerNanoEpoch(existing.CreatedAt) == fillerNanoEpoch(proposed.CreatedAt) &&
		slices.Equal(existingPayload, proposedPayload)
}
