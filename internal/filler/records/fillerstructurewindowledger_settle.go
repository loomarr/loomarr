package records

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/loomarr/loomarr/internal/filler/structure"
	"github.com/loomarr/loomarr/internal/filler/structure/window"
)

func (s *sqlStore) SettleStructureWindowCall(ctx context.Context, record window.CallRecord) error {
	if err := window.ValidateCallRecord(record); err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin structure window call settlement: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	row, err := s.getStructureWindowCallLedgerRow(ctx, tx, record.RequestSHA256, true)
	if err != nil {
		return err
	}
	if row.Entry.State == window.CallLedgerSettled {
		if row.Entry.Record != nil && reflect.DeepEqual(*row.Entry.Record, record) {
			return nil
		}
		return window.ErrCallLedgerConflict
	}
	if err := window.ValidateCallLedgerEntry(window.CallLedgerEntry{
		Reservation: row.Entry.Reservation, State: window.CallLedgerSettled, Record: &record,
	}); err != nil {
		return window.ErrCallLedgerConflict
	}
	if row.Entry.State == window.CallLedgerHeldBudget {
		if record.State != structure.AssessmentRecordHeldBudget {
			return window.ErrCallLedgerConflict
		}
		evaluation, evaluationErr := scanInferenceEvaluation(tx.QueryRowContext(ctx,
			s.ph(inferenceEvaluationSelect+` WHERE id = ?`), row.EvaluationID))
		if evaluationErr != nil || evaluation.State != InferenceHeldBudget || evaluation.ReservedNanoUSD != 0 {
			return window.ErrCallLedgerConflict
		}
	} else {
		if record.State == structure.AssessmentRecordHeldBudget {
			return window.ErrCallLedgerConflict
		}
		settled, overBudget, settleErr := s.settleInferenceEvaluation(ctx, tx, row.EvaluationID, structureWindowCallSettlement(record, row.Entry.Reservation))
		if settleErr != nil {
			return fmt.Errorf("settle structure window inference accounting: %w", settleErr)
		}
		if overBudget != (record.State == structure.AssessmentRecordOverReservation) ||
			settled.ChargedNanoUSD != record.ChargedNanoUSD || settled.ReservedNanoUSD != record.AccountedNanoUSD {
			return window.ErrCallLedgerConflict
		}
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode structure window call settlement: %w", err)
	}
	result, err := tx.ExecContext(ctx, s.ph(`UPDATE filler_structure_window_call_ledger SET
		state = ?, record_sha256 = ?, record_json = ?, assessed_at = ?
		WHERE request_sha256 = ? AND attempt = ? AND state = ?`), string(window.CallLedgerSettled),
		record.SHA256, string(raw), epoch(record.AssessedAt), record.RequestSHA256, row.Attempt, string(row.Entry.State))
	if err != nil {
		return fmt.Errorf("write structure window call settlement: %w", err)
	}
	if updated, updateErr := result.RowsAffected(); updateErr != nil || updated != 1 {
		return window.ErrCallLedgerConflict
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit structure window call settlement: %w", err)
	}
	return nil
}

func structureWindowCallSettlement(record window.CallRecord, reservation window.CallReservation) InferenceSettlement {
	state, outcome := InferenceFailed, ""
	if record.State == structure.AssessmentRecordAccepted {
		state, outcome = InferenceCompleted, InferenceWindowComplete
	}
	currency := ""
	if record.ChargeKnown {
		currency = "USD"
	}
	return InferenceSettlement{
		ResolvedProvider: record.ResolvedProvider, ResolvedModel: record.ResolvedModel, UpstreamProvider: record.UpstreamProvider,
		Tokens: InferenceTokens{
			Prompt: record.Tokens.Prompt, Completion: record.Tokens.Completion, Reasoning: record.Tokens.Reasoning,
			Cached: record.Tokens.Cached, CacheWrite: record.Tokens.CacheWrite, Image: record.Tokens.Image,
			Audio: record.Tokens.Audio, Video: record.Tokens.Video,
		},
		ChargedAmount: record.ChargedAmountUSD, ChargedCurrency: currency, ChargedNanoUSD: record.ChargedNanoUSD,
		EstimatedNanoUSD: reservation.MaximumChargeNanoUSD, PriceSnapshot: record.MetadataSnapshotSHA256,
		Attempts: structureWindowCallAttempts(record), GenerationID: record.GenerationID, Outcome: outcome,
		FailureReason: record.Failure, State: state, RetainReservation: record.State == structure.AssessmentRecordUnsettled,
		UpdatedAt: record.AssessedAt,
	}
}

func structureWindowCallAttempts(record window.CallRecord) int {
	if record.ResponseSHA256 == "" {
		return 0
	}
	return 1
}
