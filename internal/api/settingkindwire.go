package api

import (
	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/platform/settings"
)

// The components every setting entry reads, so orval emits one SettingKind, SettingPresentation
// and SettingKey symbol for the frontend to use instead of re-typing the registry.
const (
	settingKindSchema         = "SettingKind"
	settingPresentationSchema = "SettingPresentation"
)

// TransformSchema publishes SettingEntry.kind, .presentation and .key as the registry's closed
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
	s.Properties["key"] = httpcore.NamedEnum(r, httpcore.SettingKeySchema, httpcore.SettingKeyWire)
	return s
}
