package fillersafety

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/openroutermedia"
)

type memoryExecutionRepository struct {
	run          LedgerRun
	begun        bool
	events       []LedgerEvent
	budgetHeld   bool
	reserveErr   error
	settleErr    error
	reservations []HostedCallReservation
	settlements  []HostedCallSettlement
}

func (r *memoryExecutionRepository) PutSpokenSafetyRun(_ context.Context, run LedgerRun) error {
	_, err := r.BeginSpokenSafetyRun(context.Background(), run)
	return err
}

func (r *memoryExecutionRepository) BeginSpokenSafetyRun(_ context.Context, run LedgerRun) (bool, error) {
	if err := ValidateLedgerRun(run); err != nil {
		return false, err
	}
	if r.begun {
		if !reflect.DeepEqual(r.run, run) {
			return false, ErrLedgerConflict
		}
		return false, nil
	}
	r.run, r.begun = run, true
	return true, nil
}

func (r *memoryExecutionRepository) AppendSpokenSafetyEvent(_ context.Context, event LedgerEvent) error {
	if _, err := CanonicalLedgerEvent(event); err != nil {
		return err
	}
	r.events = append(r.events, event)
	return nil
}

func (r *memoryExecutionRepository) GetSpokenSafetyRun(context.Context, string) (LedgerRun, error) {
	if !r.begun {
		return LedgerRun{}, errors.New("missing")
	}
	return r.run, nil
}

func (r *memoryExecutionRepository) ListSpokenSafetyEvents(context.Context, string) ([]LedgerEvent, error) {
	return slices.Clone(r.events), nil
}

func (r *memoryExecutionRepository) ReserveSpokenSafetyCall(_ context.Context, command HostedCallReservation) (LedgerEvent, error) {
	r.reservations = append(r.reservations, command)
	if r.reserveErr != nil {
		return LedgerEvent{}, r.reserveErr
	}
	state, reserved := ReservationAccepted, command.RequestedNanoUSD
	if r.budgetHeld {
		state, reserved = ReservationHeldBudget, 0
	}
	event := LedgerEvent{
		ID: command.EventID, RunID: command.RunID, Ordinal: command.Ordinal,
		Kind: LedgerInferenceReserved, CreatedAt: command.CreatedAt,
		Reserve: &InferenceReserved{
			EvaluationID: command.EvaluationID, RequestSHA256: command.RequestSHA256,
			RequestedProvider: command.RequestedProvider, RequestedModel: command.RequestedModel,
			UpstreamProvider: command.UpstreamProvider, Role: command.Role, Rung: command.Rung,
			CapabilitySHA256: command.Versions.CapabilitySHA256,
			PromptSHA256:     command.Versions.PromptSHA256, SchemaSHA256: command.Versions.SchemaSHA256,
			CandidateID: command.CandidateID,
			Modalities:  slices.Clone(command.Modalities), DerivativeBytes: command.DerivativeBytes,
			DerivativeDurationMS: command.DerivativeDurationMS, DerivativePixels: command.DerivativePixels,
			RequestedNanoUSD: command.RequestedNanoUSD,
			ReservedNanoUSD:  reserved, State: state,
		},
	}
	if err := r.AppendSpokenSafetyEvent(context.Background(), event); err != nil {
		return LedgerEvent{}, err
	}
	return event, nil
}

func (r *memoryExecutionRepository) SettleSpokenSafetyCall(_ context.Context, command HostedCallSettlement) (LedgerEvent, error) {
	r.settlements = append(r.settlements, command)
	if r.settleErr != nil {
		return LedgerEvent{}, r.settleErr
	}
	reservation := r.events[len(r.events)-1]
	state := SettlementCompleted
	if command.Failure != FailureNone {
		state = SettlementFailed
	}
	event := LedgerEvent{
		ID: command.EventID, RunID: command.RunID, Ordinal: command.Ordinal,
		Kind: LedgerInferenceSettled, CreatedAt: command.CreatedAt,
		Settle: &InferenceSettled{
			ReservationEventID: command.ReservationEventID, EvaluationID: reservation.Reserve.EvaluationID,
			ResponseSHA256: command.ResponseSHA256, ResolvedProvider: command.ResolvedProvider,
			ResolvedModel: command.ResolvedModel, UpstreamProvider: command.UpstreamProvider,
			GenerationID: command.GenerationID, State: state, Failure: command.Failure,
			Outcome: command.Outcome, ChargedAmountUSD: command.ChargedAmountUSD,
			ChargedNanoUSD: command.ChargedNanoUSD, AccountedNanoUSD: command.ChargedNanoUSD,
			ChargeKnown: command.ChargeKnown, PromptTokens: command.PromptTokens,
			CompletionTokens: command.CompletionTokens,
		},
	}
	if err := r.AppendSpokenSafetyEvent(context.Background(), event); err != nil {
		return LedgerEvent{}, err
	}
	return event, nil
}

type operationAudioAdapter struct {
	state         AudioState
	err           error
	unknownCharge bool
	calls         int
}

func (a *operationAudioAdapter) identity(int64) hostedCallIdentity {
	return validHostedCallIdentityFixture()
}

func (a *operationAudioAdapter) adjudicate(_ context.Context, candidate Candidate, _ []byte, reserve func(string) error) (audioAttempt, error) {
	attempt := audioAttempt{Assessment: AudioAssessment{CandidateID: candidate.ID, State: a.state}, MatchedRuleIDs: []string{}}
	if a.state == AudioDetected {
		attempt.MatchedRuleIDs = []string{"rule-000000000000000000000001"}
	}
	if err := reserve(strings.Repeat("d", 64)); err != nil {
		return attempt, err
	}
	a.calls++
	attempt.Transport = hostedTransportFixture("audio", a.unknownCharge)
	return attempt, a.err
}

type operationVideoAdapter struct {
	state VideoState
	err   error
	calls int
}

func (v *operationVideoAdapter) identity(int64) hostedCallIdentity {
	return validHostedCallIdentityFixture()
}

func (v *operationVideoAdapter) corroborate(_ context.Context, _ *CompleteMediaPlan, reserve func(string) error) (videoAttempt, error) {
	attempt := videoAttempt{State: v.state, Flags: []videoFlag{}}
	if err := reserve(strings.Repeat("e", 64)); err != nil {
		return attempt, err
	}
	v.calls++
	attempt.Transport = hostedTransportFixture("video", false)
	return attempt, v.err
}

func hostedTransportFixture(kind string, unknown bool) openroutermedia.Result {
	result := openroutermedia.Result{
		GenerationID: "generation-" + kind, ResponseSHA256: strings.Repeat("f", 64),
		PromptTokens: 10, CompletionTokens: 2, ChargedAmountUSD: "0.00000005",
		ChargedNanoUSD: 50, ChargeKnown: true,
	}
	if unknown {
		result.ChargedAmountUSD, result.ChargedNanoUSD, result.ChargeKnown = "", 0, false
	}
	return result
}

type operationFixture struct {
	request    EvaluationRequest
	repository *memoryExecutionRepository
	proposer   *fakeAcousticProposer
	audio      *operationAudioAdapter
	video      *operationVideoAdapter
	operation  *evaluationOperation
}

func newOperationFixture(t *testing.T, intervals []proposedInterval) operationFixture {
	t.Helper()
	contents := []byte("complete operation source")
	path := filepath.Join(t.TempDir(), "opaque-source.mp4")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	authority := validSourceAuthority()
	authority.SourceID = "opaque-source-1"
	authority.SourceSHA256, authority.SourceBytes = sourceIdentity(contents)
	identity := validProposerIdentityFixture()
	repository := &memoryExecutionRepository{}
	proposer := &fakeAcousticProposer{output: proposalOutput{Identity: identity, Complete: true, Candidates: intervals}}
	audio := &operationAudioAdapter{state: AudioAbsent}
	video := &operationVideoAdapter{state: VideoNoSignal}
	nowAt := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	now := func() time.Time {
		nowAt = nowAt.Add(time.Millisecond)
		return nowAt
	}
	operation, err := newEvaluationOperation(repository, evaluator{
		proposer: proposer, proposerIdentity: identity, audioExtractor: &fakeAudioExtractor{},
		audio: audio, video: video,
	}, HostedCallBudget{PerClipNanoUSD: 1_000, PerDayNanoUSD: 1_000, PerRunNanoUSD: 1_000}, now)
	if err != nil {
		t.Fatal(err)
	}
	return operationFixture{
		request: EvaluationRequest{
			RunID: "spoken-run-1", StartedAt: nowAt.Add(-time.Second),
			Source: SourceRequest{Authority: authority, Path: path},
		},
		repository: repository, proposer: proposer, audio: audio, video: video, operation: operation,
	}
}
