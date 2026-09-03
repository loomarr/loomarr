// Package fillerstructureopenrouter adapts the bounded OpenRouter media transport to the
// provider-neutral complete-timeline assessor port.
package fillerstructureopenrouter

import (
	"context"
	"net/http"
	"time"

	"github.com/loomarr/loomarr/internal/fillerstructure"
)

const MaximumVideoBytes int64 = 64 << 20

type ReservationState string

const (
	ReservationAccepted   ReservationState = "accepted"
	ReservationHeldBudget ReservationState = "held_budget"
)

// Reservation is the durable, pre-request accounting authority. RequestSHA256 includes the exact
// model, route, prompt, schema, duration, and base64 media bytes built by the transport.
type Reservation struct {
	RequestSHA256    string
	Source           fillerstructure.Source
	SourceBytes      int64
	Assessor         fillerstructure.AssessorProfile
	RequestedNanoUSD int64
	RequestedAt      time.Time
}

// Ledger must commit Reserve before returning and atomically close that request in Settle. A
// process crash between those calls intentionally leaves a discoverable open reservation.
type Ledger interface {
	Reserve(context.Context, Reservation) (ReservationState, error)
	Settle(context.Context, fillerstructure.AssessmentRecord) error
}

type Config struct {
	Profile              fillerstructure.AssessorProfile
	APIKey               string
	BaseURL              string
	Model                string
	ResolvedModel        string
	UpstreamProvider     string
	UpstreamProviderSlug string
	ReservationNanoUSD   int64
	MaximumChargeNanoUSD int64
	MaxTokens            int
	DisableReasoning     bool
	EnableReasoning      bool
	AllowInsecureTestURL bool
	Client               *http.Client
	Ledger               Ledger
	Now                  func() time.Time
}
