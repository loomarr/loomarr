// Package openrouter adapts the bounded OpenRouter media transport to one
// complete planned-window assessment call.
package openrouter

import (
	"context"
	"net/http"
	"time"

	"github.com/loomarr/loomarr/internal/filler/structure"
	"github.com/loomarr/loomarr/internal/filler/structure/window"
	"github.com/loomarr/loomarr/internal/openroutermedia"
)

type Ledger interface {
	Reserve(context.Context, window.CallReservation) (window.CallReservationState, error)
	Settle(context.Context, window.CallRecord) error
}

type Config struct {
	RouteAuthority         openroutermedia.RouteAuthority
	Profile                structure.AssessorProfile
	MetadataSnapshotSHA256 string
	APIKey                 string
	BaseURL                string
	Model                  string
	ResolvedModel          string
	UpstreamProvider       string
	UpstreamProviderSlug   string
	ReservationNanoUSD     int64
	MaximumChargeNanoUSD   int64
	MaxTokens              int
	DisableReasoning       bool
	EnableReasoning        bool
	AllowInsecureTestURL   bool
	Client                 *http.Client
	Ledger                 Ledger
	Now                    func() time.Time
}
