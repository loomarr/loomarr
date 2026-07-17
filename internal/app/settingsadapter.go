package app

import (
	"context"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/llm"
	"github.com/mantonx/loomarr/internal/programmer"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/requester"
	"github.com/mantonx/loomarr/internal/settings"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/tmdb"
)

// settingsAdapter maps the settings subsystem (settings.Service + settings.Secrets
// + the store) to api.SettingsService, converting settings.* → api.* so the API
// package needn't import internal/settings (config-design §8; matches the
// searchAdapter/suggestAdapter house pattern). Secrets are masked by the service
// before they reach here — a value never crosses this boundary except a freshly
// regenerated DISPLAYABLE one (§4).
type settingsAdapter struct {
	svc     *settings.Service
	secrets *settings.Secrets
	store   store.Store
	// tests maps a check name → a live connection probe (media_server, tunarr, …).
	// nil-safe: an unknown/unconfigured check returns a neutral "not configured".
	tests map[string]func(ctx context.Context) (bool, string)
}

// storePersister adapts store.Store to settings.Persister (the PATCH write path).
type storePersister struct{ st store.Store }

func (p storePersister) Upsert(ctx context.Context, key, value, updatedBy string, at time.Time) error {
	return p.st.UpsertSetting(ctx, store.SettingRow{Key: key, Value: value, UpdatedBy: updatedBy, UpdatedAt: at})
}
func (p storePersister) Delete(ctx context.Context, key string) error {
	return p.st.DeleteSetting(ctx, key)
}

// storeAuditLister adapts store.ListSettings to settings.AuditLister (the audit
// metadata List attaches per key).
type storeAuditLister struct{ st store.Store }

func (a storeAuditLister) Audit(ctx context.Context) (map[string]settings.AuditRow, error) {
	rows, err := a.st.ListSettings(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]settings.AuditRow, len(rows))
	for _, r := range rows {
		out[r.Key] = settings.AuditRow{UpdatedBy: r.UpdatedBy, UpdatedAt: r.UpdatedAt}
	}
	return out, nil
}

func (a settingsAdapter) List(ctx context.Context) []api.SettingEntry {
	entries := a.svc.List(ctx, storeAuditLister{st: a.store})
	out := make([]api.SettingEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, toAPIEntry(e))
	}
	return out
}

func (a settingsAdapter) Patch(ctx context.Context, edits map[string]string, updatedBy string) []api.SettingResult {
	results, err := a.svc.Patch(ctx, storePersister{st: a.store}, edits, updatedBy)
	out := make([]api.SettingResult, 0, len(results))
	for _, r := range results {
		out = append(out, api.SettingResult{Key: r.Key, Status: string(r.Status), Problem: r.Problem})
	}
	// A persistence error surfaces as an extra synthetic invalid result so the UI
	// shows the save failed rather than silently reporting nothing.
	if err != nil {
		out = append(out, api.SettingResult{Key: "", Status: string(settings.PatchInvalid), Problem: "save failed"})
	}
	return out
}

func (a settingsAdapter) Features(ctx context.Context) map[string]bool {
	f := a.svc.Features()
	return map[string]bool{
		string(settings.FeatureAcquisition): f.Acquisition,
		string(settings.FeatureSuggestions): f.Suggestions,
		string(settings.FeatureFiller):      f.Filler,
	}
}

func (a settingsAdapter) RegenerateSecret(ctx context.Context, name string) (string, bool, error) {
	g := settings.GeneratedSecret(name)
	value, err := a.secrets.Regenerate(ctx, g)
	if err != nil {
		return "", false, err
	}
	// Refresh the redactor so the OLD value stops being scrubbed and the NEW one
	// starts (the caller wires the redactor; here we just report displayability).
	if !g.Displayable() {
		return "", false, nil
	}
	return value, true, nil
}

func (a settingsAdapter) Test(ctx context.Context, check string) (bool, string) {
	if fn, ok := a.tests[check]; ok && fn != nil {
		return fn(ctx)
	}
	return false, "not configured"
}

// connectionTests builds the named connection probes for POST /v1/setup/test
// (config-design §8). Each reads the LIVE connection via the settings snapshot, so
// a Test button reflects the value currently in the form's saved state. A probe is
// a shallow reachability check (a cheap authenticated call), not a full sweep.
func connectionTests(set resolved) map[string]func(ctx context.Context) (bool, string) {
	return map[string]func(ctx context.Context) (bool, string){
		"media_server": func(ctx context.Context) (bool, string) {
			flavor, err := library.ParseFlavor(set.str("library.flavor"))
			if err != nil {
				return false, "set a media server flavor (emby | jellyfin)"
			}
			if set.str("library.url") == "" {
				return false, "set the media server URL"
			}
			lib := library.NewDynamic(flavor, set.libraryConn(), "loomarr-test")
			if _, err := lib.ListUsers(ctx); err != nil {
				return false, "could not reach the media server: " + err.Error()
			}
			return true, ""
		},
		"tunarr": func(ctx context.Context) (bool, string) {
			if set.str("tunarr.url") == "" {
				return false, "set the Tunarr URL"
			}
			prog := programmer.NewDynamic(set.tunarrConn(), set.str("tunarr.transcode_config_id"))
			// A GET on a non-existent channel id is a cheap reachability probe: a
			// transport error means unreachable; a clean (not-found) answer means up.
			if _, _, err := prog.GetChannel(ctx, "loomarr-probe"); err != nil {
				return false, "could not reach Tunarr: " + err.Error()
			}
			return true, ""
		},
		// requester (§6): Seerr is the implemented requester; the probe validates the
		// URL + API key. Direct Sonarr/Radarr is a separate requester (not yet a Test
		// target) — guide the user to Seerr when only that pair is set.
		"requester": func(ctx context.Context) (bool, string) {
			if set.str("seerr.url") == "" {
				if set.str("sonarr.url") != "" || set.str("radarr.url") != "" {
					return false, "connection testing is available for Seerr; set the Seerr URL"
				}
				return false, "set the Seerr URL"
			}
			if err := requester.NewSeerrDynamic(set.seerrConn()).Reachable(ctx); err != nil {
				return false, "could not reach Seerr: " + err.Error()
			}
			return true, ""
		},
		// llm (§8/§8.1): local Ollama is probed live (reachable + the selected model
		// pulled). Hosted providers (openrouter/custom) aren't reachability-probed
		// here — their key is validated via POST /v1/system/llm/test; the checklist
		// only confirms a provider + key are configured, and points at the picker.
		"llm": func(ctx context.Context) (bool, string) {
			provider := set.str("llm.provider")
			if provider != "" && provider != "ollama" {
				if set.str("llm.api_key") == "" {
					return false, "set the " + provider + " API key (AI settings), then Test it"
				}
				if set.str("llm.model") == "" {
					return false, "choose a model for " + provider + " (AI settings)"
				}
				return true, "" // key present; live validation is the §8.1 Test button
			}
			base := set.str("llm.url")
			if base == "" {
				return false, "set the Ollama URL, or configure a hosted provider in AI settings"
			}
			probe := llm.NewProber(base).Probe(ctx)
			if !probe.Reachable {
				return false, "could not reach the LLM host at " + base
			}
			model := set.str("llm.model")
			if model == "" {
				return false, "select a model in AI settings (the model picker, §8.1)"
			}
			for _, m := range probe.PulledModels {
				if m == model {
					return true, ""
				}
			}
			return false, "model " + model + " is not pulled yet — pull it in AI settings"
		},
		// tmdb (§7.2): validate the key with a cheap lookup of a stable known id
		// (The Matrix, tmdb 603). A rejected key surfaces as a non-2xx error.
		"tmdb": func(ctx context.Context) (bool, string) {
			key := set.str("tmdb.api_key")
			if key == "" {
				return false, "set your TMDB API key"
			}
			if _, err := tmdb.New(key).Exists(ctx, provision.Movie, 603); err != nil {
				return false, "TMDB rejected the key or was unreachable: " + err.Error()
			}
			return true, ""
		},
	}
}

// toAPIEntry converts a settings.Entry to the API view, stringifying the typed
// value for transport and flattening the setting's declaration fields.
func toAPIEntry(e settings.Entry) api.SettingEntry {
	out := api.SettingEntry{
		Key:         e.Setting.Key,
		Group:       string(e.Setting.Group),
		Kind:        string(e.Setting.Kind),
		Provenance:  string(e.Provenance),
		Caution:     e.Caution,
		Advanced:    e.Setting.Advanced,
		Secret:      e.Setting.IsSecret(),
		Enum:        e.Setting.Enum,
		RequiredFor: string(e.Setting.Required),
		Doc:         e.Setting.Doc,
		UpdatedBy:   e.UpdatedBy,
		Set:         e.Set,
		Preview:     e.Preview,
	}
	if !e.UpdatedAt.IsZero() {
		out.UpdatedAt = e.UpdatedAt.UTC().Format(time.RFC3339)
	}
	if !e.Setting.IsSecret() && e.Value != nil {
		out.Value = settings.ValueString(e.Value)
	}
	return out
}
