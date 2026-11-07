package fillerstructure

// AssessmentMaximumResponseBytes bounds one provider response and its structured output.
const AssessmentMaximumResponseBytes = 256 << 10

// AssessmentRecordState is the closed outcome of one paid assessment call.
type AssessmentRecordState string

const (
	AssessmentRecordAccepted        AssessmentRecordState = "accepted"
	AssessmentRecordFailed          AssessmentRecordState = "failed"
	AssessmentRecordUnsettled       AssessmentRecordState = "unsettled"
	AssessmentRecordHeldBudget      AssessmentRecordState = "held_budget"
	AssessmentRecordOverReservation AssessmentRecordState = "over_reservation"
)

// The closed failure causes a call records alongside a non-accepted state.
const (
	AssessmentFailureTransport       = "transport"
	AssessmentFailureProvider        = "provider"
	AssessmentFailureInvalidResponse = "invalid_response"
	AssessmentFailureRouteMismatch   = "route_mismatch"
	AssessmentFailureBudget          = "budget"
	AssessmentFailureUnsettled       = "unsettled"
	AssessmentFailureOverReservation = "over_reservation"
)

// AssessmentTokenUsage is the provider-reported token accounting of one call.
type AssessmentTokenUsage struct {
	Prompt     int64 `json:"prompt"`
	Completion int64 `json:"completion"`
	Reasoning  int64 `json:"reasoning"`
	Cached     int64 `json:"cached"`
	CacheWrite int64 `json:"cacheWrite"`
	Image      int64 `json:"image"`
	Audio      int64 `json:"audio"`
	Video      int64 `json:"video"`
}
