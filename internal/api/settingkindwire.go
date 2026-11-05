package api

import (
	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/settings"
)

// The components every setting entry reads, so orval emits one SettingKind, SettingPresentation
// and SettingKey symbol for the frontend to use instead of re-typing the registry.
const (
	settingKindSchema         = "SettingKind"
	settingPresentationSchema = "SettingPresentation"
	settingKeySchema          = "SettingKey"
)

// TransformSchema publishes SettingEntry.kind, .presentation and .key as the registry's closed
// sets. The Go fields stay strings; the enums come from the registry, so a new kind, editor or
// key reaches the spec without a second list to keep in step.
func (SettingEntry) TransformSchema(r huma.Registry, s *huma.Schema) *huma.Schema {
	s.Properties["kind"] = namedSettingEnum(r, settingKindSchema, func() *huma.Schema {
		return stringEnum(settings.Kinds(),
			"A setting's value type (config-design §2): it decides how the value parses and which control edits it.")
	})
	s.Properties["presentation"] = namedSettingEnum(r, settingPresentationSchema, func() *huma.Schema {
		return stringEnum(settings.Presentations(),
			"Human editor semantics beyond kind, e.g. bytes, path, or language. Absent for the kind's plain control.")
	})
	s.Properties["key"] = namedSettingEnum(r, settingKeySchema, settingKeyWire)
	return s
}

// TransformSchema publishes JobView.scheduleKey as a SettingKey: it names the setting the
// Modify Job dialog edits, whose presets are the job's schedule choices.
func (JobView) TransformSchema(r huma.Registry, s *huma.Schema) *huma.Schema {
	s.Properties["scheduleKey"] = namedSettingEnum(r, settingKeySchema, settingKeyWire)
	return s
}

// namedSettingEnum registers a component once and returns the $ref every use reads.
func namedSettingEnum(r huma.Registry, name string, build func() *huma.Schema) *huma.Schema {
	if _, ok := r.Map()[name]; !ok {
		r.Map()[name] = build()
	}
	return &huma.Schema{Ref: "#/components/schemas/" + name}
}

func stringEnum[T ~string](values []T, description string) *huma.Schema {
	enum := make([]any, 0, len(values))
	for _, v := range values {
		enum = append(enum, string(v))
	}
	return &huma.Schema{Type: huma.TypeString, Enum: enum, Description: description}
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
