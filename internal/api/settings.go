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
	huma.Register(api, withRole(huma.Operation{
		OperationID: "settings-list", Method: http.MethodGet, Path: "/v1/settings",
		Summary: "List settings", Description: "Admin only. Every registry setting with resolved value (secrets masked), provenance, and audit (config-design §8).",
		Tags: []string{"settings"},
	}, RoleAdmin), s.settingsList)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "settings-patch", Method: http.MethodPatch, Path: "/v1/settings",
		Summary: "Update settings", Description: "Admin only. Per-key results (saved | invalid | pinned); hot-applies on success (config-design §8).",
		Tags: []string{"settings"},
	}, RoleAdmin), s.settingsPatch)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "settings-clear", Method: http.MethodDelete, Path: "/v1/settings/{key}",
		Summary: "Clear a setting", Description: "Admin only. Drops the stored override so the key reverts to env/default — the explicit clear, and the only way to unset a secret (config-design §8/§9).",
		Tags: []string{"settings"}, DefaultStatus: http.StatusNoContent,
	}, RoleAdmin), s.settingsClear)

	// The unlock (config-design §3.1). A separate operation rather than a field on PATCH:
	// taking a key from the deploy config is a deliberate act, not something that should
	// ride along with an ordinary save of its value.
	huma.Register(api, withRole(huma.Operation{
		OperationID: "settings-env-override", Method: http.MethodPut, Path: "/v1/settings/{key}/env-override",
		Summary: "Take a setting back from the environment", Description: "Admin only. Sets or clears the durable claim that this key is app-managed even though its environment variable is set (config-design §3.1). Unlocking seeds the stored value from the env value it takes over, so nothing changes until the operator saves; a secret never seeds. 404 unknown key (bootstrap keys are not in the registry), 409 when the environment does not pin the key.",
		Tags: []string{"settings"}, DefaultStatus: http.StatusNoContent,
	}, RoleAdmin), s.settingsEnvOverride)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "setup-test", Method: http.MethodPost, Path: "/v1/setup/test",
		Summary: "Run one connection check", Description: "Admin only. Powers the per-block Test buttons (config-design §8).",
		Tags: []string{"settings"},
	}, RoleAdmin), s.settingsTest)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "secret-reveal", Method: http.MethodGet, Path: "/v1/settings/secrets/{name}",
		Summary: "Reveal a generated secret", Description: "Admin only. Returns a displayable generated secret's current value (API_TOKEN — config-design §4's eye toggle). SESSION_SECRET reports displayable:false and withholds the value. Reading never rotates.",
		Tags: []string{"settings"},
	}, RoleAdmin), s.secretReveal)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "secret-regenerate", Method: http.MethodPost, Path: "/v1/settings/secrets/{name}/regenerate",
		Summary: "Regenerate a generated secret", Description: "Admin only. Rotates SESSION_SECRET | API_TOKEN with the §4 side-effects; the new value is returned only if displayable.",
		Tags: []string{"settings"},
	}, RoleAdmin), s.secretRegenerate)
}

type settingsListOutput struct {
	Body struct {
		Settings []SettingEntry  `json:"settings"`
		Features map[string]bool `json:"features" doc:"Computed feature availability (config-design §7)."`
	}
}

func (s *Server) settingsList(ctx context.Context, _ *struct{}) (*settingsListOutput, error) {
	if s.settings == nil {
		return nil, errNotImplemented("Settings unavailable", "The settings service isn't running, so this can't be changed right now.")
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
	if s.settings == nil {
		return nil, errNotImplemented("Settings unavailable", "The settings service isn't running, so this can't be changed right now.")
	}
	out := &settingsPatchOutput{}
	out.Body.Results = s.settings.Patch(ctx, in.Body.Edits, auditActor(ctx))
	// Wiring Tunarr into the TV guide and pointing it at the library are pure
	// consequences of the connection settings, not decisions an operator makes:
	// they're idempotent and fully derived from the saved URL/token, so saving a
	// connection IS the intent to wire it. Run them automatically here instead of
	// making the operator find and click a button (config-design §5). Non-fatal —
	// the save already succeeded; a wiring failure is not the save's failure, and
	// it surfaces on the relevant connection's own status check.
	s.autoWireAfterSave(ctx, in.Body.Edits)
	return out, nil
}

// connectionKeys are the saved settings whose values the two wiring actions derive
// from — a PATCH touching any of them may newly enable (or re-enable) wiring.
var connectionKeys = map[string]struct{}{
	"tunarr.url":     {},
	"library.url":    {},
	"library.token":  {},
	"library.flavor": {},
}

// autoWireAfterSave runs the idempotent Live TV + media-source wiring when a save
// touched a connection key and the prerequisites are now configured. Every path is
// best-effort and logged, never returned: the wiring is a follow-on effect of a
// save that already succeeded, and both connectors no-op when already wired, so
// re-running on each connection save is harmless (§6 idempotent).
func (s *Server) autoWireAfterSave(ctx context.Context, edits map[string]string) {
	touched := false
	for k := range edits {
		if _, ok := connectionKeys[k]; ok {
			touched = true
			break
		}
	}
	if !touched {
		return
	}

	// Live TV: needs only the Tunarr URL (it derives Tunarr's guide/tuner URLs).
	if s.livetv != nil && !s.unconfigured("tunarr.url") {
		if tunerAdded, listingAdded, err := s.livetv.Connect(ctx); err != nil {
			s.logw("auto-wire live TV failed after save", err)
		} else if tunerAdded || listingAdded {
			s.logi("auto-wired Tunarr into the TV guide after save")
		}
	}

	// Media source: needs both Tunarr and the media server reachable.
	if s.tunarrConnect != nil && !s.unconfigured("tunarr.url", "library.url") {
		if _, enabled, err := s.tunarrConnect.Connect(ctx); err != nil {
			s.logw("auto-wire Tunarr media source failed after save", err)
		} else if enabled > 0 {
			s.logi("auto-wired Tunarr's media source after save")
		}
	}
}

// logw / logi guard against a nil logger (unit tests wire deps directly).
func (s *Server) logw(msg string, err error) {
	if s.log != nil {
		s.log.Warn(msg, "error", err)
	}
}

func (s *Server) logi(msg string) {
	if s.log != nil {
		s.log.Info(msg)
	}
}

type settingsClearInput struct {
	Key string `path:"key" doc:"Registry key to clear, e.g. seerr.api_key."`
}

// settingsClear is the explicit clear (config-design §8). It exists because an
// empty-string PATCH on a secret is rejected (§9, replace-only) — so unsetting one
// has to be a deliberate act rather than a side effect of writing settings back.
func (s *Server) settingsClear(ctx context.Context, in *settingsClearInput) (*struct{}, error) {
	if s.settings == nil {
		return nil, errNotImplemented("Settings unavailable", "The settings service isn't running, so this can't be changed right now.")
	}
	switch res := s.settings.Clear(ctx, in.Key); res.Status {
	case "invalid":
		return nil, errNotFound("Setting not found", "That setting doesn't exist — check the key and try again.")
	case "pinned":
		return nil, errConflict("Set by environment", "This setting is pinned by an environment variable. Unset that variable to manage it here.")
	}
	return nil, nil
}

type settingsEnvOverrideInput struct {
	Key  string `path:"key" doc:"Registry key, e.g. seerr.url."`
	Body struct {
		Enabled bool `json:"enabled" doc:"true takes the key back from the environment; false hands it back (the stored value is kept either way)."`
	}
}

// settingsEnvOverride is §3.1's unlock.
//
// ⚠ Admin-only, like every settings write — but worth stating plainly, because this is the
// one control that overrides the deploy configuration. It is audited through the same
// updated_by the rest of the settings surface uses, so an operator debugging a box that is
// not behaving like its `.env` can find out from the app that someone took a key back.
func (s *Server) settingsEnvOverride(ctx context.Context, in *settingsEnvOverrideInput) (*struct{}, error) {
	if s.settings == nil {
		return nil, errNotImplemented("Settings unavailable", "The settings service isn't running, so this can't be changed right now.")
	}
	switch res := s.settings.SetEnvOverride(ctx, in.Key, in.Body.Enabled, auditActor(ctx)); res.Status {
	case "unknown":
		// Bootstrap keys land here too: they are read before the database opens, so a flag
		// stored in that database could not affect them, and they are not in the registry.
		return nil, errNotFound("Setting not found", "That setting doesn't exist — check the key and try again.")
	case "not_pinned":
		return nil, errConflict("Not set by environment",
			"No environment variable is setting this, so there's nothing to take over. You can edit it directly.")
	}
	return nil, nil
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
	if s.settings == nil {
		return nil, errNotImplemented("Settings unavailable", "The settings service isn't running, so this can't be changed right now.")
	}
	ok, hint := s.settings.Test(ctx, in.Body.Check)
	out := &settingsTestOutput{}
	out.Body.OK = ok
	out.Body.Hint = hint
	return out, nil
}

type secretRevealInput struct {
	Name string `path:"name" enum:"session_secret,api_token" doc:"Which generated secret to reveal."`
}

type secretRevealOutput struct {
	Body struct {
		// Value is present only for a displayable secret (API_TOKEN). SESSION_SECRET
		// has nothing to paste anywhere, so it is never returned (§4).
		Value       string `json:"value,omitempty"`
		Displayable bool   `json:"displayable" doc:"Whether the value is returned (config-design §4)."`
	}
}

// secretReveal is the read half of §4's "viewable on demand" — the eye toggle for the
// API_TOKEN an operator pastes into machine clients. It never rotates: revealing is
// looking at what's already in use.
func (s *Server) secretReveal(ctx context.Context, in *secretRevealInput) (*secretRevealOutput, error) {
	if s.settings == nil {
		return nil, errNotImplemented("Settings unavailable", "The settings service isn't running, so this can't be changed right now.")
	}
	value, displayable, err := s.settings.RevealSecret(ctx, in.Name)
	if err != nil {
		return nil, apiErrWithCause(http.StatusUnprocessableEntity, "Can't reveal secret", "This secret can't be shown. It may not be a viewable secret.", err)
	}
	out := &secretRevealOutput{}
	out.Body.Displayable = displayable
	if displayable {
		out.Body.Value = value
	}
	return out, nil
}

type secretRegenerateInput struct {
	Name string `path:"name" enum:"session_secret,api_token" doc:"Which generated secret to rotate."`
}

type secretRegenerateOutput struct {
	Body struct {
		// Value is the new secret, returned ONLY for displayable secrets (API_TOKEN).
		// For SESSION_SECRET it is withheld (§4) — Regenerate is the
		// only affordance and there is nothing to paste.
		Value       string `json:"value,omitempty"`
		Displayable bool   `json:"displayable" doc:"Whether the new value is returned (config-design §4)."`
	}
}

func (s *Server) secretRegenerate(ctx context.Context, in *secretRegenerateInput) (*secretRegenerateOutput, error) {
	if s.settings == nil {
		return nil, errNotImplemented("Settings unavailable", "The settings service isn't running, so this can't be changed right now.")
	}
	value, displayable, err := s.settings.RegenerateSecret(ctx, in.Name)
	if err != nil {
		return nil, apiErrWithCause(http.StatusUnprocessableEntity, "Can't regenerate secret", "This secret couldn't be regenerated. Try again in a moment.", err)
	}
	out := &secretRegenerateOutput{}
	out.Body.Displayable = displayable
	if displayable {
		out.Body.Value = value
	}
	return out, nil
}
