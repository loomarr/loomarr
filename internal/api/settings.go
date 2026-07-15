package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// registerSettings mounts /v1/settings* (config-design §8): the typed settings
// surface the Settings UI and the wizard both drive. All admin-only; secrets are
// masked by the service before they cross this boundary (§4).
func (s *Server) registerSettings(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "settings-list", Method: http.MethodGet, Path: "/v1/settings",
		Summary: "List settings", Description: "Admin only. Every registry setting with resolved value (secrets masked), provenance, and audit (config-design §8).",
		Tags: []string{"settings"},
	}, s.settingsList)

	huma.Register(api, huma.Operation{
		OperationID: "settings-patch", Method: http.MethodPatch, Path: "/v1/settings",
		Summary: "Update settings", Description: "Admin only. Per-key results (saved | invalid | pinned); hot-applies on success (config-design §8).",
		Tags: []string{"settings"},
	}, s.settingsPatch)

	huma.Register(api, huma.Operation{
		OperationID: "setup-test", Method: http.MethodPost, Path: "/v1/setup/test",
		Summary: "Run one connection check", Description: "Admin only. Powers the per-block Test buttons (config-design §8).",
		Tags: []string{"settings"},
	}, s.settingsTest)

	huma.Register(api, huma.Operation{
		OperationID: "secret-regenerate", Method: http.MethodPost, Path: "/v1/settings/secrets/{name}/regenerate",
		Summary: "Regenerate a generated secret", Description: "Admin only. Rotates SESSION_SECRET | API_TOKEN | WEBHOOK_SECRET with the §4 side-effects; the new value is returned only if displayable.",
		Tags: []string{"settings"},
	}, s.secretRegenerate)
}

type settingsListOutput struct {
	Body struct {
		Settings []SettingEntry  `json:"settings"`
		Features map[string]bool `json:"features" doc:"Computed feature availability (config-design §7)."`
	}
}

func (s *Server) settingsList(ctx context.Context, _ *struct{}) (*settingsListOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.settings == nil {
		return nil, huma.Error501NotImplemented("settings service not configured")
	}
	out := &settingsListOutput{}
	out.Body.Settings = s.settings.List(ctx)
	out.Body.Features = s.settings.Features(ctx)
	return out, nil
}

type settingsPatchInput struct {
	Body struct {
		// Edits maps setting key → new raw value. An empty value clears an optional
		// key (reverts to env/default, config-design §9).
		Edits map[string]string `json:"edits" doc:"key → new value; empty clears an optional key."`
	}
}

type settingsPatchOutput struct {
	Body struct {
		Results []SettingResult `json:"results"`
	}
}

func (s *Server) settingsPatch(ctx context.Context, in *settingsPatchInput) (*settingsPatchOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.settings == nil {
		return nil, huma.Error501NotImplemented("settings service not configured")
	}
	out := &settingsPatchOutput{}
	out.Body.Results = s.settings.Patch(ctx, in.Body.Edits, auditActor(ctx))
	return out, nil
}

// auditActor is the admin's user id for the §3 audit trail (updated_by). The
// API_TOKEN break-glass path has no user → empty (stored NULL), same as a system
// write.
func auditActor(ctx context.Context) string {
	if u, ok := userFrom(ctx); ok {
		return u.ID
	}
	return ""
}

type settingsTestInput struct {
	Body struct {
		Check string `json:"check" doc:"Named connection check (e.g. media_server, tunarr, requester)."`
	}
}

type settingsTestOutput struct {
	Body struct {
		OK   bool   `json:"ok"`
		Hint string `json:"hint,omitempty"`
	}
}

func (s *Server) settingsTest(ctx context.Context, in *settingsTestInput) (*settingsTestOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.settings == nil {
		return nil, huma.Error501NotImplemented("settings service not configured")
	}
	ok, hint := s.settings.Test(ctx, in.Body.Check)
	out := &settingsTestOutput{}
	out.Body.OK = ok
	out.Body.Hint = hint
	return out, nil
}

type secretRegenerateInput struct {
	Name string `path:"name" enum:"session_secret,api_token,webhook_secret" doc:"Which generated secret to rotate."`
}

type secretRegenerateOutput struct {
	Body struct {
		// Value is the new secret, returned ONLY for displayable secrets (API_TOKEN,
		// WEBHOOK_SECRET). For SESSION_SECRET it is withheld (§4) — Regenerate is the
		// only affordance and there is nothing to paste.
		Value       string `json:"value,omitempty"`
		Displayable bool   `json:"displayable" doc:"Whether the new value is returned (config-design §4)."`
	}
}

func (s *Server) secretRegenerate(ctx context.Context, in *secretRegenerateInput) (*secretRegenerateOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.settings == nil {
		return nil, huma.Error501NotImplemented("settings service not configured")
	}
	value, displayable, err := s.settings.RegenerateSecret(ctx, in.Name)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("cannot regenerate secret", err)
	}
	out := &secretRegenerateOutput{}
	out.Body.Displayable = displayable
	if displayable {
		out.Body.Value = value
	}
	return out, nil
}
