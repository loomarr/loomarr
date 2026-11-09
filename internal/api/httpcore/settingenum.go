package httpcore

import (
	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/platform/settings"
)

// SettingKeySchema is the component name of the public setting-key enum, shared by every operation
// whose payload names a setting, so orval emits one SettingKey symbol for the frontend.
const SettingKeySchema = "SettingKey"

// NamedEnum registers a component once and returns the $ref every use reads.
func NamedEnum(r huma.Registry, name string, build func() *huma.Schema) *huma.Schema {
	if _, ok := r.Map()[name]; !ok {
		r.Map()[name] = build()
	}
	return &huma.Schema{Ref: "#/components/schemas/" + name}
}

// StringEnum is a string schema restricted to values.
func StringEnum[T ~string](values []T, description string) *huma.Schema {
	enum := make([]any, 0, len(values))
	for _, v := range values {
		enum = append(enum, string(v))
	}
	return &huma.Schema{Type: huma.TypeString, Enum: enum, Description: description}
}

// SettingKeyWire lists every public key in registry order. x-enum-varnames gives each generated
// member the Go handle's name, so SettingKey.FillerHomeCountry and settings.FillerHomeCountry
// name the same key; orval would otherwise drop the dots and emit fillerhome_country.
func SettingKeyWire() *huma.Schema {
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
