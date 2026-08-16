package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/filler"
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
	log     *slog.Logger
	// refreshRedactor re-snapshots every generated and app-managed secret after a
	// mutation. It is intentionally callback-only: the adapter never receives or
	// exposes the redactor's values.
	refreshRedactor func()
	readSecret      func(context.Context, settings.GeneratedSecret) (string, error)
	// tests maps a check name → a live connection probe (media_server, tunarr, …).
	// nil-safe: an unknown/unconfigured check returns a neutral "not configured".
	tests map[string]func(ctx context.Context) (bool, string)
}

// storePersister adapts store.Store to settings.Persister (the PATCH write path).
type storePersister struct{ st store.Store }

func (p storePersister) Apply(ctx context.Context, batch settings.PersistenceBatch) error {
	storeBatch := store.SettingBatch{
		Upserts:   make([]store.SettingMutation, 0, len(batch.Upserts)),
		Deletes:   append([]string(nil), batch.Deletes...),
		UpdatedBy: batch.UpdatedBy,
		UpdatedAt: batch.UpdatedAt,
	}
	for _, row := range batch.Upserts {
		storeBatch.Upserts = append(storeBatch.Upserts, store.SettingMutation{
			Key: row.Key, Value: row.Value,
		})
	}
	return p.st.ApplySettingBatch(ctx, storeBatch)
}

// SetEnvOverride satisfies settings.EnvOverrideSetter (§3.1). Kept on the same adapter as
// the value writes but routed to a DIFFERENT store method, because the two must not share
// an UPSERT: a save that also wrote the claim would re-lock a key on the operator's next
// edit (pinned by the store conformance suite).
func (p storePersister) SetEnvOverride(ctx context.Context, key string, on bool, seed, by string) error {
	return p.st.SetSettingEnvOverride(ctx, key, on, seed, by)
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
	if a.refreshRedactor != nil {
		a.refreshRedactor()
	}
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

// Clear is the explicit unset (config-design §8) — the only way to drop a secret,
// since an empty-string PATCH on one is rejected as replace-only (§9).
func (a settingsAdapter) Clear(ctx context.Context, key string) api.SettingResult {
	res, err := a.svc.Clear(ctx, storePersister{st: a.store}, key)
	if a.refreshRedactor != nil {
		a.refreshRedactor()
	}
	if err != nil {
		return api.SettingResult{Key: key, Status: string(settings.PatchInvalid), Problem: "clear failed"}
	}
	return api.SettingResult{Key: res.Key, Status: string(res.Status), Problem: res.Problem}
}

// SetEnvOverride is §3.1's unlock: claim a key for the app, or hand it back.
func (a settingsAdapter) SetEnvOverride(ctx context.Context, key string, on bool, updatedBy string) api.SettingResult {
	st, err := a.svc.SetEnvOverride(ctx, storePersister{st: a.store}, key, on, updatedBy)
	if a.refreshRedactor != nil {
		a.refreshRedactor()
	}
	if err != nil {
		return api.SettingResult{Key: key, Status: string(settings.PatchInvalid), Problem: "could not change who manages this setting"}
	}
	return api.SettingResult{Key: key, Status: string(st)}
}

func (a settingsAdapter) Features(ctx context.Context) map[string]bool {
	f := a.svc.Features()
	return map[string]bool{
		string(settings.FeatureAcquisition): f.Acquisition,
		string(settings.FeatureSuggestions): f.Suggestions,
		string(settings.FeatureFiller):      f.Filler,
		string(settings.FeatureUserSync):    f.UserSync,
		string(settings.FeatureIngest):      f.Ingest,
		// ⚠ Per-source (V38b). `ingest` alone cannot say WHICH source works, and reporting one
		// blanket verdict from two independent capabilities is what made a box with ffmpeg and no
		// yt-dlp claim it could not download at all.
		string(settings.FeatureIngestArchive): f.IngestArchive,
		string(settings.FeatureIngestYouTube): f.IngestYouTube,
	}
}

func (a settingsAdapter) RegenerateSecret(ctx context.Context, name string) (string, error) {
	g := settings.GeneratedSecret(name)
	value, err := a.secrets.Regenerate(ctx, g)
	if err != nil {
		return "", err
	}
	if a.refreshRedactor != nil {
		a.refreshRedactor()
	}
	return value, nil
}

// RevealSecret returns a generated token's current value (§4 eye toggle).
// Reading never rotates: the Live TV step must show the URL already pasted into
// the media server, not silently invalidate it.
func (a settingsAdapter) RevealSecret(ctx context.Context, name string) (string, error) {
	g := settings.GeneratedSecret(name)
	if a.readSecret != nil {
		return a.readSecret(ctx, g)
	}
	return a.secrets.Value(g), nil
}

func (a settingsAdapter) Test(ctx context.Context, check string) (bool, string) {
	fn, ok := a.tests[check]
	if !ok || fn == nil {
		return false, "not configured"
	}
	// Re-read the store before probing so a Test reflects what is actually PERSISTED,
	// never a stale snapshot (config-design §3 hot-apply refreshes on write only — a §18
	// multi-replica write, a restore, or an out-of-band edit would otherwise leave the
	// snapshot behind, and the probe reads the snapshot). The probe closures read live via
	// `resolved.svc`, so a refresh here propagates to them. A refresh error is non-fatal:
	// fall through to the probe against the current snapshot rather than failing the test.
	if err := a.svc.Refresh(ctx); err != nil {
		a.log.Warn("settings refresh before connection test failed; probing current snapshot", "check", check, "err", err)
	}
	return fn(ctx)
}

// connectionTests builds the named connection probes for POST /v1/setup/test
// (config-design §8). The media-server probe reuses the application library client,
// including its stable install device id, and snapshots its LIVE connection after
// settings refresh. A probe is a shallow reachability check (a cheap authenticated
// call), not a full sweep.
func connectionTests(
	set resolved, libraryClient *library.Client, tmdbClient *tmdb.Client,
) map[string]func(ctx context.Context) (bool, string) {
	return map[string]func(ctx context.Context) (bool, string){
		"media_server": func(ctx context.Context) (bool, string) {
			lib := libraryClient.Snapshot()
			_, err := lib.Connection().Validate()
			switch {
			case errors.Is(err, library.ErrConnectionFlavorRequired):
				return false, "set a media server flavor (emby | jellyfin)"
			case errors.Is(err, library.ErrConnectionURLRequired):
				return false, "set the media server URL"
			case errors.Is(err, library.ErrConnectionTokenRequired):
				return false, "set the media server API token"
			case err != nil:
				return false, "set a complete media server connection"
			}
			if _, err := lib.ListUsers(ctx); err != nil {
				return false, "could not reach the media server: " + err.Error()
			}
			return true, ""
		},
		"tunarr": func(ctx context.Context) (bool, string) {
			if set.str("tunarr.url") == "" {
				return false, "set the Tunarr URL"
			}
			prog := programmer.NewDynamic(set.tunarrConfig())
			// A GET on a non-existent channel id is a cheap reachability probe: a
			// transport error means unreachable; a clean (not-found) answer means up.
			if _, _, err := prog.GetChannel(ctx, "loomarr-probe"); err != nil {
				return false, "could not reach Tunarr: " + err.Error()
			}
			// Reachable is not enough: every channel create carries a transcode-config
			// uuid, and a missing/unresolvable one makes Tunarr 400 EVERY create while
			// this check reads green (FINDING 5 — a channel that sits `building` forever
			// with nothing the operator can see). Resolving it here (the configured id,
			// or the auto-selected Default) turns that silent failure into an actionable
			// red.
			if _, err := prog.TranscodeConfigID(ctx); err != nil {
				return false, "Tunarr is reachable but no transcode config is usable: " + err.Error()
			}
			return true, ""
		},
		// requester (§6): Seerr is the implemented requester; the probe validates the
		// URL + API key. Direct Sonarr/Radarr is a separate requester (not yet a Test
		// target) — guide the user to Seerr when only that pair is set.
		"requester": func(ctx context.Context) (bool, string) {
			// Branch on the selected provider (§6): probe the direct arr(s) or Seerr.
			if set.str("requester.provider") == "arr" {
				if set.str("sonarr.url") == "" && set.str("radarr.url") == "" {
					return false, "set a Sonarr and/or Radarr URL"
				}
				if err := requester.NewArr(set.arrConns()).Reachable(ctx); err != nil {
					return false, "could not reach Sonarr/Radarr: " + err.Error()
				}
				return true, ""
			}
			if set.str("seerr.url") == "" {
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
			sel := resolveSelection(set)
			provider := sel.Provider
			if provider != "" && provider != "ollama" {
				if sel.APIKey == "" {
					return false, "set the " + provider + " API key (AI settings), then Test it"
				}
				if sel.Model == "" {
					return false, "choose a model for " + provider + " (AI settings)"
				}
				return true, "" // key present; live validation is the §8.1 Test button
			}
			base := sel.URL
			if base == "" {
				return false, "set the Ollama URL, or configure a hosted provider in AI settings"
			}
			probe := llm.NewProber(base).Probe(ctx)
			if !probe.Reachable {
				return false, "could not reach the LLM host at " + base
			}
			model := sel.Model
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
			if set.str("tmdb.api_key") == "" || tmdbClient == nil {
				return false, "set your TMDB API key"
			}
			if _, err := tmdbClient.Exists(ctx, provision.Movie, 603); err != nil {
				return false, "TMDB rejected the key or was unreachable: " + err.Error()
			}
			return true, ""
		},
		// filler (§10): unlike a remote integration, the dependency is local storage. The probe
		// verifies the catalog root can be listed and written, then checks the effective drop folder
		// (including the derived <root>/_watch default). A non-empty configured path is not health.
		"filler": func(_ context.Context) (bool, string) {
			root := set.str("filler.dir")
			if root == "" {
				return false, "set the filler clip library folder"
			}
			layout, err := filler.NewLayout(root, set.str("filler.watch_dir"))
			if err != nil {
				return false, "filler storage layout is unsafe: " + err.Error()
			}
			if err := probeWritableDirectory(layout.ClipDir()); err != nil {
				return false, "clip library is not usable: " + err.Error()
			}
			if err := probeWritableDirectory(layout.WatchDir()); err != nil {
				return false, "drop folder is not usable: " + err.Error()
			}
			return true, ""
		},
	}
}

func probeWritableDirectory(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s does not exist", dir)
		}
		return fmt.Errorf("inspect %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a folder", dir)
	}
	f, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}
	_, readErr := f.Readdirnames(1)
	closeErr := f.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return fmt.Errorf("read %s: %w", dir, readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close %s: %w", dir, closeErr)
	}
	probe, err := os.CreateTemp(dir, ".loomarr-health-*")
	if err != nil {
		return fmt.Errorf("write %s: %w", dir, err)
	}
	name := probe.Name()
	if err := probe.Close(); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("write %s: %w", dir, err)
	}
	if err := os.Remove(name); err != nil {
		return fmt.Errorf("clean up test file in %s: %w", dir, err)
	}
	return nil
}

// toAPIEnumOptions carries the registry's {value, label} enum choices to the API
// so the UI shows registry-owned labels ("OpenAI") instead of re-deriving them.
func toAPIEnumOptions(opts []settings.EnumOption) []api.SettingEnumOption {
	if len(opts) == 0 {
		return nil
	}
	out := make([]api.SettingEnumOption, len(opts))
	for i, o := range opts {
		out[i] = api.SettingEnumOption{Value: o.Value, Label: o.Label}
	}
	return out
}

// toAPIEntry converts a settings.Entry to the API view, stringifying the typed
// value for transport and flattening the setting's declaration fields.
func toAPIEntry(e settings.Entry) api.SettingEntry {
	apply := "live"
	if e.Setting.Apply == settings.ApplyRestart {
		apply = "restart"
	}
	out := api.SettingEntry{
		Key:          e.Setting.Key,
		Label:        e.Setting.Label,
		Group:        string(e.Setting.Group),
		Kind:         string(e.Setting.Kind),
		Apply:        apply,
		Presentation: string(e.Setting.Presentation),
		Provenance:   string(e.Provenance),
		Caution:      e.Caution,
		Advanced:     e.Setting.Advanced,
		Secret:       e.Setting.IsSecret(),
		Enum:         e.Setting.EnumValues(),
		EnumOptions:  toAPIEnumOptions(e.Setting.Enum),
		ShowWhen:     e.Setting.ShowWhen,
		RequiredFor:  string(e.Setting.Required),
		Doc:          e.Setting.Doc,
		UpdatedBy:    e.UpdatedBy,
		Set:          e.Set,
		Preview:      e.Preview,
		EnvOverride:  e.EnvOverride,
		EnvPinnable:  e.EnvPinnable,
		EnvVar:       e.Setting.EnvVar,
	}
	if !e.UpdatedAt.IsZero() {
		out.UpdatedAt = e.UpdatedAt.UTC().Format(time.RFC3339)
	}
	if !e.Setting.IsSecret() && e.Value != nil {
		out.Value = settings.ValueString(e.Value)
	}
	return out
}

