package api

import (
	"reflect"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/storagegovernor"
)

// durationWire is how a schedule.Duration is described ON THE WIRE (§7.1, §14.1).
//
// ⚠ **This lives here, in the transport layer, because it is transport knowledge.** It used to be
// a `Schema(huma.Registry) *huma.Schema` method on `schedule.Duration` itself, which made the
// PURE DOMAIN package import the HTTP framework — `internal/schedule` is 4,000-odd lines of
// I/O-free scheduling logic, and it depended on huma/v2 to describe one field's JSON encoding.
// The architecture gate bans `internal/api` from the domain but says nothing about the framework
// the API happens to use, so nothing caught it.
//
// Huma discovers wire formats by interface (`SchemaProvider`), so the method could not simply be
// moved — the domain type would stop being recognised. `Registry.RegisterTypeAlias` is the
// supported way out: it is consulted BEFORE the SchemaProvider check, so aliasing Duration to a
// type declared here routes generation through this schema instead.
//
// ⚠ Aliasing to a bare `string` would ALSO have worked and would have been wrong. The description
// below appears 5 times in api/openapi.yaml and as a JSDoc comment on 3 generated FE models, and
// it documents a real footgun ("Empty/omitted means no limit"). Dropping the domain's huma import
// at the cost of the FE's documentation would trade a structural nicety for a usability
// regression. A named type keeps both.
type durationWire string

// Schema declares the string form. `Duration.MarshalJSON` already emits a Go-duration string
// ("168h"); this makes the OpenAPI schema — and so the generated FE types — match that runtime
// reality. Without it the spec declared `type: integer, format: int64` while the JSON was
// actually `"30h0m0s"`, so a client that trusted the type parsed the string as a number and got
// NaN (caught rendering a real channel's programming rules).
func (durationWire) Schema(huma.Registry) *huma.Schema {
	return &huma.Schema{
		Type:        huma.TypeString,
		Description: `A Go-duration string, e.g. "168h" (7 days) or "24h". Empty/omitted means no limit.`,
		Examples:    []any{"168h", "24h"},
	}
}

// sizeConfidenceWire is how a storagegovernor.SizeConfidence is described on the wire (#1817).
//
// Huma names only struct schemas; a string enum is inlined into each field that uses it, and
// orval then generates one enum type per DTO (`ProposalItemSizeConfidence`,
// `PullPlanRowDTOSizeConfidence`, …). Registering a single component and returning a $ref is
// what lets every approval surface — requests now, filler pulls next — read one shape.
type sizeConfidenceWire string

const sizeConfidenceSchema = "SizeConfidence"

func (sizeConfidenceWire) Schema(r huma.Registry) *huma.Schema {
	if _, ok := r.Map()[sizeConfidenceSchema]; !ok {
		values := make([]any, 0, len(storagegovernor.SizeConfidences()))
		for _, confidence := range storagegovernor.SizeConfidences() {
			values = append(values, string(confidence))
		}
		r.Map()[sizeConfidenceSchema] = &huma.Schema{
			Type: huma.TypeString,
			Enum: values,
			Description: "How far a stated byte count can be trusted: `exact` is declared by the media " +
				"server or provider for the file itself; `estimated` is derived and may differ on disk. " +
				"An unknown size has no confidence: both size fields are absent, never 0.",
		}
	}
	return &huma.Schema{Ref: "#/components/schemas/" + sizeConfidenceSchema}
}

// registerWireAliases points the schema generator at this package's wire descriptions for domain
// types that must not know about HTTP. Call before any route is registered — an alias added after
// a type has already been generated does not retroactively change the emitted schema.
func registerWireAliases(reg huma.Registry) {
	reg.RegisterTypeAlias(reflect.TypeOf(schedule.Duration(0)), reflect.TypeOf(durationWire("")))
	reg.RegisterTypeAlias(reflect.TypeOf(storagegovernor.SizeConfidence("")), reflect.TypeOf(sizeConfidenceWire("")))
}
