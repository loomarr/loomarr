package fillerstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/loomarr/loomarr/internal/fillerscreening"
	"github.com/loomarr/loomarr/internal/store/storedb"
)

// RecoverInterruptedSpokenSafetyRuns closes attempts owned by a previous
// process. Unknown provider settlement keeps the full reservation accounted.
func (s *sqlStore) RecoverInterruptedSpokenSafetyRuns(ctx context.Context, at time.Time) (int, error) {
	if at.IsZero() {
		return 0, fillerscreening.ErrSpokenSafetyLedgerInvalid
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM filler_spoken_safety_runs r
		WHERE NOT EXISTS (SELECT 1 FROM filler_spoken_safety_events e
			WHERE e.run_id = r.id AND e.kind = 'terminal')
		ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return 0, fmt.Errorf("list interrupted spoken-safety runs: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}

	recovered := 0
	for _, id := range ids {
		didRecover, err := s.recoverSpokenSafetyRun(ctx, id, at)
		if err != nil {
			return recovered, err
		}
		if didRecover {
			recovered++
		}
	}
	return recovered, nil
}

func (s *sqlStore) recoverSpokenSafetyRun(ctx context.Context, runID string, at time.Time) (bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	query := storedb.LockRows(s.dialect, spokenSafetyRunSelect+` WHERE id = ?`)
	run, err := scanSpokenSafetyRun(tx.QueryRowContext(ctx, s.ph(query), runID))
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	events, err := listSpokenSafetyEvents(ctx, tx, s.ph, runID)
	if err != nil {
		return false, err
	}
	if len(events) > 0 && events[len(events)-1].Kind == fillerscreening.LedgerTerminal {
		return false, nil
	}

	if len(events) == 0 {
		event := fillerscreening.SpokenSafetyLedgerEvent{
			ID: recoveryLedgerID(runID, "source"), RunID: runID, Ordinal: 0,
			Kind: fillerscreening.LedgerSourcePlanned,
			Source: &fillerscreening.SpokenSafetySourcePlanned{
				Audio: fillerscreening.SpokenSafetySpan{EndMS: run.DurationMS}, Video: fillerscreening.SpokenSafetySpan{EndMS: run.DurationMS},
			},
			CreatedAt: at,
		}
		if err := s.appendSpokenSafetyEvent(ctx, tx, event); err != nil {
			return false, err
		}
		events = append(events, event)
	}
	if len(events) == 1 {
		event := fillerscreening.SpokenSafetyLedgerEvent{
			ID: recoveryLedgerID(runID, "proposal"), RunID: runID, Ordinal: 1,
			Kind: fillerscreening.LedgerProposalCompleted,
			Proposal: &fillerscreening.SpokenSafetyProposalCompleted{
				State: fillerscreening.ProposalFailed, ProposerSHA256: run.ProposerSHA256, Candidates: []fillerscreening.SpokenSafetyCandidate{},
			},
			CreatedAt: at,
		}
		if err := s.appendSpokenSafetyEvent(ctx, tx, event); err != nil {
			return false, err
		}
		events = append(events, event)
	}

	settled := map[string]struct{}{}
	for _, event := range events {
		if event.Settle != nil {
			settled[event.Settle.ReservationEventID] = struct{}{}
		}
	}
	for _, reservation := range slices.Clone(events) {
		if reservation.Reserve == nil || reservation.Reserve.State != fillerscreening.ReservationAccepted {
			continue
		}
		if _, ok := settled[reservation.ID]; ok {
			continue
		}
		stored, err := scanInferenceEvaluation(tx.QueryRowContext(ctx,
			s.ph(inferenceEvaluationSelect+` WHERE id = ?`), reservation.Reserve.EvaluationID))
		if err != nil {
			return false, fmt.Errorf("read interrupted spoken-safety inference: %w", err)
		}
		if stored.State == InferenceReserved {
			stored, _, err = s.settleInferenceEvaluation(ctx, tx, stored.ID, InferenceSettlement{
				State: InferenceFailed, FailureReason: "process interrupted before settlement",
				RetainReservation: true, UpdatedAt: at,
			})
			if err != nil {
				return false, err
			}
		}
		event := fillerscreening.SpokenSafetyLedgerEvent{
			ID: recoveryLedgerID(runID, reservation.ID), RunID: runID, Ordinal: len(events),
			Kind: fillerscreening.LedgerInferenceSettled,
			Settle: &fillerscreening.SpokenSafetyInferenceSettled{
				ReservationEventID: reservation.ID, EvaluationID: stored.ID,
				State: fillerscreening.SettlementUnknown, Failure: fillerscreening.FailureInterrupted,
				AccountedNanoUSD: stored.ReservedNanoUSD,
			},
			CreatedAt: at,
		}
		if err := s.appendSpokenSafetyEvent(ctx, tx, event); err != nil {
			return false, err
		}
		events = append(events, event)
	}

	evidence := recoveredSpokenSafetyEvidence(events)
	ids := make([]string, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.ID)
	}
	terminal := fillerscreening.SpokenSafetyLedgerEvent{
		ID: recoveryLedgerID(runID, "terminal"), RunID: runID, Ordinal: len(events),
		Kind: fillerscreening.LedgerTerminal,
		Terminal: &fillerscreening.SpokenSafetyTerminalResult{
			Evidence: evidence, Result: fillerscreening.ReduceSpokenSafety(evidence), EventIDs: ids,
		},
		CreatedAt: at,
	}
	if err := s.appendSpokenSafetyEvent(ctx, tx, terminal); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func recoveredSpokenSafetyEvidence(events []fillerscreening.SpokenSafetyLedgerEvent) fillerscreening.SpokenSafetyEvidence {
	failed := fillerscreening.SpokenSafetyEvidence{
		ProposalState: fillerscreening.ProposalFailed, Candidates: []fillerscreening.SpokenSafetyCandidate{},
		Audio: []fillerscreening.SpokenSafetyAudioAssessment{}, Video: fillerscreening.VideoNotRun,
	}
	var proposal *fillerscreening.SpokenSafetyProposalCompleted
	for _, event := range events {
		if event.Proposal != nil {
			proposal = event.Proposal
			break
		}
	}
	if proposal == nil || proposal.State != fillerscreening.ProposalComplete {
		return failed
	}
	evidence := fillerscreening.SpokenSafetyEvidence{
		ProposalState: fillerscreening.ProposalComplete, Candidates: slices.Clone(proposal.Candidates),
		Audio: make([]fillerscreening.SpokenSafetyAudioAssessment, 0, len(proposal.Candidates)), Video: fillerscreening.VideoNotRun,
	}
	for _, candidate := range proposal.Candidates {
		evidence.Audio = append(evidence.Audio, fillerscreening.SpokenSafetyAudioAssessment{
			CandidateID: candidate.ID, State: recoveredAudioState(events, candidate.ID),
		})
	}
	allAbsent := true
	for _, assessment := range evidence.Audio {
		if assessment.State != fillerscreening.AudioAbsent {
			allAbsent = false
			break
		}
	}
	if allAbsent {
		evidence.Video = recoveredVideoState(events)
	}
	return evidence
}

func recoveredAudioState(events []fillerscreening.SpokenSafetyLedgerEvent, candidateID string) fillerscreening.SpokenSafetyAudioState {
	states := []fillerscreening.SpokenSafetyAudioState{}
	for _, event := range events {
		if event.Reserve == nil || event.Reserve.CandidateID != candidateID {
			continue
		}
		state := fillerscreening.AudioFailed
		for _, later := range events {
			if later.Settle != nil && later.Settle.ReservationEventID == event.ID && later.Settle.State == fillerscreening.SettlementCompleted {
				state = fillerscreening.SpokenSafetyAudioState(later.Settle.Outcome)
			}
		}
		states = append(states, state)
	}
	if len(states) == 1 {
		return states[0]
	}
	return fillerscreening.AudioFailed
}

func recoveredVideoState(events []fillerscreening.SpokenSafetyLedgerEvent) fillerscreening.SpokenSafetyVideoState {
	states := []fillerscreening.SpokenSafetyVideoState{}
	for _, event := range events {
		if event.Reserve == nil || event.Reserve.CandidateID != "" {
			continue
		}
		state := fillerscreening.VideoFailed
		for _, later := range events {
			if later.Settle != nil && later.Settle.ReservationEventID == event.ID && later.Settle.State == fillerscreening.SettlementCompleted {
				state = fillerscreening.SpokenSafetyVideoState(later.Settle.Outcome)
			}
		}
		states = append(states, state)
	}
	if len(states) == 1 {
		return states[0]
	}
	return fillerscreening.VideoFailed
}

func recoveryLedgerID(runID, part string) string {
	digest := sha256.Sum256([]byte("spoken-safety-recovery\x00" + runID + "\x00" + part))
	return fmt.Sprintf("recovery-%x", digest[:12])
}
