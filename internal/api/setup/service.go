package setup

import (
	"context"

	"github.com/loomarr/loomarr/internal/people/notifications"
)

// SettingsService is the settings surface the API depends on (config-design §8).
// Implemented by a settings.Service adapter in the composition root. Its read
// side is the typed settings.Reader on Options.LiveSettings; this interface is the
// management surface. Secrets are masked here — a value never crosses this
// boundary (§4).
type SettingsService interface {
	// List returns every operator-managed setting with its resolved value (secrets
	// masked) + provenance + audit metadata (config-design §8).
	List(ctx context.Context) []SettingEntry
	// Patch applies per-key edits, returning a per-key result (saved | invalid |
	// pinned). Hot-applies on success. updatedBy is the admin's id (§3 audit).
	Patch(ctx context.Context, edits map[string]string, updatedBy string) []SettingResult
	// Clear drops one key's stored override (reverts to env/default) — the explicit
	// clear, and the only way to unset a secret (config-design §8/§9). The result
	// status maps to HTTP: invalid → 404, pinned → 409, saved → 204.
	Clear(ctx context.Context, key string) SettingResult
	// SetEnvOverride takes a key back from the environment, or hands it back
	// (config-design §3.1). Status maps to HTTP: unknown → 404, not_pinned → 409,
	// applied → 204. updatedBy is the admin's id, so the field's existing
	// "changed by … · when" line reports who overrode the deploy config.
	SetEnvOverride(ctx context.Context, key string, on bool, updatedBy string) SettingResult
	// Features returns the computed feature availability (config-design §7).
	Features(ctx context.Context) map[string]bool
	// MissingRequirements explains a false feature verdict with the same registry-owned
	// prerequisite calculation. Values are setting keys ordered for recovery.
	MissingRequirements(ctx context.Context, feature string) []string
	// RegenerateSecret rotates a generated API or playout token and returns the new
	// value (config-design §4).
	RegenerateSecret(ctx context.Context, name string) (value string, err error)
	// RevealSecret returns a generated API or playout token's CURRENT value
	// (config-design §4's eye-toggle). Reading must never rotate.
	RevealSecret(ctx context.Context, name string) (value string, err error)
	// Test runs one named connection check (config-design §8, powers Test buttons).
	Test(ctx context.Context, check string) (ok bool, hint string)
}

type NotificationDestinationService interface {
	Create(context.Context, notifications.Principal, notifications.DestinationCommand) (notifications.DestinationSummary, error)
	List(context.Context, notifications.Principal) ([]notifications.DestinationSummary, error)
	Update(context.Context, notifications.Principal, string, notifications.DestinationUpdateCommand) (notifications.DestinationSummary, error)
	Delete(context.Context, notifications.Principal, string, string) (notifications.DestinationDeleteResult, error)
	Test(context.Context, notifications.Principal, string, string) (notifications.DestinationTestResult, error)
}

// BackendTransitioner is the one deep settings consequence for playout publication. The
// implementation holds its cross-replica lock while invoking mutation and all publication phases.
// mutation returns whether it durably changed a transition input; false skips transition repair.
type BackendTransitioner interface {
	ApplyMutation(ctx context.Context, mutation func(context.Context) bool) error
	// Reconnect force-repairs the durably applied tuner/listing target under the same
	// cross-replica serialization boundary as ordinary backend publication.
	Reconnect(ctx context.Context) (tunersReset int, err error)
}

// SettingEntry is the API view of one setting (config-design §8). For a secret,
// Value is empty and Set/Preview carry the masked state — the value never appears.
// SettingEnumOption is one choice of an enum setting: the stored value + its display
// label (config-design §5). The label is registry-owned so the UI never re-derives it.
type SettingEnumOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// SettingPreset is one common choice for an open-ended setting, offered ahead of a custom value.
// Value is canonical, so it equals a resolved value that matches it.
type SettingPreset struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type SettingEntry struct {
	Key          string              `json:"key"`
	Label        string              `json:"label,omitempty" doc:"Registry-owned human label for workflow forms."`
	Group        string              `json:"group"`
	Owner        string              `json:"owner" doc:"Stable task id for the setting's one curated editor. Browser paths are frontend-owned."`
	Kind         string              `json:"kind"`
	Apply        string              `json:"apply" enum:"live,restart" doc:"When a saved value becomes active: live on the next owning operation, or restart in the next app generation."`
	Presentation string              `json:"presentation,omitempty" doc:"Human editor semantics beyond kind, e.g. bytes, path, or language."`
	Unit         string              `json:"unit,omitempty" doc:"What a plain number counts, shown beside its input, e.g. GiB, MiB or %. Empty for a bare count."`
	Value        string              `json:"value,omitempty" doc:"Resolved value (non-secret). Empty for secrets."`
	Set          bool                `json:"set" doc:"For secrets: whether a value is stored."`
	Preview      string              `json:"preview,omitempty" doc:"For secrets: masked '…a1b2' tail (§4)."`
	BuiltIn      bool                `json:"builtIn,omitempty" doc:"For secrets: none is set, and the key Loomarr ships with is in use instead (the bundled TMDB key). Never previewed."`
	Provenance   string              `json:"provenance" enum:"env,db,default" doc:"env locks the UI field (§3)."`
	Caution      bool                `json:"caution,omitempty" doc:"A stored value self-healed to default (§3)."`
	EnvPinnable  bool                `json:"envPinnable,omitempty" doc:"The environment sets this key, so it can be taken back with the unlock (§3.1). Only meaningful together with provenance/envOverride."`
	EnvOverride  bool                `json:"envOverride,omitempty" doc:"An admin took this key back from the environment (§3.1): the env var is set but deliberately not winning. Reported alongside provenance — such a key resolves honestly as 'db' — so the UI can say 'overriding SEERR_URL' rather than implying the variable is unset."`
	EnvVar       string              `json:"envVar,omitempty" doc:"The environment variable that pins (or would pin) this key — named so the UI can tell the operator exactly what it is overriding, and what to unset to hand it back."`
	Advanced     bool                `json:"advanced"`
	Secret       bool                `json:"secret"`
	Enum         []string            `json:"enum,omitempty" doc:"Enum values (the closed set) — labels are in enumOptions."`
	EnumOptions  []SettingEnumOption `json:"enumOptions,omitempty" doc:"Enum choices with display labels (config-design §5)."`
	Presets      []SettingPreset     `json:"presets,omitempty" doc:"Common choices offered before a custom value, in canonical form. Any value the setting accepts still saves."`
	ShowWhen     map[string][]string `json:"showWhen,omitempty" doc:"Conditional visibility: show only when a named key's current value is listed (§5)."`
	RequiredFor  string              `json:"requiredFor,omitempty"`
	Doc          string              `json:"doc" doc:"One plain sentence, shown under the field."`
	Detail       string              `json:"detail,omitempty" doc:"The rest of the help: trade-offs, units and edge cases, shown behind the field's info icon. Absent when the sentence says it all."`
	UpdatedBy    string              `json:"updatedBy,omitempty" doc:"Display name of the person who last changed it. Empty when that person is unknown; never a raw user id (#2736)."`
	UpdatedByYou bool                `json:"updatedByYou,omitempty" doc:"The signed-in caller made the last change, so the UI can say 'you'."`
	UpdatedAt    string              `json:"updatedAt,omitempty" doc:"RFC3339; empty for env/system writes."`
}

// SettingResult is one key's PATCH outcome (config-design §8).
type SettingResult struct {
	Key     string `json:"key"`
	Status  string `json:"status" enum:"saved,invalid,pinned" doc:"saved | invalid(problem) | pinned (§8)."`
	Problem string `json:"problem,omitempty" doc:"Validation message when status=invalid (never echoes a secret, §4)."`
}

// LiveTVService backs the Live TV setup status check (§6/§7). Mutating repair belongs to
// BackendTransitioner so it cannot bypass durable publication ordering.
type LiveTVService interface {
	Wired(ctx context.Context) (bool, error)
}

// TunarrConnector wires the media server as *Tunarr's* media source (§6) so Tunarr
// can stream + index the library — backs POST /v1/setup/tunarr-connect and the
// tunarr_library setup check. Idempotent. Implemented by livetvpublish.MediaSourceConnector.
type TunarrConnector interface {
	Connect(ctx context.Context) (sourceID string, librariesEnabled int, err error)
	LibrariesReady(ctx context.Context) (bool, error)
}

// ArrDiscovery reads the Sonarr and Radarr the requester (Seerr) already knows, so the
// wizard can fill in their connections instead of asking for them (#2151).
type ArrDiscovery interface {
	// ArrServers is Seerr's default Sonarr and Radarr; none when Seerr isn't the requester.
	ArrServers(ctx context.Context) ([]DiscoveredArr, error)
	// Reachable probes one arr at url with apiKey.
	Reachable(ctx context.Context, app, url, apiKey string) error
}

// DiscoveredArr is one arr the requester knows: the addresses to try, in order, and its key.
type DiscoveredArr struct {
	App    string
	URLs   []string
	APIKey string
}
