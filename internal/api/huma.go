package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/mantonx/loomarr/internal/auth"
	"github.com/mantonx/loomarr/internal/store"
)

// Server holds the API dependencies and builds the Huma API on a stdlib mux
// (§7.1, §14: code-first OpenAPI 3.1 via humago — no third-party router).
type Server struct {
	store store.Store
	auth  Authorizer
	log   *slog.Logger
	// backupSQLite is set when the store is the SQLite backend (GET /v1/backup —
	// §16). Nil for Postgres, which returns 501 + a pg_dump docs pointer.
	backupSQLite BackupStreamer
	// login/sessions wire /v1/auth/* (§11); nil until Phase 9 is configured.
	login        LoginService
	sessions     SessionManager
	userSync     UserSyncer
	cookieSecure string // COOKIE_SECURE: auto|true|false (§11)
	// channels/livetv wire /v1/channels* and /v1/setup/* (§7/§9); nil until
	// Phase 10 is configured.
	channels ChannelService
	livetv   LiveTVService
	// tunarrConnect wires the media server as Tunarr's media source (§6) — backs
	// POST /v1/setup/tunarr-connect + the tunarr_library setup check. Nil ⇒ 501.
	tunarrConnect TunarrConnector
	// suggest/search wire /v1/suggestions*, /v1/search (§7.2/§8); nil until
	// Phase 11 is configured.
	suggest SuggestService
	search  SearchService
	events  EventSource   // /v1/events SSE (Phase 11); nil ⇒ route 501
	filler  FillerService // /v1/filler* (Phase 12); nil ⇒ sync/tag routes 501
	// systemLLM wires /v1/system/llm* (§8.1 model selection); nil ⇒ routes 501.
	systemLLM SystemLLMService
	// settings wires /v1/settings* + secrets regeneration (config-design §8);
	// nil ⇒ routes 501. Implemented by a thin adapter over settings.Service.
	settings SettingsService
	// provision wires /v1/setup/bootstrap + /v1/users/import (§11); nil ⇒ routes
	// absent. Implemented by auth.Provisioner.
	provision Provisioner
	// liveConfig reads a setting's live resolved value (config-design §3 hot-apply).
	// The composition root always-constructs the feature services and passes this so
	// a route gates on the CURRENT config (a saved connection enables it with no
	// restart, §8.1). Nil in unit tests that wire deps directly — then the nil-dep
	// check alone gates, preserving the old contract.
	liveConfig func(key string) string
	// schemaOnly is set ONLY by ExportOpenAPI (§7.1): it makes the register* funcs
	// emit every operation's SCHEMA into the spec even when its live service is nil,
	// so the exported `api/openapi.yaml` is complete (auth, bootstrap, import, sync)
	// and orval can type the whole surface. Handlers are never invoked during export,
	// so a nil service behind a registered op is harmless. Runtime nil-guarding is
	// unchanged — at runtime schemaOnly is false and an absent service still 404s.
	schemaOnly bool
}

// featureOff reports whether a named feature (config-design §7) is configured-off
// right now. Safe when settings is unset (unit tests) — reports false so a wired
// dep still serves.
func (s *Server) featureOff(ctx context.Context, feature string) bool {
	return s.settings != nil && !s.settings.Features(ctx)[feature]
}

// unconfigured reports whether a required setting is empty right now. Safe when
// liveConfig is unset (unit tests) — reports false so a wired dep still serves.
func (s *Server) unconfigured(keys ...string) bool {
	if s.liveConfig == nil {
		return false
	}
	for _, k := range keys {
		if s.liveConfig(k) == "" {
			return true
		}
	}
	return false
}

// SettingsService is the settings surface the API depends on (config-design §8).
// Implemented by a settings.Service adapter in the composition root; abstracted
// so the API package needn't import internal/settings. Secrets are masked here —
// a value never crosses this boundary (§4).
type SettingsService interface {
	// List returns every registry setting with its resolved value (secrets
	// masked) + provenance + audit metadata (config-design §8).
	List(ctx context.Context) []SettingEntry
	// Patch applies per-key edits, returning a per-key result (saved | invalid |
	// pinned). Hot-applies on success. updatedBy is the admin's id (§3 audit).
	Patch(ctx context.Context, edits map[string]string, updatedBy string) []SettingResult
	// Clear drops one key's stored override (reverts to env/default) — the explicit
	// clear, and the only way to unset a secret (config-design §8/§9). The result
	// status maps to HTTP: invalid → 404, pinned → 409, saved → 204.
	Clear(ctx context.Context, key string) SettingResult
	// Features returns the computed feature availability (config-design §7).
	Features(ctx context.Context) map[string]bool
	// RegenerateSecret rotates a generated secret and returns the new value if it
	// is displayable (config-design §4); displayable=false ⇒ value withheld.
	RegenerateSecret(ctx context.Context, name string) (value string, displayable bool, err error)
	// RevealSecret returns a generated secret's CURRENT value if it is displayable
	// (config-design §4's eye-toggle). Reading must never rotate — the §13 webhook
	// panel shows the URL an operator already pasted into Sonarr/Radarr.
	RevealSecret(ctx context.Context, name string) (value string, displayable bool, err error)
	// Test runs one named connection check (config-design §8, powers Test buttons).
	Test(ctx context.Context, check string) (ok bool, hint string)
}

// SettingEntry is the API view of one setting (config-design §8). For a secret,
// Value is empty and Set/Preview carry the masked state — the value never appears.
type SettingEntry struct {
	Key         string   `json:"key"`
	Group       string   `json:"group"`
	Kind        string   `json:"kind"`
	Value       string   `json:"value,omitempty" doc:"Resolved value (non-secret). Empty for secrets."`
	Set         bool     `json:"set" doc:"For secrets: whether a value is stored."`
	Preview     string   `json:"preview,omitempty" doc:"For secrets: masked '…a1b2' tail (§4)."`
	Provenance  string   `json:"provenance" enum:"env,db,default" doc:"env locks the UI field (§3)."`
	Caution     bool     `json:"caution,omitempty" doc:"A stored value self-healed to default (§3)."`
	Advanced    bool     `json:"advanced"`
	Secret      bool     `json:"secret"`
	Enum        []string `json:"enum,omitempty"`
	RequiredFor string   `json:"requiredFor,omitempty"`
	Doc         string   `json:"doc"`
	UpdatedBy   string   `json:"updatedBy,omitempty"`
	UpdatedAt   string   `json:"updatedAt,omitempty" doc:"RFC3339; empty for env/system writes."`
}

// SettingResult is one key's PATCH outcome (config-design §8).
type SettingResult struct {
	Key     string `json:"key"`
	Status  string `json:"status" enum:"saved,invalid,pinned" doc:"saved | invalid(problem) | pinned (§8)."`
	Problem string `json:"problem,omitempty" doc:"Validation message when status=invalid (never echoes a secret, §4)."`
}

// FillerService backs the filler ingestion routes (§10): catalog sync and the AI
// tagging job. Implemented by filler.Syncer + filler.Tagger. list/patch read/write
// the store directly (no service needed).
type FillerService interface {
	// Sync reconciles the clip catalog from the media server's filler library.
	Sync(ctx context.Context) (total, added, updated, pruned int, err error)
	// Tag runs AI classification over untagged commercials.
	Tag(ctx context.Context) (considered, tagged, partial, skipped int, err error)
}

// SuggestService is the suggestion surface the API depends on (§8). Implemented
// by suggest.Service + the store; abstracted so the API doesn't couple to the
// worker internals.
type SuggestService interface {
	// Submit enqueues a suggestion job for an intent and returns the job id.
	Submit(ctx context.Context, description, era, tone string, mustInclude, mustExclude []string, maxAcquire int, createdBy string) (jobID string, err error)
}

// SearchService backs GET /v1/search (§7.2) — the SAME catalog impl as the LLM
// grounding tool. Returns candidates as generic maps so the API layer needn't
// import the catalog types (kept dependency-light).
type SearchService interface {
	Search(ctx context.Context, query, scope string, limit int) ([]SearchCandidate, error)
}

// SearchCandidate is the API view of a catalog candidate (§7.2).
type SearchCandidate struct {
	MediaType     string `json:"mediaType"`
	TMDBID        int    `json:"tmdbId,omitempty"`
	TVDBID        int    `json:"tvdbId,omitempty"`
	Name          string `json:"name"`
	Year          int    `json:"year,omitempty"`
	InLibrary     bool   `json:"inLibrary"`
	LibraryItemID string `json:"libraryItemId,omitempty"`
}

// ChannelService is the channel-management surface the API depends on (§9).
// Implemented by channels.Engine + the store; abstracted so the API doesn't
// couple to the reconcile internals.
type ChannelService interface {
	// Reconcile forces a desired→Tunarr reconciliation for one channel (§9,
	// POST /v1/channels/{id}/reconcile).
	Reconcile(ctx context.Context, channelID string) error
}

// LiveTVService backs the Live TV setup routes (§6/§7): idempotent connect and
// the "wired?" status check. Implemented by setup.LiveTVConnector.
type LiveTVService interface {
	Connect(ctx context.Context) (tunerAdded, listingAdded bool, err error)
	Wired(ctx context.Context) (bool, error)
}

// TunarrConnector wires the media server as *Tunarr's* media source (§6) so Tunarr
// can stream + index the library — backs POST /v1/setup/tunarr-connect and the
// tunarr_library setup check. Idempotent. Implemented by setup.MediaSourceConnector.
type TunarrConnector interface {
	Connect(ctx context.Context) (sourceID string, librariesEnabled int, err error)
	LibrariesReady(ctx context.Context) (bool, error)
}

// UserSyncer refreshes already-imported users from the media server (§11).
// Returns the count synced. It never ADDS users (import defines the allowlist).
type UserSyncer interface {
	Sync(ctx context.Context) (int, error)
}

// Provisioner owns first-run bootstrap + explicit import (§11) — the only paths
// that create users. Implemented by auth.Provisioner.
type Provisioner interface {
	// Bootstrap creates the first local admin, once (ErrBootstrapClosed after).
	Bootstrap(ctx context.Context, username, password string) (store.User, error)
	// Import allowlists the named media-server user ids (admin-only).
	Import(ctx context.Context, ids []string, makeAdmin bool) (int, error)
	// Candidates lists media-server accounts available to import, each flagged with
	// whether it is already allowlisted (§11). Read-only — listing never provisions.
	Candidates(ctx context.Context) ([]auth.Candidate, error)
}

// LoginService verifies credentials and issues a session (Phase 9, §11).
type LoginService interface {
	Login(ctx context.Context, username, password, rateKey string) (token string, expires time.Time, u store.User, err error)
	Disable(ctx context.Context, userID string) error
}

// SessionManager revokes sessions (logout) (§11).
type SessionManager interface {
	Revoke(ctx context.Context, token string) error
}

// BackupStreamer streams a consistent DB snapshot (§16). Implemented by the
// SQLite backend via VACUUM INTO. Matches store.SQLiteBackuper's return type.
type BackupStreamer interface {
	StreamBackup(ctx context.Context, w io.Writer) error
}

// Options configures the API server.
type Options struct {
	Store         store.Store
	Auth          Authorizer
	Log           *slog.Logger
	BackupSQLite  BackupStreamer // nil ⇒ /v1/backup returns 501 (Postgres)
	Ingest        http.Handler   // POST /hooks/arr (Phase 6); mounted outside Huma
	Ready         ReadyFunc
	Login         LoginService     // /v1/auth/login + user disable (Phase 9); nil ⇒ routes absent
	Sessions      SessionManager   // /v1/auth/logout (Phase 9)
	UserSync      UserSyncer       // POST /v1/users/sync (Phase 9); nil ⇒ route absent
	CookieSecure  string           // COOKIE_SECURE: auto|true|false (§11)
	Channels      ChannelService   // /v1/channels* reconcile (Phase 10); nil ⇒ reconcile route absent
	LiveTV        LiveTVService    // /v1/setup/* (Phase 10); nil ⇒ setup routes absent
	TunarrConnect TunarrConnector  // /v1/setup/tunarr-connect + tunarr_library check (§6); nil ⇒ 501
	Suggest       SuggestService   // /v1/suggestions submit (Phase 11); nil ⇒ submit route 501
	Search        SearchService    // /v1/search (Phase 11); nil ⇒ search route 501
	Events        EventSource      // /v1/events SSE (Phase 11); nil ⇒ route 501
	Filler        FillerService    // /v1/filler sync/tag (Phase 12); nil ⇒ those routes 501
	SystemLLM     SystemLLMService // /v1/system/llm* model selection (§8.1); nil ⇒ routes 501
	Settings      SettingsService  // /v1/settings* (config-design §8); nil ⇒ routes 501
	Provision     Provisioner      // /v1/setup/bootstrap + /v1/users/import (§11); nil ⇒ routes absent
	// LiveConfig reads a setting's live resolved value so feature routes gate on the
	// CURRENT config (a saved connection enables the route with no restart, §8.1).
	// The composition root passes settings.Service.String; unit tests omit it.
	LiveConfig func(key string) string
}

// humaConfig builds the OpenAPI 3.1 config with our metadata (§7.1).
func humaConfig() huma.Config {
	cfg := huma.DefaultConfig("Loomarr API", "0.1.0")
	cfg.Info.Description = "Turn a sentence into a self-maintaining Tunarr channel. " +
		"Every /v1 route requires a session cookie or Authorization: Bearer API_TOKEN (§7)."
	// Serve our own docs assets offline; Huma's default loads Stoplight from a
	// CDN which violates the offline rule (§7.1). We disable the built-in docs
	// path and mount our own at /docs (docs.go).
	cfg.DocsPath = ""
	return cfg
}
