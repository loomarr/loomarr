package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/metrics"
	"github.com/loomarr/loomarr/internal/notifications"
	"github.com/loomarr/loomarr/internal/programmer"
	"github.com/loomarr/loomarr/internal/requester"
	"github.com/loomarr/loomarr/internal/secretprotection"
	"github.com/loomarr/loomarr/internal/settings"
	"github.com/loomarr/loomarr/internal/store"
)

// envLookup is the process env accessor the secrets lifecycle uses to honor env-pinned generated
// tokens (config-design §4). ⚠ Not WEBHOOK_SECRET (retired-ok) — that
// never existed as a generated secret, and the arm webhook it named was deleted.
var envLookup = os.LookupEnv

// resolved wraps the settings service with typed getters so the composition root
// reads settings the way it used to read cfg.X. Reads resolve env > db > default
// through the live snapshot unless freeze pins a generation-scoped key. Connection
// getters are returned as closures so adapters read them per operation.
type resolved struct {
	svc        *settings.Service
	frozen     map[string]settings.Resolved
	protection *secretprotection.Manager
}

type generatedSecretValues interface {
	Value(settings.GeneratedSecret) string
	Current(context.Context, settings.GeneratedSecret) (string, error)
}

// currentGeneratedSecret keeps SQLite's single-process request path cache-only,
// while Postgres re-reads the shared durable value at security/publication
// boundaries so a rotation handled by another replica is visible immediately.
// onChange refreshes systemic redaction only when the durable read actually
// advances this process's cache.
func currentGeneratedSecret(
	ctx context.Context,
	dialect store.Dialect,
	secrets generatedSecretValues,
	secret settings.GeneratedSecret,
	onChange func(),
) (string, error) {
	if secrets == nil {
		return "", fmt.Errorf("read %s: generated secrets are unavailable", secret)
	}
	before := secrets.Value(secret)
	if dialect != store.DialectPostgres {
		if before == "" {
			return "", fmt.Errorf("read %s: generated secret is empty", secret)
		}
		return before, nil
	}
	current, err := secrets.Current(ctx, secret)
	if err != nil {
		return "", err
	}
	if current != before && onChange != nil {
		onChange()
	}
	return current, nil
}

// Every getter tolerates a nil service, because "no store ⇒ no settings service" is a
// SUPPORTED state, not a bug: starting without DATABASE_URL logs "running without a
// store (not ready)" and is expected to keep serving so /readyz can report why. It did
// not — the first unguarded read panicked during application composition, so a misconfigured
// container crash-looped instead of answering the probe that would have explained it.
// An unset key already resolves to a zero value; an absent service is the same answer.
func (r resolved) value(key string) any {
	if value, ok := r.frozen[key]; ok {
		return value.Value
	}
	if r.svc == nil {
		return nil
	}
	return r.svc.Resolve(key).Value
}

func (r resolved) str(key string) string {
	value := r.value(key)
	if value == nil {
		return ""
	}
	str, ok := value.(string)
	if !ok {
		panic(fmt.Sprintf("app.resolved.str: %s is %T, not string", key, value))
	}
	return str
}
func (r resolved) dur(key string) time.Duration {
	if d, ok := r.value(key).(time.Duration); ok {
		return d
	}
	return 0
}
func (r resolved) intv(key string) int {
	if n, ok := r.value(key).(int); ok {
		return n
	}
	return 0
}

// boolOn reads a bool setting whose SAFE value is true.
//
// ⚠ `boolv` answers false for a nil service or a non-bool value, which is right for a knob that
// is off by default and wrong for one that is on by default: inheriting it means a degraded boot
// silently DISABLES the feature. `filler.source.folder.enabled` is the first of those — a false
// there stops the catalog scan, so an install whose settings service could not answer would quietly
// stop finding clips. This fails OPEN instead, matching the declared default.
func (r resolved) boolOn(key string) bool {
	if r.svc == nil {
		return true
	}
	if b, ok := r.value(key).(bool); ok {
		return b
	}
	return true
}

func (r resolved) boolv(key string) bool {
	if b, ok := r.value(key).(bool); ok {
		return b
	}
	return false
}

func (r resolved) emailConfig() notifications.EmailConfig {
	if r.svc == nil {
		return notifications.EmailConfig{}
	}
	values := r.svc.ResolveMany(
		"notifications.email.enabled",
		"notifications.smtp.host",
		"notifications.smtp.port",
		"notifications.smtp.security",
		"notifications.smtp.username",
		"notifications.smtp.password",
		"notifications.email.from_address",
		"notifications.email.from_name",
	)
	stringValue := func(key string) string {
		value, _ := values[key].Value.(string)
		return value
	}
	boolValue, _ := values["notifications.email.enabled"].Value.(bool)
	port, _ := values["notifications.smtp.port"].Value.(int)
	return notifications.EmailConfig{
		Enabled: boolValue, Host: stringValue("notifications.smtp.host"), Port: port,
		Security: notifications.EmailSecurity(stringValue("notifications.smtp.security")),
		Username: stringValue("notifications.smtp.username"), Password: stringValue("notifications.smtp.password"),
		FromAddress: stringValue("notifications.email.from_address"), FromName: stringValue("notifications.email.from_name"),
	}
}

// freeze captures a coherent generation-scoped snapshot. Persistence remains
// live in the service, but every typed read through the returned facade keeps
// the applied values until the next application generation is constructed.
func (r resolved) freeze(keys ...string) (resolved, map[string]string) {
	if r.svc == nil || len(keys) == 0 {
		return r, nil
	}
	values := r.svc.ResolveMany(keys...)
	frozen := make(map[string]settings.Resolved, len(r.frozen)+len(values))
	for key, value := range r.frozen {
		frozen[key] = value
	}
	applied := make(map[string]string, len(values))
	for key, value := range values {
		frozen[key] = value
		applied[key] = settings.ValueString(value.Value)
	}
	r.frozen = frozen
	return r, applied
}

// libraryConnection resolves one coherent media-server connection. Flavor, URL,
// and token are coupled security inputs: reading them independently could send a
// credential to the wrong server or select the wrong authentication header while
// an admin saves a replacement connection.
func (r resolved) libraryConnection() library.Connection {
	if r.svc == nil {
		return library.Connection{}
	}
	return r.svc.LibraryConnection()
}

// libraryConn is the dynamic provider shared by every always-wired library
// adapter. The library module invokes it once at the start of an operation.
func (r resolved) libraryConn() library.ConnectionSource {
	return r.libraryConnection
}

func (r resolved) libraryConfigured() bool {
	_, err := r.libraryConnection().Validate()
	return err == nil
}

// seerrConn is the Seerr connection provider.
func (r resolved) seerrConn() func() (string, string) {
	return func() (string, string) { return r.str("seerr.url"), r.str("seerr.api_key") }
}

// arrConns builds the per-app connection + optional override resolvers for the direct
// Sonarr/Radarr requester (all resolved per call for hot-apply, like seerrConn).
func (r resolved) arrConns() requester.ArrConns {
	return requester.ArrConns{
		Sonarr:               func() (string, string) { return r.str("sonarr.url"), r.str("sonarr.api_key") },
		Radarr:               func() (string, string) { return r.str("radarr.url"), r.str("radarr.api_key") },
		SonarrQualityProfile: func() string { return r.str("sonarr.quality_profile") },
		SonarrRootFolder:     func() string { return r.str("sonarr.root_folder") },
		RadarrQualityProfile: func() string { return r.str("radarr.quality_profile") },
		RadarrRootFolder:     func() string { return r.str("radarr.root_folder") },
	}
}

// requesterFor builds the Requester the composition root uses, branching on
// requester.provider (§6): the direct Sonarr/Radarr adapter when "arr", else Seerr.
func (r resolved) requesterFor(recorder *metrics.Recorder) requester.Requester {
	if a := r.arrRequester(recorder); a != nil {
		return a
	}
	return requester.NewSeerrDynamicObserved(r.seerrConn(), recorder)
}

// arrRequester returns the concrete direct-arr requester when requester.provider=arr, else nil.
// The queue poller (§18.1) needs the concrete *Arr for its QueueStatus capability, which the
// Requester interface doesn't expose.
func (r resolved) arrRequester(recorder *metrics.Recorder) *requester.Arr {
	if r.str("requester.provider") != "arr" {
		return nil
	}
	return requester.NewArrObserved(r.arrConns(), recorder)
}

// seerrRequester returns the concrete Seerr requester when Seerr is the active provider (the
// default — anything other than "arr"), else nil. Mirrors arrRequester: the queue poller (§18.1)
// needs the concrete *Seerr for its QueueStatus capability (coarse status from /media), which the
// Requester interface doesn't expose. Only meaningful once seerr.url is set — an unconfigured
// Seerr QueueStatus just errors and the poll pass is a harmless no-op.
func (r resolved) seerrRequester(recorder *metrics.Recorder) *requester.Seerr {
	if r.str("requester.provider") == "arr" {
		return nil
	}
	return requester.NewSeerrDynamicObserved(r.seerrConn(), recorder)
}

// tunarrConfig snapshots every live Tunarr setting together at the start of one
// programmer operation. This keeps a multi-request push internally coherent while
// preserving hot-apply for the next operation (config-design §3).
func (r resolved) tunarrConfig() func() programmer.Config {
	return func() programmer.Config {
		if r.svc == nil {
			return programmer.Config{}
		}
		values := r.svc.ResolveMany(
			"tunarr.url",
			"tunarr.transcode_config_id",
			"filler.weight",
			"filler.cooldown_seconds",
		)
		stringValue := func(key string) string {
			value, _ := values[key].Value.(string)
			return value
		}
		intValue := func(key string) int {
			value, _ := values[key].Value.(int)
			return value
		}
		return programmer.Config{
			BaseURL:               stringValue("tunarr.url"),
			TranscodeConfigID:     stringValue("tunarr.transcode_config_id"),
			FillerWeight:          intValue("filler.weight"),
			FillerCooldownSeconds: intValue("filler.cooldown_seconds"),
		}
	}
}

// bootSettings builds the settings runtime at startup (config-design §11 Phase 1):
// the registry, the resolution service (env pins validated → boot error), the
// generated secrets (idempotent), and the redactor wired into slog. It returns the
// resolved-config facade, the secrets, and the redactor so the caller can feed the
// redactor the app-managed secret values too and refresh it after a secret change.
func bootSettings(ctx context.Context, st store.Store, protection *secretprotection.Manager, baseLog *slog.Logger) (resolved, *settings.Secrets, *settings.Redactor, *slog.Logger, error) {
	reg := settings.NewRegistry()
	loader := settings.StoreLoader{List: func(ctx context.Context) ([]settings.SettingRow, error) {
		rows, err := loadProtectedSettings(ctx, st, reg, protection)
		if err != nil {
			return nil, err
		}
		out := make([]settings.SettingRow, len(rows))
		for i, r := range rows {
			// EnvOverride rides through here or §3.1's claim silently loads as false on
			// every boot — the flag would persist correctly and never be read.
			out[i] = settings.SettingRow{
				Key: r.Key, Value: r.Value, UpdatedBy: r.UpdatedBy, EnvOverride: r.EnvOverride,
			}
		}
		return out, nil
	}}
	svc, err := settings.New(ctx, reg, loader, baseLog)
	if err != nil {
		return resolved{}, nil, nil, baseLog, err
	}
	secrets, err := settings.NewSecrets(ctx, secretStoreAdapter{st: st, protection: protection}, envLookup)
	if err != nil {
		return resolved{}, nil, nil, baseLog, err
	}
	// Wire the redactor into slog so no secret — generated or app-managed — is ever
	// logged (config-design §4). Seed it with the generated secrets + the resolved
	// app-managed secret settings; refreshSecrets updates it after a change.
	red := settings.NewRedactor()
	r := resolved{svc: svc, protection: protection}
	red.Set(collectSecrets(reg, svc, secrets))
	log := slog.New(red.Handler(baseLog.Handler()))
	return r, secrets, red, log, nil
}

// collectSecrets gathers every live secret value (generated + app-managed) so the
// redactor scrubs all of them (config-design §4).
func collectSecrets(reg *settings.Registry, svc *settings.Service, secrets *settings.Secrets) []string {
	var vals []string
	for _, s := range reg.All() {
		if s.IsSecret() {
			if v := svc.String(s.Key); v != "" {
				vals = append(vals, v)
			}
		}
	}
	vals = append(vals, secrets.RedactionValues()...)
	return vals
}

// secretStoreAdapter adapts store.Store to settings.SecretStore.
type secretStoreAdapter struct {
	st         store.Store
	protection *secretprotection.Manager
}

func (a secretStoreAdapter) Get(ctx context.Context, k string) (string, bool, error) {
	v, err := a.st.GetSetting(ctx, k)
	if err == store.ErrNotFound {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !secretprotection.IsEnvelope(v) {
		return "", false, fmt.Errorf("generated secret %q is not protected", k)
	}
	plain, err := a.protection.OpenLatest(ctx, settingRecord(k), v)
	if err != nil {
		return "", false, err
	}
	return string(plain), true, nil
}
func (a secretStoreAdapter) Set(ctx context.Context, k, v string) error {
	if err := a.protection.Refresh(ctx); err != nil {
		return err
	}
	envelope, err := a.protection.Seal(settingRecord(k), []byte(v))
	if err != nil {
		return err
	}
	return a.st.SetSetting(ctx, k, envelope)
}

func (a secretStoreAdapter) WithLock(
	ctx context.Context,
	key string,
	fn func(context.Context) error,
) error {
	return a.st.WithSettingLock(ctx, key, fn)
}
