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

// A setting's presentation is the registry's closed set too, so the UI picks its editor from the
// generated SettingPresentation symbol. The default presentation is absent on the wire, not "".
func TestSettingPresentationIsTheRegistrysClosedSet(t *testing.T) {
	_, humaAPI := schemaOnlyAPI(slog.New(slog.DiscardHandler))
	schemas := humaAPI.OpenAPI().Components.Schemas.Map()

	named, ok := schemas["SettingPresentation"]
	if !ok {
		t.Fatal("components.schemas.SettingPresentation is missing")
	}
	var want []any
	for _, p := range settings.Presentations() {
		want = append(want, string(p))
	}
	if named.Type != "string" || !slices.Equal(named.Enum, want) {
		t.Errorf("SettingPresentation = type %q enum %v, want string %v", named.Type, named.Enum, want)
	}
	if got := schemas["SettingEntry"].Properties["presentation"]; got == nil || got.Ref != "#/components/schemas/SettingPresentation" {
		t.Errorf("SettingEntry.presentation = %+v, want a $ref to SettingPresentation", got)
	}
}

// Every public setting key is one named enum whose member names are the Go handle names, so the
// frontend reads SettingKey.FillerHomeCountry where the backend reads settings.FillerHomeCountry
// and a mistyped key is a compile error on both sides (#2147 wave 4).
func TestSettingKeyIsTheRegistrysPublicKeys(t *testing.T) {
	_, humaAPI := schemaOnlyAPI(slog.New(slog.DiscardHandler))
	schemas := humaAPI.OpenAPI().Components.Schemas.Map()

	named, ok := schemas["SettingKey"]
	if !ok {
		t.Fatal("components.schemas.SettingKey is missing")
	}
	var keys, names []any
	for _, s := range settings.NewRegistry().All() {
		if s.MigrationOnly {
			continue
		}
		keys = append(keys, s.Key)
		names = append(names, settings.HandleName(s.Key))
	}
	if named.Type != "string" || !slices.Equal(named.Enum, keys) {
		t.Errorf("SettingKey = type %q enum %v, want string %v", named.Type, named.Enum, keys)
	}
	if got, _ := named.Extensions["x-enum-varnames"].([]any); !slices.Equal(got, names) {
		t.Errorf("SettingKey x-enum-varnames = %v, want %v", got, names)
	}

	entry := schemas["SettingEntry"]
	if got := entry.Properties["key"]; got == nil || got.Ref != "#/components/schemas/SettingKey" {
		t.Errorf("SettingEntry.key = %+v, want a $ref to SettingKey", got)
	}
}
