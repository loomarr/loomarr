package api

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/schedule"
)

// scopePolicy is how a schedule.ScopePolicy is described on the wire (#1877): the domain
// fields plus the retired `era` alias, which requests may still send and responses never
// carry. ScopePolicy has no Era field — its UnmarshalJSON folds `era` into `dates` — so
// without this the spec's `additionalProperties: false` would reject an `era` request.
// Registered as an alias in registerWireAliases (durationwire.go), like Duration.
//
// ⚠ Lower-case on purpose: Huma names the schema from the type name with its first letter
// upper-cased, so this stays `ScopePolicy` in api/openapi.yaml and the generated FE types.
type scopePolicy struct {
	schedule.ScopePolicy
	Era *schedule.Range `json:"era,omitempty" writeOnly:"true" deprecated:"true" doc:"Shortcut for dates: the same year range on movieRelease, seriesPremiere and seriesAiring. Accepted on requests only and stored as dates; never returned. Sending both era and dates is rejected with 422."`
}

// ambiguousDatesError is the 422 for a policy request carrying both `era` and `dates`
// (schedule.ErrEraAndDates): era is a shortcut for dates, so the request does not say
// which one it means.
func ambiguousDatesError(err error) huma.StatusError {
	return apiErrWithCause(http.StatusUnprocessableEntity, "Era and dates both set",
		"Send either an era or programming dates, not both. An era is a shortcut for the same year range on movie release, series premiere and episode airing.", err)
}
