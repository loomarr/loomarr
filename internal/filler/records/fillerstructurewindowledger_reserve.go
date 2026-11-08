package records

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/loomarr/loomarr/internal/filler/structure"
	"github.com/loomarr/loomarr/internal/filler/structure/window"
	"github.com/loomarr/loomarr/internal/platform/store"
)

func (s *sqlStore) ReserveStructureWindowCall(ctx context.Context, reservation window.CallReservation, budget InferenceBudget) (window.CallReservationState, error) {
	if err := window.ValidateCallReservation(reservation); err != nil {
		return "", err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin structure window call reservation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	attempt, err := s.nextStructureWindowCallAttempt(ctx, tx, reservation.RequestSHA256)
	if err != nil {
		return "", err
	}
	evaluation := structureWindowCallEvaluation(reservation, attempt)
	if err := validateInferenceReservation(evaluation, budget); err != nil {
		return "", err
	}
	evaluation, err = s.reserveInferenceEvaluation(ctx, tx, evaluation, budget)
	if err != nil {
		return "", err
	}
	state := window.CallReservationAccepted
	ledgerState := window.CallLedgerOpen
	if evaluation.State == InferenceHeldBudget {
		state = window.CallReservationHeldBudget
		ledgerState = window.CallLedgerHeldBudget
	}
	raw, err := json.Marshal(reservation)
	if err != nil {
		return "", fmt.Errorf("encode structure window call reservation: %w", err)
	}
	result, err := tx.ExecContext(ctx, s.ph(`INSERT INTO filler_structure_window_call_ledger (
		request_sha256, attempt, evaluation_id, source_sha256, media_set_sha256, window_ordinal,
		assessor_id, state, reservation_json, record_sha256, record_json, requested_at, assessed_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '', NULL, ?, 0) ON CONFLICT DO NOTHING`),
		reservation.RequestSHA256, attempt, evaluation.ID, reservation.MediaSet.Plan.Source.SHA256,
		reservation.MediaSet.SHA256, reservation.WindowOrdinal, reservation.Assessor.ID,
		string(ledgerState), string(raw), epoch(reservation.RequestedAt))
	if err != nil {
		return "", fmt.Errorf("insert structure window call reservation: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil || inserted != 1 {
		return "", window.ErrCallLedgerConflict
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit structure window call reservation: %w", err)
	}
	return state, nil
}

// nextStructureWindowCallAttempt decides which attempt a reservation for requestSHA256 is. A first
// reservation is attempt 1. Only a call that produced no answer is reserved again:
//   - a settled transport failure stays as it is, because the lost request may have been charged
//     and its reservation keeps counting against the budget; the retry is the next attempt;
//   - a budget hold (settled or not) reserved nothing, so the retry replaces it as the same attempt.
//
// Everything else conflicts: an open reservation is a recovery hold, never permission to resend,
// and a settled answer replays instead of reserving.
func (s *sqlStore) nextStructureWindowCallAttempt(ctx context.Context, tx store.Tx, requestSHA256 string) (int, error) {
	prior, err := s.getStructureWindowCallLedgerRow(ctx, tx, requestSHA256, true)
	if errors.Is(err, store.ErrNotFound) {
		return 1, nil
	}
	if err != nil {
		return 0, fmt.Errorf("check structure window call reservation: %w", err)
	}
	switch {
	case prior.Entry.State == window.CallLedgerHeldBudget,
		prior.Entry.Record != nil && prior.Entry.Record.State == structure.AssessmentRecordHeldBudget:
		if err := s.deleteStructureWindowCallAttempt(ctx, tx, requestSHA256, prior); err != nil {
			return 0, err
		}
		return prior.Attempt, nil
	case prior.Entry.State == window.CallLedgerSettled && window.RetryableCallRecord(*prior.Entry.Record):
		return prior.Attempt + 1, nil
	default:
		return 0, window.ErrCallLedgerConflict
	}
}

// deleteStructureWindowCallAttempt removes a budget-held attempt and its evaluation, which holds
// no reservation, so the retry reserves under the same identifiers without leaking a row.
func (s *sqlStore) deleteStructureWindowCallAttempt(ctx context.Context, tx store.Tx, requestSHA256 string, row structureWindowCallLedgerRow) error {
	if _, err := tx.ExecContext(ctx, s.ph(`DELETE FROM filler_structure_window_call_ledger WHERE request_sha256 = ? AND attempt = ?`), requestSHA256, row.Attempt); err != nil {
		return fmt.Errorf("replace held structure window call reservation: %w", err)
	}
	if _, err := tx.ExecContext(ctx, s.ph(`DELETE FROM filler_inference_evaluations WHERE id = ? AND state = ? AND reserved_nano_usd = 0`), row.EvaluationID, string(InferenceHeldBudget)); err != nil {
		return fmt.Errorf("replace held structure window call evaluation: %w", err)
	}
	return nil
}

func structureWindowCallEvaluation(reservation window.CallReservation, attempt int) InferenceEvaluation {
	media := reservation.MediaSet.Windows[reservation.WindowOrdinal].Media
	id := "structure-window-" + reservation.RequestSHA256
	if attempt > 1 {
		id += "-" + strconv.Itoa(attempt)
	}
	return InferenceEvaluation{
		ID: id, ClipHash: reservation.MediaSet.Plan.Source.SHA256,
		Role: "complete_timeline_structure", Rung: "window_video",
		RequestedProvider: reservation.Assessor.Provider, RequestedModel: reservation.Assessor.Model,
		UpstreamProvider: reservation.UpstreamProvider, Modalities: []string{"text", "video"},
		DerivativeBytes: media.Bytes, DerivativeDurationMS: media.DurationMS,
		EstimatedNanoUSD: reservation.MaximumChargeNanoUSD, ReservedNanoUSD: reservation.RequestedNanoUSD,
		Versions: InferenceVersions{
			Evidence: reservation.MediaSet.SHA256, Extractor: media.ProfileSHA256,
			Prompt: reservation.PromptSHA256, Schema: reservation.SchemaSHA256,
			Taxonomy: reservation.Assessor.PromptVersion, AdmissionPolicy: "structure-window-shadow-v1",
			RolePolicy: structure.ReducerContractVersion, CapabilitySnapshot: reservation.Assessor.CapabilitySHA256,
		},
		CreatedAt: reservation.RequestedAt,
	}
}
