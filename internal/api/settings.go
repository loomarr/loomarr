package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/mantonx/loomarr/internal/schedule"
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
		Summary: "Regenerate a generated secret", Description: "Admin only. Rotates SESSION_SECRET | API_TOKEN | PLAYOUT_TOKEN with the §4 side-effects; the new value is returned only if displayable.",
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
	s.settingsApplyMu.Lock()
	defer s.settingsApplyMu.Unlock()
	out := &settingsPatchOutput{}
	out.Body.Results = s.settings.Patch(ctx, in.Body.Edits, auditActor(ctx))
	// Wiring the selected playout backend into the TV guide, and pointing Tunarr at the
	// library when selected, are pure consequences of settings rather than separate
	// operator decisions. They are idempotent and fully derived, so saving an input IS
	// the intent to wire it. Run them automatically here instead of
	// making the operator find and click a button (config-design §5). Non-fatal —
	// the save already succeeded; a wiring failure is not the save's failure, and
	// it surfaces on the relevant connection's own status check.
	s.autoWireAfterSave(ctx, savedEdits(in.Body.Edits, out.Body.Results))
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

// autoWireAfterSave runs the derived Live TV transition + Tunarr media-source wiring when a
// save touched their inputs. Both are follow-on effects: failure is logged and never changes a
// setting result that already committed. The production backend transition owns prerequisite
// validation and durable retries; the media-source connector retains its independent gates.
func (s *Server) autoWireAfterSave(ctx context.Context, edits map[string]string) {
	if len(edits) == 0 {
		return
	}

	// The durable coordinator separates desired from applied, prepares inherited channels and
	// target transport, then publishes and retires in order. URL/credential-only changes still
	// enter the same seam so steady-state repair cannot be skipped.
	liveTVTouched := touchesAny(edits, liveTVWiringKeys)
	if liveTVTouched {
		s.applyLiveTVAfterSave(ctx)
	}

	// Media source: needs both Tunarr and the media server reachable.
	if touchesAny(edits, mediaSourceWiringKeys) && s.tunarrConnect != nil && !s.unconfigured("tunarr.url", "library.url") {
		if _, enabled, err := s.tunarrConnect.Connect(ctx); err != nil {
			s.logw("auto-wire Tunarr media source failed after save", err)
		} else if enabled > 0 {
			s.logi("auto-wired Tunarr's media source after save")
		}
	}
}

func (s *Server) applyLiveTVAfterSave(ctx context.Context) {
	if s.backendTransition != nil {
		desired := schedule.PlayoutBackendTunarr
		if s.liveConfig != nil {
			desired = s.liveConfig("playout.backend")
		}
		if err := s.backendTransition.Apply(ctx, desired); err != nil {
			s.logw("playout backend transition remains pending after save", err)
		}
		return
	}
	if s.livetv != nil && s.liveTVWireable() {
		// Legacy unit/embedded seam. Production always supplies BackendTransition.
		s.autoWireLiveTV(ctx)
	}
}

func (s *Server) autoWireLiveTV(ctx context.Context) {
	wired, err := s.livetv.Wired(ctx)
	if err != nil {
		s.logw("couldn't verify the current Live TV wiring after save; keeping the existing tuner", err)
		return
	}
	if wired {
		return
	}
	if err := s.convergeInheritedChannels(ctx); err != nil {
		s.logw("Live TV wiring differs but inherited channels have not converged; keeping the existing tuner", err)
		return
	}
	if tunerAdded, listingAdded, err := s.livetv.Connect(ctx); err != nil {
		s.logw("auto-wire live TV failed after save", err)
	} else if tunerAdded || listingAdded {
		s.logi("auto-wired the selected playout backend into the TV guide after save")
	}
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

// convergeInheritedChannels is the ordering barrier for a fleet-wide backend transition.
// Explicitly pinned channels do not move with the global. Detached and paused channels are
// outside reconciliation by lifecycle contract, so they cannot block the active fleet.
func (s *Server) convergeInheritedChannels(ctx context.Context) error {
	if s.store == nil {
		return errors.New("channel store unavailable")
	}
	channels, err := s.store.ListChannels(ctx)
	if err != nil {
		return fmt.Errorf("list channels for backend transition: %w", err)
	}
	for _, ch := range channels {
		if ch.Policy.Playout != nil && strings.TrimSpace(ch.Policy.Playout.Backend) != "" {
			continue
		}
		if !ch.Status.Reconcilable() {
			continue
		}
		if s.channels == nil {
			return errors.New("channel reconciliation unavailable")
		}
		if err := s.channels.Reconcile(ctx, ch.ID); err != nil {
			return fmt.Errorf("converge inherited channel %s: %w", ch.ID, err)
		}
	}
	return nil
}

func touchesAny(edits map[string]string, keys map[string]struct{}) bool {
	for key := range edits {
		if _, ok := keys[key]; ok {
			return true
		}
	}
	return false
}

func (s *Server) liveTVWireable() bool {
	if s.liveConfig == nil {
		return true // preserve wired test/embedded callers with no settings resolver
	}
	if strings.TrimSpace(s.liveConfig("playout.backend")) == schedule.PlayoutBackendInternal {
		return !s.unconfigured("server.public_url")
	}
	return !s.unconfigured("tunarr.url")
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
	s.settingsApplyMu.Lock()
	defer s.settingsApplyMu.Unlock()
	switch res := s.settings.Clear(ctx, in.Key); res.Status {
	case "invalid":
		return nil, errNotFound("Setting not found", "That setting doesn't exist — check the key and try again.")
	case "pinned":
		return nil, errConflict("Set by environment", "This setting is pinned by an environment variable. Unset that variable to manage it here.")
	}
	s.autoWireAfterSave(ctx, map[string]string{in.Key: ""})
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
	s.settingsApplyMu.Lock()
	defer s.settingsApplyMu.Unlock()
	switch res := s.settings.SetEnvOverride(ctx, in.Key, in.Body.Enabled, auditActor(ctx)); res.Status {
	case "unknown":
		// Bootstrap keys land here too: they are read before the database opens, so a flag
		// stored in that database could not affect them, and they are not in the registry.
		return nil, errNotFound("Setting not found", "That setting doesn't exist — check the key and try again.")
	case "not_pinned":
		return nil, errConflict("Not set by environment",
			"No environment variable is setting this, so there's nothing to take over. You can edit it directly.")
	}
	s.autoWireAfterSave(ctx, map[string]string{in.Key: ""})
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
	Name string `path:"name" enum:"session_secret,api_token,playout_token" doc:"Which generated secret to reveal."`
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
	Name string `path:"name" enum:"session_secret,api_token,playout_token" doc:"Which generated secret to rotate."`
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
	if in.Name == "playout_token" {
		s.settingsApplyMu.Lock()
		defer s.settingsApplyMu.Unlock()
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
	if in.Name == "playout_token" {
		// Internal tuner/listing URLs embed this credential. The rotation is already durable;
		// publication repair is non-fatal and retryable just like a settings write.
		s.applyLiveTVAfterSave(ctx)
	}
	return out, nil
}
