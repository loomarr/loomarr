package api

import (
	"log/slog"
	"slices"
	"testing"

	"github.com/loomarr/loomarr/internal/storagegovernor"
)

// Every approval surface that states a size reads ONE named enum (#1817): a request's owned
// titles here, a filler pull's clips later. An inline enum per field would generate one
// orval type per DTO, and the UI would grow a renderer per type for the same two words.
func TestSizeConfidenceIsOneNamedSchema(t *testing.T) {
	_, humaAPI := schemaOnlyAPI(slog.New(slog.DiscardHandler))
	schemas := humaAPI.OpenAPI().Components.Schemas.Map()

	named, ok := schemas["SizeConfidence"]
	if !ok {
		t.Fatal("components.schemas.SizeConfidence is missing")
	}
	var want []any
	for _, confidence := range storagegovernor.SizeConfidences() {
		want = append(want, string(confidence))
	}
	if named.Type != "string" || !slices.Equal(named.Enum, want) {
		t.Errorf("SizeConfidence = type %q enum %v, want string %v", named.Type, named.Enum, want)
	}

	item, ok := schemas["ProposalItem"]
	if !ok {
		t.Fatal("components.schemas.ProposalItem is missing")
	}
	if got := item.Properties["sizeConfidence"]; got == nil || got.Ref != "#/components/schemas/SizeConfidence" {
		t.Errorf("ProposalItem.sizeConfidence = %+v, want a $ref to SizeConfidence", got)
	}
	if got := item.Properties["sizeBytes"]; got == nil || got.Type != "integer" || got.Format != "int64" {
		t.Errorf("ProposalItem.sizeBytes = %+v, want integer int64", got)
	}
	// Absent when unavailable: neither field may be required, or the wire would need a 0.
	for _, field := range []string{"sizeBytes", "sizeConfidence"} {
		if slices.Contains(item.Required, field) {
			t.Errorf("ProposalItem.%s is required; an unavailable size must be absent", field)
		}
	}
}
