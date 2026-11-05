package fillerscreening

import (
	"context"
	"errors"
	"time"
)

// ErrSpokenSafetyEvaluationInvalid reports an invalid hosted-call command without carrying source identity,
// paths, or provider detail.
var ErrSpokenSafetyEvaluationInvalid = errors.New("spoken-safety evaluation: invalid")

// SpokenSafetyHostedCallBudget carries the existing V62 spend ceilings into the domain-
// owned persistence port.
type SpokenSafetyHostedCallBudget struct {
	PerClipNanoUSD int64
	PerDayNanoUSD  int64
	PerRunNanoUSD  int64
}

// SpokenSafetyHostedCallVersions binds every semantic, media, policy, and route input
// that can change one hosted inference answer.
type SpokenSafetyHostedCallVersions struct {
	EvidenceSHA256      string
	ExtractorSHA256     string
	PromptSHA256        string
	SchemaSHA256        string
	TaxonomySHA256      string
	CertificationSHA256 string
	PolicySHA256        string
	CapabilitySHA256    string
}

// SpokenSafetyHostedCallReservation is the complete closed command that the persistence
// adapter commits with a V62 budget reservation before HTTP begins.
type SpokenSafetyHostedCallReservation struct {
	EventID, RunID, EvaluationID, ClipHash, CandidateID     string
	RequestSHA256                                           string
	Role, Rung                                              string
	RequestedProvider, RequestedModel, UpstreamProvider     string
	Modalities                                              []string
	DerivativeBytes, DerivativeDurationMS, DerivativePixels int64
	RequestedNanoUSD                                        int64
	Budget                                                  SpokenSafetyHostedCallBudget
	Versions                                                SpokenSafetyHostedCallVersions
	Ordinal                                                 int
	CreatedAt                                               time.Time
}

// SpokenSafetyHostedCallSettlement contains only bounded response authority and a closed
// failure class. It never accepts raw provider errors or response bodies.
type SpokenSafetyHostedCallSettlement struct {
	EventID, RunID, ReservationEventID                string
	ResponseSHA256                                    string
	ResolvedProvider, ResolvedModel, UpstreamProvider string
	GenerationID, Outcome                             string
	Failure                                           SpokenSafetySettlementFailure
	ChargedAmountUSD                                  string
	ChargedNanoUSD                                    int64
	ChargeKnown                                       bool
	PromptTokens, CompletionTokens                    int64
	LatencyMS                                         int64
	Ordinal                                           int
	CreatedAt                                         time.Time
}

// SpokenSafetyExecutionRepository is the persistence port for one evaluation. Admission
// and operator projections deliberately cannot implement it.
type SpokenSafetyExecutionRepository interface {
	FindSpokenSafetyRun(context.Context, string) (SpokenSafetyLedgerRun, bool, error)
	BeginSpokenSafetyRun(context.Context, SpokenSafetyLedgerRun) (bool, error)
	AppendSpokenSafetyEvent(context.Context, SpokenSafetyLedgerEvent) error
	ListSpokenSafetyEvents(context.Context, string) ([]SpokenSafetyLedgerEvent, error)
	ReserveSpokenSafetyCall(context.Context, SpokenSafetyHostedCallReservation) (SpokenSafetyLedgerEvent, error)
	SettleSpokenSafetyCall(context.Context, SpokenSafetyHostedCallSettlement) (SpokenSafetyLedgerEvent, error)
}
