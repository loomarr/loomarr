package api

import (
	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/settings"
)

// The components every setting entry reads, so orval emits one SettingKind and one SettingKey
// symbol for the frontend to use instead of re-typing the registry.
const (
	settingKindSchema = "SettingKind"
	settingKeySchema  = "SettingKey"
)

// TransformSchema publishes SettingEntry.kind and SettingEntry.key as the registry's closed sets.
// The Go fields stay strings; the enums come from the registry, so a new kind or key reaches the
// spec without a second list to keep in step.
func (SettingEntry) TransformSchema(r huma.Registry, s *huma.Schema) *huma.Schema {
	if _, ok := r.Map()[settingKindSchema]; !ok {
		values := make([]any, 0, len(settings.Kinds()))
		for _, kind := range settings.Kinds() {
			values = append(values, string(kind))
		}
		r.Map()[settingKindSchema] = &huma.Schema{
			Type:        huma.TypeString,
			Enum:        values,
			Description: "A setting's value type (config-design §2): it decides how the value parses and which control edits it.",
		}
	}
	if _, ok := r.Map()[settingKeySchema]; !ok {
		r.Map()[settingKeySchema] = settingKeyWire()
	}
	s.Properties["kind"] = &huma.Schema{Ref: "#/components/schemas/" + settingKindSchema}
	s.Properties["key"] = &huma.Schema{Ref: "#/components/schemas/" + settingKeySchema}
	return s
}

// settingKeyWire lists every public key in registry order. x-enum-varnames gives each generated
// member the Go handle's name, so SettingKey.FillerHomeCountry and settings.FillerHomeCountry
// name the same key; orval would otherwise drop the dots and emit fillerhome_country.
func settingKeyWire() *huma.Schema {
	var keys, names []any
	for _, s := range settings.NewRegistry().All() {
		if s.MigrationOnly {
			continue
		}
		keys = append(keys, s.Key)
		names = append(names, settings.HandleName(s.Key))
	}
	return &huma.Schema{
		Type:        huma.TypeString,
		Enum:        keys,
		Extensions:  map[string]any{"x-enum-varnames": names},
		Description: "A public setting key (config-design §2). Member names match the backend's typed handles.",
	}
}
