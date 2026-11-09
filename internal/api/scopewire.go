package api

import (
	"github.com/loomarr/loomarr/internal/channels/schedule"
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
