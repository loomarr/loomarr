package api

import (
	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/settings"
)

// settingKindSchema names the one component every setting's kind reads, so orval emits a single
// SettingKind symbol for the frontend to switch on instead of re-typing the registry's kinds.
const settingKindSchema = "SettingKind"

// TransformSchema publishes SettingEntry.kind as the registry's closed set. The Go field stays a
// string; the enum comes from settings.Kinds, so a new kind reaches the spec without a second
// list to keep in step.
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
	s.Properties["kind"] = &huma.Schema{Ref: "#/components/schemas/" + settingKindSchema}
	return s
}
