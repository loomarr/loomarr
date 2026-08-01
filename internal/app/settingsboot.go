package app

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/mantonx/loomarr/internal/requester"
	"github.com/mantonx/loomarr/internal/settings"
	"github.com/mantonx/loomarr/internal/store"
)

// envLookup is the process env accessor the secrets lifecycle uses to honor an
// env-pinned SESSION_SECRET/API_TOKEN (config-design §4). ⚠ Not WEBHOOK_SECRET (retired-ok) — that
// never existed as a generated secret, and the arm webhook it named was deleted.
var envLookup = os.LookupEnv

// resolved wraps the settings service with typed getters so the composition root
// reads settings the way it used to read cfg.X — but every read now resolves
// env > db > default through the live snapshot (config-design §3). Connection
// getters are returned as closures so adapters read them PER CALL (hot-apply).
type resolved struct {
	svc *settings.Service
}

// Every getter tolerates a nil service, because "no store ⇒ no settings service" is a
// SUPPORTED state, not a bug: starting without DATABASE_URL logs "running without a
// store (not ready)" and is expected to keep serving so /readyz can report why. It did
// not — the first unguarded read panicked during BuildHandler, so a misconfigured
// container crash-looped instead of answering the probe that would have explained it.
// An unset key already resolves to a zero value; an absent service is the same answer.
func (r resolved) str(key string) string {
	if r.svc == nil {
		return ""
	}
	return r.svc.String(key)
}
func (r resolved) dur(key string) time.Duration {
	if r.svc == nil {
		return 0
	}
	if d, ok := r.svc.Resolve(key).Value.(time.Duration); ok {
		return d
	}
	return 0
}
func (r resolved) intv(key string) int {
	if r.svc == nil {
		return 0
	}
	if n, ok := r.svc.Resolve(key).Value.(int); ok {
		return n
	}
	return 0
}
func (r resolved) boolv(key string) bool {
	if r.svc == nil {
		return false
	}
	if b, ok := r.svc.Resolve(key).Value.(bool); ok {
		return b
	}
	return false
}

// libraryConn is the media-server connection provider (url, token) read per call.
func (r resolved) libraryConn() func() (string, string) {
	return func() (string, string) { return r.str("library.url"), r.str("library.token") }
}

// seerrConn / tunarrConn are the requester + Tunarr connection providers.
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
func (r resolved) requesterFor() requester.Requester {
	if a := r.arrRequester(); a != nil {
		return a
	}
	return requester.NewSeerrDynamic(r.seerrConn())
}

// arrRequester returns the concrete direct-arr requester when requester.provider=arr, else nil.
// The queue poller (§18.1) needs the concrete *Arr for its QueueStatus capability, which the
// Requester interface doesn't expose.
func (r resolved) arrRequester() *requester.Arr {
	if r.str("requester.provider") != "arr" {
		return nil
	}
	return requester.NewArr(r.arrConns())
}

// seerrRequester returns the concrete Seerr requester when Seerr is the active provider (the
// default — anything other than "arr"), else nil. Mirrors arrRequester: the queue poller (§18.1)
// needs the concrete *Seerr for its QueueStatus capability (coarse status from /media), which the
// Requester interface doesn't expose. Only meaningful once seerr.url is set — an unconfigured
// Seerr QueueStatus just errors and the poll pass is a harmless no-op.
func (r resolved) seerrRequester() *requester.Seerr {
	if r.str("requester.provider") == "arr" {
		return nil
	}
	return requester.NewSeerrDynamic(r.seerrConn())
}
func (r resolved) tunarrConn() func() string {
	return func() string { return r.str("tunarr.url") }
}

// bootSettings builds the settings runtime at startup (config-design §11 Phase 1):
// the registry, the resolution service (env pins validated → boot error), the
// generated secrets (idempotent), and the redactor wired into slog. It returns the
// resolved-config facade, the secrets, and the redactor so the caller can feed the
// redactor the app-managed secret values too and refresh it after a secret change.
func bootSettings(ctx context.Context, st store.Store, baseLog *slog.Logger) (resolved, *settings.Secrets, *settings.Redactor, *slog.Logger, error) {
	reg := settings.NewRegistry()
	loader := settings.StoreLoader{List: func(ctx context.Context) ([]settings.SettingRow, error) {
		rows, err := st.ListSettings(ctx)
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
	secrets, err := settings.NewSecrets(ctx, secretStoreAdapter{st}, envLookup)
	if err != nil {
		return resolved{}, nil, nil, baseLog, err
	}
	// Wire the redactor into slog so no secret — generated or app-managed — is ever
	// logged (config-design §4). Seed it with the generated secrets + the resolved
	// app-managed secret settings; refreshSecrets updates it after a change.
	red := settings.NewRedactor()
	r := resolved{svc: svc}
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
type secretStoreAdapter struct{ st store.Store }

func (a secretStoreAdapter) Get(ctx context.Context, k string) (string, bool, error) {
	v, err := a.st.GetSetting(ctx, k)
	if err == store.ErrNotFound {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}
func (a secretStoreAdapter) Set(ctx context.Context, k, v string) error {
	return a.st.SetSetting(ctx, k, v)
}
