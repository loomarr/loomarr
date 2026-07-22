package schedule

import "github.com/danielgtaylor/huma/v2"

// Schema tells Huma that a Duration is a STRING on the wire, not the int64 nanosecond
// count its underlying time.Duration would otherwise reflect to. Duration.MarshalJSON
// already emits a Go-duration string ("168h") — this makes the OpenAPI schema (and thus
// the generated FE types) match that runtime reality. Without it the spec declared
// `type: integer, format: int64` while the JSON was actually `"30h0m0s"`, so a client that
// trusted the type parsed the string as a number and got NaN (caught rendering a real
// channel's programming rules).
//
// This is the one place the domain touches the HTTP framework; it lives beside the
// (un)marshal methods because, like them, it is purely about Duration's wire format.
func (Duration) Schema(_ huma.Registry) *huma.Schema {
	return &huma.Schema{
		Type:        huma.TypeString,
		Description: `A Go-duration string, e.g. "168h" (7 days) or "24h". Empty/omitted means no limit.`,
		Examples:    []any{"168h", "24h"},
	}
}
