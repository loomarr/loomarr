package fillerscreening

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const (
	SpokenSafetyLedgerSchemaVersion = 1
	maxLedgerIDBytes                = 128
	maxLedgerPayload                = 1 << 20
)

var (
	ErrSpokenSafetyLedgerInvalid  = errors.New("spoken-safety ledger: invalid")
	ErrSpokenSafetyLedgerConflict = errors.New("spoken-safety ledger: immutable conflict")
)

// SpokenSafetyLedgerRepository is the append-only persistence port for one spoken-safety
// execution. Admission and operator projections deliberately do not implement it.
type SpokenSafetyLedgerRepository interface {
	PutSpokenSafetyRun(context.Context, SpokenSafetyLedgerRun) error
	AppendSpokenSafetyEvent(context.Context, SpokenSafetyLedgerEvent) error
	GetSpokenSafetyRun(context.Context, string) (SpokenSafetyLedgerRun, error)
	ListSpokenSafetyEvents(context.Context, string) ([]SpokenSafetyLedgerEvent, error)
}

// SpokenSafetyLedgerRun is the immutable, path-free identity of one complete-source attempt.
type SpokenSafetyLedgerRun struct {
	ID, ClipHash, AuthoritySHA256, SourceSHA256       string
	CertificationSHA256, PolicySHA256, ProposerSHA256 string
	Implementation                                    string
	SourceBytes, DurationMS                           int64
	CreatedAt                                         time.Time
}

type SpokenSafetyLedgerEventKind string

const (
	LedgerSourcePlanned     SpokenSafetyLedgerEventKind = "source_planned"
	LedgerProposalCompleted SpokenSafetyLedgerEventKind = "proposal_completed"
	LedgerInferenceReserved SpokenSafetyLedgerEventKind = "inference_reserved"
	LedgerInferenceSettled  SpokenSafetyLedgerEventKind = "inference_settled"
	LedgerTerminal          SpokenSafetyLedgerEventKind = "terminal"
)

// SpokenSafetyLedgerEvent carries exactly one closed payload selected by Kind. No payload
// admits source text, paths, prompts, media, or free-form provider output.
type SpokenSafetyLedgerEvent struct {
	ID, RunID string
	Ordinal   int
	Kind      SpokenSafetyLedgerEventKind
	Source    *SpokenSafetySourcePlanned
	Proposal  *SpokenSafetyProposalCompleted
	Reserve   *SpokenSafetyInferenceReserved
	Settle    *SpokenSafetyInferenceSettled
	Terminal  *SpokenSafetyTerminalResult
	CreatedAt time.Time
}

type SpokenSafetySourcePlanned struct {
	Audio SpokenSafetySpan `json:"audio"`
	Video SpokenSafetySpan `json:"video"`
}

// SpokenSafetySpan is one complete source-relative media interval.
type SpokenSafetySpan struct {
	StartMS int64 `json:"startMs"`
	EndMS   int64 `json:"endMs"`
}

type SpokenSafetyProposalCompleted struct {
	State          SpokenSafetyProposalState `json:"state"`
	ProposerSHA256 string                    `json:"proposerSha256"`
	Candidates     []SpokenSafetyCandidate   `json:"candidates"`
}

type SpokenSafetyInferenceReserved struct {
	EvaluationID         string                       `json:"evaluationId"`
	RequestSHA256        string                       `json:"requestSha256"`
	RequestedProvider    string                       `json:"requestedProvider"`
	RequestedModel       string                       `json:"requestedModel"`
	UpstreamProvider     string                       `json:"upstreamProvider"`
	Role                 string                       `json:"role,omitempty"`
	Rung                 string                       `json:"rung,omitempty"`
	CapabilitySHA256     string                       `json:"capabilitySha256"`
	PromptSHA256         string                       `json:"promptSha256"`
	SchemaSHA256         string                       `json:"schemaSha256,omitempty"`
	CandidateID          string                       `json:"candidateId,omitempty"`
	Modalities           []string                     `json:"modalities"`
	DerivativeBytes      int64                        `json:"derivativeBytes,omitempty"`
	DerivativeDurationMS int64                        `json:"derivativeDurationMs,omitempty"`
	DerivativePixels     int64                        `json:"derivativePixels,omitempty"`
	RequestedNanoUSD     int64                        `json:"requestedNanoUsd"`
	ReservedNanoUSD      int64                        `json:"reservedNanoUsd"`
	State                SpokenSafetyReservationState `json:"state"`
}

type SpokenSafetyReservationState string

const (
	ReservationAccepted   SpokenSafetyReservationState = "accepted"
	ReservationHeldBudget SpokenSafetyReservationState = "held_budget"
)

type SpokenSafetySettlementState string

const (
	SettlementCompleted SpokenSafetySettlementState = "completed"
	SettlementFailed    SpokenSafetySettlementState = "failed"
	SettlementUnknown   SpokenSafetySettlementState = "unknown"
)

type SpokenSafetySettlementFailure string

const (
	FailureNone            SpokenSafetySettlementFailure = ""
	FailureTransport       SpokenSafetySettlementFailure = "transport"
	FailureInvalidResponse SpokenSafetySettlementFailure = "invalid_response"
	FailureRouteMismatch   SpokenSafetySettlementFailure = "route_mismatch"
	FailureBudget          SpokenSafetySettlementFailure = "budget"
	FailureInterrupted     SpokenSafetySettlementFailure = "interrupted"
)

type SpokenSafetyInferenceSettled struct {
	ReservationEventID string                        `json:"reservationEventId"`
	EvaluationID       string                        `json:"evaluationId"`
	ResponseSHA256     string                        `json:"responseSha256,omitempty"`
	ResolvedProvider   string                        `json:"resolvedProvider,omitempty"`
	ResolvedModel      string                        `json:"resolvedModel,omitempty"`
	UpstreamProvider   string                        `json:"upstreamProvider,omitempty"`
	GenerationID       string                        `json:"generationId,omitempty"`
	State              SpokenSafetySettlementState   `json:"state"`
	Failure            SpokenSafetySettlementFailure `json:"failure"`
	Outcome            string                        `json:"outcome,omitempty"`
	ChargedAmountUSD   string                        `json:"chargedAmountUsd,omitempty"`
	ChargedNanoUSD     int64                         `json:"chargedNanoUsd"`
	AccountedNanoUSD   int64                         `json:"accountedNanoUsd"`
	ChargeKnown        bool                          `json:"chargeKnown"`
	PromptTokens       int64                         `json:"promptTokens"`
	CompletionTokens   int64                         `json:"completionTokens"`
}

type SpokenSafetyTerminalResult struct {
	Evidence SpokenSafetyEvidence `json:"evidence"`
	Result   SpokenSafetyResult   `json:"result"`
	EventIDs []string             `json:"eventIds"`
}

func (run SpokenSafetyLedgerRun) String() string {
	return fmt.Sprintf("spoken-safety run %s", run.ID)
}
