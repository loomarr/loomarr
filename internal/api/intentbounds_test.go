package api

import (
	"log/slog"
	"testing"

	"github.com/loomarr/loomarr/internal/proposals"
)

// The spec's minLength on the channel description is proposals.MinDescriptionLen, so the client
// reading the generated constant and the server's trimmed check can never disagree.
func TestIntentDescriptionMinLengthIsTheDomainMinimum(t *testing.T) {
	_, humaAPI := schemaOnlyAPI(slog.New(slog.DiscardHandler))
	schema := humaAPI.OpenAPI().Components.Schemas.Map()["Intent"]
	if schema == nil {
		t.Fatal("no Intent schema in the spec")
	}
	got := schema.Properties["description"].MinLength
	if got == nil {
		t.Fatalf("Intent.description declares no minLength, want proposal.MinDescriptionLen (%d)", proposals.MinDescriptionLen)
	}
	if *got != proposals.MinDescriptionLen {
		t.Fatalf("Intent.description minLength = %d, want proposal.MinDescriptionLen (%d)", *got, proposals.MinDescriptionLen)
	}
}
