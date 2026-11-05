package api

import (
	"log/slog"
	"slices"
	"testing"

	"github.com/loomarr/loomarr/internal/settings"
)

// A setting's kind is the registry's closed set on the wire, so the frontend switches on the
// generated SettingKind symbol instead of re-typing "duration" (#2147 wave 4).
func TestSettingKindIsTheRegistrysClosedSet(t *testing.T) {
	_, humaAPI := schemaOnlyAPI(slog.New(slog.DiscardHandler))
	schemas := humaAPI.OpenAPI().Components.Schemas.Map()

	named, ok := schemas["SettingKind"]
	if !ok {
		t.Fatal("components.schemas.SettingKind is missing")
	}
	var want []any
	for _, kind := range settings.Kinds() {
		want = append(want, string(kind))
	}
	if named.Type != "string" || !slices.Equal(named.Enum, want) {
		t.Errorf("SettingKind = type %q enum %v, want string %v", named.Type, named.Enum, want)
	}

	entry, ok := schemas["SettingEntry"]
	if !ok {
		t.Fatal("components.schemas.SettingEntry is missing")
	}
	if got := entry.Properties["kind"]; got == nil || got.Ref != "#/components/schemas/SettingKind" {
		t.Errorf("SettingEntry.kind = %+v, want a $ref to SettingKind", got)
	}
	if !slices.Contains(entry.Required, "kind") {
		t.Error("SettingEntry.kind is no longer required")
	}
}
