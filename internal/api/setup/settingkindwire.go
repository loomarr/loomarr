package setup

import (
	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/platform/settings"
)

// The components every setting entry reads, so orval emits one SettingKind, SettingPresentation,
// SettingUnit and SettingKey symbol for the frontend to use instead of re-typing the registry.
const (
	settingKindSchema         = "SettingKind"
	settingPresentationSchema = "SettingPresentation"
	settingUnitSchema         = "SettingUnit"
)

// TransformSchema publishes SettingEntry.kind, .presentation, .unit and .key as the registry's closed
// sets. The Go fields stay strings; the enums come from the registry, so a new kind, editor or
// key reaches the spec without a second list to keep in step.
func (SettingEntry) TransformSchema(r huma.Registry, s *huma.Schema) *huma.Schema {
	s.Properties["kind"] = httpcore.NamedEnum(r, settingKindSchema, func() *huma.Schema {
		return httpcore.StringEnum(settings.Kinds(),
			"A setting's value type (config-design §2): it decides how the value parses and which control edits it.")
	})
	s.Properties["presentation"] = httpcore.NamedEnum(r, settingPresentationSchema, func() *huma.Schema {
		return httpcore.StringEnum(settings.Presentations(),
			"Human editor semantics beyond kind, e.g. bytes, path, or language. Absent for the kind's plain control.")
	})
	s.Properties["unit"] = httpcore.NamedEnum(r, settingUnitSchema, func() *huma.Schema {
		return httpcore.StringEnum(settings.Units(),
			"What a plain number counts, e.g. GiB, MiB, %, seconds or hours. A time unit is edited with a unit picker; the others are shown beside the input. Absent for a bare count.")
	})
	s.Properties["key"] = httpcore.NamedEnum(r, httpcore.SettingKeySchema, httpcore.SettingKeyWire)
	return s
}
