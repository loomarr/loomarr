package records

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/loomarr/loomarr/internal/filler/screening"
	"github.com/loomarr/loomarr/internal/store"
)

// SpokenSafetyInferenceReservation is the caller-owned identity of one
// pre-request ledger event. The store supplies its budget disposition from the
// inference reservation committed in the same transaction.
type SpokenSafetyInferenceReservation struct {
	EventID, RunID, CandidateID, RequestSHA256 string
	Ordinal                                    int
	CreatedAt                                  time.Time
}

// SpokenSafetyInferenceSettlement supplies only closed ledger facts that are
// not already authority-owned by the V62 settlement.
type SpokenSafetyInferenceSettlement struct {
	EventID, RunID, ReservationEventID, ResponseSHA256 string
	Failure                                            screening.SpokenSafetySettlementFailure
	ChargeKnown                                        bool
	Ordinal                                            int
	CreatedAt                                          time.Time
}

const spokenSafetyRunSelect = `SELECT id, clip_hash, authority_sha256, source_sha256,
	source_bytes, duration_ms, certification_sha256, policy_sha256, proposer_sha256, implementation, created_at
	FROM filler_spoken_safety_runs`

const spokenSafetyEventSelect = `SELECT id, run_id, ordinal, kind, payload_json, created_at
	FROM filler_spoken_safety_events`

func (s *sqlStore) PutSpokenSafetyRun(ctx context.Context, run screening.SpokenSafetyLedgerRun) error {
	_, err := s.BeginSpokenSafetyRun(ctx, run)
	return err
}

func (s *sqlStore) BeginSpokenSafetyRun(ctx context.Context, run screening.SpokenSafetyLedgerRun) (bool, error) {
	if err := screening.ValidateSpokenSafetyLedgerRun(run); err != nil {
		return false, err
	}
	res, err := s.db.ExecContext(ctx, s.ph(`INSERT INTO filler_spoken_safety_runs (
		id, clip_hash, authority_sha256, source_sha256, source_bytes, duration_ms, certification_sha256,
		policy_sha256, proposer_sha256, implementation, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO NOTHING`),
		run.ID, run.ClipHash, run.AuthoritySHA256, run.SourceSHA256, run.SourceBytes, run.DurationMS,
		run.CertificationSHA256, run.PolicySHA256, run.ProposerSHA256, run.Implementation, fillerNanoEpoch(run.CreatedAt))
	if err != nil {
		return false, fmt.Errorf("insert spoken-safety run: %w", err)
	}
	inserted, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("inspect spoken-safety run insert: %w", err)
	}
	if inserted != 0 {
		return true, nil
	}
	existing, err := scanSpokenSafetyRun(s.db.QueryRowContext(ctx,
		s.ph(spokenSafetyRunSelect+` WHERE id = ?`), run.ID))
	if err != nil {
		return false, fmt.Errorf("read existing spoken-safety run: %w", err)
	}
	if existing != run {
		return false, screening.ErrSpokenSafetyLedgerConflict
	}
	return false, nil
}

func (s *sqlStore) GetSpokenSafetyRun(ctx context.Context, id string) (screening.SpokenSafetyLedgerRun, error) {
	run, found, err := s.FindSpokenSafetyRun(ctx, id)
	if err != nil {
		return screening.SpokenSafetyLedgerRun{}, err
	}
	if !found {
		return screening.SpokenSafetyLedgerRun{}, store.ErrNotFound
	}
	return run, nil
}

func (s *sqlStore) FindSpokenSafetyRun(ctx context.Context, id string) (screening.SpokenSafetyLedgerRun, bool, error) {
	run, err := scanSpokenSafetyRun(s.db.QueryRowContext(ctx,
		s.ph(spokenSafetyRunSelect+` WHERE id = ?`), id))
	if errors.Is(err, sql.ErrNoRows) {
		return screening.SpokenSafetyLedgerRun{}, false, nil
	}
	if err != nil {
		return screening.SpokenSafetyLedgerRun{}, false, fmt.Errorf("find spoken-safety run: %w", err)
	}
	return run, true, nil
}

func scanSpokenSafetyRun(row scannable) (screening.SpokenSafetyLedgerRun, error) {
	var run screening.SpokenSafetyLedgerRun
	var createdAt int64
	if err := row.Scan(&run.ID, &run.ClipHash, &run.AuthoritySHA256, &run.SourceSHA256,
		&run.SourceBytes, &run.DurationMS, &run.CertificationSHA256, &run.PolicySHA256, &run.ProposerSHA256, &run.Implementation,
		&createdAt); err != nil {
		return screening.SpokenSafetyLedgerRun{}, err
	}
	run.CreatedAt = fromFillerNanoEpoch(createdAt)
	return run, nil
}

func (s *sqlStore) AppendSpokenSafetyEvent(ctx context.Context, event screening.SpokenSafetyLedgerEvent) error {
	if event.Kind == screening.LedgerInferenceReserved || event.Kind == screening.LedgerInferenceSettled {
		return screening.ErrSpokenSafetyLedgerInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin spoken-safety event: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.appendSpokenSafetyEvent(ctx, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit spoken-safety event: %w", err)
	}
	return nil
}

func (s *sqlStore) ReserveSpokenSafetyInference(
	ctx context.Context,
	command SpokenSafetyInferenceReservation,
	evaluation InferenceEvaluation,
	budget InferenceBudget,
) (InferenceEvaluation, screening.SpokenSafetyLedgerEvent, error) {
	if command.EventID == "" || command.RunID == "" || command.Ordinal < 0 || command.CreatedAt.IsZero() ||
		command.RequestSHA256 == "" || evaluation.RunID != command.RunID {
		return InferenceEvaluation{}, screening.SpokenSafetyLedgerEvent{}, screening.ErrSpokenSafetyLedgerInvalid
	}
	if err := validateInferenceReservation(evaluation, budget); err != nil {
		return InferenceEvaluation{}, screening.SpokenSafetyLedgerEvent{}, err
	}
	evaluation.Modalities = slices.Clone(evaluation.Modalities)
	slices.Sort(evaluation.Modalities)
	requestedNanoUSD := evaluation.ReservedNanoUSD
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return InferenceEvaluation{}, screening.SpokenSafetyLedgerEvent{}, fmt.Errorf("begin spoken-safety inference reservation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if existing, found, err := getSpokenSafetyEvent(ctx, tx, s.ph, command.EventID); err != nil {
		return InferenceEvaluation{}, screening.SpokenSafetyLedgerEvent{}, err
	} else if found {
		stored, err := scanInferenceEvaluation(tx.QueryRowContext(ctx,
			s.ph(inferenceEvaluationSelect+` WHERE id = ?`), screening.SpokenSafetyLedgerEventInferenceID(existing)))
		if err != nil || !sameSpokenSafetyReservationCommand(existing, command, evaluation, requestedNanoUSD) {
			return InferenceEvaluation{}, screening.SpokenSafetyLedgerEvent{}, screening.ErrSpokenSafetyLedgerConflict
		}
		return stored, existing, nil
	}

	run, err := scanSpokenSafetyRun(tx.QueryRowContext(ctx,
		s.ph(spokenSafetyRunSelect+` WHERE id = ?`), command.RunID))
	if errors.Is(err, sql.ErrNoRows) {
		return InferenceEvaluation{}, screening.SpokenSafetyLedgerEvent{}, store.ErrNotFound
	}
	if err != nil {
		return InferenceEvaluation{}, screening.SpokenSafetyLedgerEvent{}, fmt.Errorf("read spoken-safety reservation run: %w", err)
	}
	if evaluation.ClipHash != run.ClipHash || !evaluation.CreatedAt.Equal(command.CreatedAt) {
		return InferenceEvaluation{}, screening.SpokenSafetyLedgerEvent{}, screening.ErrSpokenSafetyLedgerInvalid
	}
	evaluation, err = s.reserveInferenceEvaluation(ctx, tx, evaluation, budget)
	if err != nil {
		return InferenceEvaluation{}, screening.SpokenSafetyLedgerEvent{}, err
	}
	state := screening.ReservationAccepted
	if evaluation.State == InferenceHeldBudget {
		state = screening.ReservationHeldBudget
	}
	event := screening.SpokenSafetyLedgerEvent{
		ID: command.EventID, RunID: command.RunID, Ordinal: command.Ordinal,
		Kind: screening.LedgerInferenceReserved, CreatedAt: command.CreatedAt,
		Reserve: &screening.SpokenSafetyInferenceReserved{
			EvaluationID: evaluation.ID, RequestSHA256: command.RequestSHA256,
			RequestedProvider: evaluation.RequestedProvider, RequestedModel: evaluation.RequestedModel,
			UpstreamProvider: evaluation.UpstreamProvider, Role: evaluation.Role, Rung: evaluation.Rung,
			CapabilitySHA256: evaluation.Versions.CapabilitySnapshot,
			PromptSHA256:     evaluation.Versions.Prompt, SchemaSHA256: evaluation.Versions.Schema,
			CandidateID: command.CandidateID,
			Modalities:  evaluation.Modalities, DerivativeBytes: evaluation.DerivativeBytes,
			DerivativeDurationMS: evaluation.DerivativeDurationMS, DerivativePixels: evaluation.DerivativePixels,
			RequestedNanoUSD: requestedNanoUSD,
			ReservedNanoUSD:  evaluation.ReservedNanoUSD, State: state,
		},
	}
	if err := s.appendSpokenSafetyEvent(ctx, tx, event); err != nil {
		return InferenceEvaluation{}, screening.SpokenSafetyLedgerEvent{}, err
	}
	if err := tx.Commit(); err != nil {
		return InferenceEvaluation{}, screening.SpokenSafetyLedgerEvent{}, fmt.Errorf("commit spoken-safety inference reservation: %w", err)
	}
	return evaluation, event, nil
}

func sameSpokenSafetyReservationCommand(
	event screening.SpokenSafetyLedgerEvent,
	command SpokenSafetyInferenceReservation,
	evaluation InferenceEvaluation,
	requestedNanoUSD int64,
) bool {
	reservation := event.Reserve
	return reservation != nil && event.ID == command.EventID && event.RunID == command.RunID &&
		event.Ordinal == command.Ordinal && event.CreatedAt.Equal(command.CreatedAt) &&
		reservation.EvaluationID == evaluation.ID && reservation.RequestSHA256 == command.RequestSHA256 &&
		reservation.RequestedProvider == evaluation.RequestedProvider && reservation.RequestedModel == evaluation.RequestedModel &&
		reservation.UpstreamProvider == evaluation.UpstreamProvider && reservation.Role == evaluation.Role && reservation.Rung == evaluation.Rung &&
		reservation.CapabilitySHA256 == evaluation.Versions.CapabilitySnapshot &&
		reservation.PromptSHA256 == evaluation.Versions.Prompt && reservation.SchemaSHA256 == evaluation.Versions.Schema &&
		reservation.CandidateID == command.CandidateID && reservation.DerivativeBytes == evaluation.DerivativeBytes &&
		reservation.DerivativeDurationMS == evaluation.DerivativeDurationMS && reservation.DerivativePixels == evaluation.DerivativePixels &&
		slices.Equal(reservation.Modalities, evaluation.Modalities) && reservation.RequestedNanoUSD == requestedNanoUSD
}

func (s *sqlStore) SettleSpokenSafetyInference(
	ctx context.Context,
	command SpokenSafetyInferenceSettlement,
	settlement InferenceSettlement,
) (InferenceEvaluation, screening.SpokenSafetyLedgerEvent, error) {
	if command.EventID == "" || command.RunID == "" || command.ReservationEventID == "" ||
		command.Ordinal < 0 || command.CreatedAt.IsZero() || !settlement.UpdatedAt.Equal(command.CreatedAt) {
		return InferenceEvaluation{}, screening.SpokenSafetyLedgerEvent{}, screening.ErrSpokenSafetyLedgerInvalid
	}
	if command.Failure == screening.FailureNone && settlement.State != "" && settlement.State != InferenceCompleted ||
		command.Failure != screening.FailureNone && settlement.State != InferenceFailed {
		return InferenceEvaluation{}, screening.SpokenSafetyLedgerEvent{}, screening.ErrSpokenSafetyLedgerInvalid
	}
	if command.Failure == screening.FailureInterrupted && !settlement.RetainReservation ||
		command.Failure != screening.FailureInterrupted && settlement.RetainReservation {
		return InferenceEvaluation{}, screening.SpokenSafetyLedgerEvent{}, screening.ErrSpokenSafetyLedgerInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return InferenceEvaluation{}, screening.SpokenSafetyLedgerEvent{}, fmt.Errorf("begin spoken-safety inference settlement: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if existing, found, err := getSpokenSafetyEvent(ctx, tx, s.ph, command.EventID); err != nil {
		return InferenceEvaluation{}, screening.SpokenSafetyLedgerEvent{}, err
	} else if found {
		stored, err := scanInferenceEvaluation(tx.QueryRowContext(ctx,
			s.ph(inferenceEvaluationSelect+` WHERE id = ?`), screening.SpokenSafetyLedgerEventInferenceID(existing)))
		if err != nil || !sameSpokenSafetySettlementCommand(existing, command, settlement) {
			return InferenceEvaluation{}, screening.SpokenSafetyLedgerEvent{}, screening.ErrSpokenSafetyLedgerConflict
		}
		return stored, existing, nil
	}

	reservation, found, err := getSpokenSafetyEvent(ctx, tx, s.ph, command.ReservationEventID)
	if err != nil {
		return InferenceEvaluation{}, screening.SpokenSafetyLedgerEvent{}, err
	}
	if !found || reservation.RunID != command.RunID || reservation.Reserve == nil ||
		reservation.Reserve.State != screening.ReservationAccepted {
		return InferenceEvaluation{}, screening.SpokenSafetyLedgerEvent{}, screening.ErrSpokenSafetyLedgerConflict
	}
	stored, overBudget, err := s.settleInferenceEvaluation(ctx, tx, reservation.Reserve.EvaluationID, settlement)
	if err != nil {
		return InferenceEvaluation{}, screening.SpokenSafetyLedgerEvent{}, err
	}
	state := screening.SettlementCompleted
	failure := command.Failure
	if failure == screening.FailureInterrupted {
		state = screening.SettlementUnknown
	} else if stored.State != InferenceCompleted || failure != screening.FailureNone {
		state = screening.SettlementFailed
	}
	if overBudget {
		state, failure = screening.SettlementFailed, screening.FailureBudget
	}
	outcome := stored.Outcome
	if state != screening.SettlementCompleted {
		outcome = ""
	}
	event := screening.SpokenSafetyLedgerEvent{
		ID: command.EventID, RunID: command.RunID, Ordinal: command.Ordinal,
		Kind: screening.LedgerInferenceSettled, CreatedAt: command.CreatedAt,
		Settle: &screening.SpokenSafetyInferenceSettled{
			ReservationEventID: command.ReservationEventID, EvaluationID: stored.ID,
			ResponseSHA256: command.ResponseSHA256, ResolvedProvider: stored.ResolvedProvider,
			ResolvedModel: stored.ResolvedModel, UpstreamProvider: stored.UpstreamProvider,
			GenerationID: stored.GenerationID, State: state, Failure: failure, Outcome: outcome,
			ChargedAmountUSD: stored.ChargedAmount, ChargedNanoUSD: stored.ChargedNanoUSD,
			AccountedNanoUSD: stored.ReservedNanoUSD,
			ChargeKnown:      command.ChargeKnown, PromptTokens: stored.Tokens.Prompt,
			CompletionTokens: stored.Tokens.Completion,
		},
	}
	if err := s.appendSpokenSafetyEvent(ctx, tx, event); err != nil {
		return InferenceEvaluation{}, screening.SpokenSafetyLedgerEvent{}, err
	}
	if err := tx.Commit(); err != nil {
		return InferenceEvaluation{}, screening.SpokenSafetyLedgerEvent{}, fmt.Errorf("commit spoken-safety inference settlement: %w", err)
	}
	if overBudget {
		return stored, event, ErrInferenceBudgetExceeded
	}
	return stored, event, nil
}

func sameSpokenSafetySettlementCommand(
	event screening.SpokenSafetyLedgerEvent,
	command SpokenSafetyInferenceSettlement,
	settlement InferenceSettlement,
) bool {
	result := event.Settle
	failureMatches := result != nil && (result.Failure == command.Failure ||
		result.Failure == screening.FailureBudget && command.Failure == screening.FailureNone)
	if result == nil || !failureMatches || event.ID != command.EventID || event.RunID != command.RunID || event.Ordinal != command.Ordinal ||
		!event.CreatedAt.Equal(command.CreatedAt) || result.ReservationEventID != command.ReservationEventID ||
		result.ResponseSHA256 != command.ResponseSHA256 ||
		result.ChargeKnown != command.ChargeKnown {
		return false
	}
	return result.ResolvedProvider == settlement.ResolvedProvider && result.ResolvedModel == settlement.ResolvedModel &&
		result.UpstreamProvider == settlement.UpstreamProvider && result.GenerationID == settlement.GenerationID &&
		result.ChargedAmountUSD == settlement.ChargedAmount && result.ChargedNanoUSD == settlement.ChargedNanoUSD &&
		result.PromptTokens == settlement.Tokens.Prompt && result.CompletionTokens == settlement.Tokens.Completion
}
