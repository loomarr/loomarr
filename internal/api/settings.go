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
		Summary: "Reveal a generated token", Description: "Admin only. Returns API_TOKEN or PLAYOUT_TOKEN for config-design §4's eye toggle. Reading never rotates.",
		Tags: []string{"settings"},
	}, RoleAdmin), s.secretReveal)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "secret-regenerate", Method: http.MethodPost, Path: "/v1/settings/secrets/{name}/regenerate",
		Summary: "Regenerate a generated token", Description: "Admin only. Rotates API_TOKEN or PLAYOUT_TOKEN with the §4 side-effects and returns the new value.",
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
	var saved map[string]string
	err := s.mutateLiveTVSettings(ctx, touchesAny(in.Body.Edits, liveTVWiringKeys), func(mutationCtx context.Context) bool {
		out.Body.Results = s.settings.Patch(mutationCtx, in.Body.Edits, auditActor(mutationCtx))
		saved = savedEdits(in.Body.Edits, out.Body.Results)
		return touchesAny(saved, liveTVWiringKeys) || settingsPersistenceFailed(out.Body.Results)
	})
	if err != nil {
		return nil, err
	}
	s.autoWireMediaSourceAfterSave(ctx, saved)
	return out, nil
}

// liveTVWiringKeys are settings that can change either the media-server connection or the
// tuner/guide URLs it should consume. A PATCH touching one may require idempotent re-wiring.
var liveTVWiringKeys = map[string]struct{}{
	"tunarr.url":        {},
	"library.url":       {},
	"library.token":     {},
	"library.flavor":    {},
	"playout.backend":   {},
	"server.public_url": {},
}

// mediaSourceWiringKeys are the original Tunarr media-source inputs. Playout URL changes do
// not affect that integration and must not cause an unrelated Tunarr library scan.
var mediaSourceWiringKeys = map[string]struct{}{
	"tunarr.url":     {},
	"library.url":    {},
	"library.token":  {},
	"library.flavor": {},
}

// autoWireMediaSourceAfterSave retains the independent Tunarr media-source consequence. Live TV
// publication is intentionally absent: mutateLiveTVSettings owns that complete distributed
// workflow, so there is no second transition algorithm in the HTTP layer.
func (s *Server) autoWireMediaSourceAfterSave(ctx context.Context, edits map[string]string) {
	// Media source: needs both Tunarr and the media server reachable.
	if touchesAny(edits, mediaSourceWiringKeys) && s.tunarrConnect != nil && !s.unconfigured("tunarr.url", "library.url") {
		if _, enabled, err := s.tunarrConnect.Connect(ctx); err != nil {
			s.logw("auto-wire Tunarr media source failed after save", err)
		} else if enabled > 0 {
			s.logi("auto-wired Tunarr's media source after save")
		}
	}
}

// mutateLiveTVSettings runs a transition-affecting setting mutation under the transition module's
// cross-replica lock. If lock acquisition fails, mutation never ran and the request must fail. Once
// mutation ran, its result is authoritative: later convergence/publication errors are logged and
// retried by maintenance rather than misreported as a failed save.
func (s *Server) mutateLiveTVSettings(
	ctx context.Context,
	affectsLiveTV bool,
	mutation func(context.Context) bool,
) error {
	if !affectsLiveTV {
		mutation(ctx)
		return nil
	}
	if s.backendTransition == nil {
		return errNotImplemented("Settings workflow unavailable",
			"The Live TV settings workflow isn't running, so this can't be changed right now.")
	}
	mutationRan := false
	err := s.backendTransition.ApplyMutation(ctx, func(lockCtx context.Context) bool {
		mutationRan = true
		return mutation(lockCtx)
	})
	if err == nil {
		return nil
	}
	if !mutationRan {
		return apiErrWithCause(http.StatusServiceUnavailable, "Couldn't save settings",
			"The settings workflow couldn't be started. Try again in a moment.", err)
	}
	s.logw("playout backend transition remains pending after save", err)
	return nil
}

// savedEdits filters PATCH's requested values to the keys the settings service actually
// committed. Invalid and environment-pinned inputs must not trigger external effects.
func savedEdits(edits map[string]string, results []SettingResult) map[string]string {
	saved := make(map[string]string, len(results))
	for _, result := range results {
		if result.Status == "saved" {
			saved[result.Key] = edits[result.Key]
		}
	}
	return saved
}

// settingsPersistenceFailed identifies the adapter's synthetic batch failure. A PATCH can have
// committed an earlier key before a later store write fails, so a transition-affecting batch must
// conservatively repair from current durable desired even when no per-key saved result survived.
func settingsPersistenceFailed(results []SettingResult) bool {
	for _, result := range results {
		if result.Key == "" && result.Status == "invalid" {
			return true
		}
	}
	return false
}

func touchesAny(edits map[string]string, keys map[string]struct{}) bool {
	for key := range edits {
		if _, ok := keys[key]; ok {
			return true
		}
	}
	return false
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
	var res SettingResult
	err := s.mutateLiveTVSettings(ctx, hasKey(liveTVWiringKeys, in.Key), func(mutationCtx context.Context) bool {
		res = s.settings.Clear(mutationCtx, in.Key)
		return res.Status == "saved"
	})
	if err != nil {
		return nil, err
	}
	switch res.Status {
	case "invalid":
		return nil, errNotFound("Setting not found", "That setting doesn't exist — check the key and try again.")
	case "pinned":
		return nil, errConflict("Set by environment", "This setting is pinned by an environment variable. Unset that variable to manage it here.")
	}
	s.autoWireMediaSourceAfterSave(ctx, map[string]string{in.Key: ""})
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
	var res SettingResult
	err := s.mutateLiveTVSettings(ctx, hasKey(liveTVWiringKeys, in.Key), func(mutationCtx context.Context) bool {
		res = s.settings.SetEnvOverride(mutationCtx, in.Key, in.Body.Enabled, auditActor(mutationCtx))
		return res.Status == "applied"
	})
	if err != nil {
		return nil, err
	}
	switch res.Status {
	case "unknown":
		// Bootstrap keys land here too: they are read before the database opens, so a flag
		// stored in that database could not affect them, and they are not in the registry.
		return nil, errNotFound("Setting not found", "That setting doesn't exist — check the key and try again.")
	case "not_pinned":
		return nil, errConflict("Not set by environment",
			"No environment variable is setting this, so there's nothing to take over. You can edit it directly.")
	}
	s.autoWireMediaSourceAfterSave(ctx, map[string]string{in.Key: ""})
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
	Name string `path:"name" enum:"api_token,playout_token" doc:"Which generated token to reveal."`
}

type secretRevealOutput struct {
	Body struct {
		Value string `json:"value" doc:"The current generated token."`
	}
}

// secretReveal is the read half of §4's "viewable on demand" — the eye toggle for the
// API_TOKEN an operator pastes into machine clients. It never rotates: revealing is
// looking at what's already in use.
func (s *Server) secretReveal(ctx context.Context, in *secretRevealInput) (*secretRevealOutput, error) {
	if s.settings == nil {
		return nil, errNotImplemented("Settings unavailable", "The settings service isn't running, so this can't be changed right now.")
	}
	value, err := s.settings.RevealSecret(ctx, in.Name)
	if err != nil {
		return nil, apiErrWithCause(http.StatusUnprocessableEntity, "Can't reveal token", "This token's current value couldn't be loaded.", err)
	}
	out := &secretRevealOutput{}
	out.Body.Value = value
	return out, nil
}

type secretRegenerateInput struct {
	Name string `path:"name" enum:"api_token,playout_token" doc:"Which generated token to rotate."`
}

type secretRegenerateOutput struct {
	Body struct {
		Value string `json:"value" doc:"The newly generated token."`
	}
}

func (s *Server) secretRegenerate(ctx context.Context, in *secretRegenerateInput) (*secretRegenerateOutput, error) {
	if s.settings == nil {
		return nil, errNotImplemented("Settings unavailable", "The settings service isn't running, so this can't be changed right now.")
	}
	var value string
	var mutationErr error
	err := s.mutateLiveTVSettings(ctx, in.Name == "playout_token", func(mutationCtx context.Context) bool {
		value, mutationErr = s.settings.RegenerateSecret(mutationCtx, in.Name)
		return mutationErr == nil
	})
	if err != nil {
		return nil, err
	}
	err = mutationErr
	if err != nil {
		return nil, apiErrWithCause(http.StatusUnprocessableEntity, "Can't regenerate secret", "This secret couldn't be regenerated. Try again in a moment.", err)
	}
	out := &secretRegenerateOutput{}
	out.Body.Value = value
	return out, nil
}

func hasKey(keys map[string]struct{}, key string) bool {
	_, ok := keys[key]
	return ok
}
