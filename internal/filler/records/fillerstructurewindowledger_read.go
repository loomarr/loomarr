package records

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/loomarr/loomarr/internal/filler/structure/window"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

const structureWindowCallLedgerSelect = `SELECT request_sha256, evaluation_id, source_sha256,
	media_set_sha256, window_ordinal, assessor_id, state, reservation_json, record_sha256,
	record_json, requested_at, assessed_at, attempt FROM filler_structure_window_call_ledger`

type structureWindowCallLedgerRow struct {
	Entry        window.CallLedgerEntry
	EvaluationID string
	// Attempt numbers the reservations of one request digest from 1. A call that produced no
	// answer is reserved again as the next attempt, and the latest attempt governs replay.
	Attempt int
}

func (s *sqlStore) GetStructureWindowCallLedgerEntry(ctx context.Context, requestSHA256 string) (window.CallLedgerEntry, error) {
	if requestSHA256 == "" {
		return window.CallLedgerEntry{}, errors.New("get structure window call ledger entry: request identity is required")
	}
	row, err := s.getStructureWindowCallLedgerRow(ctx, s.db, requestSHA256, false)
	if err != nil {
		return window.CallLedgerEntry{}, err
	}
	return row.Entry, nil
}

func (s *sqlStore) ListOpenStructureWindowCallLedgerEntries(ctx context.Context, limit int) ([]window.CallLedgerEntry, error) {
	if limit < 1 || limit > 1000 {
		return nil, errors.New("list open structure window call ledger entries: limit must be between 1 and 1000")
	}
	rows, err := s.db.QueryContext(ctx, s.ph(structureWindowCallLedgerSelect+`
		WHERE state <> ? ORDER BY requested_at ASC, request_sha256 ASC, attempt ASC LIMIT ?`),
		string(window.CallLedgerSettled), limit)
	if err != nil {
		return nil, fmt.Errorf("list open structure window call ledger entries: %w", err)
	}
	defer func() { _ = rows.Close() }()
	entries := make([]window.CallLedgerEntry, 0)
	for rows.Next() {
		row, scanErr := scanStructureWindowCallLedgerRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		entries = append(entries, row.Entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list open structure window call ledger entries: %w", err)
	}
	return entries, nil
}

func (s *sqlStore) getStructureWindowCallLedgerRow(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, requestSHA256 string, lock bool) (structureWindowCallLedgerRow, error) {
	query := structureWindowCallLedgerSelect + ` WHERE request_sha256 = ? ORDER BY attempt DESC LIMIT 1`
	if lock {
		query = storedb.LockRows(s.dialect, query)
	}
	row, err := scanStructureWindowCallLedgerRow(queryer.QueryRowContext(ctx, s.ph(query), requestSHA256))
	if errors.Is(err, sql.ErrNoRows) {
		return structureWindowCallLedgerRow{}, store.ErrNotFound
	}
	if err != nil {
		return structureWindowCallLedgerRow{}, fmt.Errorf("read structure window call ledger entry: %w", err)
	}
	return row, nil
}

func scanStructureWindowCallLedgerRow(scanner scannable) (structureWindowCallLedgerRow, error) {
	var requestSHA, evaluationID, sourceSHA, mediaSetSHA, assessorID, state, reservationRaw, recordSHA, recordRaw string
	var ordinal, attempt int
	var requestedAt, assessedAt int64
	if err := scanner.Scan(&requestSHA, &evaluationID, &sourceSHA, &mediaSetSHA, &ordinal, &assessorID,
		&state, &reservationRaw, &recordSHA, &recordRaw, &requestedAt, &assessedAt, &attempt); err != nil {
		return structureWindowCallLedgerRow{}, err
	}
	var reservation window.CallReservation
	if err := decodeStructureLedgerJSON([]byte(reservationRaw), &reservation); err != nil {
		return structureWindowCallLedgerRow{}, fmt.Errorf("decode structure window call reservation: %w", err)
	}
	entry := window.CallLedgerEntry{Reservation: reservation, State: window.CallLedgerState(state)}
	if recordRaw != "" {
		var record window.CallRecord
		if err := decodeStructureLedgerJSON([]byte(recordRaw), &record); err != nil {
			return structureWindowCallLedgerRow{}, fmt.Errorf("decode structure window call settlement: %w", err)
		}
		entry.Record = &record
	}
	if reservation.RequestSHA256 != requestSHA || reservation.MediaSet.Plan.Source.SHA256 != sourceSHA ||
		reservation.MediaSet.SHA256 != mediaSetSHA || reservation.WindowOrdinal != ordinal ||
		reservation.Assessor.ID != assessorID || epoch(reservation.RequestedAt) != requestedAt ||
		(entry.Record == nil && (recordSHA != "" || assessedAt != 0)) ||
		(entry.Record != nil && (entry.Record.SHA256 != recordSHA || epoch(entry.Record.AssessedAt) != assessedAt)) {
		return structureWindowCallLedgerRow{}, errors.New("structure window call ledger columns drift from their documents")
	}
	if err := window.ValidateCallLedgerEntry(entry); err != nil {
		return structureWindowCallLedgerRow{}, err
	}
	return structureWindowCallLedgerRow{Entry: entry, EvaluationID: evaluationID, Attempt: attempt}, nil
}

// decodeStructureLedgerJSON strictly decodes one stored ledger document: unknown fields and
// trailing values are drift.
func decodeStructureLedgerJSON(raw []byte, output any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return fmt.Errorf("trailing JSON value")
	}
	return nil
}
